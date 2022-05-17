// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armrpcv1

import (
	"time"

	"github.com/project-radius/radius/pkg/radrp/armerrors"
)

// AsyncOperationStatus is asynchronous operation status resource model.
// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/async-api-reference.md#azure-asyncoperation-resource-format
type AsyncOperationStatus struct {
	// ID represents the id of async operation resource.
	ID string `json:"id"`
	// Name is GUID representing the name of async operation resource.
	Name string `json:"name"`
	// Status is the provisioning status.
	Status string `json:"status"`
	// StartTime is the start time of async operation.
	StartTime time.Time `json:"startTime"`
	// EndTime is the end time of async operation.
	EndTime time.Time `json:"endTime"`
	// Optional. Properties is the result when operation is succeeded.
	Properties interface{} `json:"properties"`
	// Error is the error response when operation is cancelled or failed.
	Error *armerrors.ErrorResponse `json:"error,omitempty"`
}
