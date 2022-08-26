// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
)

type PlaneKind string

type PlaneProperties struct {
	Kind              PlaneKind
	URL               *string
	ResourceProviders map[string]*string
}

// Plane represents UCP Plane.
type Plane struct {
	TrackedResource v1.TrackedResource

	// Properties is the properties of the resource.
	Properties PlaneProperties `json:"properties"`
}

func (p Plane) ResourceTypeName() string {
	return "UCP/Planes"
}
