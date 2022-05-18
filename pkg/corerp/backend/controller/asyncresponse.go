// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"time"

	"github.com/google/uuid"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/basedatamodel"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
)

// AsyncResponse is the response of async operation controller.
type AsyncResponse struct {
	// OperationID represents the async operation id.
	OperationID uuid.UUID
	// OperationName represents the async operation name.
	OperationName string
	// ResourceID represents the linked resource.
	ResourceID azresources.ResourceID
	// Status represents the provisioning status.
	Status basedatamodel.ProvisioningStates
	// StartTime represents the start time of async request operation.
	StartTime *time.Time
	// EndTime represents the end time of async request operation.
	EndTime *time.Time
	// Error represents the error when status is Cancelled or Failed.
	Error *armerrors.ErrorDetails
}

// NewAsyncResponse creates AsyncResponse object.
func NewAsyncResponse(operationID uuid.UUID, operationName string, resourceID azresources.ResourceID, status basedatamodel.ProvisioningStates) *AsyncResponse {
	now := time.Now().UTC()
	return &AsyncResponse{
		OperationID:   operationID,
		OperationName: operationName,
		ResourceID:    resourceID,
		Status:        status,
		StartTime:     &now,
	}
}

// SetSucceeded sets the response status to Succeeded.
func (a *AsyncResponse) SetSucceeded() {
	now := time.Now().UTC()
	a.EndTime = &now
	a.Status = basedatamodel.ProvisioningStateSucceeded
}

// SetFailed sets the error response with Failed status.
func (a *AsyncResponse) SetFailed(err armerrors.ErrorDetails) {
	now := time.Now().UTC()
	a.EndTime = &now
	a.Status = basedatamodel.ProvisioningStateFailed
}

// SetCanceled sets the response status to Canceled.
func (a *AsyncResponse) SetCanceled(message string) {
	now := time.Now().UTC()
	a.EndTime = &now
	a.Status = basedatamodel.ProvisioningStateCanceled
	a.Error = &armerrors.ErrorDetails{
		Code:    armerrors.OperationCanceled,
		Message: message,
	}
}
