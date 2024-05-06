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
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
)

// GenericPlane is the representation used for serializing planes when queried generically.
type GenericPlane struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties any `json:"properties"`
}

// ResourceTypeName returns the type of the Plane as a string.
func (p GenericPlane) ResourceTypeName() string {
	return p.Type
}
