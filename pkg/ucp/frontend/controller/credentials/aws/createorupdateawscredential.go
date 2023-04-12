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

var _ armrpc_controller.Controller = (*CreateOrUpdateAWSCredential)(nil)

// CreateOrUpdateAWSCredential is the controller implementation to create/update a UCP AWS credential.
type CreateOrUpdateAWSCredential struct {
	armrpc_controller.Operation[*datamodel.AWSCredential, datamodel.AWSCredential]
	secretClient secret.Client
}

// NewCreateOrUpdateAWSCredential creates a new CreateOrUpdateAWSCredential.
func NewCreateOrUpdateAWSCredential(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &CreateOrUpdateAWSCredential{
		Operation: armrpc_controller.NewOperation(opts.Options,
			armrpc_controller.ResourceOptions[datamodel.AWSCredential]{
				RequestConverter:  converter.AWSCredentialDataModelFromVersioned,
				ResponseConverter: converter.AWSCredentialDataModelToVersioned,
			},
		),
		secretClient: opts.SecretClient,
	}, nil
}

func (c *CreateOrUpdateAWSCredential) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
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
	err = secret.SaveSecret(ctx, c.secretClient, secretName, newResource.Properties.AWSCredential)
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
