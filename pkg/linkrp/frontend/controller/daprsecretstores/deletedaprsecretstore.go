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

package daprsecretstores

import (
	"context"
	"errors"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel/converter"
	frontend_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*DeleteDaprSecretStore)(nil)

// DeleteDaprSecretStore is the controller implementation to delete daprSecretStore link resource.
type DeleteDaprSecretStore struct {
	ctrl.Operation[*datamodel.DaprSecretStore, datamodel.DaprSecretStore]
	dp deployment.DeploymentProcessor
}

// NewDeleteDaprSecretStore creates a new instance DeleteDaprSecretStore.
func NewDeleteDaprSecretStore(opts frontend_ctrl.Options) (ctrl.Controller, error) {
	return &DeleteDaprSecretStore{
		Operation: ctrl.NewOperation(opts.Options,
			ctrl.ResourceOptions[datamodel.DaprSecretStore]{
				RequestConverter:  converter.DaprSecretStoreDataModelFromVersioned,
				ResponseConverter: converter.DaprSecretStoreDataModelToVersioned,
			}),
		dp: opts.DeployProcessor,
	}, nil
}

func (daprSecretStore *DeleteDaprSecretStore) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	old, etag, err := daprSecretStore.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if old == nil {
		return rest.NewNoContentResponse(), nil
	}

	if etag == "" {
		return rest.NewNoContentResponse(), nil
	}

	r, err := daprSecretStore.PrepareResource(ctx, req, nil, old, etag)
	if r != nil || err != nil {
		return r, err
	}

	err = daprSecretStore.dp.Delete(ctx, serviceCtx.ResourceID, old.Properties.Status.OutputResources)
	if err != nil {
		return nil, err
	}

	err = daprSecretStore.StorageClient().Delete(ctx, serviceCtx.ResourceID.String())
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNoContentResponse(), nil
		}
		return nil, err
	}

	return rest.NewOKResponse(nil), nil
}
