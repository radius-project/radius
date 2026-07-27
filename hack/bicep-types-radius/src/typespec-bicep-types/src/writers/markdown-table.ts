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
  ArrayType,
  BicepType,
  BuiltInType,
  BuiltInTypeKind,
  DiscriminatedObjectType,
  ObjectType,
  ObjectTypeProperty,
  ObjectTypePropertyFlags,
  ResourceType,
  StringLiteralType,
  TypeBaseKind,
  TypeReference,
  UnionType
} from "bicep-types";

/**
 * ARM envelope properties that are not authored on the resource and are hidden
 * from the reference docs so the output focuses on the resource-specific
 * properties, matching `rad resource-type show`. The resource-specific
 * properties live under the envelope's `properties` object, which this writer
 * descends into for the top-level table.
 */
const ENVELOPE_PROPERTIES = new Set([
  "id",
  "name",
  "type",
  "apiversion",
  "location",
  "tags",
  "systemdata"
]);

/** The set of properties displayed for an object, plus any map value type. */
interface PropertyBag {
  properties: Record<string, ObjectTypeProperty>;
  additionalProperties?: TypeReference;
  /**
   * Discriminated-union metadata, present when the object selects its shape via
   * a discriminator property. Each element is documented as its own variant
   * section and the discriminator row links to them.
   */
  discriminator?: {
    propertyName: string;
    elements: Record<string, TypeReference>;
  };
}

/** A pending object section to render, keyed by its dotted-path heading. */
interface Section extends PropertyBag {
  heading: string;
  /**
   * Type indices of the sections on the path from the root to this one
   * (inclusive of this section's own type). Distinct paths that share a type
   * are each expanded into their own section (matching the CLI), while this set
   * guards against genuinely recursive types producing an unbounded traversal.
   */
  ancestors: Set<number>;
}

/**
 * Renders one resource type as reference markdown that mirrors the layout of
 * `rad resource-type show`: an optional description block, a `Top-Level
 * Properties` table, and an `Object Properties` section with one dotted-path
 * table per nested object.
 *
 * The document body starts at heading level 2. The docs pipeline
 * (`generate_resource_references.py` in the docs repo) wraps this content with
 * Hugo front matter, so this writer owns every heading below the page title.
 *
 * @param resourceTypes The resource types to render (the emitter passes one).
 * @param types The full Bicep type graph the resource types index into.
 * @param description The resource-level TypeSpec `@doc`, when authored.
 */
export function writeTableMarkdown(
  resourceTypes: ResourceType[],
  types: BicepType[],
  description?: string
): string {
  const lines: string[] = [];

  if (description && description.trim().length > 0) {
    lines.push("## Description", "", description.trim(), "");
  }

  for (const resourceType of resourceTypes) {
    const topLevel = getTopLevelProperties(types, resourceType);
    writeResource(lines, types, topLevel);
  }

  return lines.join("\n").replace(/\n+$/, "\n");
}

/**
 * Resolves the resource-specific properties shown as the top-level table. These
 * live under the envelope's `properties` object; if that is absent, the envelope
 * properties are used with the ARM envelope keys filtered out.
 */
function getTopLevelProperties(
  types: BicepType[],
  resourceType: ResourceType
): PropertyBag {
  const body = types[resourceType.body.index];
  if (body.type !== TypeBaseKind.ObjectType) {
    return { properties: {} };
  }

  const envelope = body as ObjectType;
  const propertiesRef = envelope.properties["properties"];
  if (propertiesRef) {
    const bag = resolveSection(types, propertiesRef.type);
    if (bag) {
      return {
        properties: bag.properties,
        additionalProperties: bag.additionalProperties
      };
    }
  }

  const filtered: Record<string, ObjectTypeProperty> = {};
  for (const [name, property] of Object.entries(envelope.properties)) {
    if (!ENVELOPE_PROPERTIES.has(name.toLowerCase())) {
      filtered[name] = property;
    }
  }
  return { properties: filtered };
}

/**
 * Walks the resource's properties breadth-first, emitting the top-level table
 * followed by one table per nested object under dotted-path headings. This
 * mirrors the traversal in the `rad resource-type show` display.
 */
