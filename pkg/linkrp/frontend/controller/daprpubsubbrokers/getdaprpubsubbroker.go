// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubbrokers

import (
	"context"
	"errors"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel/converter"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*GetDaprPubSubBroker)(nil)

// GetDaprPubSubBroker is the controller implementation to get the daprPubSubBroker conenctor resource.
type GetDaprPubSubBroker struct {
	ctrl.BaseController
}

// NewGetDaprPubSubBroker creates a new instance of GetDaprPubSubBroker.
func NewGetDaprPubSubBroker(opts ctrl.Options) (ctrl.Controller, error) {
	return &GetDaprPubSubBroker{ctrl.NewBaseController(opts)}, nil
}

func (daprPubSub *GetDaprPubSubBroker) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	existingResource := &datamodel.DaprPubSubBroker{}
	_, err := daprPubSub.GetResource(ctx, serviceCtx.ResourceID.String(), existingResource)
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
		}
		return nil, err
	}

	versioned, _ := converter.DaprPubSubBrokerDataModelToVersioned(existingResource, serviceCtx.APIVersion)
	return rest.NewOKResponse(versioned), nil
}
