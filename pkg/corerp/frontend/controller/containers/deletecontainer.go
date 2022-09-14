// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containers

import (
	"context"
	"fmt"
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
	AsyncDeleteContainerOperationTimeout = time.Duration(120) * time.Second
)

// DeleteContainer is the controller implementation to delete container resource.
type DeleteContainer struct {
	ctrl.Operation[*datamodel.ContainerResource, datamodel.ContainerResource]
}

// NewDeleteContainer creates a new DeleteContainer.
func NewDeleteContainer(opts ctrl.Options) (ctrl.Controller, error) {
	return &DeleteContainer{
		ctrl.NewOperation(opts, converter.ContainerDataModelFromVersioned, converter.ContainerDataModelToVersioned),
	}, nil
}

func (dc *DeleteContainer) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	old, etag, isNewResource, err := dc.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if isNewResource {
		return rest.NewNoContentResponse(), nil
	}

	if err := dc.ValidateResource(ctx, req, nil, old, etag, isNewResource); err != nil {
		return nil, err
	}

	if !old.Properties.ProvisioningState.IsTerminal() {
		return nil, rest.NewConflictResponse(fmt.Sprintf(ctrl.InProgressStateMessageFormat, old.Properties.ProvisioningState))
	}

	if err := dc.StatusManager().QueueAsyncOperation(ctx, serviceCtx, AsyncDeleteContainerOperationTimeout); err != nil {
		old.Properties.ProvisioningState = v1.ProvisioningStateFailed
		_, rbErr := dc.SaveResource(ctx, serviceCtx.ResourceID.String(), old, etag)
		if rbErr != nil {
			return nil, rbErr
		}
		return nil, err
	}

	return dc.ConstructAsyncResponse(ctx, req.Method, etag, old)
}
