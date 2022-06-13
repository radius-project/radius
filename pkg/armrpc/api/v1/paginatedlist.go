// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v1

// PaginatedList represents the object for resource list pagination.
type PaginatedList struct {
	Value    []interface{} `json:"value"`
	NextLink string        `json:"nextLink,omitempty"`
}
