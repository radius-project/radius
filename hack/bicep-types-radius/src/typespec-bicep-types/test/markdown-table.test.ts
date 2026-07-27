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

import { describe, expect, it } from "vitest";
import {
  ObjectTypePropertyFlags,
  ResourceType,
  ScopeType,
  TypeFactory,
  createObjectProperty
} from "../src/bicep.js";
import { writeTableMarkdown } from "../src/writers/markdown-table.js";

/**
 * Builds a small resource type graph shaped like a real ARM resource: an
 * envelope with the standardized properties plus a `properties` object holding
 * the resource-specific fields, one of which nests another object
 * (`providers` -> `providers.azure`).
 */
function buildResource(): { factory: TypeFactory; resourceType: ResourceType } {
  const factory = new TypeFactory();
  const stringRef = factory.addStringType();
  const boolRef = factory.addBooleanType();
  const stringArrayRef = factory.addArrayType(factory.addStringType());
  const enumRef = factory.addUnionType([
    factory.addStringLiteralType("Succeeded"),
    factory.addStringLiteralType("Failed")
  ]);

  const azureRef = factory.addObjectType("ProvidersAzure", {
    subscriptionId: createObjectProperty(
      stringRef,
      ObjectTypePropertyFlags.Required,
      "(Required) ID of the Azure subscription."
    ),
    resourceGroupName: createObjectProperty(
      stringRef,
      ObjectTypePropertyFlags.None,
      "(Optional) Name of the Azure resource group."
    )
  });

  const providersRef = factory.addObjectType("Providers", {
    azure: createObjectProperty(
      azureRef,
      ObjectTypePropertyFlags.None,
      "(Optional) Configuration for deploying resources to Azure."
    )
  });

  // A map<RecipeDefinition>: an object with no named properties whose value
  // type is documented as its own section.
  const recipeDefinitionRef = factory.addObjectType("RecipeDefinition", {
    kind: createObjectProperty(
      factory.addUnionType([
        factory.addStringLiteralType("bicep"),
        factory.addStringLiteralType("terraform")
      ]),
      ObjectTypePropertyFlags.Required,
      "(Required) The kind of recipe."
    ),
    source: createObjectProperty(
      stringRef,
      ObjectTypePropertyFlags.Required,
      "(Required) The source of the recipe."
    )
  });
  const recipesMapRef = factory.addObjectType(
    "RecipeDefinitionMap",
    {},
    recipeDefinitionRef
  );

  // A map whose value type is an array of objects (map<ProviderConfig[]>). The
  // element object shape must still expand into its own linked section even
  // though the map value is an ArrayType rather than an ObjectType.
  const providerConfigRef = factory.addObjectType("ProviderConfig", {
    name: createObjectProperty(
      stringRef,
      ObjectTypePropertyFlags.Required,
      "(Required) The provider config name."
    )
  });
  const providerConfigsMapRef = factory.addObjectType(
    "ProviderConfigMap",
    {},
    factory.addArrayType(providerConfigRef)
  );

  // An array<Extension>: the element object is documented as its own section
  // and the Type column links to it as `[object](#extensions)[]`.
  const extensionRef = factory.addObjectType("Extension", {
    manifest: createObjectProperty(
      stringRef,
      ObjectTypePropertyFlags.None,
      "(Optional) The extension manifest."
    )
  });
  const extensionsArrayRef = factory.addArrayType(extensionRef);

  // A discriminated union (like EnvironmentCompute): a `kind` discriminator with
  // shared base properties plus per-variant properties. The variant objects
  // carry the discriminator literal, which the writer omits from their tables.
  const computeRef = factory.addDiscriminatedObjectType(
    "EnvironmentCompute",
    "kind",
    {
      resourceId: createObjectProperty(
        stringRef,
        ObjectTypePropertyFlags.None,
        "(Optional) The compute resource id."
      )
    },
    {
      kubernetes: factory.addObjectType("KubernetesCompute", {
        kind: createObjectProperty(
          factory.addStringLiteralType("kubernetes"),
          ObjectTypePropertyFlags.Required,
          "Discriminator value."
        ),
        namespace: createObjectProperty(
          stringRef,
          ObjectTypePropertyFlags.Required,
          "(Required) The Kubernetes namespace."
        )
      }),
      aci: factory.addObjectType("ACICompute", {
        kind: createObjectProperty(
          factory.addStringLiteralType("aci"),
          ObjectTypePropertyFlags.Required,
          "Discriminator value."
        ),
        resourceGroup: createObjectProperty(
          stringRef,
          ObjectTypePropertyFlags.Required,
          "(Required) The ACI resource group."
        )
      })
    }
  );

  // A shared object type referenced from two properties to prove each path is
  // expanded into its own section rather than de-duplicated by type identity.
  const endpointRef = factory.addObjectType("EndpointInfo", {
    host: createObjectProperty(
      stringRef,
      ObjectTypePropertyFlags.Required,
      "(Required) The endpoint host."
    )
  });

  const propertiesRef = factory.addObjectType("EnvironmentProperties", {
    providers: createObjectProperty(
      providersRef,
      ObjectTypePropertyFlags.None,
      "(Optional) Target compute platform."
    ),
    compute: createObjectProperty(
      computeRef,
      ObjectTypePropertyFlags.None,
      "(Optional) The compute platform."
    ),
    primaryEndpoint: createObjectProperty(
      endpointRef,
      ObjectTypePropertyFlags.None,
      "(Optional) The primary endpoint."
    ),
    secondaryEndpoint: createObjectProperty(
      endpointRef,
      ObjectTypePropertyFlags.None,
      "(Optional) The secondary endpoint."
    ),
    recipes: createObjectProperty(
      recipesMapRef,
      ObjectTypePropertyFlags.None,
      "(Optional) Recipes keyed by name."
    ),
    providerConfigs: createObjectProperty(
      providerConfigsMapRef,
      ObjectTypePropertyFlags.None,
      "(Optional) Provider configs keyed by name."
    ),
    extensions: createObjectProperty(
      extensionsArrayRef,
      ObjectTypePropertyFlags.None,
      "(Optional) Extensions applied to the Environment."
    ),
    simulated: createObjectProperty(
      boolRef,
      ObjectTypePropertyFlags.None,
      "(Optional) When true, the Environment is simulated."
    ),
    provisioningState: createObjectProperty(
      enumRef,
      ObjectTypePropertyFlags.ReadOnly,
      "(Read Only) The status of the Environment."
    ),
    recipePacks: createObjectProperty(
      stringArrayRef,
      ObjectTypePropertyFlags.None,
      "(Optional) Resource IDs of the Recipe Packs."
    )
  });

  const bodyRef = factory.addObjectType("Radius.Core/environments", {
    id: createObjectProperty(
      stringRef,
      ObjectTypePropertyFlags.ReadOnly |
        ObjectTypePropertyFlags.DeployTimeConstant,
      "The resource id"
    ),
    name: createObjectProperty(
      factory.addStringType(),
      ObjectTypePropertyFlags.Required,
      "The resource name"
    ),
    type: createObjectProperty(
      factory.addStringLiteralType("Radius.Core/environments"),
      ObjectTypePropertyFlags.ReadOnly,
      "The resource type"
    ),
    apiVersion: createObjectProperty(
      factory.addStringLiteralType("2025-08-01-preview"),
      ObjectTypePropertyFlags.ReadOnly,
      "The resource api version"
    ),
    tags: createObjectProperty(
      factory.addStringType(),
      ObjectTypePropertyFlags.None,
      "Resource tags."
    ),
    properties: createObjectProperty(
      propertiesRef,
      ObjectTypePropertyFlags.Required,
      "The resource-specific properties for this resource."
    )
  });

  const resourceRef = factory.addResourceType(
    "Radius.Core/environments@2025-08-01-preview",
    bodyRef,
    ScopeType.ResourceGroup,
    ScopeType.ResourceGroup
  );

  return {
    factory,
    resourceType: factory.lookupType(resourceRef) as ResourceType
  };
}

