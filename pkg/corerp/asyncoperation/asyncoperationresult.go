// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package asyncoperation

import (
	"time"

	"github.com/google/uuid"
	"github.com/project-radius/radius/pkg/basedatamodel"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

const (
	// DefaultRetryAfter is the default value in seconds for the Retry-After header.
	DefaultRetryAfter = "60"
)

// AsyncOperationResult is the response of async operation controller.
type AsyncOperationResult struct {
	// OperationID represents the async operation id.
	OperationID uuid.UUID
	// OperationName represents the async operation name.
	OperationName string

	// ResourceID represents the linked resource.
	ResourceID resources.ID

	// Status represents the provisioning status.
	Status basedatamodel.ProvisioningStates

	// StartTime represents the start time of async request operation.
	StartTime *time.Time
	// EndTime represents the end time of async request operation.
	EndTime *time.Time

	// Error represents the error when status is Cancelled or Failed.
	Error *armerrors.ErrorDetails
}

// NewAsyncOperationResult creates NewAsyncOperationResult object.
func NewAsyncOperationResult(
	operationID uuid.UUID, operationName string,
	resourceID resources.ID,
	status basedatamodel.ProvisioningStates) *AsyncOperationResult {
	now := time.Now().UTC()
	return &AsyncOperationResult{
		OperationID:   operationID,
		OperationName: operationName,
		ResourceID:    resourceID,
		Status:        status,
		StartTime:     &now,
	}
}

// SetSucceeded sets the response status to Succeeded.
func (a *AsyncOperationResult) SetSucceeded() {
	now := time.Now().UTC()
	a.EndTime = &now
	a.Status = basedatamodel.ProvisioningStateSucceeded
}

// SetFailed sets the error response with Failed status.
func (a *AsyncOperationResult) SetFailed(err armerrors.ErrorDetails) {
	now := time.Now().UTC()
	a.EndTime = &now
	a.Status = basedatamodel.ProvisioningStateFailed
	a.Error = &armerrors.ErrorDetails{
		Code:    err.Code,
		Message: err.Message,
	}
}

// SetCanceled sets the response status to Canceled.
func (a *AsyncOperationResult) SetCanceled(message string) {
	now := time.Now().UTC()
	a.EndTime = &now
	a.Status = basedatamodel.ProvisioningStateCanceled
	a.Error = &armerrors.ErrorDetails{
		Code:    armerrors.OperationCanceled,
		Message: message,
	}
}
