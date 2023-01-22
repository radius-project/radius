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

	"github.com/go-logr/logr"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/middleware"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/datamodel/converter"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/frontend/controller/credentials"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/secret"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ armrpc_controller.Controller = (*CreateOrUpdateCredential)(nil)

// CreateOrUpdateCredential is the controller implementation to create/update a UCP credential.
type CreateOrUpdateCredential struct {
	ctrl.BaseController
}

// NewCreateOrUpdateCredential creates a new CreateOrUpdateCredential.
func NewCreateOrUpdateCredential(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &CreateOrUpdateCredential{ctrl.NewBaseController(opts)}, nil
}

func (p *CreateOrUpdateCredential) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	path := middleware.GetRelativePath(p.Options.BasePath, req.URL.Path)

	body, err := ctrl.ReadRequestBody(req)
	if err != nil {
		return nil, err
	}

	apiVersion := ctrl.GetAPIVersion(req)
	newResource, err := converter.CredentialDataModelFromVersioned(body, apiVersion)
	if errors.Is(err, v1.ErrUnsupportedAPIVersion) ||
		errors.Is(err, v1.ErrInvalidModelConversion) ||
		errors.Is(err, &v1.ErrModelConversion{}) {
		return armrpc_rest.NewBadRequestResponse(err.Error()), nil
	} else if err != nil {
		return nil, err
	}

	if newResource.Properties.Kind != datamodel.AWSCredentialKind {
		return armrpc_rest.NewBadRequestResponse("Invalid Credential Kind"), nil
	}

	id, err := resources.Parse(path)
	// cannot parse ID something wrong with request
	if err != nil {
		return armrpc_rest.NewBadRequestResponse(err.Error()), nil
	}

	newResource.ID = id.String()
	newResource.Name = id.Name()
	newResource.Type = id.Type()

	logger := logr.FromContextOrDiscard(ctx)

	// Check if the credential already exists in database
	existingResource := datamodel.Credential{}
	etag, err := p.GetResource(ctx, newResource.TrackedResource.ID, &existingResource)
	if err != nil && !errors.Is(err, &store.ErrNotFound{}) {
		return nil, err
	}

	secretName := credentials.GetSecretName(id)
	if newResource.Properties.Storage.Kind == datamodel.InternalStorageKind {
		newResource.Properties.Storage.InternalCredential.SecretName = secretName
	}

	// Save the credential secret
	err = secret.SaveSecret(ctx, p.Options.SecretClient, secretName, newResource.Properties.AWSCredential)
	if err != nil {
		return nil, err
	}

	// Do not save the secret in metadata store.
	newResource.Properties.AWSCredential.SecretAccessKey = ""

	// Save the data model credential to the database
	_, err = p.SaveResource(ctx, newResource.TrackedResource.ID, *newResource, etag)
	if err != nil {
		return nil, err
	}

	// Return a versioned response of the credential
	versioned, err := converter.CredentialDataModelToVersioned(newResource, apiVersion)
	if err != nil {
		return nil, err
	}

	restResp := armrpc_rest.NewOKResponse(versioned)
	logger.Info(fmt.Sprintf("Created credential %s successfully", newResource.TrackedResource.ID))
	return restResp, nil
}
