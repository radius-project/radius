// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"

// ResourceGroup represents UCP ResourceGroup.
type ResourceGroup struct {
	v1.BaseResource
}

func (p ResourceGroup) ResourceTypeName() string {
	return "System.Resources/resourceGroups"
}
