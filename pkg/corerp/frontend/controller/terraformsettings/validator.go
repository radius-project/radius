/*
Copyright 2026 The Radius Authors.

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

// Package terraformsettings validates direct updates to Terraform state locations.
package terraformsettings

import (
	"context"

	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
)

// ValidateRequest applies equally to PUT and replacement-semantics PATCH requests.
func ValidateRequest(ctx context.Context, newResource, oldResource *datamodel.TerraformSettings, options *controller.Options) (rest.Response, error) {
	if oldResource != nil && oldResource.Properties.Backend != nil &&
		!oldResource.Properties.Backend.SameLocation(newResource.Properties.Backend) {
		return rest.NewBadRequestResponse("backend location cannot change or be removed; include the unchanged backend in PUT and PATCH requests. Terraform state migration is not supported."), nil
	}
	if err := newResource.Properties.Backend.Validate(); err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}
	return nil, nil
}
