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

package containers

import (
	"context"

	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
)

// ValidateAndMutateRequest checks if the newResource has a user-defined identity and if so, returns a bad request
// response, otherwise it sets the identity of the newResource to the identity of the oldResource if it exists.
func ValidateAndMutateRequest(ctx context.Context, newResource, oldResource *datamodel.ContainerResource, options *controller.Options) (rest.Response, error) {
	if newResource.Properties.Identity != nil {
		return rest.NewBadRequestResponse("User-defined identity in Applications.Core/containers is not supported."), nil
	}

	if oldResource != nil {
		// Identity property is populated during deployment.
		// Model converter will not convert .Properties.Identity to datamodel so that newResource.Properties.Identity is always nil.
		// This will populate the existing identity to new resource to keep the identity info.
		newResource.Properties.Identity = oldResource.Properties.Identity
	}

	return nil, nil
}
