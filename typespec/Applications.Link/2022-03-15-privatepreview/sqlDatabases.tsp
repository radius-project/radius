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

using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Versioning;
using Azure.ResourceManager;
using OpenAPI;

namespace Applications.Link;

@doc("SqlDatabase properties")
model SqlDatabaseProperties extends BasicResourceProperties {
  @doc("Provisioning state of the Sql database link at the time the operation was called")
  @visibility("read")
  provisioningState?: ProvisioningState;

  @doc("The name of the Sql database.")
  database?: string;

  @doc("The fully qualified domain name of the Sql database.")
  server?: string;

  @doc("Port value of the target Sql database")
  port?: int32;

  @doc("Username to use when connecting to the target Sql database")
  username?: string;

  @doc("The recipe used to automatically deploy underlying infrastructure for the sqldatabases link")
  recipe?: Recipe;

  @doc("List of the resource IDs that support the SqlDatabase resource")
  resources?: ResourceReference[];

  @doc("Specifies how the underlying service/resource is provisioned and managed.")
  resourceProvisioning?: ResourceProvisioning;

  @doc("Secret values provided for the resource")
  secrets?: SqlDatabaseSecrets;
}

@doc("The secret values for the given SqlDatabase resource")
model SqlDatabaseListSecretsResult is SqlDatabaseSecrets;

@doc("The secret values for the given SqlDatabase resource")
model SqlDatabaseSecrets {
  @doc("Password to use when connecting to the target Sql database")
  password?: string;

  @doc("Connection string used to connect to the target Sql database")
  connectionString?: string;
}

@doc("SqlDatabase link")
model SqlDatabaseResource is TrackedResource<SqlDatabaseProperties> {
  @doc("The name of the SqlDatabase link resource")
  @key("sqlDatabaseName")
  @path
  @segment("sqlDatabases")
  name: ResourceNameString;
}

#suppress "@azure-tools/typespec-azure-core/casing-style"
@armResourceOperations
interface SqlDatabases {
  get is ArmResourceRead<
    SqlDatabaseResource,
    UCPBaseParameters<SqlDatabaseResource>
  >;
  create is ArmResourceCreateOrReplaceAsync<
    SqlDatabaseResource,
    UCPBaseParameters<SqlDatabaseResource>
  >;
  update is ArmResourcePatchAsync<
    SqlDatabaseResource,
    SqlDatabaseProperties,
    UCPBaseParameters<SqlDatabaseResource>
  >;
  delete is ArmResourceDeleteAsync<
    SqlDatabaseResource,
    UCPBaseParameters<SqlDatabaseResource>
  >;
  listByScope is ArmResourceListByParent<
    SqlDatabaseResource,
    UCPBaseParameters<SqlDatabaseResource>,
    "Scope",
    "Scope"
  >;

  @doc("Lists secrets values for the specified SqlDatabase resource")
  @action("listSecrets")
  listSecrets is ArmResourceActionSync<
    SqlDatabaseResource,
    {},
    SqlDatabaseListSecretsResult,
    UCPBaseParameters<SqlDatabaseResource>
  >;
}