function writeResource(
  lines: string[],
  types: BicepType[],
  topLevel: PropertyBag
): void {
  const queue: Section[] = [
    { heading: "", ancestors: new Set<number>(), ...topLevel }
  ];
  let state: "none" | "top-level" | "object" = "none";

  while (queue.length > 0) {
    const section = queue.shift() as Section;
    const names = sortedKeys(section.properties);

    // Expand each nested object into its own section keyed by dotted path, so a
    // type shared across several paths is documented once per path (matching
    // the CLI, which walks an inlined JSON schema). Child anchors are recorded
    // per property name for this section's Type-cell links. The ancestors set
    // guards against genuinely recursive types without collapsing distinct
    // paths. Top-level object properties are keyed by name, deeper ones by path.
    const childAnchors = new Map<string, string>();
    const childHeading = (key: string): string =>
      state === "none" ? key : `${section.heading}.${key}`;

    for (const name of names) {
      const target = resolveLinkTarget(types, section.properties[name].type);
      if (!target || section.ancestors.has(target.section.index)) {
        continue;
      }
      const heading = childHeading(name);
      childAnchors.set(name, anchor(heading));
      queue.push({
        heading,
        ancestors: new Set(section.ancestors).add(target.section.index),
        ...target.section
      });
    }

    // Document each discriminated-union variant as its own section, dropping the
    // discriminator literal from the variant table since it is already shown on
    // the parent's discriminator row.
    if (section.discriminator) {
      const { propertyName, elements } = section.discriminator;
      for (const value of sortedKeys(elements)) {
        const ref = elements[value];
        const elementType = types[ref.index];
        if (
          elementType.type !== TypeBaseKind.ObjectType ||
          section.ancestors.has(ref.index)
        ) {
          continue;
        }
        const properties: Record<string, ObjectTypeProperty> = {};
        for (const [key, prop] of Object.entries(
          (elementType as ObjectType).properties
        )) {
          if (key !== propertyName) {
            properties[key] = prop;
          }
        }
        queue.push({
          heading: childHeading(value),
          ancestors: new Set(section.ancestors).add(ref.index),
          properties,
          additionalProperties: (elementType as ObjectType).additionalProperties
        });
      }
    }

    if (state === "none") {
      state = "top-level";
      lines.push("## Top-Level Properties", "");
    } else {
      if (state === "top-level") {
        lines.push("## Object Properties", "");
      }
      state = "object";
      if (section.heading) {
        lines.push(
          `### \`${section.heading}\` {#${anchor(section.heading)}}`,
          ""
        );
      }
    }

    writeTable(lines, types, section, childAnchors);
    lines.push("");
  }
}

/** Emits a property table for a single object section. */
function writeTable(
  lines: string[],
  types: BicepType[],
  section: Section,
  childAnchors: Map<string, string>
): void {
  const names = sortedKeys(section.properties);
  if (
    names.length === 0 &&
    !section.additionalProperties &&
    !section.discriminator
  ) {
    lines.push("_No properties._");
    return;
  }

  lines.push("| Property | Type | Required | Read-Only | Description |");
  lines.push("|----------|------|----------|-----------|-------------|");

  if (section.discriminator) {
    lines.push(writeDiscriminatorRow(section));
  }

  for (const name of names) {
    const property = section.properties[name];
    const enumValues = getEnumValues(types, property.type);
    const type =
      enumValues ? "string" : (
        getLinkedType(types, property.type, childAnchors.get(name))
      );
    const required = (property.flags & ObjectTypePropertyFlags.Required) !== 0;
    const readOnly = (property.flags & ObjectTypePropertyFlags.ReadOnly) !== 0;
    let description = sanitizeCell(property.description ?? "");
    if (enumValues) {
      const allowed = enumValues.map((value) => `\`${value}\``).join(", ");
      const sentence = `Allowed values: ${allowed}.`;
      description = description ? `${description}<br />${sentence}` : sentence;
    }
    lines.push(
      `| \`${name}\` | ${type} | ${required} | ${readOnly} | ${description} |`
    );
  }

  if (section.additionalProperties) {
    const type = getPropertyType(types, section.additionalProperties);
    lines.push(
      `| \`*\` | ${type} | false | false | Additional properties keyed by name. |`
    );
  }
}

