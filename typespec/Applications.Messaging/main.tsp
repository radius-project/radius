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

import "@typespec/versioning";
import "@azure-tools/typespec-azure-resource-manager";

import "./rabbitMQQueues.tsp";

using TypeSpec.Versioning;
using Azure.ResourceManager;

@armProviderNamespace
@service({
  title: "Applications.Messaging Resource Management APIs",
})
@doc("REST APIs for Applications.Messaging")
@versioned(Versions)
namespace Applications.Messaging;

interface Operations extends Azure.ResourceManager.Operations {}

@doc("Supported API versions for the Applications.Messaging resource provider.")
enum Versions {
  @doc("2022-03-15-privatepreview")
  @useDependency(Azure.Core.Versions.v1_0_Preview_2)
  @useDependency(Azure.ResourceManager.Versions.v1_0_Preview_1)
  v2022_03_15_privatepreview: "2022-03-15-privatepreview",
}
