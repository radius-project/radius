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
	"github.com/project-radius/radius/pkg/datastoresrp/datamodel"
	"github.com/project-radius/radius/pkg/datastoresrp/datamodel/converter"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
)

var _ ctrl.Controller = (*ListSecretsMongoDatabase)(nil)

// ListSecretsMongoDatabase is the controller implementation to list secrets for the to access the connected mongo database resource resource id passed in the request body.
type ListSecretsMongoDatabase struct {
	ctrl.Operation[*datamodel.MongoDatabase, datamodel.MongoDatabase]
}

// # Function Explanation
//
// NewListSecretsMongoDatabase creates a new instance of ListSecretsMongoDatabase, or an error if the controller could not be created.
func NewListSecretsMongoDatabase(opts ctrl.Options) (ctrl.Controller, error) {
	return &ListSecretsMongoDatabase{
		Operation: ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.MongoDatabase]{
				RequestConverter:  converter.MongoDatabaseDataModelFromVersioned,
				ResponseConverter: converter.MongoDatabaseDataModelToVersioned,
			}),
	}, nil
}

// # Function Explanation
//
// Run returns secrets values for the specified MongoDatabase resource.
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

	mongoSecrets := datamodel.MongoDatabaseSecrets{}
	if password, ok := resource.SecretValues[renderers.PasswordStringHolder]; ok {
		mongoSecrets.Password = password.Value
	}
	if connectionString, ok := resource.SecretValues[renderers.ConnectionStringValue]; ok {
		mongoSecrets.ConnectionString = connectionString.Value
	}

	versioned, err := converter.MongoDatabaseSecretsDataModelToVersioned(&mongoSecrets, sCtx.APIVersion)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), err
	}
	return rest.NewOKResponse(versioned), nil
}
