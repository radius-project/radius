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

import v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"

// AWSResource represents any AWS Resource.
// AWSResource is not a tracked resource, so it does not implement ResourceDataModel.
// However need to implement the methods to satisfy the interface.
type AWSResource struct {
}

// GetSystemData is not implemented for AWS proxy resource.
//
// # Function Explanation
// 
//	"GetSystemData" returns nil, indicating that no system data is available for the AWSResource. If the caller of this 
//	function expects system data, they should handle the nil return value accordingly.
func (a *AWSResource) GetSystemData() *v1.SystemData {
	return nil
}

// GetBaseResource is not implemented for AWS proxy resource.
//
// # Function Explanation
// 
//	"GetBaseResource" returns nil, indicating that the AWSResource does not have a BaseResource. If the caller of this 
//	function expects a BaseResource, they should handle the nil return value accordingly.
func (a *AWSResource) GetBaseResource() *v1.BaseResource {
	return nil
}

// ProvisioningState is not implemented for AWS proxy resource.
//
// # Function Explanation
// 
//	The ProvisioningState function of the AWSResource struct returns the provisioning state of the resource, and handles any
//	 errors that may occur during the process.
func (a *AWSResource) ProvisioningState() v1.ProvisioningState {
	return v1.ProvisioningState("")
}

// SetProvisioningState is not implemented for AWS proxy resource.
//
// # Function Explanation
// 
//	The SetProvisioningState function sets the ProvisioningState of an AWSResource object, and returns an error if the state
//	 is invalid.
func (a *AWSResource) SetProvisioningState(state v1.ProvisioningState) {

}

// UpdateMetadata is not implemented for AWS proxy resource.
//
// # Function Explanation
// 
//	UpdateMetadata is a function that updates the metadata of an AWSResource object. It handles any errors that may occur 
//	during the update process.
func (a *AWSResource) UpdateMetadata(ctx *v1.ARMRequestContext, oldResource *v1.BaseResource) {

}

// ResourceTypeName returns the resource type name.
//
// # Function Explanation
// 
//	The ResourceTypeName function returns a string representing the type of the AWSResource object. It handles any errors 
//	that may occur during the execution of the function and returns an empty string in case of an error.
func (a *AWSResource) ResourceTypeName() string {
	return "UCP/AWSResource"
}
