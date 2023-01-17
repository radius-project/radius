// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package extenders

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
)

var _ ctrl.Controller = (*ListSecretsExtender)(nil)

// ListSecretsExtender is the controller implementation to list secrets for the to access the connected extender resource resource id passed in the request body.
type ListSecretsExtender struct {
	ctrl.Operation[*datamodel.Extender, datamodel.Extender]
	dp deployment.DeploymentProcessor
}

// NewListSecretsExtender creates a new instance of ListSecretsExtender.
func NewListSecretsExtender(opts frontend_ctrl.Options) (ctrl.Controller, error) {
	return &ListSecretsExtender{
		Operation: ctrl.NewOperation(opts.Options,
			ctrl.ResourceOptions[datamodel.Extender]{
				RequestConverter:  converter.ExtenderDataModelFromVersioned,
				ResponseConverter: converter.ExtenderDataModelToVersioned,
			}),
		dp: opts.DeployProcessor,
	}, nil
}

// Run returns secrets values for the specified Extender resource
func (ctrl *ListSecretsExtender) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	sCtx := v1.ARMRequestContextFromContext(ctx)

	// Request route for listsecrets has name of the operation as suffix which should be removed to get the resource id.
	// route id format: subscriptions/<subscription_id>/resourceGroups/<resource_group>/providers/Applications.Link/extenders/<resource_name>/listsecrets
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

	return rest.NewOKResponse(secrets), nil
}
