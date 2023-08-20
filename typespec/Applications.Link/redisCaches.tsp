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

@doc("RedisCache link")
model RedisCacheResource is TrackedResource<RedisCacheProperties> {
  @path
  @doc("The name of the RedisCache link resource")
  @key("redisCacheName")
  @segment("redisCaches")
  name: ResourceNameString;
}

@doc("The secret values for the given RedisCache resource")
model RedisCacheListSecretsResult is RedisCacheSecrets;

@doc("The secret values for the given RedisCache resource")
model RedisCacheSecrets {
  @doc("The connection string used to connect to the Redis cache")
  connectionString?: string;

  @doc("The password for this Redis cache instance")
  password?: string;

  @doc("The URL used to connect to the Redis cache")
  url?: string;
}

@doc("RedisCache link properties")
model RedisCacheProperties extends BasicResourceProperties {
  @doc("Provisioning state of the redis cache link at the time the operation was called")
  @visibility("read")
  provisioningState?: ProvisioningState;

  @doc("Secrets provided by resource")
  secrets?: RedisCacheSecrets;

  @doc("The host name of the target Redis cache")
  host?: string;

  @doc("The port value of the target Redis cache")
  port?: int32;

  @doc("The username for Redis cache")
  username?: string;

  @doc("Specifies whether to enable SSL connections to the Redis cache")
  tls?: boolean;

  @doc("The recipe used to automatically deploy underlying infrastructure for the Redis caches link")
  recipe?: Recipe;

  @doc("List of the resource IDs that support the Redis resource")
  resources?: ResourceReference[];

  @doc("Specifies how the underlying service/resource is provisioned and managed.")
  resourceProvisioning?: ResourceProvisioning;
}

#suppress "@azure-tools/typespec-azure-core/casing-style"
@armResourceOperations
interface RedisCaches {
  get is ArmResourceRead<
    RedisCacheResource,
    UCPBaseParameters<RedisCacheResource>
  >;
  create is ArmResourceCreateOrUpdateAsync<
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
  listByRootScope is ArmResourceListByParent<
    RedisCacheResource,
    UCPBaseParameters<RedisCacheResource>,
    "RootScope",
    "RootScope"
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
