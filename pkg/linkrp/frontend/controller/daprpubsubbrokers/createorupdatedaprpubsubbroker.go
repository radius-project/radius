// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubbrokers

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
	"github.com/project-radius/radius/pkg/linkrp/renderers/daprpubsubbrokers"
	rp_frontend "github.com/project-radius/radius/pkg/rp/frontend"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	kube "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ ctrl.Controller = (*CreateOrUpdateDaprPubSubBroker)(nil)

// CreateOrUpdateDaprPubSubBroker is the controller implementation to create or update DaprPubSubBroker link resource.
type CreateOrUpdateDaprPubSubBroker struct {
	ctrl.Operation[*datamodel.DaprPubSubBroker, datamodel.DaprPubSubBroker]
	KubeClient kube.Client
	dp         deployment.DeploymentProcessor
}

// NewCreateOrUpdateDaprPubSubBroker creates a new instance of CreateOrUpdateDaprPubSubBroker.
func NewCreateOrUpdateDaprPubSubBroker(opts frontend_ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateDaprPubSubBroker{
		Operation: ctrl.NewOperation(opts.Options,
			ctrl.ResourceOptions[datamodel.DaprPubSubBroker]{
				RequestConverter:  converter.DaprPubSubBrokerDataModelFromVersioned,
				ResponseConverter: converter.DaprPubSubBrokerDataModelToVersioned,
			}),
		KubeClient: opts.KubeClient,
		dp:         opts.DeployProcessor,
	}, nil
}

// Run executes CreateOrUpdateDaprPubSubBroker operation.
func (daprPubSubBroker *CreateOrUpdateDaprPubSubBroker) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	isSupported, err := datamodel.IsDaprInstalled(ctx, daprPubSubBroker.KubeClient)
	if err != nil {
		return nil, err
	} else if !isSupported {
		return rest.NewBadRequestResponse(datamodel.DaprMissingError), nil
	}

	newResource, err := daprPubSubBroker.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	old, etag, err := daprPubSubBroker.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	r, err := daprPubSubBroker.PrepareResource(ctx, req, newResource, old, etag)
	if r != nil || err != nil {
		return r, err
	}

	r, err = rp_frontend.PrepareRadiusResource(ctx, newResource, old, daprPubSubBroker.Options())
	if r != nil || err != nil {
		return r, err
	}

	rendererOutput, err := daprPubSubBroker.dp.Render(ctx, serviceCtx.ResourceID, newResource)
	if err != nil {
		return nil, err
	}

	deploymentOutput, err := daprPubSubBroker.dp.Deploy(ctx, serviceCtx.ResourceID, rendererOutput)
	if err != nil {
		return nil, err
	}

	newResource.Properties.Status.OutputResources = deploymentOutput.Resources
	newResource.ComputedValues = deploymentOutput.ComputedValues
	newResource.SecretValues = deploymentOutput.SecretValues
	if topic, ok := deploymentOutput.ComputedValues[daprpubsubbrokers.TopicNameKey].(string); ok {
		newResource.Properties.Topic = topic
	}

	if componentName, ok := deploymentOutput.ComputedValues[renderers.ComponentNameKey].(string); ok {
		newResource.Properties.ComponentName = componentName
	}

	if old != nil {
		diff := outputresource.GetGCOutputResources(newResource.Properties.Status.OutputResources, old.Properties.Status.OutputResources)
		err = daprPubSubBroker.dp.Delete(ctx, deployment.ResourceData{ID: serviceCtx.ResourceID, Resource: newResource, OutputResources: diff, ComputedValues: newResource.ComputedValues, SecretValues: newResource.SecretValues, RecipeData: newResource.RecipeData})
		if err != nil {
			return nil, err
		}
	}

	newResource.SetProvisioningState(v1.ProvisioningStateSucceeded)
	newEtag, err := daprPubSubBroker.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	return daprPubSubBroker.ConstructSyncResponse(ctx, req.Method, newEtag, newResource)
}
