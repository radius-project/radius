// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package gateway

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/rp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*DeleteGateway)(nil)

var (
	// AsyncDeleteGatewayOperationTimeout is the default timeout duration of async delete gateway operation.
	AsyncDeleteGatewayOperationTimeout = time.Duration(120) * time.Second
)

// DeleteGateway is the controller implementation to delete gateway resource.
type DeleteGateway struct {
	ctrl.BaseController
}

// NewDeleteGateway creates a new DeleteGateway.
func NewDeleteGateway(opts ctrl.Options) (ctrl.Controller, error) {
	return &DeleteGateway{ctrl.NewBaseController(opts)}, nil
}

func (dc *DeleteGateway) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	old := &datamodel.Gateway{}
	etag, err := dc.GetResource(ctx, serviceCtx.ResourceID.String(), old)
	if err != nil && !errors.Is(&store.ErrNotFound{}, err) {
		return nil, err
	}

	if err != nil && errors.Is(&store.ErrNotFound{}, err) {
		return rest.NewNoContentResponse(), nil
	}

	if !old.Properties.ProvisioningState.IsTerminal() {
		return rest.NewConflictResponse(fmt.Sprintf(ctrl.InProgressStateMessageFormat, old.Properties.ProvisioningState)), nil
	}

	err = ctrl.ValidateETag(*serviceCtx, etag)
	if err != nil {
		return rest.NewPreconditionFailedResponse(serviceCtx.ResourceID.String(), err.Error()), nil
	}

	err = dc.StatusManager().QueueAsyncOperation(ctx, serviceCtx, AsyncDeleteGatewayOperationTimeout)
	if err != nil {
		old.Properties.ProvisioningState = v1.ProvisioningStateFailed
		_, rbErr := dc.SaveResource(ctx, serviceCtx.ResourceID.String(), old, etag)
		if rbErr != nil {
			return nil, rbErr
		}
		return nil, err
	}

	old.Properties.ProvisioningState = v1.ProvisioningStateDeleting

	return rest.NewAsyncOperationResponse(old, old.TrackedResource.Location, http.StatusAccepted, serviceCtx.ResourceID, serviceCtx.OperationID, serviceCtx.APIVersion), nil
}
