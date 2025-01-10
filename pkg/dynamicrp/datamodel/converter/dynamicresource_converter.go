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
package converter

import (
	"encoding/json"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/dynamicrp/api"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
)

// DynamicResourceDataModelFromVersioned converts version agnostic datamodel to versioned model.
func DynamicResourceDataModelToVersioned(model *datamodel.DynamicResource, version string) (v1.VersionedModelInterface, error) {
	// NOTE: DynamicResource is used for all API versions.
	//
	// We don't/can't validate the API version here, that must be done before calling the API.
	versioned := &api.DynamicResource{}
	if err := versioned.ConvertFrom(model); err != nil {
		return nil, err
	}
	return versioned, nil
}

// DynamicResourceDataModelFromVersioned converts versioned model to datamodel.
func DynamicResourceDataModelFromVersioned(content []byte, version string) (*datamodel.DynamicResource, error) {
	// NOTE: DynamicResource is used for all API versions.
	//
	// We don't/can't validate the API version here, that must be done before calling the API.
	vm := &api.DynamicResource{}
	if err := json.Unmarshal(content, vm); err != nil {
		return nil, err
	}
	dm, err := vm.ConvertTo()
	if err != nil {
		return nil, err
	}
	return dm.(*datamodel.DynamicResource), nil
}
