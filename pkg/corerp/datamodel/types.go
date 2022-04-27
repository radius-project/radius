// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

// ProvisioningStates is the state of resource.
type ProvisioningStates string

const (
	ProvisioningStateNone         ProvisioningStates = "None"
	ProvisioningStateUpdating     ProvisioningStates = "Updating"
	ProvisioningStateDeleting     ProvisioningStates = "Deleting"
	ProvisioningStateAccepted     ProvisioningStates = "Accepted"
	ProvisioningStateSucceeded    ProvisioningStates = "Succeeded"
	ProvisioningStateProvisioning ProvisioningStates = "Provisioning"
	ProvisioningStateFailed       ProvisioningStates = "Failed"
	ProvisioningStateCanceled     ProvisioningStates = "Canceled"
)

// TrackedResource represents the common tracked resource.
type TrackedResource struct {
	// ID is the fully qualified resource ID for the resource.
	ID string `json:"id"`
	// Name is the resource name.
	Name string `json:"name"`
	// Type is the resource type.
	Type string `json:"type"`
	// Location is the geo-location where resource is located.
	Location string `json:"location"`
	// Tags is the resource tags.
	Tags map[string]string `json:"tags,omitempty"`
}

// InternalMetadata represents internal DataModel specific metadata.
type InternalMetadata struct {
	// TenantID is the tenant id of the resource.
	TenantID string `json:"tenantId"`
	// CreatedAPIVersion is an api-version used when creating this model.
	CreatedAPIVersion string `json:"createdApiVersion"`
	// UpdatedAPIVersion is an api-version used when updating this model.
	UpdatedAPIVersion string `json:"updatedApiVersion,omitempty"`

	// TODO: will add more properties.
}
