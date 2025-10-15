import "@typespec/rest";
import "@typespec/versioning";
import "@typespec/openapi";
import "@azure-tools/typespec-autorest";
import "@azure-tools/typespec-azure-core";
import "@azure-tools/typespec-azure-resource-manager";

import "../radius/v1/ucprootscope.tsp";
import "../radius/v1/resources.tsp";
import "../radius/v1/trackedresource.tsp";
import "./environments.tsp";
import "./recipePacks.tsp";

using Azure.ResourceManager;
using TypeSpec.Versioning;
using TypeSpec.Http;
using TypeSpec.Rest;
using Autorest;
using Azure.Core;
using OpenAPI;

@armProviderNamespace
@service({
  title: "Radius Core Resource Provider",
})
@doc("REST APIs for next-generation Radius Core resources (preview).")
@versioned(Versions)
namespace Radius.Core;

@doc("Supported API versions for the Radius.Core resource provider.")
enum Versions {
  @doc("2025-08-01-preview")
  @useDependency(Azure.Core.Versions.v1_0_Preview_2)
  @useDependency(Azure.ResourceManager.Versions.v1_0_Preview_1)
  v2025_08_01_preview: "2025-08-01-preview",
}

interface Operations extends Azure.ResourceManager.Operations {}
