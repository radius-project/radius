// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"

// ResourceGroup represents UCP ResourceGroup.
type ResourceGroup struct {
	TrackedResource v1.TrackedResource
}

func (p ResourceGroup) ResourceTypeName() string {
	return "UCP/ResourceGroups"
}
