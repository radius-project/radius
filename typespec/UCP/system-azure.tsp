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
import "./ucp-operations.tsp";

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

@doc("Azure credential kinds supported.")
enum AzureCredentialKind {
  @doc("The Service Principal Credential")
  ServicePrincipal,
}

@discriminator("kind")
@doc("The base properties of Azure Credential")
model AzureCredentialProperties {
  @doc("The kind of Azure credential")
  kind: AzureCredentialKind;

  @doc("The status of the asynchronous operation.")
  @visibility("read")
  provisioningState?: ProvisioningState;
}

@doc("The properties of Service Principal credential storage")
model AzureServicePrincipalProperties extends AzureCredentialProperties {
  @doc("Service principal kind")
  kind: AzureCredentialKind.ServicePrincipal;

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

alias AzureCredentialBaseParameter<TResource> = CredentialBaseParameters<
  TResource,
  AzurePlaneNameParameter
>;

@autoRoute
@armResourceOperations
interface AzureCredentials {
  @doc("List Azure credentials")
  listByRootScope is UcpResourceListByRootScope<
    AzureCredentialResource,
    {
      ...ApiVersionParameter;
      ...AzurePlaneNameParameter;
    }
  >;

  @doc("Get an Azure credential")
  get is UcpResourceRead<
    AzureCredentialResource,
    AzureCredentialBaseParameter<AzureCredentialResource>
  >;

  @doc("Create or update an Azure credential")
  createOrUpdate is UcpResourceCreateOrUpdateSync<
    AzureCredentialResource,
    AzureCredentialBaseParameter<AzureCredentialResource>
  >;

  @doc("Delete an Azure credential")
  delete is UcpResourceDeleteSync<
    AzureCredentialResource,
    AzureCredentialBaseParameter<AzureCredentialResource>
  >;
}
