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

#suppress "@azure-tools/typespec-azure-resource-manager/arm-resource-path-segment-invalid-chars"
model AwsCredentialResource is TrackedResource<AwsCredentialProperties> {
  @key("credentialName")
  @doc("The AWS credential name.")
  @path
  @segment("providers/System.AWS/credentials")
  name: ResourceNameString;
}

@doc("The parameter for AWS plane name")
model AwsPlaneNameParameter {
  @doc("The name of AWS plane")
  @path
  @segment("planes/aws")
  @extension("x-ms-skip-url-encoding", true)
  @extension("x-ms-parameter-location", "method")
  planeName: ResourceNameString;
}

@doc("AWS credential kind")
enum AWSCredentialKind {
  @doc("The AWS Access Key credential")
  AccessKey,
}

@discriminator("kind")
@doc("AWS Credential properties")
model AwsCredentialProperties {
  @doc("The AWS credential kind")
  kind: AWSCredentialKind;

  @doc("The status of the asynchronous operation.")
  @visibility("read")
  provisioningState?: ProvisioningState;
}

@doc("AWS credential storage properties")
model AwsAccessKeyCredentialProperties extends AwsCredentialProperties {
  @doc("Access Key kind")
  kind: AWSCredentialKind.AccessKey;

  @doc("Access key ID for AWS identity")
  @secret
  accessKeyId: string;

  @doc("Secret Access Key for AWS identity")
  @secret
  secretAccessKey: string;

  @doc("The storage properties")
  storage: CredentialStorageProperties;
}

alias AwsCredentialBaseParameter<TResource> = CredentialBaseParameters<
  TResource,
  AwsPlaneNameParameter
>;

@armResourceOperations
interface AwsCredentials {
  @doc("List AWS credentials")
  listByRootScope is UcpResourceListByRootScope<
    AwsCredentialResource,
    {
      ...ApiVersionParameter;
      ...AwsPlaneNameParameter;
    }
  >;

  @doc("Get an AWS credential")
  get is UcpResourceRead<
    AwsCredentialResource,
    AwsCredentialBaseParameter<AwsCredentialResource>
  >;

  @doc("Create or update an AWS credential")
  createOrUpdate is UcpResourceCreateOrUpdateSync<
    AwsCredentialResource,
    AwsCredentialBaseParameter<AwsCredentialResource>
  >;

  @doc("Update an AWS credential")
  update is UcpCustomPatchSync<
    AwsCredentialResource,
    AwsCredentialBaseParameter<AwsCredentialResource>
  >;

  @doc("Delete an AWS credential")
  delete is UcpResourceDeleteSync<
    AwsCredentialResource,
    AwsCredentialBaseParameter<AwsCredentialResource>
  >;
}
