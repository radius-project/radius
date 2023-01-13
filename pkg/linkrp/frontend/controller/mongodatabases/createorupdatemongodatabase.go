// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodatabases

import (
	"context"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel/converter"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/linkrp/renderers"

	"github.com/project-radius/radius/pkg/armrpc/rest"
	fctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller"
	rp_frontend "github.com/project-radius/radius/pkg/rp/frontend"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ ctrl.Controller = (*CreateOrUpdateMongoDatabase)(nil)

// CreateOrUpdateMongoDatabase is the controller implementation to create or update MongoDatabase link resource.
type CreateOrUpdateMongoDatabase struct {
	ctrl.Operation[*datamodel.MongoDatabase, datamodel.MongoDatabase]

	KubeClient runtimeclient.Client
	dp         deployment.DeploymentProcessor
}

// NewCreateOrUpdateMongoDatabase creates a new instance of CreateOrUpdateMongoDatabase.
func NewCreateOrUpdateMongoDatabase(opts fctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateMongoDatabase{
		Operation: ctrl.NewOperation(opts.Options,
			ctrl.ResourceOptions[datamodel.MongoDatabase]{
				RequestConverter:  converter.MongoDatabaseDataModelFromVersioned,
				ResponseConverter: converter.MongoDatabaseDataModelToVersioned,
			}),
		KubeClient: opts.KubeClient,
		dp:         opts.DeployProcessor,
	}, nil
}

// Run executes CreateOrUpdateMongoDatabase operation.
func (m *CreateOrUpdateMongoDatabase) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	newResource, err := m.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	old, etag, err := m.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if r, err := m.PrepareResource(ctx, req, newResource, old, etag); r != nil || err != nil {
		return r, err
	}

	if r, err := rp_frontend.PrepareRadiusResource(ctx, newResource, old, m.Options()); r != nil || err != nil {
		return r, err
	}

	rendererOutput, err := m.dp.Render(ctx, serviceCtx.ResourceID, newResource)
	if err != nil {
		return nil, err
	}
	deploymentOutput, err := m.dp.Deploy(ctx, serviceCtx.ResourceID, rendererOutput)
	if err != nil {
		return nil, err
	}

	newResource.Properties.BasicResourceProperties.Status.OutputResources = deploymentOutput.Resources
	newResource.ComputedValues = deploymentOutput.ComputedValues
	newResource.SecretValues = deploymentOutput.SecretValues
	newResource.RecipeData = deploymentOutput.RecipeData

	if database, ok := deploymentOutput.ComputedValues[renderers.DatabaseNameValue].(string); ok {
		newResource.Properties.Database = database
	}

	if old != nil {
		diff := outputresource.GetGCOutputResources(newResource.Properties.Status.OutputResources, old.Properties.Status.OutputResources)
		err = m.dp.Delete(ctx, deployment.ResourceData{ID: serviceCtx.ResourceID, Resource: newResource, OutputResources: diff, ComputedValues: newResource.ComputedValues, SecretValues: newResource.SecretValues, RecipeData: newResource.RecipeData})
		if err != nil {
			return nil, err
		}
	}

	newResource.SetProvisioningState(v1.ProvisioningStateSucceeded)
	newEtag, err := m.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	return m.ConstructSyncResponse(ctx, req.Method, newEtag, newResource)
}
