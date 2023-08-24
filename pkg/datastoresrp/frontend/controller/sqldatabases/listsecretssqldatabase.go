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
	"github.com/project-radius/radius/pkg/datastoresrp/datamodel"
	"github.com/project-radius/radius/pkg/datastoresrp/datamodel/converter"
	"github.com/project-radius/radius/pkg/portableresources/renderers"
)

var _ ctrl.Controller = (*ListSecretsSqlDatabase)(nil)

// ListSecretsSqlDatabase is the controller implementation to list secrets for the to access the connected SQL database resource resource id passed in the request body.
type ListSecretsSqlDatabase struct {
	ctrl.Operation[*datamodel.SqlDatabase, datamodel.SqlDatabase]
}

// NewListSecretsSqlDatabase creates a new instance of ListSecretsSqlDatabase.
func NewListSecretsSqlDatabase(opts ctrl.Options) (ctrl.Controller, error) {
	return &ListSecretsSqlDatabase{
		Operation: ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.SqlDatabase]{
				RequestConverter:  converter.SqlDatabaseDataModelFromVersioned,
				ResponseConverter: converter.SqlDatabaseDataModelToVersioned,
			}),
	}, nil
}

// Run returns secrets values for the specified SQL database resource
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

	sqlSecrets := datamodel.SqlDatabaseSecrets{}
	if password, ok := resource.SecretValues[renderers.PasswordStringHolder]; ok {
		sqlSecrets.Password = password.Value
	}
	if connectionString, ok := resource.SecretValues[renderers.ConnectionStringValue]; ok {
		sqlSecrets.ConnectionString = connectionString.Value
	}

	versioned, err := converter.SqlDatabaseSecretsDataModelToVersioned(&sqlSecrets, sCtx.APIVersion)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), err
	}
	return rest.NewOKResponse(versioned), nil
}
