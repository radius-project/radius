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
using Azure.ResourceManager;
using OpenAPI;

namespace Applications.Datastores;

@doc("SqlDatabase portable resource")
model SqlDatabaseResource is TrackedResource<SqlDatabaseProperties> {
  @doc("The name of your resource.")
  @key("sqlDatabaseName")
  @path
  @segment("sqlDatabases")
  name: ResourceNameString;
}

@doc("SqlDatabase properties")
model SqlDatabaseProperties {
  ...EnvironmentScopedResource;

  @doc("The name of the SQL database.")
  database?: string;

  @doc("The fully qualified domain name of the SQL server.")
  server?: string;

  @doc("The SQL database port.")
  port?: int32;

  @doc("The username for the SQL database.")
  username?: string;

  @doc("An array of IDs of the underlying resources.")
  resources?: ResourceReference[];

  @doc("Secrets used when building the resource from values.")
  secrets?: SqlDatabaseSecrets;

  ...RecipeBaseProperties;
}

@doc("The secret values for the given SqlDatabase resource")
model SqlDatabaseListSecretsResult is SqlDatabaseSecrets;

@doc("The secret values for the given SqlDatabase resource")
model SqlDatabaseSecrets {
  @doc("The password for the SQL database. Write only.")
  password?: string;

  @doc("The connection string for the SQL database. Write only.")
  connectionString?: string;
}

#suppress "@azure-tools/typespec-azure-core/casing-style"
@armResourceOperations
interface SqlDatabases {
  get is ArmResourceRead<
    SqlDatabaseResource,
    UCPBaseParameters<SqlDatabaseResource>
  >;

  createOrUpdate is ArmResourceCreateOrReplaceAsync<
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