/**
 * Renders the discriminator property row for a discriminated-union section. The
 * allowed values link to each variant's section so readers can jump to the
 * shape selected by that value.
 */
function writeDiscriminatorRow(section: Section): string {
  const { propertyName, elements } = section.discriminator!;
  const allowed = sortedKeys(elements)
    .map((value) => {
      const path = section.heading ? `${section.heading}.${value}` : value;
      return `[\`${value}\`](#${anchor(path)})`;
    })
    .join(", ");
  const description = `Discriminator property that selects the variant. Allowed values: ${allowed}.`;
  return `| \`${propertyName}\` | string | true | false | ${description} |`;
}

/** A resolved object section together with the index of its source type. */
type ResolvedSection = PropertyBag & { index: number };

/**
 * Resolves the object section a property should document. Named objects and
 * discriminated objects resolve to their own properties. A map/`Record` (an
 * object with no named properties) unwraps to its value type, so a
 * `map<RecipeDefinition>` documents the RecipeDefinition shape rather than an
 * empty section. Scalar-valued maps and non-object types resolve to nothing and
 * are surfaced inline on the parent row instead.
 */
function resolveSection(
  types: BicepType[],
  reference: TypeReference
): ResolvedSection | undefined {
  const type = types[reference.index];
  if (type.type === TypeBaseKind.ObjectType) {
    const objectType = type as ObjectType;
    if (Object.keys(objectType.properties).length > 0) {
      return {
        index: reference.index,
        properties: objectType.properties,
        additionalProperties: objectType.additionalProperties
      };
    }
    if (objectType.additionalProperties) {
      return resolveSection(types, objectType.additionalProperties);
    }
    return undefined;
  }
  if (type.type === TypeBaseKind.DiscriminatedObjectType) {
    const discriminated = type as DiscriminatedObjectType;
    return {
      index: reference.index,
      properties: discriminated.baseProperties,
      discriminator: {
        propertyName: discriminated.discriminator,
        elements: discriminated.elements
      }
    };
  }
  return undefined;
}

/** A resolved section together with whether the property is an array of it. */
interface LinkTarget {
  section: ResolvedSection;
  isArray: boolean;
}

/**
 * Resolves the object section a property (or its array element / map value)
 * expands to, so the Type column can link to it. Arrays set `isArray` so the
 * Type renders `[object](#anchor)[]`. Returns undefined when nothing expands.
 */
function resolveLinkTarget(
  types: BicepType[],
  reference: TypeReference
): LinkTarget | undefined {
  const type = types[reference.index];
  if (type.type === TypeBaseKind.ArrayType) {
    const inner = resolveLinkTarget(types, (type as ArrayType).itemType);
    return inner ? { section: inner.section, isArray: true } : undefined;
  }
  const section = resolveSection(types, reference);
  return section ? { section, isArray: false } : undefined;
}

/**
 * Builds a deterministic heading anchor from a dotted section path. Section
 * paths are unique, so the slug is collision-free. Emitting an explicit `{#id}`
 * on each heading avoids depending on Hugo's auto-slugger and its `-1`/`-2`
 * dedup behavior, keeping the Type-column links stable.
 */
function anchor(headingPath: string): string {
  return headingPath
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/(^-|-$)/g, "");
}

/**
 * Renders a property's Type cell. Object-valued properties (including maps and
 * arrays of objects) link to the `### <path>` section that expands them, so
 * `object` becomes a jump link and arrays render `[object](#anchor)[]`. Types
 * without an expanded section fall back to their plain type name.
 */
function getLinkedType(
  types: BicepType[],
  reference: TypeReference,
  anchorId: string | undefined
): string {
  if (anchorId) {
    const target = resolveLinkTarget(types, reference);
    if (target) {
      const link = `[object](#${anchorId})`;
      return target.isArray ? `${link}[]` : link;
    }
  }
  return getPropertyType(types, reference);
}

/**
 * Maps a Bicep type to a JSON-schema-style type name (`string`, `integer`,
 * `boolean`, `object`, `array`, `any`) to match the CLI's property tables.
 * Enums (unions of string literals) render their allowed values, for example
 * `'terraform' \| 'bicep'`.
 */
