// ------------------------------------------------------------
// Copyright 2023 The Radius Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// ------------------------------------------------------------.

import {
  getDiscriminator,
  getDoc,
  getLifecycleVisibilityEnum,
  getVisibilityForClass,
  type Model,
  type ModelProperty,
  type Program,
  type Scalar,
  type Type
} from "@typespec/compiler";
import { getExtensions } from "@typespec/openapi";
import type { HttpOperation } from "@typespec/http";
import {
  createObjectProperty,
  DiscriminatedObjectType,
  FunctionParameter,
  ObjectType,
  ObjectTypeProperty,
  ObjectTypePropertyFlags,
  ResourceTypeFunction,
  TypeFactory,
  TypeReference
} from "./bicep.js";
import { getStandardizedResourceProperties } from "./standardized-props.js";
import type {
  DiscoveredAction,
  DiscoveredResource
} from "./resource-discovery.js";

/** The standardized envelope keys, owned by {@link getStandardizedResourceProperties}. */
const ENVELOPE_KEYS = new Set(["id", "name", "type", "apiVersion"]);

/**
 * Translates a TypeSpec type into a Bicep type and registers it with the factory,
 * returning a reference. This is the TypeSpec-native equivalent of the AutoRest
 * extension's `parseType`, reading the compiler's type graph directly instead of
 * a modelerfour CodeModel.
 *
 * The `cache` keyed by TypeSpec type breaks reference cycles and de-duplicates
 * repeated models (mirroring the AutoRest extension's `namedDefinitions`).
 */
export function parseType(
  program: Program,
  factory: TypeFactory,
  type: Type,
  cache: Map<Type, TypeReference>
): TypeReference {
  const cached = cache.get(type);
  if (cached) {
    return cached;
  }

  switch (type.kind) {
    case "Scalar":
      return mapScalar(factory, type);
    case "Boolean":
      return factory.addBooleanType();
    case "Number":
      // Bicep has no numeric-literal type; fall back to integer (as AutoRest did).
      return factory.addIntegerType();
    case "String":
      return factory.addStringLiteralType(type.value);
    case "Model":
      return parseModel(program, factory, type, cache);
    case "Enum": {
      const elements = [...type.members.values()].map((member) => {
        const value = member.value ?? member.name;
        return typeof value === "number" ?
            factory.addIntegerType()
          : factory.addStringLiteralType(String(value));
      });
      return factory.addUnionType(elements);
    }
    case "Union": {
      const variantTypes = [...type.variants.values()].map(
        (variant) => variant.type
      );
      // Extensible enums in TypeSpec are unions of string literals plus a bare
      // `string` (e.g. ARM's `createdByType`). Bicep represents these as the
      // closed set of known values, so drop the open `string` arm when string
      // literals are present - matching the AutoRest output.
      const hasStringLiteral = variantTypes.some(
        (variant) => variant.kind === "String"
      );
      const elements = variantTypes
        .filter((variant) => !(hasStringLiteral && isStringScalar(variant)))
        .map((variant) => parseType(program, factory, variant, cache));
      return factory.addUnionType(elements);
    }
    case "Intrinsic":
      return type.name === "null" ?
          factory.addNullType()
        : factory.addAnyType();
    default:
      return factory.addAnyType();
  }
}

/**
 * Translates the effective properties of a model (its own properties plus those
 * inherited from base models) into Bicep object-type properties.
 *
 * @param skipKeys - Property names to omit (e.g. the standardized envelope keys
 *   already added by {@link getStandardizedResourceProperties}).
 */
export function translateModelProperties(
  program: Program,
  factory: TypeFactory,
  model: Model,
  cache: Map<Type, TypeReference>,
  skipKeys?: Set<string>,
  topLevel = false
): Record<string, ObjectTypeProperty> {
  return translateProperties(
    program,
    factory,
    collectProperties(model),
    cache,
    skipKeys,
    topLevel
  );
}

/**
 * Translates a set of model properties into Bicep object-type properties,
 * applying `properties`-bag flattening and `@visibility`-derived flags. Callers
 * pass either a model's full (base-chain) properties or a subtype's own-only
 * properties (for discriminated elements).
 */
