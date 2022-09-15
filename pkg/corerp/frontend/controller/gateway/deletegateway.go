// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package gateway

import (
	"context"
	"fmt"
	"net/http"
	"time"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
)

var _ ctrl.Controller = (*DeleteGateway)(nil)

var (
	// AsyncDeleteGatewayOperationTimeout is the default timeout duration of async delete gateway operation.
	AsyncDeleteGatewayOperationTimeout = time.Duration(120) * time.Second
)

// DeleteGateway is the controller implementation to delete gateway resource.
type DeleteGateway struct {
	ctrl.Operation[*rm, rm]
}

// NewDeleteGateway creates a new DeleteGateway.
func NewDeleteGateway(opts ctrl.Options) (ctrl.Controller, error) {
	return &DeleteGateway{
		ctrl.NewOperation(opts, converter.GatewayDataModelFromVersioned, converter.GatewayDataModelToVersioned),
	}, nil
}

func (dc *DeleteGateway) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	old, etag, err := dc.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if old == nil {
		return rest.NewNoContentResponse(), nil
	}

	if err := dc.ValidateResource(ctx, req, nil, old, etag); err != nil {
		return nil, err
	}

	if !old.Properties.ProvisioningState.IsTerminal() {
		return rest.NewConflictResponse(fmt.Sprintf(ctrl.InProgressStateMessageFormat, old.Properties.ProvisioningState)), nil
	}

	// TODO: Do we need to update ProvisioningState here?

	if err := dc.StatusManager().QueueAsyncOperation(ctx, serviceCtx, AsyncDeleteGatewayOperationTimeout); err != nil {
		old.Properties.ProvisioningState = v1.ProvisioningStateFailed
		_, rbErr := dc.SaveResource(ctx, serviceCtx.ResourceID.String(), old, etag)
		if rbErr != nil {
			return nil, rbErr
		}
		return nil, err
	}

	return dc.ConstructAsyncResponse(ctx, req.Method, etag, old)
}
