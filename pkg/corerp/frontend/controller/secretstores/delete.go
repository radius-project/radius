// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package secretstores

import (
	"context"
	"errors"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	"github.com/project-radius/radius/pkg/ucp/store"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ ctrl.Controller = (*DeleteSecretStore)(nil)

// DeleteSecretStore is the controller implementation to delete application resource.
type DeleteSecretStore struct {
	ctrl.Operation[*datamodel.SecretStore, datamodel.SecretStore]
	KubeClient runtimeclient.Client
}

// NewDeleteApplication creates a new DeleteApplication.
func NewDeleteSecretStore(opts ctrl.Options) (ctrl.Controller, error) {
	return &DeleteSecretStore{
		Operation: ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.SecretStore]{
				RequestConverter:  converter.SecretStoreModelFromVersioned,
				ResponseConverter: converter.SecretStoreModelToVersioned,
			},
		),
		KubeClient: opts.KubeClient,
	}, nil
}

func (a *DeleteSecretStore) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	old, etag, err := a.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if old == nil {
		return rest.NewNoContentResponse(), nil
	}

	if r, err := a.PrepareResource(ctx, req, nil, old, etag); r != nil || err != nil {
		return r, err
	}

	if r, err := DeleteRadiusSecret(ctx, nil, old, a.Options()); r != nil || err != nil {
		return r, err
	}

	if err := a.StorageClient().Delete(ctx, serviceCtx.ResourceID.String()); err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNoContentResponse(), nil
		}
		return nil, err
	}

	return rest.NewOKResponse(nil), nil
}