function translateProperties(
  program: Program,
  factory: TypeFactory,
  effective: Map<string, ModelProperty>,
  cache: Map<Type, TypeReference>,
  skipKeys?: Set<string>,
  topLevel = false
): Record<string, ObjectTypeProperty> {
  const properties: Record<string, ObjectTypeProperty> = {};

  // Precompute the full sibling-name set so the flatten collision check can see
  // siblings that appear later in iteration, mirroring the AutoRest extension.
  const siblingNames = new Set<string>(skipKeys);
  for (const name of effective.keys()) {
    siblingNames.add(name);
  }

  for (const [name, property] of effective) {
    if (skipKeys?.has(name)) {
      continue;
    }

    // `@extension("x-ms-client-flatten", true)` on the ARM `properties` bag:
    // hoist its children as ReadOnly aliases, then fall through to also emit the
    // original wrapper property (the writable authoring envelope).
    if (isFlattenedBag(program, property)) {
      hoistFlattenedChildren(
        program,
        factory,
        property,
        properties,
        siblingNames,
        cache
      );
    }

    properties[name] = createObjectProperty(
      parseType(program, factory, property.type, cache),
      topLevelPropertyFlags(program, property, name, topLevel),
      getDoc(program, property)
    );
  }

  return properties;
}

/**
 * Creates a translation cache keyed by TypeSpec `Type`. One cache is shared
 * across every resource in a namespace so that types referenced by more than one
 * resource (e.g. `SystemData`, provisioning-state enums, `EnvironmentCompute`)
 * are emitted once and shared by reference - matching how the AutoRest path
 * de-duplicated shared definitions within a single `types.json`.
 */
export function newTranslationCache(): Map<Type, TypeReference> {
  return new Map<Type, TypeReference>();
}

/**
 * Builds the Bicep resource type for a discovered resource and registers it with
 * the factory, returning the resource type reference.
 *
 * The body merges the standardized `id`/`name`/`type`/`apiVersion` envelope with
 * the translated properties of the resource model (including `properties`-bag
 * flattening, discriminated types, and `@visibility`-derived flags).
 */
export function buildResourceType(
  program: Program,
  factory: TypeFactory,
  resource: DiscoveredResource,
  cache: Map<Type, TypeReference>
): TypeReference {
  // The resource name is modeled as a plain string. Name-schema parsing for a
  // constant or constrained name segment is not ported (no Radius resource
  // currently relies on it).
  const resourceName = factory.addStringType();

  const properties = getStandardizedResourceProperties(
    factory,
    resource.fullyQualifiedType,
    resource.apiVersion,
    resourceName
  );

  Object.assign(
    properties,
    translateModelProperties(
      program,
      factory,
      resource.bodyModel,
      cache,
      ENVELOPE_KEYS,
      true
    )
  );

  const body = factory.addObjectType(resource.fullyQualifiedType, properties);
  const functions = buildResourceFunctions(
    program,
    factory,
    resource.actions,
    cache
  );

  return factory.addResourceType(
    resource.resourceTypeName,
    body,
    resource.readableScopes,
    resource.writableScopes,
    functions
  );
}

/**
 * Builds the resource's functions (e.g. `listSecrets`) from its ARM actions,
 * mirroring the AutoRest extension: each action becomes a FunctionType whose
 * parameters come from the request body's properties and whose output is the
 * response body. Actions without a response body are skipped.
 */
function buildResourceFunctions(
  program: Program,
  factory: TypeFactory,
  actions: DiscoveredAction[],
  cache: Map<Type, TypeReference>
): Record<string, ResourceTypeFunction> {
  const functions: Record<string, ResourceTypeFunction> = {};

  for (const action of actions) {
    const responseType = getSuccessResponseBody(action.httpOperation);
    if (!responseType) {
      continue;
    }

    const output = parseType(program, factory, responseType, cache);
    const parameters = buildFunctionParameters(
      program,
      factory,
      action.httpOperation,
      cache
    );

    functions[action.name] = {
      type: factory.addFunctionType(parameters, output),
      description: action.name
    };
  }

  return functions;
}

