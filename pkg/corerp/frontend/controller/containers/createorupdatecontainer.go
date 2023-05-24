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
	"net/http"
	"time"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	rp_frontend "github.com/project-radius/radius/pkg/rp/frontend"
)

var _ ctrl.Controller = (*CreateOrUpdateContainer)(nil)

var (
	// AsyncPutContainerOperationTimeout is the default timeout duration of async put container operation.
	AsyncPutContainerOperationTimeout = time.Duration(5) * time.Minute
)

// CreateOrUpdateContainer is the controller implementation to create or update a container resource.
type CreateOrUpdateContainer struct {
	ctrl.Operation[*datamodel.ContainerResource, datamodel.ContainerResource]
}

// NewCreateOrUpdateContainer creates a new CreateOrUpdateContainer.
func NewCreateOrUpdateContainer(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateContainer{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.ContainerResource]{
				RequestConverter:  converter.ContainerDataModelFromVersioned,
				ResponseConverter: converter.ContainerDataModelToVersioned,
			},
		),
	}, nil
}

// Run executes CreateOrUpdateContainer operation.
func (e *CreateOrUpdateContainer) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	newResource, err := e.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	old, etag, err := e.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if r, err := e.PrepareResource(ctx, req, newResource, old, etag); r != nil || err != nil {
		return r, err
	}

	if r, err := rp_frontend.PrepareRadiusResource(ctx, newResource, old, e.Options()); r != nil || err != nil {
		return r, err
	}

	if r, err := ValidateAndMutateRequest(ctx, newResource, old, e.Options()); r != nil || err != nil {
		return r, err
	}

	if r, err := e.PrepareAsyncOperation(ctx, newResource, v1.ProvisioningStateAccepted, AsyncPutContainerOperationTimeout, &etag); r != nil || err != nil {
		return r, err

	}
	return e.ConstructAsyncResponse(ctx, req.Method, etag, newResource)
}
