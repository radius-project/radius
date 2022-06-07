// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package asyncoperation

import (
	"github.com/project-radius/radius/pkg/basedatamodel"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
)

// Result is the response of async operation controller.
type Result struct {
	// Requeue tells the Controller to requeue the reconcile key. Defaults to false.
	Requeue bool

	// Error represents the error when status is Cancelled or Failed.
	Error *armerrors.ErrorDetails

	// state represents the provisioning status.
	state *basedatamodel.ProvisioningStates
}

// NewCanceledResult creates the canceled asynchronous operation result.
func NewCanceledResult(message string) Result {
	r := Result{}
	r.SetCanceled(message)
	return r
}

// NewFailedResult creates the failed asynchronous operation result.
func NewFailedResult(err armerrors.ErrorDetails) Result {
	r := Result{}
	r.SetFailed(err, false)
	return r
}

// SetFailed sets the error response with Failed status.
func (r *Result) SetFailed(err armerrors.ErrorDetails, requeue bool) {
	if r == nil {
		r = &Result{}
	}
	r.Requeue = requeue
	r.SetProvisioningState(basedatamodel.ProvisioningStateFailed)
	r.Error = &armerrors.ErrorDetails{
		Code:    err.Code,
		Message: err.Message,
	}
}

// SetCanceled sets the response status to Canceled.
func (r *Result) SetCanceled(message string) {
	if r == nil {
		r = &Result{}
	}
	r.Requeue = false
	r.SetProvisioningState(basedatamodel.ProvisioningStateCanceled)
	r.Error = &armerrors.ErrorDetails{
		Code:    armerrors.OperationCanceled,
		Message: message,
	}
}

// SetProvisioningState sets provisioning state.
func (r *Result) SetProvisioningState(s basedatamodel.ProvisioningStates) {
	r.state = &s
}

// ProvisioningState gets the provisioning state of the result.
func (r *Result) ProvisioningState() basedatamodel.ProvisioningStates {
	if r.state == nil {
		return basedatamodel.ProvisioningStateSucceeded
	}
	return *r.state
}
