/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

package v1

// TODO: Remove DataModelInterface when we migrate Controller to Operation base struct for controller
// DataModelInterface is the interface for version agnostic datamodel.
type DataModelInterface interface {
	// ResourceTypeName returns the resource type name.
	ResourceTypeName() string
}

// ResourceDataModel represents the datamodel with helper methods.
type ResourceDataModel interface {
	DataModelInterface
	// GetSystemData gets SystemData from the resource.
	GetSystemData() *SystemData
	// GetBaseResource gets BaseResource from the resource.
	GetBaseResource() *BaseResource
	// ProvisioningState gets the provisioning state of the resource.
	ProvisioningState() ProvisioningState
	// SetProvisioningState sets the provisioning state of the resource.
	SetProvisioningState(state ProvisioningState)
	// UpdateMetadata updates and populates metadata to the resource.
	UpdateMetadata(ctx *ARMRequestContext, oldResource *BaseResource)
}

// VersionedModelInterface is the interface for versioned models.
type VersionedModelInterface interface {
	// ConvertFrom converts version agnostic datamodel to versioned model.
	ConvertFrom(src DataModelInterface) error

	// ConvertTo converts versioned model to version agnostic datamodel.
	ConvertTo() (DataModelInterface, error)
}

// ConvertToDataModel is the function to convert to data model.
type ConvertToDataModel[T any] func(content []byte, version string) (*T, error)

// ConvertToAPIModel is the function to convert data model to version model.
type ConvertToAPIModel[T any] func(model *T, version string) (VersionedModelInterface, error)
