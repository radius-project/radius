// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

import { ChoiceSchema, CodeModel, ComplexSchema, HttpMethod, HttpParameter, HttpRequest, HttpResponse, ImplementationLocation, isObjectSchema, ObjectSchema, Operation, Parameter, ParameterLocation, Request, Response, Schema, SchemaResponse, SealedChoiceSchema, Metadata } from "@autorest/codemodel";
import { Channel, AutorestExtensionHost } from "@autorest/extension-base";
import { keys, Dictionary, values, groupBy, uniqBy, chain, flatten } from 'lodash';
import { success, failure, Result } from './utils';
import { ScopeType } from "bicep-types";

export interface ResourceDescriptor {
  scopeType: ScopeType;
  namespace: string;
  typeSegments: string[];
  apiVersion: string;
  constantName?: string;
  readonlyScopes?: ScopeType;
}

export interface ProviderDefinition {
  namespace: string;
  apiVersion: string;
  resourcesByType: Dictionary<ResourceDefinition[]>;
  resourceActions: ResourceListActionDefinition[];
}

export interface ResourceOperationDefintion {
  request: HttpRequest;
  response?: HttpResponse;
  parameters: Parameter[];
  requestSchema?: ObjectSchema;
  responseSchema?: ObjectSchema;
}

export interface ResourceDefinition {
  descriptor: ResourceDescriptor;
  putOperation?: ResourceOperationDefintion;
  getOperation?: ResourceOperationDefintion;
}

export interface ResourceListActionDefinition {
  actionName: string;
  descriptor: ResourceDescriptor;
  postRequest: HttpRequest;
  requestSchema?: Schema;
  responseSchema?: Schema;
}

const parentScopePrefix = /^.*\/providers\//ig;
const managementGroupPrefix = /^\/providers\/Microsoft.Management\/managementGroups\/{\w+}\/$/i;
const tenantPrefix = /^\/$/i;
const subscriptionPrefix = /^\/subscriptions\/{\w+}\/$/i;
const resourceGroupPrefix = /^\/subscriptions\/{\w+}\/resourceGroups\/{\w+}\/$/i;
const resourceGroupMethod = /^\/subscriptions\/{\w+}\/resourceGroups\/{\w+}$/i;

function trimScope(scope: string) {
  return scope.replace(/\/*$/, '').replace(/^\/*/, '');
}

function isPathVariable(pathSegment: string) {
  return pathSegment.startsWith('{') && pathSegment.endsWith('}');
}

function trimParamBraces(pathSegment: string) {
  return pathSegment.substr(1, pathSegment.length - 2);
}

function normalizeListActionName(actionName: string) {
  if (actionName.toLowerCase().startsWith('list')) {
    // force lower-case on the 'list' prefix for consistency
    return `list${actionName.substr(4)}`;
  }

  return actionName;
}

export function getFullyQualifiedType(descriptor: ResourceDescriptor) {
  return [descriptor.namespace, ...descriptor.typeSegments].join('/');
}

function groupByType<T extends { descriptor: ResourceDescriptor }>(items: T[]) {
  return groupBy(items, x => getFullyQualifiedType(x.descriptor).toLowerCase());
}

export function isRootType(descriptor: ResourceDescriptor) {
  return descriptor.typeSegments.length === 1;
}

function getHttpRequests(requests: Request[] | undefined) {
  return requests?.map(request => ({request, httpRequest: request.protocol.http as HttpRequest}))
    .filter(x => !!x.httpRequest) ?? [];
}

function hasStatusCode(response: Response, statusCode: string) {
  const statusCodes = (response.protocol.http as HttpResponse)?.statusCodes;
  if (!statusCodes) {
    return;
  }

  return (statusCodes as string[]).includes(statusCode);
}

function getNormalizedMethodPath(path: string) {
  if (resourceGroupMethod.test(path)) {
    // resource groups are a special case - the swagger API is not defined as a provider API, but they are still deployable in a template as if it was.
    return "/subscriptions/{subscriptionId}/providers/Microsoft.Resources/resourceGroups/{resourceGroupName}";
  }

  return path;
}

export function getSerializedName(metadata: Metadata) {
  return metadata.language.default.serializedName ?? metadata.language.default.name;
}

