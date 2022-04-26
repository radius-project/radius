// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"context"
	"net/http"

	"github.com/project-radius/radius/pkg/corerp/frontend/controller"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/db"
	"github.com/project-radius/radius/pkg/radrp/rest"
)

var _ controller.ControllerInterface = (*CreateOrUpdateSubscriptionConnector)(nil)

// CreateOrUpdateSubscriptionConnector controller implementation to manage arm subscription lifecycle
type CreateOrUpdateSubscriptionConnector struct {
	controller.BaseController
}

func NewCreateOrUpdateSubscriptionConnector(db db.RadrpDB, jobEngine deployment.DeploymentProcessor) (*CreateOrUpdateSubscriptionConnector, error) {
	return &CreateOrUpdateSubscriptionConnector{
		BaseController: controller.BaseController{
			DBProvider: db,
			JobEngine:  jobEngine,
		},
	}, nil
}

// Run this is triggered when the state of the user subscription is changed (setup or tear down)
// Spec: https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/subscription-lifecycle-api-reference.md#subscription-lifecycle-api-reference
func (a *CreateOrUpdateSubscriptionConnector) Run(ctx context.Context, req *http.Request) (rest.Response, error) {

	// TODO: placeholder for now, will be implemented as a part of https://github.com/project-radius/core-team/issues/147
	return nil, nil
}
