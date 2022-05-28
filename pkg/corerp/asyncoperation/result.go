// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package asyncoperation

import (
	"github.com/project-radius/radius/pkg/basedatamodel"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
)

const (
	// DefaultRetryAfter is the default value in seconds for the Retry-After header.
	DefaultRetryAfter = "60"
)

// Result is the response of async operation controller.
type Result struct {
	// Requeue tells the Controller to requeue the reconcile key. Defaults to false.
	Requeue bool

	// Status represents the provisioning status.
	Status basedatamodel.ProvisioningStates

	// Error represents the error when status is Cancelled or Failed.
	Error *armerrors.ErrorDetails
}

// SetSucceeded sets the response status to Succeeded.
func (r *Result) SetSucceeded() {
	if r == nil {
		r = &Result{}
	}
	r.Requeue = false
	r.Status = basedatamodel.ProvisioningStateSucceeded
}

// SetFailed sets the error response with Failed status.
func (r *Result) SetFailed(err armerrors.ErrorDetails) {
	if r == nil {
		r = &Result{}
	}
	r.Requeue = true
	r.Status = basedatamodel.ProvisioningStateFailed
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
	r.Status = basedatamodel.ProvisioningStateCanceled
	r.Error = &armerrors.ErrorDetails{
		Code:    armerrors.OperationCanceled,
		Message: message,
	}
}
