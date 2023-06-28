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
package azure

import (
	"context"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/datamodel/converter"
	"github.com/project-radius/radius/pkg/ucp/frontend/controller/credentials"
	"github.com/project-radius/radius/pkg/ucp/secret"
)

var _ armrpc_controller.Controller = (*CreateOrUpdateAzureCredential)(nil)

// CreateOrUpdateAzureCredential is the controller implementation to create/update a UCP Azure credential.
type CreateOrUpdateAzureCredential struct {
	armrpc_controller.Operation[*datamodel.AzureCredential, datamodel.AzureCredential]
	secretClient secret.Client
}

// NewCreateOrUpdateAzureCredential creates a new CreateOrUpdateAzureCredential.
//
// # Function Explanation
//
// NewCreateOrUpdateAzureCredential creates a new CreateOrUpdateAzureCredential controller which is used to create or
// update Azure credentials and returns it along with a nil error.
func NewCreateOrUpdateAzureCredential(opts armrpc_controller.Options, secretClient secret.Client) (armrpc_controller.Controller, error) {
	return &CreateOrUpdateAzureCredential{
		Operation: armrpc_controller.NewOperation(opts,
			armrpc_controller.ResourceOptions[datamodel.AzureCredential]{
				RequestConverter:  converter.AzureCredentialDataModelFromVersioned,
				ResponseConverter: converter.AzureCredentialDataModelToVersioned,
			},
		),
		secretClient: secretClient,
	}, nil
}

// # Function Explanation
//
// CreateOrUpdateAzureCredential Run function saves an Azure credential secret in the secret store and updates the
// metadata store with the new resource, setting the provisioning state to succeeded. If an invalid credential kind is
// provided, a bad request response is returned. If an error occurs while saving the secret or the resource, an error is
// returned.
func (c *CreateOrUpdateAzureCredential) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	newResource, err := c.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	if newResource.Properties.Kind != datamodel.AzureCredentialKind {
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
	err = secret.SaveSecret(ctx, c.secretClient, secretName, newResource.Properties.AzureCredential)
	if err != nil {
		return nil, err
	}

	// Do not save the secret in metadata store.
	newResource.Properties.AzureCredential.ClientSecret = ""

	newResource.SetProvisioningState(v1.ProvisioningStateSucceeded)
	newEtag, err := c.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	return c.ConstructSyncResponse(ctx, req.Method, newEtag, newResource)
}
