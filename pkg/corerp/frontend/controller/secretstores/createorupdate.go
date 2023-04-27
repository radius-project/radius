// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package secretstores

import (
	"context"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	rp_frontend "github.com/project-radius/radius/pkg/rp/frontend"

	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ ctrl.Controller = (*CreateOrUpdateSecretStore)(nil)

// CreateOrUpdateSecretStore is the controller implementation to create or update application resource.
type CreateOrUpdateSecretStore struct {
	ctrl.Operation[*datamodel.SecretStore, datamodel.SecretStore]
	KubeClient runtimeclient.Client
}

// NewCreateOrUpdateSecretStore creates a new instance of CreateOrUpdateSecretStore.
func NewCreateOrUpdateSecretStore(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateSecretStore{
		Operation: ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.SecretStore]{
				RequestConverter:  converter.SecretStoreModelFromVersioned,
				ResponseConverter: converter.SecretStoreModelToVersioned,
			}),
		KubeClient: opts.KubeClient,
	}, nil
}

// Run executes CreateOrUpdateSecretStore operation.
func (a *CreateOrUpdateSecretStore) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	newResource, err := a.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	old, etag, err := a.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if r, err := a.PrepareResource(ctx, req, newResource, old, etag); r != nil || err != nil {
		return r, err
	}

	if r, err := rp_frontend.PrepareRadiusResource(ctx, newResource, old, a.Options()); r != nil || err != nil {
		return r, err
	}

	if r, err := ValidateRequest(ctx, newResource, old, a.Options()); r != nil || err != nil {
		return r, err
	}

	if r, err := upsertSecret(ctx, newResource, old, a.Options()); r != nil || err != nil {
		return r, err
	}

	newResource.SetProvisioningState(v1.ProvisioningStateSucceeded)
	newEtag, err := a.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	return a.ConstructSyncResponse(ctx, req.Method, newEtag, newResource)
}
