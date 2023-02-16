// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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
)

var _ ctrl.Controller = (*DeleteContainer)(nil)

var (
	// AsyncDeleteContainerOperationTimeout is the default timeout duration of async delete container operation.
	AsyncDeleteContainerOperationTimeout = time.Duration(300) * time.Second
)

// DeleteContainer is the controller implementation to delete container resource.
type DeleteContainer struct {
	ctrl.Operation[*datamodel.ContainerResource, datamodel.ContainerResource]
}

// NewDeleteContainer creates a new DeleteContainer.
func NewDeleteContainer(opts ctrl.Options) (ctrl.Controller, error) {
	return &DeleteContainer{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.ContainerResource]{
				RequestConverter:  converter.ContainerDataModelFromVersioned,
				ResponseConverter: converter.ContainerDataModelToVersioned,
			},
		),
	}, nil
}

func (dc *DeleteContainer) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	old, etag, err := dc.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if old == nil {
		return rest.NewNoContentResponse(), nil
	}

	if r, err := dc.PrepareResource(ctx, req, nil, old, etag); r != nil || err != nil {
		return r, err
	}

	if r, err := dc.PrepareAsyncOperation(ctx, old, v1.ProvisioningStateAccepted, AsyncDeleteContainerOperationTimeout, &etag); r != nil || err != nil {
		return r, err
	}

	return dc.ConstructAsyncResponse(ctx, req.Method, etag, old)
}
