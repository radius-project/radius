// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodatabases

import (
	"context"
	"errors"
	"net/http"

	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel/converter"
	"github.com/project-radius/radius/pkg/connectorrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/connectorrp/renderers"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*ListSecretsMongoDatabase)(nil)

// ListSecretsMongoDatabase is the controller implementation to list secrets for the to access the connected mongo database resource resource id passed in the request body.
type ListSecretsMongoDatabase struct {
	ctrl.BaseController
}

// NewListSecretsMongoDatabase creates a new instance of ListSecretsMongoDatabase.
func NewListSecretsMongoDatabase(opts ctrl.Options) (ctrl.Controller, error) {
	return &ListSecretsMongoDatabase{ctrl.NewBaseController(opts)}, nil
}

// Run returns secrets values for the specified MongoDatabase resource
func (ctrl *ListSecretsMongoDatabase) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	sCtx := servicecontext.ARMRequestContextFromContext(ctx)

	resource := &datamodel.MongoDatabase{}
	parsedResourceID := sCtx.ResourceID.Truncate()
	_, err := ctrl.GetResource(ctx, parsedResourceID.String(), resource)
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNotFoundResponse(sCtx.ResourceID), nil
		}
		return nil, err
	}

	secrets, err := ctrl.DeploymentProcessor().FetchSecrets(ctx, deployment.ResourceData{ID: sCtx.ResourceID, Resource: resource, OutputResources: resource.Properties.Status.OutputResources, ComputedValues: resource.ComputedValues, SecretValues: resource.SecretValues})
	if err != nil {
		return nil, err
	}

	mongoSecrets := datamodel.MongoDatabaseSecrets{}
	if username, ok := secrets[renderers.UsernameStringValue].(string); ok {
		mongoSecrets.Username = username
	}
	if password, ok := secrets[renderers.PasswordStringHolder].(string); ok {
		mongoSecrets.Password = password
	}
	if connectionString, ok := secrets[renderers.ConnectionStringValue].(string); ok {
		mongoSecrets.ConnectionString = connectionString
	}

	versioned, _ := converter.MongoDatabaseSecretsDataModelToVersioned(&mongoSecrets, sCtx.APIVersion)
	return rest.NewOKResponse(versioned), nil
}