interface ParameterizedName {
  type: 'parameterized';
  schema: Schema;
}

interface ConstantName {
  type: 'constant';
  value: string;
}

type NameSchema = ParameterizedName|ConstantName;

export function getNameSchema(request: HttpRequest, parameters: Parameter[]): Result<NameSchema, string> {
  const path = getNormalizedMethodPath(request.path);

  const finalProvidersMatch = path.match(parentScopePrefix)?.slice(-1)[0];
  if (!finalProvidersMatch) {
    return failure(`Unable to locate "/providers/" segment`);
  }

  const routingScope = trimScope(path.substr(finalProvidersMatch.length));

  // get the resource name parameter, e.g. {fooName}
  let resNameParam = routingScope.substr(routingScope.lastIndexOf('/') + 1);

  if (isPathVariable(resNameParam)) {
    // strip the enclosing braces
    resNameParam = trimParamBraces(resNameParam);

    const param = parameters.filter(p => getSerializedName(p) === resNameParam)[0];
    if (!param) {
      return failure(`Unable to locate parameter with name '${resNameParam}'`);
    }

    return success({type: 'parameterized', schema: param.schema});
  }

  if (!/^[a-zA-Z0-9]*$/.test(resNameParam)) {
    return failure(`Unable to process non-alphanumeric name '${resNameParam}'`);
  }

  return success({type: 'constant', value: resNameParam});
}

export function parseNameSchema<T>(request: HttpRequest, parameters: Parameter[], parseType: (schema: Schema) => T, createConstantName: (name: string) => T): Result<T, string> {
  const nsResult = getNameSchema(request, parameters);
  if (!nsResult.success) {
    return nsResult;
  }

  const {value} = nsResult;
  if (value.type === 'parameterized') {
    return success(parseType(value.schema));
  }

  return success(createConstantName(value.value));
}

const alwaysAllowedParameterLocations = new Set([
  ParameterLocation.Path,
  ParameterLocation.Body,
  ParameterLocation.Uri,
  ParameterLocation.Virtual,
  ParameterLocation.None,
]);

const allowedRequiredParametersByLocation: Dictionary<Set<string>> = {
  [ParameterLocation.Header]: new Set([
    'content-type',
    'accept',
    'if-match',
    'if-none-match',
  ]),
  [ParameterLocation.Query]: new Set([
    'api-version'
  ])
};

