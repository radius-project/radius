// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package defaultoperation

import (
	"context"
	"encoding/json"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/rest"
)

var _ ctrl.Controller = (*CreateOrUpdateSubscription)(nil)

// CreateOrUpdateSubscription is the controller implementation to manage arm subscription lifecycle.
type CreateOrUpdateSubscription struct {
	ctrl.BaseController
}

// NewCreateOrUpdateSubscription creates a new CreateOrUpdateSubscription.
func NewCreateOrUpdateSubscription(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateSubscription{ctrl.NewBaseController(opts)}, nil
}

// CreateOrUpdateSubscription is triggered when the state of the user subscription is changed (setup or tear down).
// Spec: https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/subscription-lifecycle-api-reference.md#subscription-lifecycle-api-reference
func (a *CreateOrUpdateSubscription) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	// TODO: implement data store check for subscriptions
	log := radlogger.GetLogger(ctx)
	log.Info("Within Create or Update Subscription")
	sCtx := servicecontext.ARMRequestContextFromContext(ctx)
	switch sCtx.APIVersion {
	case v1.SubscriptionAPIVersion:
		return rest.NewOKResponse(a.Validate(req)), nil
	}
	log.Info("Exiting Create or Update Subscription")
	return rest.NewNotFoundAPIVersionResponse("Subscriptions", "Applications.Core", sCtx.APIVersion), nil
}

func (a *CreateOrUpdateSubscription) Validate(req *http.Request) *v1.Subscription {
	content, _ := ctrl.ReadJSONBody(req)
	am := v1.Subscription{}
	err := json.Unmarshal(content, &am)
	if err != nil {
		return nil
	}

	return &am
}