/**
 * Builds function parameters from the action request body's properties. An empty
 * or non-model request body yields no parameters (e.g. `listSecrets`'s `{}`).
 */
function buildFunctionParameters(
  program: Program,
  factory: TypeFactory,
  http: HttpOperation,
  cache: Map<Type, TypeReference>
): FunctionParameter[] {
  const body = http.parameters.body?.type;
  if (!body || body.kind !== "Model") {
    return [];
  }

  const parameters: FunctionParameter[] = [];
  for (const [name, property] of collectProperties(body)) {
    parameters.push({
      name,
      type: parseType(program, factory, property.type, cache),
      description: getDoc(program, property)
    });
  }
  return parameters;
}

/** Returns the body type of the action's first 2xx response, if any. */
function getSuccessResponseBody(http: HttpOperation): Type | undefined {
  for (const response of http.responses) {
    const code = response.statusCodes;
    if (typeof code !== "number" || code < 200 || code >= 300) {
      continue;
    }
    for (const content of response.responses) {
      if (content.body) {
        return content.body.type;
      }
    }
  }
  return undefined;
}

/**
 * Translates a model that is an array, a record (dictionary), or a plain object.
 */
function parseModel(
  program: Program,
  factory: TypeFactory,
  model: Model,
  cache: Map<Type, TypeReference>
): TypeReference {
  const indexer = model.indexer;
  if (indexer) {
    // `T[]` is a model named "Array" indexed by `integer`; `Record<T>` is named
    // "Record" indexed by `string`.
    if (indexer.key.name === "integer") {
      return factory.addArrayType(
        parseType(program, factory, indexer.value, cache)
      );
    }
    if (indexer.key.name === "string") {
      return factory.addObjectType(
        model.name || "object",
        {},
        parseType(program, factory, indexer.value, cache)
      );
    }
  }

  // Plain object: pre-register an empty object type to break cycles, then fill
  // its properties in place (the factory returns the stored object by reference).
  const discriminator = getDiscriminator(program, model);
  if (discriminator?.propertyName) {
    return parseDiscriminatedType(
      program,
      factory,
      model,
      discriminator.propertyName,
      cache
    );
  }

  const ref = factory.addObjectType(model.name || "object", {});
  cache.set(model, ref);
  const objectType = factory.lookupType(ref) as ObjectType;
  objectType.properties = translateModelProperties(
    program,
    factory,
    model,
    cache
  );

  // A model that `extends Record<T>` carries the string indexer on a base model
  // rather than on `model.indexer` (the inline `Record<T>` / `is Record<T>` case
  // handled above), so detect it here and emit Bicep `additionalProperties`.
  // For example `ExtenderProperties extends Record<unknown>` must keep accepting
  // arbitrary properties such as the `message` used by failure-test recipes.
  const recordValue = inheritedRecordValue(model);
  if (recordValue) {
    objectType.additionalProperties = parseType(
      program,
      factory,
      recordValue,
      cache
    );
  }
  return ref;
}

/**
 * Walks the `baseModel` chain for a `string`-keyed indexer contributed by a
 * `Record<T>` base (`extends Record<T>`), returning the value type to emit as
 * Bicep `additionalProperties`. The model's own indexer is handled by the
 * caller, so only base models are inspected here.
 */
function inheritedRecordValue(model: Model): Type | undefined {
  for (let base = model.baseModel; base; base = base.baseModel) {
    if (base.indexer && base.indexer.key.name === "string") {
      return base.indexer.value;
    }
  }
  return undefined;
}

/**
 * Translates a model with `@discriminator` into a Bicep DiscriminatedObjectType:
 * the base's shared properties (minus the discriminator) become `baseProperties`,
 * and each subtype (from `derivedModels`, recursing through nested discriminators)
 * becomes an element keyed by its discriminator value, sorted to match AutoRest.
 */
