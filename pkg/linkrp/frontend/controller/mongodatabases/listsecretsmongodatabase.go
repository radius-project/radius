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

package mongodatabases

import (
	"context"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel/converter"
	frontend_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
)

var _ ctrl.Controller = (*ListSecretsMongoDatabase)(nil)

// ListSecretsMongoDatabase is the controller implementation to list secrets for the to access the connected mongo database resource resource id passed in the request body.
type ListSecretsMongoDatabase struct {
	ctrl.Operation[*datamodel.MongoDatabase, datamodel.MongoDatabase]
	dp deployment.DeploymentProcessor
}

// NewListSecretsMongoDatabase creates a new instance of ListSecretsMongoDatabase.
func NewListSecretsMongoDatabase(opts frontend_ctrl.Options) (ctrl.Controller, error) {
	return &ListSecretsMongoDatabase{
		Operation: ctrl.NewOperation(opts.Options,
			ctrl.ResourceOptions[datamodel.MongoDatabase]{
				RequestConverter:  converter.MongoDatabaseDataModelFromVersioned,
				ResponseConverter: converter.MongoDatabaseDataModelToVersioned,
			}),
		dp: opts.DeployProcessor,
	}, nil
}

// Run returns secrets values for the specified MongoDatabase resource
func (ctrl *ListSecretsMongoDatabase) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	sCtx := v1.ARMRequestContextFromContext(ctx)

	parsedResourceID := sCtx.ResourceID.Truncate()
	resource, _, err := ctrl.GetResource(ctx, parsedResourceID)
	if err != nil {
		return nil, err
	}

	if resource == nil {
		return rest.NewNotFoundResponse(sCtx.ResourceID), nil
	}

	secrets, err := ctrl.dp.FetchSecrets(ctx, deployment.ResourceData{ID: sCtx.ResourceID, Resource: resource, OutputResources: resource.Properties.Status.OutputResources, ComputedValues: resource.ComputedValues, SecretValues: resource.SecretValues})
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
