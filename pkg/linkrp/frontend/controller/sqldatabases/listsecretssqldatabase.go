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

package sqldatabases

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

var _ ctrl.Controller = (*ListSecretsSqlDatabase)(nil)

// ListSecretsSqlDatabase is the controller implementation to list secrets for the to access the connected sql database resource resource id passed in the request body.
type ListSecretsSqlDatabase struct {
	ctrl.Operation[*datamodel.SqlDatabase, datamodel.SqlDatabase]
	dp deployment.DeploymentProcessor
}

// NewListSecretsSqlDatabase creates a new instance of ListSecretsSqlDatabase.
func NewListSecretsSqlDatabase(opts frontend_ctrl.Options) (ctrl.Controller, error) {
	return &ListSecretsSqlDatabase{
		Operation: ctrl.NewOperation(opts.Options,
			ctrl.ResourceOptions[datamodel.SqlDatabase]{
				RequestConverter:  converter.SqlDatabaseDataModelFromVersioned,
				ResponseConverter: converter.SqlDatabaseDataModelToVersioned,
			}),
		dp: opts.DeployProcessor,
	}, nil
}

// Run returns secrets values for the specified SqlDatabase resource
func (ctrl *ListSecretsSqlDatabase) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	sCtx := v1.ARMRequestContextFromContext(ctx)

	parsedResourceID := sCtx.ResourceID.Truncate()
	resource, _, err := ctrl.GetResource(ctx, parsedResourceID)
	if err != nil {
		return nil, err
	}

	if resource == nil {
		return rest.NewNotFoundResponse(sCtx.ResourceID), nil
	}

	secrets, err := ctrl.dp.FetchSecrets(ctx,
		deployment.ResourceData{
			ID:              sCtx.ResourceID,
			Resource:        resource,
			OutputResources: resource.Properties.Status.OutputResources,
			ComputedValues:  resource.ComputedValues,
			SecretValues:    resource.SecretValues,
		},
	)
	if err != nil {
		return nil, err
	}

	sqlSecrets := datamodel.SqlDatabaseSecrets{}
	if password, ok := secrets[renderers.PasswordStringHolder].(string); ok {
		sqlSecrets.Password = password
	}
	if connectionString, ok := secrets[renderers.ConnectionStringValue].(string); ok {
		sqlSecrets.ConnectionString = connectionString
	}

	versioned, _ := converter.SqlDatabaseSecretsDataModelToVersioned(&sqlSecrets, sCtx.APIVersion)
	return rest.NewOKResponse(versioned), nil
}
