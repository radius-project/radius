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

package datamodel

import v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"

const BicepSettingsResourceType_v20250801preview = "Radius.Core/bicepSettings"

// BicepSettings_v20250801preview represents the Radius.Core/bicepSettings resource.
type BicepSettings_v20250801preview struct {
	v1.BaseResource

	// Properties of the Bicep settings resource.
	Properties BicepSettingsProperties_v20250801preview `json:"properties"`
}

// ResourceTypeName returns the resource type for Bicep settings.
func (b *BicepSettings_v20250801preview) ResourceTypeName() string {
	return BicepSettingsResourceType_v20250801preview
}

// BicepSettingsProperties_v20250801preview describes the Bicep settings payload.
type BicepSettingsProperties_v20250801preview struct {
	// RegistryAuthentication is keyed by registry hostname and references Radius secrets containing credential data.
	RegistryAuthentication map[string]RegistrySecretConfig `json:"registryAuthentication,omitempty"`
}
