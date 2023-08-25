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

@doc("MongoDatabase portable resource")
model MongoDatabaseResource is TrackedResource<MongoDatabaseProperties> {
  @doc("The name of the MongoDatabase portable resource resource")
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

@doc("MongoDatabase portable resource properties")
model MongoDatabaseProperties {
  ...EnvironmentScopedResource;

  @doc("Secret values provided for the resource")
  secrets?: MongoDatabaseSecrets;

  @doc("Host name of the target Mongo database")
  host?: string;

  @doc("Port value of the target Mongo database")
  port?: int32;

  @doc("Database name of the target Mongo database")
  database?: string;

  @doc("List of the resource IDs that support the MongoDB resource")
  resources?: ResourceReference[];

  @doc("Username to use when connecting to the target Mongo database")
  username?: string;

  ...RecipeBaseProperties;
}

#suppress "@azure-tools/typespec-azure-core/casing-style"
@armResourceOperations
interface MongoDatabases {
  get is ArmResourceRead<
    MongoDatabaseResource,
    UCPBaseParameters<MongoDatabaseResource>
  >;

  createOrUpdate is ArmResourceCreateOrReplaceAsync<
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

  listByScope is ArmResourceListByParent<
    MongoDatabaseResource,
    UCPBaseParameters<MongoDatabaseResource>,
    "Scope",
    "Scope"
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
