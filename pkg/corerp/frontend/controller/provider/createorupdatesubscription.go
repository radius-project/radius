// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"context"
	"net/http"
	"encoding/json"

	"github.com/project-radius/radius/pkg/corerp/api/armrpcv1"
	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/db"
	"github.com/project-radius/radius/pkg/radrp/rest"
)

var _ ctrl.ControllerInterface = (*CreateOrUpdateSubscription)(nil)

// CreateOrUpdateSubscription is the controller implementation to manage arm subscription lifecycle.
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
func (a *CreateOrUpdateSubscription) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	// TODO: implement data store check for subscriptions
	log := radlogger.GetLogger(ctx)
	log.Info("Within Create or Update Subscription handler")
	sCtx := servicecontext.ARMRequestContextFromContext(ctx)
	switch sCtx.APIVersion {
	case v20220315privatepreview.Version:
		return rest.NewOKResponse(a.modifySubsciprtionV1(req)), nil
	}
	return rest.NewNotFoundAPIVersionResponse("Subscriptions", "Applications.Core", sCtx.APIVersion), nil
}

func (a *CreateOrUpdateSubscription) modifySubsciprtionV1(req *http.Request) *armrpcv1.PaginatedList {
	content, _ := ctrl.ReadJSONBody(req)
	am := armrpcv1.Subscription{}
	json.Unmarshal(content, &am) 
	
	return &armrpcv1.PaginatedList{
		Value: []interface{}{
			am,
		},
	}
}