function parseDiscriminatedType(
  program: Program,
  factory: TypeFactory,
  model: Model,
  discriminator: string,
  cache: Map<Type, TypeReference>
): TypeReference {
  // Pre-register an empty discriminated type to break cycles, then fill in place.
  const ref = factory.addDiscriminatedObjectType(
    model.name || "object",
    discriminator,
    {},
    {}
  );
  cache.set(model, ref);
  const discriminated = factory.lookupType(ref) as DiscriminatedObjectType;

  discriminated.baseProperties = translateModelProperties(
    program,
    factory,
    model,
    cache,
    new Set([discriminator])
  );

  const elements: Record<string, TypeReference> = {};
  for (const subtype of collectDiscriminatedSubtypes(model)) {
    const value = getDiscriminatorValue(subtype, discriminator);
    if (value === undefined) {
      continue;
    }
    elements[value] = factory.addObjectType(
      subtype.name || "object",
      translateProperties(program, factory, subtype.properties, cache)
    );
  }

  // Sort elements by discriminator value to match the AutoRest output order.
  discriminated.elements = Object.fromEntries(
    Object.entries(elements).sort(([a], [b]) => a.localeCompare(b))
  );

  return ref;
}

/** Collects the leaf subtypes of a discriminated model, recursing through nested discriminators. */
function collectDiscriminatedSubtypes(model: Model): Model[] {
  const subtypes: Model[] = [];
  for (const derived of model.derivedModels) {
    if (derived.derivedModels.length > 0) {
      subtypes.push(...collectDiscriminatedSubtypes(derived));
    } else {
      subtypes.push(derived);
    }
  }
  return subtypes;
}

/** Reads a subtype's discriminator value (the string literal it assigns to the discriminator property). */
function getDiscriminatorValue(
  subtype: Model,
  discriminator: string
): string | undefined {
  const property = subtype.properties.get(discriminator);
  return property?.type.kind === "String" ? property.type.value : undefined;
}

/**
 * The Bicep primitive a TypeSpec scalar collapses to. Bicep models only boolean,
 * integer, and string primitives, so every numeric scalar (int and float)
 * collapses to integer and every other scalar (string, url, uuid, dates,
 * durations, bytes) collapses to string - matching the AutoRest output. A scalar
 * derives from exactly one base primitive, so the first match up the chain wins.
 */
function mapScalarKind(scalar: Scalar): "boolean" | "integer" | "string" {
  for (
    let current: Scalar | undefined = scalar;
    current;
    current = current.baseScalar
  ) {
    if (current.name === "boolean") {
      return "boolean";
    }
    if (current.name === "numeric") {
      return "integer";
    }
  }
  return "string";
}

/** Maps a scalar to the closest Bicep primitive type. */
function mapScalar(factory: TypeFactory, scalar: Scalar): TypeReference {
  const kind = mapScalarKind(scalar);
  if (kind === "boolean") {
    return factory.addBooleanType();
  }
  if (kind === "integer") {
    return factory.addIntegerType();
  }
  return factory.addStringType();
}

/**
 * True for the `string` primitive (or any scalar that collapses to it). Used to
 * detect and drop the open `string` arm of an extensible enum (a union of string
 * literals plus a bare `string`), which Bicep represents as a closed set.
 */
function isStringScalar(type: Type): boolean {
  return type.kind === "Scalar" && mapScalarKind(type) === "string";
}

/**
 * True when a property carries `@extension("x-ms-client-flatten", true)` - the
 * marker the Radius ARM `properties` envelope uses (see
 * `typespec/radius/v1/trackedresource.tsp`). This is the same signal the AutoRest
 * extension read from the generated OpenAPI.
 */
function isFlattenedBag(program: Program, property: ModelProperty): boolean {
  return getExtensions(program, property).get("x-ms-client-flatten") === true;
}

/**
 * Hoists the children of a flattened `properties` bag into `target` as ReadOnly
 * aliases. The hoist is all-or-nothing: if any child name collides with an
 * existing or pending sibling, nothing is hoisted (the nested wrapper alone is
 * kept), matching the AutoRest extension. Non-object or discriminated bags are
 * not flattened.
 */
