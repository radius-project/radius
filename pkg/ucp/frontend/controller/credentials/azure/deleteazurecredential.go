// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package azure

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/datamodel/converter"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/frontend/controller/credentials"
	"github.com/project-radius/radius/pkg/ucp/secret"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

var _ armrpc_controller.Controller = (*DeleteAzureCredential)(nil)

// DeleteAzureCredential is the controller implementation to delete a UCP Azure credential.
type DeleteAzureCredential struct {
	armrpc_controller.Operation[*datamodel.AzureCredential, datamodel.AzureCredential]
	secretClient secret.Client
}

// NewDeleteAzureCredential creates a new DeleteAzureCredential.
func NewDeleteAzureCredential(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &DeleteAzureCredential{
		Operation: armrpc_controller.NewOperation(opts.Options,
			armrpc_controller.ResourceOptions[datamodel.AzureCredential]{
				RequestConverter:  converter.AzureCredentialDataModelFromVersioned,
				ResponseConverter: converter.AzureCredentialDataModelToVersioned,
			},
		),
		secretClient: opts.SecretClient,
	}, nil
}

func (c *DeleteAzureCredential) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	old, etag, err := c.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if old == nil {
		return armrpc_rest.NewNoContentResponse(), nil
	}

	secretName := credentials.GetSecretName(serviceCtx.ResourceID)

	// Delete the credential secret.
	err = c.secretClient.Delete(ctx, secretName)
	if errors.Is(err, &secret.ErrNotFound{}) {
		return armrpc_rest.NewNoContentResponse(), nil
	} else if err != nil {
		return nil, err
	}

	if r, err := c.PrepareResource(ctx, req, nil, old, etag); r != nil || err != nil {
		return r, err
	}

	if err := c.StorageClient().Delete(ctx, serviceCtx.ResourceID.String()); err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return armrpc_rest.NewNoContentResponse(), nil
		}
		return nil, err
	}

	logger.Info(fmt.Sprintf("Deleted Azure Credential %s successfully", serviceCtx.ResourceID))
	return armrpc_rest.NewOKResponse(nil), nil
}
