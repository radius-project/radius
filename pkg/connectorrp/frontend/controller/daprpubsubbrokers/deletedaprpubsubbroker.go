// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubbrokers

import (
	"context"
	"errors"
	"net/http"

	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*DeleteDaprPubSubBroker)(nil)

// DeleteDaprPubSubBroker is the controller implementation to delete daprPubSubBroker connector resource.
type DeleteDaprPubSubBroker struct {
	ctrl.BaseController
}

// NewDeleteDaprPubSubBroker creates a new instance DeleteDaprPubSubBroker.
func NewDeleteDaprPubSubBroker(opts ctrl.Options) (ctrl.Controller, error) {
	return &DeleteDaprPubSubBroker{ctrl.NewBaseController(opts)}, nil
}

func (daprPubSub *DeleteDaprPubSubBroker) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	// Read resource metadata from the storage
	existingResource := &datamodel.DaprPubSubBroker{}
	etag, err := daprPubSub.GetResource(ctx, serviceCtx.ResourceID.String(), existingResource)
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNoContentResponse(), nil
		}
		return nil, err
	}

	if etag == "" {
		return rest.NewNoContentResponse(), nil
	}

	err = ctrl.ValidateETag(*serviceCtx, etag)
	if err != nil {
		return rest.NewPreconditionFailedResponse(serviceCtx.ResourceID.String(), err.Error()), nil
	}

	err = daprPubSub.DeploymentProcessor().Delete(ctx, serviceCtx.ResourceID, existingResource.Properties.Status.OutputResources)
	if err != nil {
		return nil, err
	}
	err = daprPubSub.StorageClient().Delete(ctx, serviceCtx.ResourceID.String())
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNoContentResponse(), nil
		}
		return nil, err
	}

	return rest.NewOKResponse(nil), nil
}
