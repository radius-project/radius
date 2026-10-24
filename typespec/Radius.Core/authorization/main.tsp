import "@typespec/versioning";
import "@azure-tools/typespec-azure-resource-manager";
import "../authorization.tsp";

using Azure.ResourceManager;
using TypeSpec.Versioning;

#suppress "@azure-tools/typespec-azure-resource-manager/missing-operations-endpoint" "UCP owns this contract; provider discovery is served separately."
@armProviderNamespace
@service(#{ title: "Radius Authorization" })
@doc("Preview contracts for UCP-served Radius authorization. These contracts do not register management routes or enable enforcement.")
@versioned(Versions)
namespace Radius.Core;

@doc("Supported user-facing authorization API versions.")
enum Versions {
  @doc("Initial authorization contract.")
  v2025_08_01_preview: "2025-08-01-preview",
}
