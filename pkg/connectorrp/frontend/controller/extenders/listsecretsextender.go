// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package extenders

import (
	"context"
	"errors"
	"net/http"

	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*ListSecretsExtender)(nil)

// ListSecretsExtender is the controller implementation to list secrets for the to access the connected extender resource resource id passed in the request body.
type ListSecretsExtender struct {
	ctrl.BaseController
}

// NewListSecretsExtender creates a new instance of ListSecretsExtender.
func NewListSecretsExtender(opts ctrl.Options) (ctrl.Controller, error) {
	return &ListSecretsExtender{ctrl.NewBaseController(opts)}, nil
}

// Run returns secrets values for the specified Extender resource
func (ctrl *ListSecretsExtender) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	sCtx := servicecontext.ARMRequestContextFromContext(ctx)

	resource := &datamodel.Extender{}
	// Request route for listsecrets has name of the operation as suffix which should be removed to get the resource id.
	// route id format: subscriptions/<subscription_id>/resourceGroups/<resource_group>/providers/Applications.Connector/extenders/<resource_name>/listsecrets
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

	return rest.NewOKResponse(secrets), nil
}