export function getProviderDefinitions(codeModel: CodeModel, host: AutorestExtensionHost): ProviderDefinition[] {
  function logWarning(message: string) {
    host.Message({
      Channel: Channel.Warning,
      Text: message,
    })
  }

  function getProviderDefinitions() {
    const apiVersions = codeModel.operationGroups
      .flatMap(group => group.operations
        .flatMap(op => (op.apiVersions ?? []).map(v => v.version)))
      .filter((x, i, a) => a.indexOf(x) === i);

    return apiVersions.flatMap(v => getProviderDefinitionsForApiVersion(v));
  }

  function getExtensions(schema: ComplexSchema) {
    // extensions are defined as Record<string, any> in autorest
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const extensions: Record<string, any> = {};
    if (isObjectSchema(schema)) {
      for (const parent of schema.parents?.all || []) {
        for (const [key, value] of Object.entries(getExtensions(parent))) {
          extensions[key] = value;
        }
      }
    }

    for (const [key, value] of Object.entries(schema.extensions || {})) {
      extensions[key] = value;
    }

    return extensions;
  }

  function isResourceSchema(schema?: ComplexSchema) {
    return schema && getExtensions(schema)['x-ms-azure-resource'];
  }

  function* gatherParameterWarnings(parameterBearer: string, parameters: Iterable<Parameter>): Iterable<string> {
    for (const parameter of parameters) {
      // if the parameter is optional or part of the URL, don't generate a warning
      const {
        required,
        language: {
          default: {
            name: parameterDisplayName,
            serializedName = parameterDisplayName
          }
        },
        protocol: {
          http: {
            in: location
          } = { in: ParameterLocation.None }
        }
      } = parameter;

      if (!required || alwaysAllowedParameterLocations.has(location)) {
        continue;
      }

      const allowedRequiredParameters = allowedRequiredParametersByLocation[location];

      // there are some required headers and qs params that are part of the ARM RPC contract, such as
      // `api-version` in the querystring or the `Content-Type` header. If the parameter is one of those,
      // don't generate a warning
      if (allowedRequiredParameters.has(serializedName.toLowerCase())) {
        continue;
      }

      yield `Skipping ${parameterBearer} due to required ${location} parameter "${parameterDisplayName}"`;
    }
  }

  function getProviderDefinitionsForApiVersion(apiVersion: string) {
    const providerDefinitions: Dictionary<ProviderDefinition> = {};
    const operations = codeModel.operationGroups.flatMap(x => x.operations);

    const getOperationsByPath: Dictionary<Operation> = {};
    const putOperationsByPath: Dictionary<Operation> = {};
    const postListOperationsByPath: Dictionary<Operation> = {};

    function addProviderDefinition(namespace: string) {
      const lcNamespace = namespace.toLowerCase();
      if (!providerDefinitions[lcNamespace]) {
        providerDefinitions[lcNamespace] = {
          namespace,
          apiVersion,
          resourcesByType: {},
          resourceActions: [],
        };
      }
    }

    operations.forEach(operation => {
      const operationId = operation.operationId ?? operation.language.default.name;
      const requests = getHttpRequests(operation.requests);
      const getRequest = requests.filter(r => r.httpRequest.method === HttpMethod.Get)[0];
      if (getRequest) {
        const getPath = getRequest.httpRequest.path.toLowerCase();
        const parameterWarnings = [
          ...gatherParameterWarnings(operationId, operation.parameters ?? []),
          ...gatherParameterWarnings(`${getPath}::GET`, getRequest.request.parameters ?? []),
        ];
        if (parameterWarnings.length > 0) {
          for (const warningText of parameterWarnings) {
            logWarning(warningText);
          }
        } else {
          getOperationsByPath[getPath] = operation;
        }
      }

      const putRequest = requests.filter(r => r.httpRequest.method === HttpMethod.Put)[0];
      if (putRequest) {
        const putPath = putRequest.httpRequest.path.toLowerCase();
        const parameterWarnings = [
          ...gatherParameterWarnings(operationId, operation.parameters ?? []),
          ...gatherParameterWarnings(`${putPath}::PUT`, putRequest.request.parameters ?? []),
        ];
        if (parameterWarnings.length > 0) {
          for (const warningText of parameterWarnings) {
            logWarning(warningText);
          }
        } else {
          putOperationsByPath[putPath] = operation;
        }
      }

      const postListRequest = requests.filter(r => {
        if (r.httpRequest.method !== HttpMethod.Post) {
          return false;
        }

        const parseResult = parseResourceScopes(r.httpRequest.path);
        if (!parseResult.success) {
          return false;
        }

        const { routingScope: actionRoutingScope } = parseResult.value;
        const actionName = actionRoutingScope.substr(actionRoutingScope.lastIndexOf('/') + 1);
        if (!actionName.toLowerCase().startsWith('list'))
        {
          return false;
        }

        const parameterWarnings = [
          ...gatherParameterWarnings(operationId, operation.parameters ?? []),
          ...gatherParameterWarnings(`${r.httpRequest.path.toLowerCase()}::POST`, r.request.parameters ?? []),
        ];

        for (const warningText of parameterWarnings) {
          logWarning(warningText);
        }

        return parameterWarnings.length === 0;
      })[0];
      if (postListRequest) {
        postListOperationsByPath[postListRequest.httpRequest.path.toLowerCase()] = operation;
      }
    });

    const resourcesByProvider: Dictionary<ResourceDefinition[]> = {};
    for (const lcPath of new Set<string>([...Object.keys(putOperationsByPath), ...Object.keys(getOperationsByPath)])) {
      const putOperation = putOperationsByPath[lcPath];
      const getOperation = getOperationsByPath[lcPath];
      const putData = getPutSchema(putOperation);
      const getData = getGetSchema(getOperation);

      let parseResult: Result<ResourceDescriptor[], string>;
      if (putData) {
        parseResult = parseResourceMethod(
          putData.request.path,
          putData.parameters,
          apiVersion,
          !!getData,
          true
        );
      } else if (getData && isResourceSchema(getData.responseSchema)) {
        parseResult = parseResourceMethod(getData.request.path, getData.parameters, apiVersion, true, false);
      } else {
        // A non-resource get with no corresponding put is most likely a collection or utility endpoint.
        // No types should be generated
        continue;
      }

      if (!parseResult.success) {
        logWarning(`Skipping path '${putData?.request.path ?? getData?.request.path ?? lcPath}': ${parseResult.error}`);
        continue;
      }

      for (const descriptor of parseResult.value) {
        addProviderDefinition(descriptor.namespace);

        const resource: ResourceDefinition = {
          descriptor,
          putOperation: putData
            ? {
              ...putData,
              requestSchema: (putData?.requestSchema instanceof ObjectSchema) ? putData.requestSchema : undefined,
              responseSchema: (putData?.responseSchema instanceof ObjectSchema) ? putData.responseSchema : undefined,
            }
            : undefined,
          getOperation: getData
            ? {
              ...getData,
              requestSchema: (getData?.requestSchema instanceof ObjectSchema) ? getData.requestSchema : undefined,
              responseSchema: (getData?.responseSchema instanceof ObjectSchema) ? getData.responseSchema : undefined,
            }
            : undefined
        };

        const lcNamespace = descriptor.namespace.toLowerCase();
        resourcesByProvider[lcNamespace] = [
          ...(resourcesByProvider[lcNamespace] || []),
          resource
        ];
      }
    }

    const actionsByProvider: Dictionary<ResourceListActionDefinition[]> = {};
    for (const lcPath in postListOperationsByPath) {
      const listOperation = postListOperationsByPath[lcPath];

      const listData = getPostSchema(listOperation);
      if (!listData) {
        continue;
      }

      const parseResult = parseResourceActionMethod(listData.request.path, listData.parameters, apiVersion);
      if (!parseResult.success) {
        logWarning(`Skipping resource POST action path '${listData.request.path}': ${parseResult.error}`);
        continue;
      }

      const { descriptors, actionName } = parseResult.value;

      for (const descriptor of descriptors) {
        addProviderDefinition(descriptor.namespace);

        const action: ResourceListActionDefinition = {
          actionName: normalizeListActionName(actionName),
          descriptor,
          postRequest: listData.request,
          requestSchema: listData.requestSchema,
          responseSchema: listData.responseSchema,
        };

        const lcNamespace = descriptor.namespace.toLowerCase();
        actionsByProvider[lcNamespace] = [
          ...(actionsByProvider[lcNamespace] || []),
          action
        ];
      }
    }

    for (const namespace of keys(providerDefinitions)) {
      providerDefinitions[namespace].resourcesByType = collapseDefinitions(resourcesByProvider[namespace]);
      providerDefinitions[namespace].resourceActions = collapseActions(actionsByProvider[namespace]);
    }

    return values(providerDefinitions);
  }

  function getRequestSchema(operation: Operation | undefined, requests: Request[]) {
    if (!operation || requests.length === 0) {
      return;
    }

    for (const request of requests) {
      const parameters = combineParameters(operation, request);

      const bodyParameter = parameters.filter(p => (p.protocol.http as HttpParameter)?.in === ParameterLocation.Body)[0];

      if (request.protocol.http instanceof HttpRequest && bodyParameter instanceof Parameter && bodyParameter.schema) {
        return {
          request: request.protocol.http,
          parameters,
          schema: bodyParameter.schema,
        };
      }
    }

    return {
      request: (requests[0].protocol.http as HttpRequest),
      parameters: combineParameters(operation, requests[0]),
    };
  }

  function getResponseSchema(operation?: Operation) {
    const responses = operation?.responses ?? [];
    const validResponses = [
      // order 200 responses before default
      ...responses.filter(r => hasStatusCode(r, "200")),
      ...responses.filter(r => hasStatusCode(r, "default")),
    ];

    if (!operation || validResponses.length === 0) {
      return;
    }

    for (const response of validResponses) {
      if (response.protocol.http instanceof HttpResponse && response instanceof SchemaResponse && response.schema) {
        return {
          response: response.protocol.http,
          schema: response.schema,
        };
      }
    }

    return {
      response: (validResponses[0].protocol.http as HttpResponse),
    };
  }

  function combineParameters(operation: Operation, request: Request) {
    return [...(operation.parameters || []), ...(request.parameters || [])];
  }

  function getGetSchema(operation?: Operation) {
    const requestSchema = getRequestSchema(
      operation,
      operation?.requests?.filter(r => r.protocol.http?.method === HttpMethod.Get) ?? []
    );
    const responseSchema = getResponseSchema(operation);

    if (!requestSchema || !responseSchema) {
      return;
    }

    return {
      request: requestSchema.request,
      response: responseSchema.response,
      parameters: requestSchema.parameters,
      requestSchema: requestSchema.schema,
      responseSchema: responseSchema.schema,
    };
  }

  function getPutSchema(operation?: Operation) {
    const requests = operation?.requests ?? [];
    const validRequests = requests.filter(r => (r.protocol.http as HttpRequest)?.method === HttpMethod.Put);
    const requestSchema = getRequestSchema(operation, validRequests);
    const responseSchema = getResponseSchema(operation);
    if (!requestSchema) {
      return;
    }

    return {
      request: requestSchema.request,
      response: responseSchema?.response,
      parameters: requestSchema.parameters,
      requestSchema: requestSchema.schema,
      responseSchema: responseSchema?.schema,
    };
  }

  function getPostSchema(operation?: Operation) {
    const requests = operation?.requests ?? [];
    const validRequests = requests.filter(r => (r.protocol.http as HttpRequest)?.method === HttpMethod.Post);

    const response = getResponseSchema(operation);
    const request = getRequestSchema(operation, validRequests);

    if (!request || !response) {
      return;
    }

    return {
      request: request.request,
      response: response.response,
      parameters: request.parameters,
      requestSchema: request.schema,
      responseSchema: response.schema,
    };
  }

  function parseResourceScopes(path: string): Result<{scopeType: ScopeType, routingScope: string}, string> {
    path = getNormalizedMethodPath(path);

    const finalProvidersMatch = path.match(parentScopePrefix)?.slice(-1)[0];
    if (!finalProvidersMatch) {
      return failure(`Unable to locate "/providers/" segment`);
    }

    const parentScope = path.substr(0, finalProvidersMatch.length - "providers/".length);
    const routingScope = trimScope(path.substr(finalProvidersMatch.length));

    const scopeType = getScopeTypeFromParentScope(parentScope);

    return success({ scopeType, routingScope });
  }

  function parseResourceDescriptors(
    parameters: Parameter[],
    apiVersion: string,
    scopeType: ScopeType,
    routingScope: string,
    readable: boolean,
    writable: boolean
  ): Result<ResourceDescriptor[], string> {
    const namespace = routingScope.substr(0, routingScope.indexOf('/'));
    if (isPathVariable(namespace)) {
      return failure(`Unable to process parameterized provider namespace "${namespace}"`);
    }

    const parseResult = parseResourceTypes(parameters, routingScope);
    if (!parseResult.success) {
      return parseResult;
    }

    const resNameParam = routingScope.substr(routingScope.lastIndexOf('/') + 1);
    const constantName = isPathVariable(resNameParam) ? undefined : resNameParam;

    const descriptors: ResourceDescriptor[] = parseResult.value.map(type => ({
      scopeType,
      namespace,
      typeSegments: type,
      apiVersion,
      constantName,
      readonlyScopes: readable && !writable ? scopeType : undefined,
    }));

    return success(descriptors);
  }

  function parseResourceMethod(
    path: string,
    parameters: Parameter[],
    apiVersion: string,
    readable: boolean,
    writable: boolean
  ) {
    const resourceScopeResult = parseResourceScopes(path);

    if (!resourceScopeResult.success) {
      return failure(resourceScopeResult.error);
    }

    const { scopeType, routingScope } = resourceScopeResult.value;

    return parseResourceDescriptors(parameters, apiVersion, scopeType, routingScope, readable, writable);
  }

  function parseResourceActionMethod(path: string, parameters: Parameter[], apiVersion: string) {
    const resourceScopeResult = parseResourceScopes(path);

    if (!resourceScopeResult.success) {
      return failure(resourceScopeResult.error);
    }

    const { routingScope: actionRoutingScope, scopeType } = resourceScopeResult.value;

    const routingScope = actionRoutingScope.substr(0, actionRoutingScope.lastIndexOf('/'));
    const actionName = actionRoutingScope.substr(actionRoutingScope.lastIndexOf('/') + 1);

    const resourceDescriptorsResult = parseResourceDescriptors(parameters, apiVersion, scopeType, routingScope, false, true);
    if (!resourceDescriptorsResult.success) {
      return failure(resourceDescriptorsResult.error);
    }

    return success({
      descriptors: resourceDescriptorsResult.value,
      actionName: actionName,
    });
  }

  function parseResourceTypes(parameters: Parameter[], routingScope: string): Result<string[][], string> {
    const typeSegments = routingScope.split('/').slice(1).filter((_, i) => i % 2 === 0);
    const nameSegments = routingScope.split('/').slice(1).filter((_, i) => i % 2 === 1);

    if (typeSegments.length === 0) {
      return failure(`Unable to find type segments`);
    }

    if (typeSegments.length !== nameSegments.length) {
      return failure(`Found mismatch between type segments (${typeSegments.length}) and name segments (${nameSegments.length})`);
    }

    let resourceTypes: string[][] = [[]];
    for (const typeSegment of typeSegments) {
      if (isPathVariable(typeSegment)) {
        const parameterName = trimParamBraces(typeSegment);
        const parameter = parameters.filter(p =>
          p.implementation === ImplementationLocation.Method &&
          getSerializedName(p) === parameterName)[0];

        if (!parameter) {
          return failure(`Found undefined parameter reference ${typeSegment}`);
        }

        const choiceSchema = parameter.schema;
        if (!(choiceSchema instanceof ChoiceSchema || choiceSchema instanceof SealedChoiceSchema)) {
          return failure(`Parameter reference ${typeSegment} is not defined as an enum`);
        }

        if (choiceSchema.choices.length === 0) {
          return failure(`Parameter reference ${typeSegment} is defined as an enum, but doesn't have any specified values`);
        }

        resourceTypes = resourceTypes.flatMap(type => choiceSchema.choices.map(v => [...type, v.value.toString()]));
      } else {
        resourceTypes = resourceTypes.map(type => [...type, typeSegment]);
      }
    }

    return success(resourceTypes);
  }

  function getScopeTypeFromParentScope(parentScope: string) {
    if (tenantPrefix.test(parentScope)) {
      return ScopeType.Tenant;
    }

    if (managementGroupPrefix.test(parentScope)) {
      return ScopeType.ManagementGroup;
    }

    if (resourceGroupPrefix.test(parentScope)) {
      return ScopeType.ResourceGroup;
    }

    if (subscriptionPrefix.test(parentScope)) {
      return ScopeType.Subscription;
    }

    if (parentScopePrefix.test(parentScope)) {
      return ScopeType.Extension;
    }

    // ambiguous - without any further information, we have to assume 'all'
    return ScopeType.Unknown;
  }

  function mergeScopes(scopeA: ScopeType, scopeB: ScopeType) {
    // We have to assume any (unknown) scope if either scope is unknown
    // Bitwise OR will not handle this case correctly as 'unknown' is 0.
    if (scopeA == ScopeType.Unknown || scopeB == ScopeType.Unknown) {
      return ScopeType.Unknown;
    }

    return scopeA | scopeB;
  }

  function mergeReadonlyScopes(
    currentScopes: ScopeType,
    currentReadonlyScopes: ScopeType|undefined,
    newScopes: ScopeType,
    newReadonlyScopes: ScopeType|undefined
  ) {
    function writableScopes(scopes: ScopeType, readonlyScopes: ScopeType|undefined) {
      return readonlyScopes !== undefined ? scopes ^ readonlyScopes : scopes;
    }
    const mergedScopes = mergeScopes(currentScopes, newScopes);
    if (mergedScopes === ScopeType.Unknown) {
      const writingPermittedSomewhere = currentScopes !== currentReadonlyScopes || newScopes !== newReadonlyScopes;
      return writingPermittedSomewhere ? undefined : ScopeType.Unknown;
    }

    const mergedWritableScopes = writableScopes(currentScopes, currentReadonlyScopes) | writableScopes(newScopes, newReadonlyScopes);
    return mergedScopes === mergedWritableScopes ? undefined : mergedScopes ^ mergedWritableScopes;
  }

  function collapseDefinitionScopes(resources: ResourceDefinition[]) {
    const definitionsByName: Dictionary<ResourceDefinition> = {};
    for (const resource of resources) {
      const name = resource.descriptor.constantName ?? '';
      if (definitionsByName[name]) {
        const curDescriptor = definitionsByName[name].descriptor;
        const newDescriptor = resource.descriptor;

        definitionsByName[name] = {
          ...definitionsByName[name],
          descriptor: {
            ...curDescriptor,
            scopeType: mergeScopes(curDescriptor.scopeType, newDescriptor.scopeType),
            readonlyScopes: mergeReadonlyScopes(curDescriptor.scopeType, curDescriptor.readonlyScopes, newDescriptor.scopeType, newDescriptor.readonlyScopes),
          },
        };
      } else {
        definitionsByName[name] = resource;
      }
    }

    return Object.values(definitionsByName);
  }

  function collapsePartiallyConstantNameResources(resources: ResourceDefinition[])
  {
    const definitionsByNormalizedPath = resources.reduce((acc, resource) => {
      const path = resource.putOperation?.request.path ?? resource.getOperation?.request.path ?? '/';
      const normalizedPath = path.substring(0, path.lastIndexOf('/') + 1);
      if (acc[normalizedPath]) {
        acc[normalizedPath].push(resource);
      } else {
        acc[normalizedPath] = [resource];
      }

      return acc;
    }, {} as Dictionary<ResourceDefinition[]>);

    function hasComparableSchemata(
      definitions: ResourceDefinition[],
      schemaExtractor: (r: ResourceDefinition) => ObjectSchema|undefined
    ) {
      return chain(definitions)
        .map(schemaExtractor)
        .filter()
        .map(s => s!.language.default.name)
        .uniq()
        .value().length < 2;
    }

    for (const path of Object.keys(definitionsByNormalizedPath)) {
      const atPath = definitionsByNormalizedPath[path];
      const parameterized = chain(atPath).map(r => r.descriptor).filter(d => d.constantName === undefined).value();

      if (
        parameterized.length === 1 &&
        hasComparableSchemata(atPath, d => d.putOperation?.requestSchema) &&
        hasComparableSchemata(atPath, d => d.getOperation?.responseSchema)
      ) {
        let scopeType = atPath[0].descriptor.scopeType;
        let readonlyScopes = atPath[0].descriptor.readonlyScopes;
        for (let i = 1; i < atPath.length; i++) {
          const {scopeType: newScopes, readonlyScopes: newReadonlyScopes} = atPath[i].descriptor;
          scopeType = mergeScopes(scopeType, newScopes);
          readonlyScopes = mergeReadonlyScopes(scopeType, readonlyScopes, newScopes, newReadonlyScopes);
        }

        definitionsByNormalizedPath[path] = [{
          descriptor: {
            ...parameterized[0],
            scopeType,
            readonlyScopes,
          },
          putOperation: chain(atPath).map(r => r.putOperation).find().value(),
          getOperation: chain(atPath).map(r => r.getOperation).find().value(),
        }];
      }
    }

    return flatten(Object.values(definitionsByNormalizedPath));
  }

  function collapseDefinitions(resources: ResourceDefinition[]) {
    const deduplicated = Object.values(groupByType(resources)).flatMap(collapsePartiallyConstantNameResources);
    const collapsedResources = Object.values(groupByType(deduplicated)).flatMap(collapseDefinitionScopes);

    return groupByType(collapsedResources);
  }

  function collapseActions(actions: ResourceListActionDefinition[]) {
    const actionsByType = groupByType(actions);

    return Object.values(actionsByType).flatMap(actions => uniqBy(actions, x => x.actionName.toLowerCase()));
  }

  return getProviderDefinitions();
}
