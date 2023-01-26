// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestores

import (
	"context"
	"net/http"
	"time"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel/converter"
	frontend_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller"
)

var _ ctrl.Controller = (*DeleteDaprStateStore)(nil)

const (
	// AsyncDeleteDeleteDaprStateStoreOperationTimeout is the default timeout duration of async delete dapr state store operation.
	// DaprStateStore takes 1-2 mins to delete.
	AsyncDeleteDeleteDaprStateStoreOperationTimeout = time.Duration(300) * time.Second
)

// DeleteDaprStateStore is the controller implementation to delete daprStateStore link resource.
type DeleteDaprStateStore struct {
	ctrl.Operation[*datamodel.DaprStateStore, datamodel.DaprStateStore]
}

// NewDeleteDaprStateStore creates a new instance DeleteDaprStateStore.
func NewDeleteDaprStateStore(opts frontend_ctrl.Options) (ctrl.Controller, error) {
	return &DeleteDaprStateStore{
		Operation: ctrl.NewOperation(opts.Options, ctrl.ResourceOptions[datamodel.DaprStateStore]{
			RequestConverter:  converter.DaprStateStoreDataModelFromVersioned,
			ResponseConverter: converter.DaprStateStoreDataModelToVersioned,
		}),
	}, nil
}

func (daprStateStore *DeleteDaprStateStore) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	// Read resource metadata from the storage
	old, etag, err := daprStateStore.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}
	if old == nil {
		return rest.NewNoContentResponse(), nil
	}

	if etag == "" {
		return rest.NewNoContentResponse(), nil
	}

	if r, err := daprStateStore.PrepareResource(ctx, req, nil, old, etag); r != nil || err != nil {
		return r, err
	}

	if r, err := daprStateStore.PrepareAsyncOperation(ctx, old, v1.ProvisioningStateAccepted, AsyncDeleteDeleteDaprStateStoreOperationTimeout, &etag); r != nil || err != nil {
		return r, err
	}

	return daprStateStore.ConstructAsyncResponse(ctx, req.Method, etag, old)
}
