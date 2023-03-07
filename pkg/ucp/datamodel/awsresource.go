// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"

// AWSResource represents any AWS Resource.
type AWSResource struct {
}

// AWSResource is not a tracked resource, so it does not implement ResourceDataModel.
// However need to implement the methods to satisfy the interface.

// Dummy interface implementation of GetSystemData to ensure that AWSResource implements ResourceDataModel.
func (a *AWSResource) GetSystemData() *v1.SystemData {
	return nil
}

// Dummy interface implementation of GetBaseResource to ensure that AWSResource implements ResourceDataModel.
func (a *AWSResource) GetBaseResource() *v1.BaseResource {
	return nil
}

// Dummy interface implementation of ProvisioningState to ensure that AWSResource implements ResourceDataModel.
func (a *AWSResource) ProvisioningState() v1.ProvisioningState {
	return v1.ProvisioningState("")
}

// Dummy interface implementation of SetProvisioningState to ensure that AWSResource implements ResourceDataModel.
func (a *AWSResource) SetProvisioningState(state v1.ProvisioningState) {

}

// Dummy interface implementation of UpdateMetadata to ensure that AWSResource implements ResourceDataModel.
func (a *AWSResource) UpdateMetadata(ctx *v1.ARMRequestContext, oldResource *v1.BaseResource) {

}

// Dummy interface implementation of ResourceTypeName to ensure that AWSResource implements ResourceDataModel.
func (a *AWSResource) ResourceTypeName() string {
	return "UCP/AWSResource"
}
