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

package controller

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
)

// Result is the response of async operation controller.
type Result struct {
	// Requeue tells the Controller to requeue the reconcile key. Defaults to false.
	Requeue bool

	// Error represents the error when status is Cancelled or Failed.
	Error *v1.ErrorDetails

	// state represents the provisioning status.
	state *v1.ProvisioningState
}

// NewCanceledResult creates the canceled asynchronous operation result.
//
// # Function Explanation
// 
//	NewCanceledResult creates a new Result object with a Canceled status and a message, and returns it to the caller. This 
//	allows callers to easily create a Result object with a Canceled status and a message to indicate an error.
func NewCanceledResult(message string) Result {
	r := Result{}
	r.SetCanceled(message)
	return r
}

// NewFailedResult creates the failed asynchronous operation result.
//
// # Function Explanation
// 
//	NewFailedResult creates a new Result object with the given error details and sets the result to failed. It is useful for
//	 callers of this function to handle errors.
func NewFailedResult(err v1.ErrorDetails) Result {
	r := Result{}
	r.SetFailed(err, false)
	return r
}

// SetFailed sets the error response with Failed status.
//
// # Function Explanation
// 
//	Result.SetFailed sets the ProvisioningState to Failed and sets the Error field to the given ErrorDetails, and sets the 
//	Requeue field to the given boolean value. It also handles the case where Result is nil.
func (r *Result) SetFailed(err v1.ErrorDetails, requeue bool) {
	if r == nil {
		r = &Result{}
	}
	r.Requeue = requeue
	r.SetProvisioningState(v1.ProvisioningStateFailed)
	r.Error = &v1.ErrorDetails{
		Code:    err.Code,
		Message: err.Message,
	}
}

// SetCanceled sets the response status to Canceled.
//
// # Function Explanation
// 
//	Result.SetCanceled sets the Result object's Requeue field to false, sets the ProvisioningState to Canceled, and sets the
//	 Error field to an ErrorDetails object with Code set to OperationCanceled and Message set to the provided message. If 
//	the Result object is nil, it is initialized before setting the fields. This allows callers to check the Result object's 
//	fields to determine if the operation was canceled.
func (r *Result) SetCanceled(message string) {
	if r == nil {
		r = &Result{}
	}
	r.Requeue = false
	r.SetProvisioningState(v1.ProvisioningStateCanceled)
	r.Error = &v1.ErrorDetails{
		Code:    v1.CodeOperationCanceled,
		Message: message,
	}
}

// SetProvisioningState sets provisioning state.
//
// # Function Explanation
// 
//	Result.SetProvisioningState sets the ProvisioningState of the Result object, returning an error if the state is invalid.
func (r *Result) SetProvisioningState(s v1.ProvisioningState) {
	r.state = &s
}

// ProvisioningState gets the provisioning state of the result.
//
// # Function Explanation
// 
//	Result.ProvisioningState() returns the ProvisioningState of the Result object, either as the stored state or as the 
//	default value of 'Succeeded' if no state has been set.
func (r *Result) ProvisioningState() v1.ProvisioningState {
	if r.state == nil {
		return v1.ProvisioningStateSucceeded
	}
	return *r.state
}
