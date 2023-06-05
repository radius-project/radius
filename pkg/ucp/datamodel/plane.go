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

// # Function Explanation
// 
//	Plane's ResourceTypeName function returns the type of the Plane as a string. If an error occurs, it will be logged and 
//	an empty string will be returned.
func (p Plane) ResourceTypeName() string {
	return p.Type
}

// # Function Explanation
// 
//	"LookupResourceProvider" searches through the "ResourceProviders" property of the Plane struct and returns the value 
//	associated with the given key, if found. If the key is not found, an empty string is returned.
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
