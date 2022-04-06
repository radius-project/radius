// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"context"
	"net/http"

	ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/db"
	"github.com/project-radius/radius/pkg/radrp/rest"
)

var _ ctrl.ControllerInterface = (*CreateOrUpdateSubscription)(nil)

// CreateOrUpdateSubscription implements the system level apis.
type CreateOrUpdateSubscription struct {
	ctrl.BaseController
}

// NewCreateOrUpdateSubscription creates a new CreateOrUpdateSubscription.
func NewCreateOrUpdateSubscription(db db.RadrpDB, jobEngine deployment.DeploymentProcessor) (*CreateOrUpdateSubscription, error) {
	return &CreateOrUpdateSubscription{
		BaseController: ctrl.BaseController{
			DBProvider: db,
			JobEngine:  jobEngine,
		},
	}, nil
}

// CreateOrUpdateSubscription is triggered when the state of the user subscription is changed (setup or tear down).
// Spec: https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/subscription-lifecycle-api-reference.md#subscription-lifecycle-api-reference
func (ctrl *CreateOrUpdateSubscription) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	// TODO: implement!
	return nil, nil
}
