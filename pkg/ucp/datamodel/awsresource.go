// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"

// AWSResource represents any AWS Resource.
// AWSResource is not a tracked resource, so it does not implement ResourceDataModel.
// However need to implement the methods to satisfy the interface.
type AWSResource struct {
}

// GetSystemData is not implemented for AWS proxy resource.
func (a *AWSResource) GetSystemData() *v1.SystemData {
	return nil
}

// GetBaseResource is not implemented for AWS proxy resource.
func (a *AWSResource) GetBaseResource() *v1.BaseResource {
	return nil
}

// ProvisioningState is not implemented for AWS proxy resource.
func (a *AWSResource) ProvisioningState() v1.ProvisioningState {
	return v1.ProvisioningState("")
}

// SetProvisioningState is not implemented for AWS proxy resource.
func (a *AWSResource) SetProvisioningState(state v1.ProvisioningState) {

}

// UpdateMetadata is not implemented for AWS proxy resource.
func (a *AWSResource) UpdateMetadata(ctx *v1.ARMRequestContext, oldResource *v1.BaseResource) {

}

// ResourceTypeName returns the resource type name.
func (a *AWSResource) ResourceTypeName() string {
	return "UCP/AWSResource"
}
