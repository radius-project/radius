// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package aws

import (
	"context"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/datamodel/converter"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/frontend/controller/credentials"
	"github.com/project-radius/radius/pkg/ucp/secret"
)

var _ armrpc_controller.Controller = (*CreateOrUpdateCredential)(nil)

// CreateOrUpdateCredential is the controller implementation to create/update a UCP credential.
type CreateOrUpdateCredential struct {
	ctrl.Operation[*datamodel.Credential, datamodel.Credential]
}

// NewCreateOrUpdateCredential creates a new CreateOrUpdateCredential.
func NewCreateOrUpdateCredential(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &CreateOrUpdateCredential{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.Credential]{
				RequestConverter:  converter.CredentialDataModelFromVersioned,
				ResponseConverter: converter.CredentialDataModelToVersioned,
			},
		),
	}, nil
}

func (c *CreateOrUpdateCredential) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	newResource, err := c.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	if newResource.Properties.Kind != datamodel.AWSCredentialKind {
		return armrpc_rest.NewBadRequestResponse("Invalid Credential Kind"), nil
	}

	old, etag, err := c.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if r, err := c.PrepareResource(ctx, req, newResource, old, etag); r != nil || err != nil {
		return r, err
	}

	secretName := credentials.GetSecretName(serviceCtx.ResourceID)
	if newResource.Properties.Storage.Kind == datamodel.InternalStorageKind {
		newResource.Properties.Storage.InternalCredential.SecretName = secretName
	}

	// Save the credential secret
	err = secret.SaveSecret(ctx, c.Options().SecretClient, secretName, newResource.Properties.AWSCredential)
	if err != nil {
		return nil, err
	}

	// Do not save the secret in metadata store.
	newResource.Properties.AWSCredential.SecretAccessKey = ""

	newResource.SetProvisioningState(v1.ProvisioningStateSucceeded)
	newEtag, err := c.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	return c.ConstructSyncResponse(ctx, req.Method, newEtag, newResource)
}
