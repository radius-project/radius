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
	ctrl.BaseController
}

// NewDeleteCredential creates a new DeleteCredential.
func NewDeleteCredential(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &DeleteCredential{ctrl.NewBaseController(opts)}, nil
}

func (p *DeleteCredential) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	path := middleware.GetRelativePath(p.Options.BasePath, req.URL.Path)
	logger := ucplog.FromContextOrDiscard(ctx)

	resourceID, err := resources.ParseResource(path)
	if err != nil {
		return armrpc_rest.NewBadRequestResponse(err.Error()), nil
	}
	// Check if the resource being deleted exists or not.
	existingResource := datamodel.Credential{}
	etag, err := p.GetResource(ctx, resourceID.String(), &existingResource)
	if errors.Is(err, &store.ErrNotFound{}) {
		return armrpc_rest.NewNoContentResponse(), nil
	} else if err != nil {
		return nil, err
	}

	secretName := credentials.GetSecretName(resourceID)

	// Delete the credential secret.
	err = p.Options.SecretClient.Delete(ctx, secretName)
	if errors.Is(err, &secret.ErrNotFound{}) {
		return armrpc_rest.NewNoContentResponse(), nil
	} else if err != nil {
		return nil, err
	}

	// Delete resource from radius.
	err = p.DeleteResource(ctx, resourceID.String(), etag)
	if errors.Is(err, &store.ErrNotFound{}) {
		return armrpc_rest.NewNoContentResponse(), nil
	} else if err != nil {
		return nil, err
	}

	logger.Info(fmt.Sprintf("Deleted Credential %s successfully", resourceID))
	return rest.NewOKResponse(nil), nil
}
