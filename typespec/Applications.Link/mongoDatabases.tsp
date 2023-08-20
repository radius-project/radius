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

import "../radius/v1/ucprootscope.tsp";
import "../radius/v1/resources.tsp";
import "./common.tsp";

using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Versioning;
using Azure.ResourceManager;
using OpenAPI;

namespace Applications.Link;

@doc("MongoDatabase link")
model MongoDatabaseResource is TrackedResource<MongoDatabaseProperties> {
  @doc("The name of the MongoDatabase link resource")
  @path
  @segment("mongoDatabases")
  @key("mongoDatabaseName")
  name: ResourceNameString;
}

@doc("The secret values for the given MongoDatabase resource")
model MongoDatabaseListSecretsResult is MongoDatabaseSecrets;

@doc("The secret values for the given MongoDatabase resource")
model MongoDatabaseSecrets {
  @doc("Password to use when connecting to the target Mongo database")
  password?: string;

  @doc("Connection string used to connect to the target Mongo database")
  connectionString?: string;
}

@doc("MongoDatabase link properties")
model MongoDatabaseProperties extends BasicResourceProperties {
  @doc("Provisioning state of the mongo database link at the time the operation was called")
  @visibility("read")
  provisioningState?: ProvisioningState;

  @doc("Secret values provided for the resource")
  secrets?: MongoDatabaseSecrets;

  @doc("Host name of the target Mongo database")
  host?: string;

  @doc("Port value of the target Mongo database")
  port?: int32;

  @doc("Database name of the target Mongo database")
  database?: string;

  @doc("The recipe used to automatically deploy underlying infrastructure for the MongoDB link")
  recipe?: Recipe;

  @doc("List of the resource IDs that support the MongoDB resource")
  resources?: ResourceReference[];

  @doc("Specifies how the underlying service/resource is provisioned and managed.")
  resourceProvisioning?: ResourceProvisioning;

  @doc("Username to use when connecting to the target Mongo database")
  username?: string;
}

#suppress "@azure-tools/typespec-azure-core/casing-style"
@armResourceOperations
interface MongoDatabases {
  get is ArmResourceRead<
    MongoDatabaseResource,
    UCPBaseParameters<MongoDatabaseResource>
  >;
  create is ArmResourceCreateOrUpdateAsync<
    MongoDatabaseResource,
    UCPBaseParameters<MongoDatabaseResource>
  >;
  update is ArmResourcePatchAsync<
    MongoDatabaseResource,
    MongoDatabaseProperties,
    UCPBaseParameters<MongoDatabaseResource>
  >;
  delete is ArmResourceDeleteAsync<
    MongoDatabaseResource,
    UCPBaseParameters<MongoDatabaseResource>
  >;
  listByRootScope is ArmResourceListByParent<
    MongoDatabaseResource,
    UCPBaseParameters<MongoDatabaseResource>,
    "RootScope",
    "RootScope"
  >;

  @doc("Lists secrets values for the specified MongoDatabases resource")
  @action("listSecrets")
  listSecrets is ArmResourceActionSync<
    MongoDatabaseResource,
    {},
    MongoDatabaseListSecretsResult,
    UCPBaseParameters<MongoDatabaseResource>
  >;
}
