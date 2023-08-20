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
import "@azure-tools/typespec-azure-core";
import "@azure-tools/typespec-azure-resource-manager";
import "@azure-tools/typespec-providerhub";

import "../../radius/v1/ucprootscope.tsp";
import "../../radius/v1/resources.tsp";
import "./common.tsp";
import "./extensions.tsp";

using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Versioning;
using Azure.ResourceManager;
using Azure.Core;
using OpenAPI;

namespace Applications.Core;

@doc("Radius Application resource")
model ApplicationResource is TrackedResource<ApplicationProperties> {
  @doc("The application name")
  @path
  @key("applicationName")
  @segment("applications")
  name: ResourceNameString;
}

@doc("Application properties")
model ApplicationProperties extends BasicResourceProperties {
  @visibility("read")
  @doc("The status of the last operation.")
  provisioningState?: ProvisioningState;

  @doc("The application extension.")
  @extension("x-ms-identifiers", [])
  extenions: Array<Extension>;
}

#suppress "@azure-tools/typespec-azure-core/casing-style"
@armResourceOperations
interface Applications {
  get is ArmResourceRead<
    ApplicationResource,
    UCPBaseParameters<ApplicationResource>
  >;
  create is ArmResourceCreateOrReplaceSync<
    ApplicationResource,
    UCPBaseParameters<ApplicationResource>
  >;
  update is ArmResourcePatchSync<
    ApplicationResource,
    EnvironmentProperties,
    UCPBaseParameters<ApplicationResource>
  >;
  delete is ArmResourceDeleteSync<
    ApplicationResource,
    UCPBaseParameters<ApplicationResource>
  >;

  listByRootScope is ArmResourceListByParent<
    ApplicationResource,
    UCPBaseParameters<ApplicationResource>,
    "RootScope",
    "RootScope"
  >;
}
