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

package frontend

import (
	"context"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/daprrp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

// PrepareRadiusResource validates the Radius resource and prepare new resource data.
func PrepareRadiusResource[P interface {
	*T
	rpv1.RadiusResourceModel
}, T any](ctx context.Context, newResource *T, oldResource *T, options *controller.Options) (rest.Response, error) {
	if oldResource == nil {
		return nil, nil
	}
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	oldProp := P(oldResource).ResourceMetadata()
	newProp := P(newResource).ResourceMetadata()

	if !oldProp.EqualLinkedResource(newProp) {
		return rest.NewLinkedResourceUpdateErrorResponse(serviceCtx.ResourceID, oldProp, newProp), nil
	}

	// Keep outputresource from existing resource since the incoming request hasn't had an outputresource
	// processed by the backend yet.
	newProp.Status.DeepCopy(&oldProp.Status)

	return nil, nil
}

// PrepareDaprResource validates if the cluster has Dapr installed.
func PrepareDaprResource[P interface {
	*T
	rpv1.RadiusResourceModel
}, T any](ctx context.Context, newResource *T, oldResource *T, options *controller.Options) (rest.Response, error) {
	isDaprSupported, err := datamodel.IsDaprInstalled(ctx, options.KubeClient)
	if err != nil {
		return nil, err
	}
	if !isDaprSupported {
		return rest.NewDependencyMissingResponse(datamodel.DaprMissingError), nil
	}

	return nil, nil
}
