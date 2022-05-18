// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	"github.com/project-radius/radius/pkg/api/armrpcv1"
)

// AsyncOperationStatus is the datamodel for Async operation statuses.
type AsyncOperationStatus struct {
	armrpcv1.AsyncOperationStatus

	// LinkedResourceID is the resource id associated with operation status.
	LinkedResourceID string `json:"resourceID"`

	// OperationName is the async operation name.
	OperationName string `json:"operationName"`

	// Location represents the location of operationstatus.
	Location string `json:"location"`

	// HomeTenantID is async operation caller's tenant id such as the value from x-ms-home-tenant-id header.
	HomeTenantID string `json:"clientTenantID,omitempty"`

	// ClientObjectID is async operation caller's client id such as the value from x-ms-client-object-id header.
	ClientObjectID string `json:"clientObjectID,omitempty"`
}
