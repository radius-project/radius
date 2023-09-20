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

package v20220901privatepreview

import (
	"errors"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
)

const (
	ResourceType = "System.Resources/resources"
)

// ConvertTo converts from the versioned GenericResource resource to version-agnostic datamodel.
func (src *GenericResource) ConvertTo() (v1.DataModelInterface, error) {
	return nil, errors.New("the GenericResource type does not support conversion from versioned models")
}

// ConvertFrom converts from version-agnostic datamodel to the versioned GenericResource resource.
func (dst *GenericResource) ConvertFrom(src v1.DataModelInterface) error {
	entry, ok := src.(*datamodel.GenericResource)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	// The properties are used to store the data of the "tracked" resource.
	dst.ID = to.Ptr(entry.Properties.ID)
	dst.Name = to.Ptr(entry.Properties.Name)
	dst.Type = to.Ptr(entry.Properties.Type)

	return nil
}
