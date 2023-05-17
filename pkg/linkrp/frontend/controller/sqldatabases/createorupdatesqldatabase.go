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
	rp_frontend "github.com/project-radius/radius/pkg/rp/frontend"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

var _ ctrl.Controller = (*CreateOrUpdateSqlDatabase)(nil)

// CreateOrUpdateSqlDatabase is the controller implementation to create or update SqlDatabase link resource.
type CreateOrUpdateSqlDatabase struct {
	ctrl.Operation[*datamodel.SqlDatabase, datamodel.SqlDatabase]
	dp deployment.DeploymentProcessor
}

// NewCreateOrUpdateSqlDatabase creates a new instance of CreateOrUpdateSqlDatabase.
func NewCreateOrUpdateSqlDatabase(opts frontend_ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateSqlDatabase{
		Operation: ctrl.NewOperation(opts.Options,
			ctrl.ResourceOptions[datamodel.SqlDatabase]{
				RequestConverter:  converter.SqlDatabaseDataModelFromVersioned,
				ResponseConverter: converter.SqlDatabaseDataModelToVersioned,
			}),
		dp: opts.DeployProcessor,
	}, nil
}

// Run executes CreateOrUpdateSqlDatabase operation.
func (sqlDatabase *CreateOrUpdateSqlDatabase) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	newResource, err := sqlDatabase.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	old, etag, err := sqlDatabase.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	r, err := sqlDatabase.PrepareResource(ctx, req, newResource, old, etag)
	if r != nil || err != nil {
		return r, err
	}

	r, err = rp_frontend.PrepareRadiusResource(ctx, newResource, old, sqlDatabase.Options())
	if r != nil || err != nil {
		return r, err
	}

	rendererOutput, err := sqlDatabase.dp.Render(ctx, serviceCtx.ResourceID, newResource)
	if err != nil {
		return nil, err
	}

	deploymentOutput, err := sqlDatabase.dp.Deploy(ctx, serviceCtx.ResourceID, rendererOutput)
	if err != nil {
		return nil, err
	}

	newResource.Properties.Status.OutputResources = deploymentOutput.DeployedOutputResources
	newResource.ComputedValues = deploymentOutput.ComputedValues
	newResource.SecretValues = deploymentOutput.SecretValues
	if server, ok := deploymentOutput.ComputedValues["server"].(string); ok {
		newResource.Properties.Server = server
	}
	if database, ok := deploymentOutput.ComputedValues["database"].(string); ok {
		newResource.Properties.Database = database
	}

	if old != nil {
		diff := rpv1.GetGCOutputResources(newResource.Properties.Status.OutputResources, old.Properties.Status.OutputResources)
		err = sqlDatabase.dp.Delete(ctx, serviceCtx.ResourceID, diff)
		if err != nil {
			return nil, err
		}
	}

	newResource.SetProvisioningState(v1.ProvisioningStateSucceeded)
	newEtag, err := sqlDatabase.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	return sqlDatabase.ConstructSyncResponse(ctx, req.Method, newEtag, newResource)
}
