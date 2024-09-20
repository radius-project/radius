/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package datamodel

import (
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
)

const (
	// RadiusPlaneResourceType is the type of the Radius plane.
	RadiusPlaneResourceType = "System.Radius/planes"
)

// RadiusPlaneProperties is the properties of a Radius plane.
type RadiusPlaneProperties struct {

	// ResourceProviders is a map of the support resource providers.
	ResourceProviders map[string]string `json:"resourceProviders"`
}

// RadiusPlane is the representation of a Radius plane.
type RadiusPlane struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties RadiusPlaneProperties `json:"properties"`
}

// ResourceTypeName returns the type of the Plane as a string.
func (p RadiusPlane) ResourceTypeName() string {
	return p.Type
}

// LookupResourceProvider checks if the input provider is in the list of configured providers.
func (plane RadiusPlane) LookupResourceProvider(key string) string {
	var value string
	for k, v := range plane.Properties.ResourceProviders {
		if strings.EqualFold(k, key) {
			value = v
			break
		}
	}
	return value
}
