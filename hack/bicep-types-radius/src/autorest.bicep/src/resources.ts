// ------------------------------------------------------------
// Copyright 2023 The Radius Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// ------------------------------------------------------------.

import { ChoiceSchema, CodeModel, HttpMethod, HttpParameter, HttpRequest, HttpResponse, ImplementationLocation, ObjectSchema, Operation, Parameter, ParameterLocation, Request, Response, Schema, SchemaResponse, SealedChoiceSchema, Metadata } from "@autorest/codemodel";
import { Channel, AutorestExtensionHost } from "@autorest/extension-base";
import { keys, Dictionary, values, groupBy, uniqBy } from 'lodash';
import { success, failure, Result } from './utils';

export enum ScopeType {
  Unknown = 0,
  Tenant = 1 << 0,
  ManagementGroup = 1 << 1,
  Subscription = 1 << 2,
  ResourceGroup = 1 << 3,
  Extension = 1 << 4,
}

export interface ResourceDescriptor {
  scopeType: ScopeType;
  namespace: string;
  typeSegments: string[];
  apiVersion: string;
  constantName?: string;
}

export interface ProviderDefinition {
  namespace: string;
  apiVersion: string;
  resourcesByType: Dictionary<ResourceDefinition[]>;
  resourceActions: ResourceListActionDefinition[];
}

export interface ResourceDefinition {
  descriptor: ResourceDescriptor;
  putRequest: HttpRequest;
  putParameters: Parameter[];
  putSchema?: ObjectSchema;
  getSchema?: ObjectSchema;
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
  return requests?.map(x => x.protocol.http as HttpRequest).filter(x => !!x) ?? [];
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

export function parseNameSchema<T>(request: HttpRequest, parameters: Parameter[], parseType: (schema: Schema) => T, createConstantName: (name: string) => T): Result<T, string> {
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

    return success(parseType(param.schema));
  }

  if (!/^[a-zA-Z0-9]*$/.test(resNameParam)) {
    return failure(`Unable to process non-alphanumeric name '${resNameParam}'`);
  }

  return success(createConstantName(resNameParam));
}

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
      const requests = getHttpRequests(operation.requests);
      const getRequest = requests.filter(r => r.method === HttpMethod.Get)[0];
      if (getRequest) {
        getOperationsByPath[getRequest.path.toLowerCase()] = operation;
      }
      const putRequest = requests.filter(r => r.method === HttpMethod.Put)[0];
      if (putRequest) {
        putOperationsByPath[putRequest.path.toLowerCase()] = operation;
      }
      const postListRequest = requests.filter(r => { 
        if (r.method !== HttpMethod.Post) {
          return false;
        }

        const parseResult = parseResourceScopes(r.path);
        if (!parseResult.success) {
          return false;
        }

        const { routingScope: actionRoutingScope } = parseResult.value;
        const actionName = actionRoutingScope.substr(actionRoutingScope.lastIndexOf('/') + 1);
        return actionName.toLowerCase().startsWith('list');
      })[0];
      if (postListRequest) {
        postListOperationsByPath[postListRequest.path.toLowerCase()] = operation;
      }
    });

    const resourcesByProvider: Dictionary<ResourceDefinition[]> = {};
    for (const lcPath in putOperationsByPath) {
      const putOperation = putOperationsByPath[lcPath];
      const getOperation = getOperationsByPath[lcPath];

      const putData = getPutSchema(putOperation);
      const getData = getGetSchema(getOperation) ?? putData;
      if (!putData || !getData) {
        continue;
      }

      const parseResult = parseResourceMethod(putData.request.path, putData.parameters, apiVersion);
      if (!parseResult.success) {
        logWarning(`Skipping path '${putData.request.path}': ${parseResult.error}`);
        continue;
      }

      for (const descriptor of parseResult.value) {
        addProviderDefinition(descriptor.namespace);

        const resource: ResourceDefinition = {
          descriptor,
          putRequest: putData.request,
          putParameters: putData.parameters,
          putSchema: (putData.schema instanceof ObjectSchema) ? putData.schema : undefined,
          getSchema: (getData.schema instanceof ObjectSchema) ? getData.schema : undefined,
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
    return getResponseSchema(operation);
  }

  function getPutSchema(operation?: Operation) {
    const requests = operation?.requests ?? [];
    const validRequests = requests.filter(r => (r.protocol.http as HttpRequest)?.method === HttpMethod.Put);

    return getRequestSchema(operation, validRequests);
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

  function parseResourceDescriptors(parameters: Parameter[], apiVersion: string, scopeType: ScopeType, routingScope: string): Result<ResourceDescriptor[], string> {
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
    }));

    return success(descriptors);
  }

  function parseResourceMethod(path: string, parameters: Parameter[], apiVersion: string) {
    const resourceScopeResult = parseResourceScopes(path);

    if (!resourceScopeResult.success) {
      return failure(resourceScopeResult.error);
    }

    const { scopeType, routingScope } = resourceScopeResult.value;

    return parseResourceDescriptors(parameters, apiVersion, scopeType, routingScope);
  }

  function parseResourceActionMethod(path: string, parameters: Parameter[], apiVersion: string) {
    const resourceScopeResult = parseResourceScopes(path);

    if (!resourceScopeResult.success) {
      return failure(resourceScopeResult.error);
    }

    const { routingScope: actionRoutingScope, scopeType } = resourceScopeResult.value;

    const routingScope = actionRoutingScope.substr(0, actionRoutingScope.lastIndexOf('/'));
    const actionName = actionRoutingScope.substr(actionRoutingScope.lastIndexOf('/') + 1);

    const resourceDescriptorsResult = parseResourceDescriptors(parameters, apiVersion, scopeType, routingScope);
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
          },
        };
      } else {
        definitionsByName[name] = resource;
      }
    }

    return Object.values(definitionsByName);
  }

  function collapseDefinitions(resources: ResourceDefinition[]) {
    const resourcesByType = groupByType(resources);
    const collapsedResources = Object.values(resourcesByType).flatMap(collapseDefinitionScopes);

    return groupByType(collapsedResources);
  }

  function collapseActions(actions: ResourceListActionDefinition[]) {
    const actionsByType = groupByType(actions);

    return Object.values(actionsByType).flatMap(actions => uniqBy(actions, x => x.actionName.toLowerCase()));
  }

  return getProviderDefinitions();
}