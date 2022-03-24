// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"context"

	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/db"
	"github.com/project-radius/radius/pkg/radrp/rest"
)

// ProviderController implements the system level apis.
type ProviderController struct {
	BaseController
}

// NewProviderController returns new instance of ProviderController.
func NewProviderController(db db.RadrpDB, deploy deployment.DeploymentProcessor, completions chan<- struct{}, scheme string) *ProviderController {
	return &ProviderController{
		BaseController: BaseController{
			db:          db,
			deploy:      deploy,
			completions: completions,
			scheme:      scheme,
		},
	}
}

// CreateOrUpdateSubscription is triggered when the state of the user subscription is changed (setup or tear down).
// Spec: https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/subscription-lifecycle-api-reference.md#subscription-lifecycle-api-reference
// PUT	https://<registered-resource-provider-endpoint>/subscriptions/{subscriptionId}?api-version=2.0
func (ctrl *ProviderController) CreateOrUpdateSubscription(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	// TODO: implement!
	return nil, nil
}

// TODO: Add preflight, admin related handlers.
