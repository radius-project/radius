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

import "./testsyncresources.tsp";
import "./testasyncresources.tsp";

using TypeSpec.Versioning;
using Azure.ResourceManager;

@armProviderNamespace
@service({
  title: "Applications.Test Resource Management APIs",
  version: "2023-08-19-preview",
})
@useDependency(Azure.ResourceManager.Versions.v1_0_Preview_1)
namespace Applications.Test;

interface Operations extends Azure.ResourceManager.Operations {}
