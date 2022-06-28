// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package db

func NewDBRadiusResource(id string, properties map[string]interface{}) RadiusResource {
	return RadiusResource{
		ID:         id,
		Definition: properties,
		Status:     RadiusResourceStatus{},
	}
}
