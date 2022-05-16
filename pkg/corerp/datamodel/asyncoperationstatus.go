// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import "github.com/project-radius/radius/pkg/api/armrpcv1"

type AsyncOperationStatus struct {
	armrpcv1.AsyncOperationStatus

	// ResourceID is the resource id associated with operation status.
	ResourceID string `json:"resourceID"`
}
