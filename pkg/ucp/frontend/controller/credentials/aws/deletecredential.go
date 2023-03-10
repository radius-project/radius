// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package aws

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/middleware"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/datamodel/converter"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/frontend/controller/credentials"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/secret"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

var _ armrpc_controller.Controller = (*DeleteCredential)(nil)

// DeleteCredential is the controller implementation to delete a UCP credential.
type DeleteCredential struct {
	armrpc_controller.Operation[*datamodel.Credential, datamodel.Credential]
	secretClient secret.Client
	basePath     string
}

// NewDeleteCredential creates a new DeleteCredential.
func NewDeleteCredential(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &DeleteCredential{
		Operation: armrpc_controller.NewOperation(opts.CommonControllerOptions,
			armrpc_controller.ResourceOptions[datamodel.Credential]{
				RequestConverter:  converter.CredentialDataModelFromVersioned,
				ResponseConverter: converter.CredentialDataModelToVersioned,
			},
		),
		secretClient: opts.SecretClient,
		basePath:     opts.BasePath,
	}, nil
}

func (c *DeleteCredential) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	path := middleware.GetRelativePath(c.basePath, req.URL.Path)
	logger := ucplog.FromContextOrDiscard(ctx)

	resourceID, err := resources.ParseResource(path)
	if err != nil {
		return armrpc_rest.NewBadRequestResponse(err.Error()), nil
	}

	old, etag, err := c.GetResource(ctx, resourceID)
	if err != nil {
		return nil, err
	}

	if old == nil {
		return rest.NewNoContentResponse(), nil
	}

	secretName := credentials.GetSecretName(resourceID)

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

	if err := c.StorageClient().Delete(ctx, resourceID.String()); err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNoContentResponse(), nil
		}
		return nil, err
	}

	logger.Info(fmt.Sprintf("Deleted Credential %s successfully", resourceID))
	return rest.NewOKResponse(nil), nil
}
