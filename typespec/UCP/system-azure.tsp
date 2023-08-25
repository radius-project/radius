/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
    
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

import "@typespec/rest";
import "@typespec/versioning";
import "@typespec/openapi";
import "@azure-tools/typespec-autorest";
import "@azure-tools/typespec-azure-core";
import "@azure-tools/typespec-azure-resource-manager";
import "@azure-tools/typespec-providerhub";

import "../radius/v1/ucprootscope.tsp";
import "../radius/v1/resources.tsp";
import "./common.tsp";

using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Versioning;
using Autorest;
using Azure.Core;
using Azure.ResourceManager;
using Azure.ResourceManager.Foundations;
using OpenAPI;

namespace Ucp;

@doc("The parameter of Azure plane")
model AzurePlaneNameParameter {
  @doc("The name of the plane")
  @path
  @extension("x-ms-skip-url-encoding", true)
  @extension("x-ms-parameter-location", "method")
  @segment("planes/azure")
  planeName: ResourceNameString;
}

#suppress "@azure-tools/typespec-azure-resource-manager/arm-resource-path-segment-invalid-chars"
@doc("Represents Azure Credential Resource")
model AzureCredentialResource is TrackedResource<AzureCredentialProperties> {
  @doc("The Azure credential name.")
  @path
  @key("credentialName")
  @segment("providers/System.Azure/credentials")
  name: ResourceNameString;
}

@discriminator("kind")
@doc("The base properties of Azure Credential")
model AzureCredentialProperties {
  @doc("The status of the asynchronous operation.")
  @visibility("read")
  provisioningState?: ProvisioningState;
}

@doc("The properties of Service Principal credential storage")
model AzureServicePrincipalProperties extends AzureCredentialProperties {
  @doc("Service principal kind")
  kind: "ServicePrincipal";

  @doc("clientId for ServicePrincipal")
  clientId: string;

  @doc("secret for ServicePrincipal")
  @extension("x-ms-secret", true)
  clientSecret: string;

  @doc("tenantId for ServicePrincipal")
  tenantId: string;

  @doc("The storage properties")
  storage: CredentialStorageProperties;
}

@doc("Credential storage properties")
@discriminator("kind")
model CredentialStorageProperties {}

@doc("Internal credential storage properties")
model InternalCredentialStorageProperties extends CredentialStorageProperties {
  @doc("Internal kind")
  kind: "Internal";

  @doc("The name of secret stored.")
  @visibility("read")
  secretName: string;
}

alias AzureCredentialBaseParameter<TResource> = CredentialBaseParameters<
  TResource,
  AzurePlaneNameParameter
>;

@autoRoute
@armResourceOperations
interface AzureCredentials {
  @doc("List Azure credentials")
  @get
  @segmentOf(AzureCredentialResource)
  @armResourceList(AzureCredentialResource)
  listByRootScope(
    ...ApiVersionParameter,
    ...AzurePlaneNameParameter,
    ...ParentKeysOf<AzureCredentialResource>,
  ): ArmResponse<ResourceListResult<AzureCredentialResource>> | ErrorResponse;

  @doc("Get an Azure credential")
  @get
  @armResourceRead(AzureCredentialResource)
  get(
    ...AzureCredentialBaseParameter<AzureCredentialResource>,
  ): ArmResponse<AzureCredentialResource> | ErrorResponse;

  @doc("Create or update an Azure credential")
  @put
  @armResourceCreateOrUpdate(AzureCredentialResource)
  createOrUpdate(
    ...AzureCredentialBaseParameter<AzureCredentialResource>,

    @doc("Resource create parameters.")
    @body
    resource: AzureCredentialResource,
  ): ArmResponse<AzureCredentialResource> | ErrorResponse;

  @doc("Delete an Azure credential")
  @delete
  @armResourceDelete(AzureCredentialResource)
  delete(...AzureCredentialBaseParameter<AzureCredentialResource>):
    | ArmDeletedResponse
    | ArmDeleteAcceptedLroResponse
    | ArmDeletedNoContentResponse
    | ErrorResponse;
}
