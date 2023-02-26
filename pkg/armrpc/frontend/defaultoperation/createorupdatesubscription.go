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
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
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
func (a *CreateOrUpdateSubscription) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	// TODO: implement data store check for subscriptions
	log := ucplog.FromContext(ctx)
	log.Info("Within Create or Update Subscription")
	sCtx := v1.ARMRequestContextFromContext(ctx)
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
