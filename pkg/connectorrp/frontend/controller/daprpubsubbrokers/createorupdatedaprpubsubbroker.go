// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubbrokers

import (
	"context"
	"errors"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel/converter"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*CreateOrUpdateDaprPubSubBroker)(nil)

// CreateOrUpdateDaprPubSubBroker is the controller implementation to create or update DaprPubSubBroker connector resource.
type CreateOrUpdateDaprPubSubBroker struct {
	ctrl.BaseController
}

// NewCreateOrUpdateDaprPubSubBroker creates a new instance of CreateOrUpdateDaprPubSubBroker.
func NewCreateOrUpdateDaprPubSubBroker(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateDaprPubSubBroker{ctrl.NewBaseController(opts)}, nil
}

// Run executes CreateOrUpdateDaprPubSubBroker operation.
func (daprPubSub *CreateOrUpdateDaprPubSubBroker) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
	newResource, err := daprPubSub.Validate(ctx, req, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	rendererOutput, err := daprPubSub.DeploymentProcessor().Render(ctx, serviceCtx.ResourceID, newResource)
	if err != nil {
		return nil, err
	}
	deploymentOutput, err := daprPubSub.DeploymentProcessor().Deploy(ctx, serviceCtx.ResourceID, rendererOutput)
	if err != nil {
		return nil, err
	}

	newResource.Properties.BasicResourceProperties.Status.OutputResources = deploymentOutput.Resources
	newResource.InternalMetadata.ComputedValues = deploymentOutput.ComputedValues
	newResource.InternalMetadata.SecretValues = deploymentOutput.SecretValues
	if topic, ok := deploymentOutput.ComputedValues["topic"].(string); ok {
		newResource.Properties.Topic = topic
	}

	// Read existing resource info from the data store
	existingResource := &datamodel.DaprPubSubBroker{}
	etag, err := daprPubSub.GetResource(ctx, serviceCtx.ResourceID.String(), existingResource)
	if err != nil && !errors.Is(&store.ErrNotFound{}, err) {
		return nil, err
	}

	err = ctrl.ValidateETag(*serviceCtx, etag)
	if err != nil {
		return rest.NewPreconditionFailedResponse(serviceCtx.ResourceID.String(), err.Error()), nil
	}

	// Add system metadata to requested resource
	newResource.SystemData = ctrl.UpdateSystemData(existingResource.SystemData, *serviceCtx.SystemData())
	if existingResource.CreatedAPIVersion != "" {
		newResource.CreatedAPIVersion = existingResource.CreatedAPIVersion
	}
	newResource.TenantID = serviceCtx.HomeTenantID

	// Add/update resource in the data store
	savedResource, err := daprPubSub.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	versioned, err := converter.DaprPubSubBrokerDataModelToVersioned(newResource, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{"ETag": savedResource.ETag}

	return rest.NewOKResponseWithHeaders(versioned, headers), nil
}

// Validate extracts versioned resource from request and validates the properties.
func (daprPubSub *CreateOrUpdateDaprPubSubBroker) Validate(ctx context.Context, req *http.Request, apiVersion string) (*datamodel.DaprPubSubBroker, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
	content, err := ctrl.ReadJSONBody(req)
	if err != nil {
		return nil, err
	}

	dm, err := converter.DaprPubSubBrokerDataModelFromVersioned(content, apiVersion)
	if err != nil {
		return nil, err
	}

	dm.ID = serviceCtx.ResourceID.String()
	dm.TrackedResource = ctrl.BuildTrackedResource(ctx)
	dm.Properties.ProvisioningState = v1.ProvisioningStateSucceeded
	return dm, nil
}
