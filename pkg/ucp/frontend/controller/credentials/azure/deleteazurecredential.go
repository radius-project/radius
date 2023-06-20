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
	"errors"
	"fmt"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/datamodel/converter"
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
func NewDeleteAzureCredential(opts armrpc_controller.Options, secretClient secret.Client) (armrpc_controller.Controller, error) {
	return &DeleteAzureCredential{
		Operation: armrpc_controller.NewOperation(opts,
			armrpc_controller.ResourceOptions[datamodel.AzureCredential]{
				RequestConverter:  converter.AzureCredentialDataModelFromVersioned,
				ResponseConverter: converter.AzureCredentialDataModelToVersioned,
			},
		),
		secretClient: secretClient,
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
		if errors.Is(&store.ErrNotFound{ID: serviceCtx.ResourceID.String()}, err) {
			return armrpc_rest.NewNoContentResponse(), nil
		}
		return nil, err
	}

	logger.Info(fmt.Sprintf("Deleted Azure Credential %s successfully", serviceCtx.ResourceID))
	return armrpc_rest.NewOKResponse(nil), nil
}
