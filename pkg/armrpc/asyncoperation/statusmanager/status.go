// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package statusmanager

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
)

// Status is the datamodel for Async operation statuses.
type Status struct {
	v1.AsyncOperationStatus

	// LinkedResourceID is the resource id associated with operation status.
	LinkedResourceID string `json:"resourceID"`

	// Location represents the location of operationstatus.
	Location string `json:"location"`

	// HomeTenantID is async operation caller's tenant id such as the value from x-ms-home-tenant-id header.
	HomeTenantID string `json:"clientTenantID,omitempty"`

	// ClientObjectID is async operation caller's client id such as the value from x-ms-client-object-id header.
	ClientObjectID string `json:"clientObjectID,omitempty"`
}
