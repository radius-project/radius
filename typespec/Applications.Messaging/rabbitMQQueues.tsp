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

namespace Applications.Messaging;

#suppress "@azure-tools/typespec-azure-core/casing-style" "The names of Model types must use PascalCase"
@doc("RabbitMQQueue portable resource")
model RabbitMQQueueResource is TrackedResource<RabbitMQQueueProperties> {
  @doc("The name of the RabbitMQQueue portable resource resource")
  @key("rabbitMQQueueName")
  @segment("rabbitMQQueues")
  @path
  name: ResourceNameString;
}

#suppress "@azure-tools/typespec-azure-core/casing-style" "The names of Model types must use PascalCase"
@doc("The secret values for the given RabbitMQQueue resource")
model RabbitMQListSecretsResult is RabbitMQSecrets;

#suppress "@azure-tools/typespec-azure-core/casing-style" "The names of Model types must use PascalCase"
@doc("The connection secrets properties to the RabbitMQ instance")
model RabbitMQSecrets {
  @doc("The password used to connect to the RabbitMQ instance")
  password?: string;

  @doc("The connection URI of the RabbitMQ instance. Generated automatically from host, port, SSL, username, password, and vhost. Can be overridden with a custom value")
  uri?: string;
}

#suppress "@azure-tools/typespec-azure-core/casing-style" "The names of Model types must use PascalCase"
@doc("RabbitMQQueue portable resource properties")
model RabbitMQQueueProperties {
  ...EnvironmentScopedResource;

  @doc("The secrets to connect to the RabbitMQ instance")
  secrets?: RabbitMQSecrets;

  @doc("The name of the queue")
  queue?: string;

  @doc("The hostname of the RabbitMQ instance")
  host?: string;

  @doc("The port of the RabbitMQ instance. Defaults to 5672")
  port?: int32;

  @doc("The RabbitMQ virtual host (vHost) the client will connect to. Defaults to no vHost.")
  vHost?: string;

  @doc("The username to use when connecting to the RabbitMQ instance")
  username?: string;

  @doc("List of the resource IDs that support the rabbitMQ resource")
  resources?: ResourceReference[];

  @doc("Specifies whether to use SSL when connecting to the RabbitMQ instance")
  tls?: boolean;

  ...RecipeBaseProperties;
}

#suppress "@azure-tools/typespec-azure-core/casing-style" "The names of Model types must use PascalCase"
@armResourceOperations
interface RabbitMQQueues {
  get is ArmResourceRead<
    RabbitMQQueueResource,
    UCPBaseParameters<RabbitMQQueueResource>
  >;

  createOrUpdate is ArmResourceCreateOrReplaceAsync<
    RabbitMQQueueResource,
    UCPBaseParameters<RabbitMQQueueResource>
  >;

  update is ArmResourcePatchAsync<
    RabbitMQQueueResource,
    RabbitMQQueueProperties,
    UCPBaseParameters<RabbitMQQueueResource>
  >;

  delete is ArmResourceDeleteAsync<
    RabbitMQQueueResource,
    UCPBaseParameters<RabbitMQQueueResource>
  >;

  listByScope is ArmResourceListByParent<
    RabbitMQQueueResource,
    UCPBaseParameters<RabbitMQQueueResource>,
    "Scope",
    "Scope"
  >;

  @doc("Lists secrets values for the specified RabbitMQQueue resource")
  @action("listSecrets")
  listSecrets is ArmResourceActionSync<
    RabbitMQQueueResource,
    {},
    RabbitMQListSecretsResult,
    UCPBaseParameters<RabbitMQQueueResource>
  >;
}
