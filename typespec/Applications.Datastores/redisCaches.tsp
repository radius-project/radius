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

@doc("RedisCache portable resource")
model RedisCacheResource is TrackedResource<RedisCacheProperties> {
  @path
  @doc("The name of your resource.")
  @key("redisCacheName")
  @segment("redisCaches")
  name: ResourceNameString;
}

@doc("The secret values for the given RedisCache resource")
model RedisCacheListSecretsResult is RedisCacheSecrets;

@doc("The secret values for the given RedisCache resource")
model RedisCacheSecrets {
  @doc("The connection string for the Redis cache. Write only.")
  connectionString?: string;

  @doc("The password for the Redis cache. Write only.")
  password?: string;

  @doc("The connection URL for the Redis cache. Set automatically based on the values provided for `host`, `port`, `username`, and `password`. Can be explicitly set to override default behavior. Write only.")
  url?: string;
}

@doc("RedisCache portable resource properties")
model RedisCacheProperties {
  ...EnvironmentScopedResource;

  @doc("Secrets used when building the resource from values.")
  secrets?: RedisCacheSecrets;

  @doc("	The Redis host name.")
  host?: string;

  @doc("The Redis port value.")
  port?: int32;

  @doc("The username for Redis cache.")
  username?: string;

  @doc("Indicates if the Redis cache is configured with SSL connections. If the `port` value is set to 6380 this defaults to `true`. Otherwise it is defaulted to `false`. If your Redis cache offers SSL connections on ports other than 6380, explicitly set this value to `true` to override the default behavior.")
  tls?: boolean;

  @doc("An array of IDs of the underlying resources.")
  resources?: ResourceReference[];

  ...RecipeBaseProperties;
}

@armResourceOperations
interface RedisCaches {
  get is ArmResourceRead<
    RedisCacheResource,
    UCPBaseParameters<RedisCacheResource>
  >;

  createOrUpdate is ArmResourceCreateOrReplaceAsync<
    RedisCacheResource,
    UCPBaseParameters<RedisCacheResource>
  >;

  update is ArmResourcePatchAsync<
    RedisCacheResource,
    RedisCacheProperties,
    UCPBaseParameters<RedisCacheResource>
  >;

  delete is ArmResourceDeleteAsync<
    RedisCacheResource,
    UCPBaseParameters<RedisCacheResource>
  >;

  listByScope is ArmResourceListByParent<
    RedisCacheResource,
    UCPBaseParameters<RedisCacheResource>,
    "Scope",
    "Scope"
  >;

  @doc("Lists secrets values for the specified RedisCache resource")
  @action("listSecrets")
  listSecrets is ArmResourceActionSync<
    RedisCacheResource,
    {},
    RedisCacheListSecretsResult,
    UCPBaseParameters<RedisCacheResource>
  >;
}