describe("writeTableMarkdown", () => {
  it("renders the resource description block, preserving multi-line markdown", () => {
    const { factory, resourceType } = buildResource();
    const description = "An Environment.\n\n## Notes\n\nUse `rad deploy`.";

    const markdown = writeTableMarkdown(
      [resourceType],
      factory.types,
      description
    );

    expect(markdown).toContain(`## Description\n\n${description}\n`);
  });

  it("omits the description block when no description is provided", () => {
    const { factory, resourceType } = buildResource();

    const markdown = writeTableMarkdown([resourceType], factory.types);

    expect(markdown).not.toContain("## Description");
    expect(markdown.startsWith("## Top-Level Properties")).toBe(true);
  });

  it("hides ARM envelope properties and lists resource-specific properties", () => {
    const { factory, resourceType } = buildResource();

    const markdown = writeTableMarkdown([resourceType], factory.types);
    const topLevel = markdown.split("## Object Properties")[0];

    expect(topLevel).toContain(
      "| `providers` | [object](#providers) | false | false |"
    );
    expect(topLevel).toContain("| `simulated` | boolean | false | false |");
    expect(topLevel).not.toContain("`id`");
    expect(topLevel).not.toContain("`apiVersion`");
    expect(topLevel).not.toContain("`tags`");
  });

  it("derives Required and Read-Only columns and lists enum and array types", () => {
    const { factory, resourceType } = buildResource();

    const markdown = writeTableMarkdown([resourceType], factory.types);

    expect(markdown).toContain(
      "| `provisioningState` | string | false | true | (Read Only) The status of the Environment.<br />Allowed values: `Failed`, `Succeeded`. |"
    );
    expect(markdown).toContain(
      "| `recipePacks` | string array | false | false |"
    );
  });

  it("documents map value types as their own section", () => {
    const { factory, resourceType } = buildResource();

    const markdown = writeTableMarkdown([resourceType], factory.types);

    // The map property appears in the top-level table as a linked object...
    expect(markdown).toContain(
      "| `recipes` | [object](#recipes) | false | false |"
    );
    // ...and its value type (RecipeDefinition) is documented under its path,
    // including the enum allowed values in the nested property's description.
    expect(markdown).toContain("### `recipes` {#recipes}\n");
    expect(markdown).toContain(
      "| `kind` | string | true | false | (Required) The kind of recipe.<br />Allowed values: `bicep`, `terraform`. |"
    );
    expect(markdown).toContain("| `source` | string | true | false |");
  });

  it("documents map values that are arrays of objects as their own section", () => {
    const { factory, resourceType } = buildResource();

    const markdown = writeTableMarkdown([resourceType], factory.types);

    // The map value is an array of objects (map<ProviderConfig[]>); the element
    // object must still expand into its own linked section.
    expect(markdown).toContain(
      "| `providerConfigs` | [object](#providerconfigs) | false | false |"
    );
    expect(markdown).toContain("### `providerConfigs` {#providerconfigs}\n");
    expect(markdown).toContain("| `name` | string | true | false |");
  });

  it("flattens nested objects into dotted-path sections", () => {
    const { factory, resourceType } = buildResource();

    const markdown = writeTableMarkdown([resourceType], factory.types);

    expect(markdown).toContain("## Object Properties");
    expect(markdown).toContain("### `providers` {#providers}\n");
    expect(markdown).toContain("### `providers.azure` {#providers-azure}\n");
    expect(markdown).toContain(
      "| `azure` | [object](#providers-azure) | false | false |"
    );
    expect(markdown).toContain("| `subscriptionId` | string | true | false |");
  });

  it("links array-of-object types and expands their element sections", () => {
    const { factory, resourceType } = buildResource();

    const markdown = writeTableMarkdown([resourceType], factory.types);

    // The array property links to the element object's section, suffixed `[]`.
    expect(markdown).toContain(
      "| `extensions` | [object](#extensions)[] | false | false |"
    );
    // ...and the element object (Extension) is expanded as its own section.
    expect(markdown).toContain("### `extensions` {#extensions}\n");
    expect(markdown).toContain("| `manifest` | string | false | false |");
  });

  it("expands a shared object type into a separate section per path", () => {
    const { factory, resourceType } = buildResource();

    const markdown = writeTableMarkdown([resourceType], factory.types);

    // Both properties reference the same EndpointInfo type but each links to
    // its own dotted-path section rather than collapsing to a shared one.
    expect(markdown).toContain(
      "| `primaryEndpoint` | [object](#primaryendpoint) | false | false |"
    );
    expect(markdown).toContain(
      "| `secondaryEndpoint` | [object](#secondaryendpoint) | false | false |"
    );
    expect(markdown).toContain("### `primaryEndpoint` {#primaryendpoint}\n");
    expect(markdown).toContain(
      "### `secondaryEndpoint` {#secondaryendpoint}\n"
    );
    // The shared property is documented under each path.
    const primary = markdown.split("### `primaryEndpoint`")[1].split("###")[0];
    expect(primary).toContain("| `host` | string | true | false |");
  });

  it("expands discriminated unions into a discriminator row and variant sections", () => {
    const { factory, resourceType } = buildResource();

    const markdown = writeTableMarkdown([resourceType], factory.types);

    // The discriminated property's section leads with a discriminator row whose
    // allowed values link to each variant section.
    expect(markdown).toContain(
      "| `kind` | string | true | false | Discriminator property that selects the variant. Allowed values: [`aci`](#compute-aci), [`kubernetes`](#compute-kubernetes). |"
    );
    // Each variant is expanded into its own section with the discriminator
    // literal omitted (shown only once on the parent row).
    expect(markdown).toContain(
      "### `compute.kubernetes` {#compute-kubernetes}\n"
    );
    expect(markdown).toContain("### `compute.aci` {#compute-aci}\n");
    expect(markdown).toContain("| `namespace` | string | true | false |");
    expect(markdown).toContain("| `resourceGroup` | string | true | false |");
  });
});
