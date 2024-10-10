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

/* Using Azure.ResoureManager namespace here is a workaround for typespec bug: https://github.com/Azure/typespec-azure/issues/110"
   ARM TrackedResource resource has "properties" field optional. We do not want this because the user can skip the
   properties block in the Radius resource and this cannot be caught by validation. Therefore, we are defining our overwhelmingly
   TrackedResourceRequired class which has the "properties" field as mandatory.
*/
namespace Azure.ResourceManager;

@doc("Concrete tracked resource types can be created by aliasing this type using a specific property type.")
@Azure.ResourceManager.Private.armResourceInternal(TProperties)
@TypeSpec.Http.includeInapplicableMetadataInPayload(false)
model TrackedResourceRequired<
  TProperties extends {},
  TResourceType extends valueof string,
  TResourceFriendlyName extends valueof string = TResourceType
> extends Azure.ResourceManager.Foundations.TrackedResource {
  #suppress "@azure-tools/typespec-azure-resource-manager/arm-resource-invalid-envelope-property" "template"
  #suppress "@azure-tools/typespec-azure-core/bad-record-type"
  @doc("The resource-specific properties for this resource.")
  @visibility("read", "create")
  @extension("x-ms-client-flatten", true)
  properties: TProperties;
}
