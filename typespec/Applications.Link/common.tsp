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

import "@typespec/openapi";

using OpenAPI;

@doc("Specifies how the underlying service/resource is provisioned and managed. Available values are 'recipe', where Radius manages the lifecycle of the resource through a Recipe, and 'manual', where a user manages the resource and provides the values.")
enum ResourceProvisioning {
    @doc("The resource lifecycle will be managed by Radius")
    recipe,
    
    @doc("The resource lifecycle will be managed by the user")
    manual,
}

@doc("The recipe used to automatically deploy underlying infrastructure for a link")
model Recipe{
  @doc("The name of the recipe within the environment to use")
  name: string;
  @doc("Key/value parameters to pass into the recipe at deployment")
  parameters?: Parameters;
}

@doc("Key/value parameters to pass into the recipe at deployment")
model Parameters{}