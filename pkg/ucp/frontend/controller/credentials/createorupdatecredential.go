// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package credentials

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/middleware"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/datamodel/converter"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/secret"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

const (
	AzureCredentialKind = "azure.com.serviceprincipal"
	AWSCredentialKind   = "aws.com.iam"
)

var _ armrpc_controller.Controller = (*CreateOrUpdateCredential)(nil)

// CreateOrUpdateCredential is the controller implementation to create/update a UCP secret.
type CreateOrUpdateCredential struct {
	ctrl.BaseController
}

// NewCreateOrUpdateCredential creates a new CreateOrUpdateSecret.
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
	if err != nil {
		return armrpc_rest.NewBadRequestResponse(err.Error()), nil
	}

	_, err = resources.Parse(path)
	// cannot parse ID something wrong with request
	if err != nil {
		return armrpc_rest.NewBadRequestResponse(err.Error()), nil
	}

	ctx = ucplog.WrapLogContext(ctx, ucplog.LogFieldCredential, newResource.Properties.Kind)
	logger := ucplog.GetLogger(ctx)

	// Check if the credential already exists in database
	credentialExists := true
	existingResource := datamodel.Credential{}
	etag, err := p.GetResource(ctx, newResource.TrackedResource.ID, &existingResource)
	if err != nil {
		if errors.Is(err, &store.ErrNotFound{}) {
			credentialExists = false
			logger.Info(fmt.Sprintf("No existing credential %s found in db", newResource.TrackedResource.ID))
		} else {
			return nil, err
		}
	}

	secretName, err := resources.ExtractSecretNameFromPath(path)
	if err != nil {
		return armrpc_rest.NewBadRequestResponse(err.Error()), nil
	}
	if *newResource.Properties.Storage.Kind == datamodel.InternalStorageKind {
		newResource.Properties.Storage.InternalCredential.SecretName = &secretName
	}

	// Save the credential secret
	credentialKind := newResource.Properties.Kind
	if strings.EqualFold(credentialKind, AzureCredentialKind) {
		err = secret.SaveSecret(ctx, p.Options.SecretClient, secretName, newResource.Properties.AzureCredential)
	} else if strings.EqualFold(credentialKind, AWSCredentialKind) {
		err = secret.SaveSecret(ctx, p.Options.SecretClient, secretName, newResource.Properties.AWSCredential)
	}
	if err != nil {
		return nil, err
	}

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
	if credentialExists {
		logger.Info(fmt.Sprintf("Updated credential %s successfully", newResource.TrackedResource.ID))
	} else {
		logger.Info(fmt.Sprintf("Created credential %s successfully", newResource.TrackedResource.ID))
	}
	return restResp, nil
}
