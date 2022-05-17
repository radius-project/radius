// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	"github.com/project-radius/radius/pkg/corerp/api/armrpcv1"
)

type AsyncOperationStatus struct {
	armrpcv1.AsyncOperationStatus

	// ResourceID is the resource id associated with operation status.
	ResourceID string `json:"resourceID"`

	// OperationName is the async operation name.
	OperationName string `json:"operationName"`

	// Location represents the location of operationstatus.
	Location string `json:"location"`

	// ClientTenantID is async operation caller's tenant id such as the value from x-ms-home-tenant-id header
	ClientTenantID string `json:"clientTenantID,omitempty"`

	// ClientObjectID is async operation caller's client id such as the value from x-ms-client-object-id header
	ClientObjectID string `json:"clientObjectID,omitempty"`
}
