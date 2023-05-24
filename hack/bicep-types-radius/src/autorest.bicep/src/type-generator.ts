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

import { AnySchema, ArraySchema, ChoiceSchema, ConstantSchema, DictionarySchema, ObjectSchema, PrimitiveSchema, Property, Schema, SchemaType, SealedChoiceSchema, StringSchema } from "@autorest/codemodel";
import { Channel, AutorestExtensionHost } from "@autorest/extension-base";
import { ArrayType, BuiltInTypeKind, DiscriminatedObjectType, ObjectProperty, ObjectPropertyFlags, ObjectType, ResourceFunctionType, ResourceType, StringLiteralType, TypeFactory, TypeReference, UnionType } from "./types";
import { uniq, keys, keyBy, Dictionary, flatMap } from 'lodash';
import { getFullyQualifiedType, getSerializedName, parseNameSchema, ProviderDefinition, ResourceDefinition, ResourceDescriptor } from "./resources";

export function generateTypes(host: AutorestExtensionHost, definition: ProviderDefinition) {
  const factory = new TypeFactory();
  const namedDefinitions: Dictionary<TypeReference> = {};

  function logWarning(message: string) {
    host.message({ Channel: Channel.Warning, Text: message, });
  }

  function logInfo(message: string) {
    host.message({ Channel: Channel.Information, Text: message, });
  }

  function processResourceBody(fullyQualifiedType: string, definition: ResourceDefinition) {
    const { descriptor, putRequest, putParameters, putSchema, getSchema, } = definition;
    const nameSchemaResult = parseNameSchema(
      putRequest,
      putParameters,
      schema => parseType(schema, schema),
      (name) => factory.addType(new StringLiteralType(name)));

    if (!nameSchemaResult.success) {
      logWarning(`Skipping resource type ${fullyQualifiedType} under path '${putRequest.path}': ${nameSchemaResult.error}`);
      return
    }

    if (!nameSchemaResult.value) {
      logWarning(`Skipping resource type ${fullyQualifiedType} under path '${putRequest.path}': failed to obtain a name value`);
      return
    }

    const resourceProperties = getStandardizedResourceProperties(descriptor, nameSchemaResult.value);

    let resourceDefinition: TypeReference;
    if (putSchema) {
      resourceDefinition = createObject(getFullyQualifiedType(descriptor), putSchema, resourceProperties);
    } else {
      logInfo(`Resource type ${fullyQualifiedType} under path '${putRequest.path}' has no body defined.`);
      resourceDefinition = factory.addType(new ObjectType(getFullyQualifiedType(descriptor), resourceProperties));
    }

    for (const { propertyName, putProperty, getProperty } of getObjectTypeProperties(putSchema, getSchema, true)) {
      if (resourceProperties[propertyName]) {
        continue;
      }

      const propertyDefinition = parseType(putProperty?.schema, getProperty?.schema);
      if (propertyDefinition) {
        const description = (putProperty?.schema ?? getProperty?.schema)?.language.default?.description;
        const flags = parsePropertyFlags(putProperty, getProperty);
        resourceProperties[propertyName] = createObjectProperty(propertyDefinition, flags, description);
      }
    }

    if (putSchema?.discriminator || getSchema?.discriminator) {
      const discriminatedObjectType = factory.lookupType(resourceDefinition) as DiscriminatedObjectType;

      handlePolymorphicType(discriminatedObjectType, putSchema, getSchema);
    }

    return resourceDefinition;
  }

  function processResource(fullyQualifiedType: string, definitions: ResourceDefinition[]) {
    if (definitions.length > 1) {
      for (const definition of definitions) {
        if (!definition.descriptor.constantName) {
          logWarning(`Skipping resource type ${fullyQualifiedType} under path '${definitions[0].putRequest.path}': Found multiple definitions for the same type`);
          return null;
        }
      }
        
      const polymorphicBodies: Dictionary<TypeReference> = {};
      for (const definition of definitions) {
        const bodyType = processResourceBody(fullyQualifiedType, definition);
        if (!bodyType || !definition.descriptor.constantName) {
          return null;
        }
        
        polymorphicBodies[definition.descriptor.constantName] = bodyType;
      }

      const discriminatedBodyType = factory.addType(new DiscriminatedObjectType(
        fullyQualifiedType,
        'name',
        {},
        polymorphicBodies));

      const descriptor = {
        ...definitions[0].descriptor,
        constantName: undefined,
      };

      return {
        descriptor,
        bodyType: discriminatedBodyType
      };
    } else {
      const definition = definitions[0];
      const bodyType = processResourceBody(fullyQualifiedType, definition);
      if (!bodyType) {
        return null;
      }

      return {
        descriptor: definition.descriptor,
        bodyType: bodyType,
      };
    }
  }

  function generateTypes() {
    const { resourcesByType, resourceActions } = definition;

    for (const fullyQualifiedType in resourcesByType) {
      const definitions = resourcesByType[fullyQualifiedType];

      const output = processResource(fullyQualifiedType, definitions);
      if (!output) {
        continue;
      }

      const { descriptor, bodyType } = output;

      factory.addType(new ResourceType(
        `${getFullyQualifiedType(descriptor)}@${descriptor.apiVersion}`,
        descriptor.scopeType,
        bodyType));
    }

    for (const action of resourceActions) {
      let request: TypeReference | undefined;
      if (action.requestSchema) {
        request = parseType(action.requestSchema, undefined);
        if (!request) {
          continue;
        }
      }

      if (!action.responseSchema) {
        logWarning(`Skipping resource action ${action.actionName} under path '${action.postRequest.path}': failed to find a response schema`);
        continue;
      }

      const response = parseType(undefined, action.responseSchema);
      if (!response) {
        continue;
      }

      const { actionName, descriptor } = action;

      factory.addType(new ResourceFunctionType(
        actionName,
        getFullyQualifiedType(descriptor),
        descriptor.apiVersion,
        response,
        request));
    }

    return factory.types;
  }

  function getStandardizedResourceProperties(descriptor: ResourceDescriptor, resourceName: TypeReference): Dictionary<ObjectProperty> {
    const type = factory.addType(new StringLiteralType(getFullyQualifiedType(descriptor)));

    return {
      id: createObjectProperty(factory.lookupBuiltInType(BuiltInTypeKind.String), ObjectPropertyFlags.ReadOnly | ObjectPropertyFlags.DeployTimeConstant, 'The resource id'),
      name: createObjectProperty(resourceName, ObjectPropertyFlags.Required | ObjectPropertyFlags.DeployTimeConstant, 'The resource name'),
      type: createObjectProperty(type, ObjectPropertyFlags.ReadOnly | ObjectPropertyFlags.DeployTimeConstant, 'The resource type'),
      apiVersion: createObjectProperty(factory.addType(new StringLiteralType(descriptor.apiVersion)), ObjectPropertyFlags.ReadOnly | ObjectPropertyFlags.DeployTimeConstant, 'The resource api version'),
    };
  }

  function createObject(definitionName: string, schema: ObjectSchema, properties: Dictionary<ObjectProperty>, additionalProperties?: TypeReference) {
    if (schema.discriminator) {
      return factory.addType(new DiscriminatedObjectType(
        definitionName,
        schema.discriminator.property.serializedName,
        properties,
        {}));
    }

    return factory.addType(new ObjectType(definitionName, properties, additionalProperties));
  }

  function combineAndThrowIfNull<TSchema extends Schema>(putSchema: TSchema | undefined, getSchema: TSchema | undefined) {
    const output = putSchema ?? getSchema;
    if (!output) {
      throw 'Unable to find PUT or GET type';
    }

    return output;
  }

  function getSchemaProperties(schema: ObjectSchema, includeBaseProperties: boolean): Dictionary<Property> {
    const objects = [schema];
    if (includeBaseProperties) {
      for (const parent of schema.parents?.all || []) {
        if (parent instanceof ObjectSchema) {
          objects.push(parent);
        }
      }
    }

    return keyBy(flatMap(objects, o => o.properties || []), p => p.serializedName);
  }

  function* getObjectTypeProperties(putSchema: ObjectSchema | undefined, getSchema: ObjectSchema | undefined, includeBaseProperties: boolean) {
    const putProperties = putSchema ? getSchemaProperties(putSchema, includeBaseProperties) : {};
    const getProperties = getSchema ? getSchemaProperties(getSchema, includeBaseProperties) : {};

    for (const propertyName of uniq([...keys(putProperties), ...keys(getProperties)])) {
      if ((putSchema?.discriminator?.property && putSchema.discriminator.property === putProperties[propertyName]) ||
        (getSchema?.discriminator?.property && getSchema.discriminator.property === getProperties[propertyName])) {
        continue;
      }

      const putProperty = putProperties[propertyName] as Property | undefined
      const getProperty = getProperties[propertyName] as Property | undefined

      yield { propertyName, putProperty, getProperty };
    }
  }

  function flattenDiscriminatorSubTypes(schema: ObjectSchema | undefined) {
    if (!schema || !schema.discriminator) {
      return {};
    }

    const output: Dictionary<ObjectSchema> = {};
    for (const key in schema.discriminator.all) {
      const value = schema.discriminator.all[key];

      if (!(value instanceof ObjectSchema)) {
        throw `Unable to flatten discriminated properties - schema '${getSerializedName(schema)}' has non-object discriminated value '${getSerializedName(value)}'.`;
      }

      if (!value.discriminator) {
        output[key] = value;
        continue;
      }

      if (schema.discriminator.property.serializedName !== value.discriminator.property.serializedName) {
        throw `Unable to flatten discriminated properties - schemas '${getSerializedName(schema)}' and '${getSerializedName(value)}' have conflicting discriminators '${schema.discriminator.property.serializedName}' and '${value.discriminator.property.serializedName}'`;
      }

      const subTypes = flattenDiscriminatorSubTypes(value);
      for (const subTypeKey in subTypes) {
        output[subTypeKey] = subTypes[subTypeKey];
      }
    }

    return output;
  }

  function* getDiscriminatedSubTypes(putSchema: ObjectSchema | undefined, getSchema: ObjectSchema | undefined) {
    const putSubTypes = flattenDiscriminatorSubTypes(putSchema);
    const getSubTypes = flattenDiscriminatorSubTypes(getSchema);

    for (const subTypeName of uniq([...keys(putSubTypes), ...keys(getSubTypes)])) {
      yield { 
        subTypeName,
        putSubType: putSubTypes[subTypeName],
        getSubType: getSubTypes[subTypeName],
      };
    }
  }

  function parseType(putSchema: Schema | undefined, getSchema: Schema | undefined): TypeReference | undefined {
    const combinedSchema = combineAndThrowIfNull(putSchema, getSchema);

    // A schema that matches a JSON object with specific properties, such as
    // { "name": { "type": "string" }, "age": { "type": "number" } }
    if (combinedSchema instanceof ObjectSchema) {
      return parseObjectType(putSchema as ObjectSchema, getSchema as ObjectSchema, true);
    }

    // A schema that matches a "dictionary" JSON object, such as
    // { "additionalProperties": { "type": "string" } }
    if (combinedSchema instanceof DictionarySchema) {
      return parseDictionaryType(putSchema as DictionarySchema, getSchema as DictionarySchema);
    }

    // A schema that matches a single value from a given set of values, such as
    // { "enum": [ "a", "b" ] }
    if (combinedSchema instanceof ChoiceSchema) {
      return parseEnumType(putSchema as ChoiceSchema, getSchema as ChoiceSchema);
    }
    if (combinedSchema instanceof SealedChoiceSchema) {
      return parseEnumType(putSchema as SealedChoiceSchema, getSchema as SealedChoiceSchema);
    }
    if (combinedSchema instanceof ConstantSchema) {
      return parseConstant(putSchema as ConstantSchema, getSchema as ConstantSchema);
    }

    // A schema that matches an array of values, such as
    // { "items": { "type": "number" } }
    if (combinedSchema instanceof ArraySchema) {
      return parseArrayType(putSchema as ArraySchema, getSchema as ArraySchema);
    }

    // A schema that matches simple values, such as { "type": "number" }
    if (combinedSchema instanceof PrimitiveSchema) {
      return parsePrimaryType(putSchema as PrimitiveSchema, getSchema as PrimitiveSchema);
    }

    // The 'any' type
    if (combinedSchema instanceof AnySchema) {
      return factory.lookupBuiltInType(BuiltInTypeKind.Any);
    }

    logWarning(`Unrecognized property type: ${combinedSchema.type}. Returning 'any'.`);
    return factory.lookupBuiltInType(BuiltInTypeKind.Any);
  }

  function getMutabilityFlags(property: Property | undefined) {
    const mutability = property?.extensions?.["x-ms-mutability"] as string[];
    if (!mutability) {
      return ObjectPropertyFlags.None;
    }

    const writable = mutability.includes('create') || mutability.includes('update');
    const readable = mutability.includes('read');

    if (writable && !readable) {
      return ObjectPropertyFlags.WriteOnly;
    }

    if (readable && !writable) {
      return ObjectPropertyFlags.ReadOnly;
    }

    return ObjectPropertyFlags.None;
  }

  function parsePropertyFlags(putProperty: Property | undefined, getProperty: Property | undefined) {
    let flags = ObjectPropertyFlags.None;

    if (putProperty && putProperty.required) {
      flags |= ObjectPropertyFlags.Required;
    }

    if (putProperty && getProperty) {
      flags |= getMutabilityFlags(putProperty);
    }

    if (!putProperty || putProperty.readOnly) {
      flags |= ObjectPropertyFlags.ReadOnly;
    }

    if (!getProperty) {
      flags |= ObjectPropertyFlags.WriteOnly;
    }

    return flags;
  }

  function parsePrimaryType(putSchema: PrimitiveSchema | undefined, getSchema: PrimitiveSchema | undefined) {
    const combinedSchema = combineAndThrowIfNull(putSchema, getSchema);

    switch (combinedSchema.type) {
      case SchemaType.Boolean:
        return factory.lookupBuiltInType(BuiltInTypeKind.Bool);
      case SchemaType.Integer:
      case SchemaType.Number:
      case SchemaType.UnixTime:
        return factory.lookupBuiltInType(BuiltInTypeKind.Int);
      case SchemaType.Object:
        return factory.lookupBuiltInType(BuiltInTypeKind.Any);
      case SchemaType.ByteArray:
        return factory.lookupBuiltInType(BuiltInTypeKind.Array);
      case SchemaType.Uri:
      case SchemaType.Date:
      case SchemaType.DateTime:
      case SchemaType.Time:
      case SchemaType.String:
      case SchemaType.Uuid:
      case SchemaType.Duration:
      case SchemaType.Credential:
        return factory.lookupBuiltInType(BuiltInTypeKind.String);
      default:
        logWarning(`Unrecognized known property type: "${combinedSchema.type}"`);
        return factory.lookupBuiltInType(BuiltInTypeKind.Any);
    }
  }

  function handlePolymorphicType(discriminatedObjectType: DiscriminatedObjectType, putSchema?: ObjectSchema, getSchema?: ObjectSchema) {
    for (const { putSubType, getSubType } of getDiscriminatedSubTypes(putSchema, getSchema)) {
      const combinedSubType = combineAndThrowIfNull(putSubType, getSubType);

      if (!combinedSubType.discriminatorValue) {
        continue;
      }

      const objectTypeRef = parseObjectType(putSubType, getSubType, false);
      const objectType = factory.lookupType(objectTypeRef);
      if (!(objectType instanceof ObjectType)) {
        logWarning(`Found unexpected element of discriminated type '${discriminatedObjectType.Name}'`)
        continue;
      }

      discriminatedObjectType.Elements[combinedSubType.discriminatorValue] = objectTypeRef;

      const description = (putSchema ?? getSchema)?.discriminator?.property.language.default.description;
      objectType.Properties[discriminatedObjectType.Discriminator] = createObjectProperty(
        factory.addType(new StringLiteralType(combinedSubType.discriminatorValue)),
        ObjectPropertyFlags.Required,
        description);
    }
  }

  function parseObjectType(putSchema: ObjectSchema | undefined, getSchema: ObjectSchema | undefined, includeBaseProperties: boolean) {
    const combinedSchema = combineAndThrowIfNull(putSchema, getSchema);
    const definitionName = getSerializedName(combinedSchema);

    if (includeBaseProperties && namedDefinitions[definitionName]) {
      // if we're building a discriminated subtype, we're going to be missing the base properties
      // so construct the type on-the-fly, and don't cache it globally
      return namedDefinitions[definitionName];
    }
    
    let additionalProperties: TypeReference | undefined;
    if (includeBaseProperties) {
      const putParentDictionary = (putSchema?.parents?.all || []).filter(x => x instanceof DictionarySchema).map(x => x as DictionarySchema)[0];
      const getParentDictionary = (getSchema?.parents?.all || []).filter(x => x instanceof DictionarySchema).map(x => x as DictionarySchema)[0];

      if (putParentDictionary || getParentDictionary) {
        additionalProperties = parseType(putParentDictionary?.elementType, getParentDictionary?.elementType);
      }
    }

    const definitionProperties: Dictionary<ObjectProperty> = {};
    const definition = createObject(definitionName, combinedSchema, definitionProperties, additionalProperties);
    if (includeBaseProperties) {
      // cache the definition so that it can be re-used
      namedDefinitions[definitionName] = definition;
    }

    for (const { propertyName, putProperty, getProperty } of getObjectTypeProperties(putSchema, getSchema, includeBaseProperties)) {
      const propertyDefinition = parseType(putProperty?.schema, getProperty?.schema);
      if (propertyDefinition) {
        const description = (putProperty?.schema ?? getProperty?.schema)?.language.default?.description;
        const flags = parsePropertyFlags(putProperty, getProperty);
        definitionProperties[propertyName] = createObjectProperty(propertyDefinition, flags, description);
      }
    }

    if (combinedSchema.discriminator) {
      const discriminatedObjectType = factory.lookupType(definition) as DiscriminatedObjectType;

      handlePolymorphicType(discriminatedObjectType, putSchema, getSchema);
    }

    return definition;
  }

  function parseEnumType(putSchema: ChoiceSchema | SealedChoiceSchema | undefined, getSchema: ChoiceSchema | SealedChoiceSchema | undefined) {
    const combinedSchema = combineAndThrowIfNull(putSchema, getSchema);

    if (!(combinedSchema.choiceType instanceof StringSchema)) {
      // we can only handle string enums right now
      return parseType(putSchema?.choiceType, getSchema?.choiceType);
    }

    const enumTypes = [];
    for (const enumValue of combinedSchema.choices) {
      const stringLiteralType = factory.addType(new StringLiteralType(enumValue.value.toString()));
      enumTypes.push(stringLiteralType);
    }

    if (enumTypes.length === 1) {
      return enumTypes[0];
    }

    return factory.addType(new UnionType(enumTypes));
  }

  function parseConstant(putSchema: ConstantSchema | undefined, getSchema: ConstantSchema | undefined) {
    const combinedSchema = combineAndThrowIfNull(putSchema, getSchema);
    const constantValue = combinedSchema.value;

    return factory.addType(new StringLiteralType(constantValue.value.toString()));
  }

  function parseDictionaryType(putSchema: DictionarySchema | undefined, getSchema: DictionarySchema | undefined) {
    const combinedSchema = combineAndThrowIfNull(putSchema, getSchema);
    const additionalPropertiesType = parseType(putSchema?.elementType, getSchema?.elementType);

    return factory.addType(new ObjectType(getSerializedName(combinedSchema), {}, additionalPropertiesType));
  }

  function parseArrayType(putSchema: ArraySchema | undefined, getSchema: ArraySchema | undefined) {
    const itemType = parseType(putSchema?.elementType, getSchema?.elementType);
    if (!itemType) {
      return factory.lookupBuiltInType(BuiltInTypeKind.Array);
    }

    return factory.addType(new ArrayType(itemType));
  }

  function createObjectProperty(type: TypeReference, flags: ObjectPropertyFlags, description?: string): ObjectProperty {
    return new ObjectProperty(type, flags, description?.trim() || undefined);
  }

  return generateTypes();
}