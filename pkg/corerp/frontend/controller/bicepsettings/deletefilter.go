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

package bicepsettings

import (
	"context"
	"fmt"

	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
)

// PreventDeleteIfReferenced is a delete filter that prevents deletion of a BicepSettings
// resource if it is still referenced by any environment.
func PreventDeleteIfReferenced(ctx context.Context, oldResource *datamodel.BicepSettings_v20250801preview, options *controller.Options) (rest.Response, error) {
	if len(oldResource.Properties.ReferencedBy) > 0 {
		return rest.NewConflictResponse(fmt.Sprintf(
			"Cannot delete bicepSettings: still referenced by environments: %v",
			oldResource.Properties.ReferencedBy,
		)), nil
	}
	return nil, nil
}