function getPropertyType(types: BicepType[], reference: TypeReference): string {
  const type = types[reference.index];
  switch (type.type) {
    case TypeBaseKind.ObjectType:
    case TypeBaseKind.DiscriminatedObjectType:
      return "object";
    case TypeBaseKind.ArrayType:
      return getArrayType(types, type as ArrayType);
    case TypeBaseKind.BooleanType:
      return "boolean";
    case TypeBaseKind.IntegerType:
      return "integer";
    case TypeBaseKind.StringType:
    case TypeBaseKind.StringLiteralType:
      return "string";
    case TypeBaseKind.AnyType:
      return "any";
    case TypeBaseKind.NullType:
      return "null";
    case TypeBaseKind.UnionType:
      return getUnionType(types, type as UnionType);
    case TypeBaseKind.BuiltInType:
      return getBuiltInType((type as BuiltInType).kind);
    default:
      return "string";
  }
}

/** Renders an array as `<scalar> array`, or `array` when the element is complex. */
function getArrayType(types: BicepType[], type: ArrayType): string {
  const element = getPropertyType(types, type.itemType);
  return /^[a-z]+$/.test(element) ? `${element} array` : "array";
}

/**
 * Returns the sorted allowed values of an enum (a union whose elements are all
 * string literals), or undefined for any other type. Used to move enum values
 * out of the Type column and into the Description.
 */
function getEnumValues(
  types: BicepType[],
  reference: TypeReference
): string[] | undefined {
  const type = types[reference.index];
  if (type.type !== TypeBaseKind.UnionType) {
    return undefined;
  }
  const values: string[] = [];
  for (const element of (type as UnionType).elements) {
    const elementType = types[element.index];
    if (elementType.type !== TypeBaseKind.StringLiteralType) {
      return undefined; // not a pure string-literal enum
    }
    values.push((elementType as StringLiteralType).value);
  }
  return values.length > 0 ? values.sort() : undefined;
}

/**
 * Renders a union. Enums (unions of string literals) list their allowed values
 * as `'a' \| 'b'`, with the pipe escaped so the value stays inside one table
 * cell. Non-literal unions collapse to their underlying type(s).
 */
function getUnionType(types: BicepType[], type: UnionType): string {
  const literals: string[] = [];
  let allLiterals = true;
  for (const element of type.elements) {
    const elementType = types[element.index];
    if (elementType.type === TypeBaseKind.StringLiteralType) {
      literals.push(`'${(elementType as StringLiteralType).value}'`);
    } else {
      allLiterals = false;
      break;
    }
  }
  if (allLiterals && literals.length > 0) {
    return literals.sort().join(" \\| ");
  }

  const kinds = new Set(
    type.elements.map((element) => getPropertyType(types, element))
  );
  if (kinds.size === 1) {
    return [...kinds][0];
  }
  return [...kinds].sort().join(" or ");
}

/** Maps a built-in type kind to its JSON-schema-style name. */
function getBuiltInType(kind: BuiltInTypeKind): string {
  switch (kind) {
    case BuiltInTypeKind.Bool:
      return "boolean";
    case BuiltInTypeKind.Int:
      return "integer";
    case BuiltInTypeKind.String:
      return "string";
    case BuiltInTypeKind.Object:
      return "object";
    case BuiltInTypeKind.Array:
      return "array";
    case BuiltInTypeKind.Null:
      return "null";
    default:
      return "any";
  }
}

/** Flattens a property description to a single table-cell-safe line. */
function sanitizeCell(text: string): string {
  return text
    .replace(/\r?\n/g, " ")
    .replace(/\|/g, "\\|")
    .replace(/\s+/g, " ")
    .trim();
}

/** Returns the keys of a record sorted case-insensitively and ascending. */
function sortedKeys(record: Record<string, unknown>): string[] {
  return Object.keys(record).sort((a, b) => {
    const ka = a.toLowerCase();
    const kb = b.toLowerCase();
    return (
      ka < kb ? -1
      : ka > kb ? 1
      : 0
    );
  });
}
