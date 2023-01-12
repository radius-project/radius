// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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
