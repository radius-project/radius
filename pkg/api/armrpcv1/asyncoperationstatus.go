// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armrpcv1

import (
	"time"

	"github.com/project-radius/radius/pkg/basedatamodel"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
)

// OperationStatus represents an OperationStatus resource.
type AsyncOperationStatus struct {

	// Id represents the async operation id.
	ID string `json:"id,omitempty"`

	// Name represents the async operation name and is usually set to the async operation id.
	Name string `json:"name,omitempty"`

	// Status represents the provisioning state of the resource.
	Status basedatamodel.ProvisioningStates `json:"status,omitempty"`

	// StartTime represents the async operation start time.
	StartTime time.Time `json:"startTime,omitempty"`

	// EndTime represents the async operation end time.
	EndTime time.Time `json:"endTime,omitempty"`

	// Error represents the error occured during provisioning.
	Error *armerrors.ErrorResponse `json:"error,omitempty"`
}
