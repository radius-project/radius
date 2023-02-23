// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"

// AWSResource represents any AWS Resource.
type AWSResource struct {
}

// Dummy interface implementation to ensure that AWSResource implements ResourceDataModel.
func (a *AWSResource) GetSystemData() *v1.SystemData {
	return nil
}

func (a *AWSResource) GetBaseResource() *v1.BaseResource {
	return nil
}

func (a *AWSResource) ProvisioningState() v1.ProvisioningState {
	return v1.ProvisioningState("")
}

func (a *AWSResource) SetProvisioningState(state v1.ProvisioningState) {

}
func (a *AWSResource) UpdateMetadata(ctx *v1.ARMRequestContext, oldResource *v1.BaseResource) {

}

func (a *AWSResource) ResourceTypeName() string {
	return "AWSResource"
}
