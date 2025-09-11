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
	v20231001preview "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
)

// RecipePackDataModelToVersioned converts version agnostic recipe pack datamodel to versioned model.
func RecipePackDataModelToVersioned(model *datamodel.RecipePack, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20231001preview.Version:
		versioned := &v20231001preview.RecipePackResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// RecipePackDataModelFromVersioned converts versioned recipe pack model to datamodel.
func RecipePackDataModelFromVersioned(content []byte, version string) (*datamodel.RecipePack, error) {
	switch version {
	case v20231001preview.Version:
		rp := &v20231001preview.RecipePackResource{}
		if err := json.Unmarshal(content, rp); err != nil {
			return nil, err
		}
		dm, err := rp.ConvertTo()
		return dm.(*datamodel.RecipePack), err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}