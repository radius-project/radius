// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	"strings"

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
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties PlaneProperties `json:"properties"`
}

func (p Plane) ResourceTypeName() string {
	return p.Type
}

func (plane *Plane) LookupResourceProvider(key string) string {
	var value string
	for k, v := range plane.Properties.ResourceProviders {
		if strings.EqualFold(k, key) {
			value = *v
			break
		}
	}
	return value
}