function hoistFlattenedChildren(
  program: Program,
  factory: TypeFactory,
  bag: ModelProperty,
  target: Record<string, ObjectTypeProperty>,
  siblingNames: Set<string>,
  cache: Map<Type, TypeReference>
): void {
  const child = bag.type;
  if (
    child.kind !== "Model" ||
    child.indexer ||
    getDiscriminator(program, child)
  ) {
    return;
  }

  const childProperties = collectProperties(child);

  for (const childName of childProperties.keys()) {
    if (siblingNames.has(childName)) {
      return;
    }
  }

  for (const [childName, childProperty] of childProperties) {
    const childFlags = propertyFlags(program, childProperty);
    target[childName] = createObjectProperty(
      parseType(program, factory, childProperty.type, cache),
      flagsForFlattenedChild(childFlags),
      getDoc(program, childProperty)
    );
    siblingNames.add(childName);
  }
}

/**
 * Derives a property's Bicep flags from its TypeSpec lifecycle visibility:
 * read-only properties (visible for Read but not Create/Update) become ReadOnly,
 * write-only become WriteOnly, and read-write properties carry Required when not
 * optional. A property with no `@visibility` is visible in every phase, so it is
 * treated as read-write. ReadOnly properties are never also Required, matching
 * the AutoRest output.
 */
function propertyFlags(
  program: Program,
  property: ModelProperty
): ObjectTypePropertyFlags {
  const visible = getVisibilityForClass(
    program,
    property,
    getLifecycleVisibilityEnum(program)
  );
  const memberNames = new Set([...visible].map((member) => member.name));

  // An empty set means no explicit visibility - visible everywhere (read-write).
  const readable = memberNames.size === 0 || memberNames.has("Read");
  const writable =
    memberNames.size === 0 ||
    memberNames.has("Create") ||
    memberNames.has("Update");

  if (readable && !writable) {
    return ObjectTypePropertyFlags.ReadOnly;
  }

  let flags = ObjectTypePropertyFlags.None;
  if (writable && !readable) {
    flags |= ObjectTypePropertyFlags.WriteOnly;
  }
  if (!property.optional) {
    flags |= ObjectTypePropertyFlags.Required;
  }
  return flags;
}

/**
 * Computes a property's flags, applying the top-level `location` exception: a
 * resource's top-level `location` envelope property is never Required, even
 * though ARM's TrackedResource declares it required. This mirrors the AutoRest
 * extension's `parsePropertyFlags` special case and preserves the established
 * Bicep authoring contract (`location` is optional on Radius resources).
 */
function topLevelPropertyFlags(
  program: Program,
  property: ModelProperty,
  name: string,
  topLevel: boolean
): ObjectTypePropertyFlags {
  const flags = propertyFlags(program, property);
  if (topLevel && name === "location") {
    return flags & ~ObjectTypePropertyFlags.Required;
  }
  return flags;
}

/**
 * Flattened children are ReadOnly output projections of the writable `properties`
 * payload, so force ReadOnly and strip Required/WriteOnly (which describe the
 * wrapper, not the alias) - matching the AutoRest extension.
 */
function flagsForFlattenedChild(
  childFlags: ObjectTypePropertyFlags
): ObjectTypePropertyFlags {
  return (
    ObjectTypePropertyFlags.ReadOnly |
    (childFlags &
      ~(ObjectTypePropertyFlags.Required | ObjectTypePropertyFlags.WriteOnly))
  );
}

/**
 * Collects a model's effective properties in AutoRest order: the most-derived
 * model's own properties first, then up the base chain. A property declared on a
 * derived model wins over (and keeps the position of) an inherited one of the
 * same name. This matches the AutoRest extension's `[schema, ...parents]`
 * ordering, which places a resource's `properties`-bag children ahead of the
 * inherited envelope extras (`tags`/`location`/`systemData`).
 */
function collectProperties(model: Model): Map<string, ModelProperty> {
  const properties = new Map<string, ModelProperty>();

  for (
    let current: Model | undefined = model;
    current;
    current = current.baseModel
  ) {
    for (const [name, property] of current.properties) {
      if (!properties.has(name)) {
        properties.set(name, property);
      }
    }
  }

  return properties;
}
