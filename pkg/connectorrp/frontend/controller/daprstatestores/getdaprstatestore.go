// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestores

import (
	"context"
	"errors"
	"net/http"

	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel/converter"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*GetDaprStateStore)(nil)

// GetDaprStateStore is the controller implementation to get the daprStateStore conenctor resource.
type GetDaprStateStore struct {
	ctrl.BaseController
}

// NewGetDaprStateStore creates a new instance of GetDaprStateStore.
func NewGetDaprStateStore(opts ctrl.Options) (ctrl.Controller, error) {
	return &GetDaprStateStore{ctrl.NewBaseController(opts)}, nil
}

func (daprStateStore *GetDaprStateStore) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	existingResource := &datamodel.DaprStateStore{}
	_, err := daprStateStore.GetResource(ctx, serviceCtx.ResourceID.String(), existingResource)
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
		}
		return nil, err
	}

	versioned, _ := converter.DaprStateStoreDataModelToVersioned(existingResource, serviceCtx.APIVersion)
	return rest.NewOKResponse(versioned), nil
}
