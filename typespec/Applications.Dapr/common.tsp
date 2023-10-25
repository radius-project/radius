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

@doc("The base properties of a Dapr component object.")
model DaprResourceProperties {
  @doc("_(read-only)_ The name of the Dapr component that is generated and applied to the underlying system. Used by the Dapr SDKs or APIs to access the Dapr component.")
  @visibility("read")
  componentName?: string;

  @doc("Metadata for the Dapr component. Schema must match [Dapr component](https://docs.dapr.io/reference/components-reference/supported-state-stores/). Used when `resourceProvisioning` is set to `manual`.")
  metadata?: {};

  #suppress "@azure-tools/typespec-azure-resource-manager/arm-resource-duplicate-property"
  @doc("The Dapr component type. Used when `resourceProvisioning` is set to `manual`.")
  type?: string;

  @doc("The version of the Dapr component. See [Dapr components](https://docs.dapr.io/reference/components-reference/supported-state-stores/) for available versions. Used when `resourceProvisioning` is set to `manual`.")
  version?: string;
}
