// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/project-radius/radius/pkg/api/armrpcv1"
	"github.com/project-radius/radius/pkg/corerp/frontend/controller"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/store"
)

var _ controller.ControllerInterface = (*CreateOrUpdateSubscription)(nil)

// CreateOrUpdateSubscription is the controller implementation to manage resource provider subscription registration.
type CreateOrUpdateSubscription struct {
	controller.BaseController
}

// NewCreateOrUpdateSubscription creates a new instance of CreateOrUpdateSubscription.
func NewCreateOrUpdateSubscription(storageClient store.StorageClient, jobEngine deployment.DeploymentProcessor) (controller.ControllerInterface, error) {
	return &CreateOrUpdateSubscription{
		BaseController: controller.BaseController{
			DBClient:  storageClient,
			JobEngine: jobEngine,
		},
	}, nil
}

// CreateOrUpdateSubscription is triggered when the state of the user subscription resource provider registration is changed (setup or tear down).
// Spec: https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/subscription-lifecycle-api-reference.md#subscription-lifecycle-api-reference
func (subscriptionCtrl *CreateOrUpdateSubscription) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	log := radlogger.GetLogger(ctx)
	log.Info("In CreateOrUpdateSubscription")
	sCtx := servicecontext.ARMRequestContextFromContext(ctx)
	switch sCtx.APIVersion {
	case armrpcv1.SubscriptionAPIVersion:
		return rest.NewOKResponse(subscriptionCtrl.Validate(req)), nil
	}

	return rest.NewNotFoundAPIVersionResponse("Subscriptions", Namespace, sCtx.APIVersion), nil
}

func (subscriptionCtrl *CreateOrUpdateSubscription) Validate(req *http.Request) *armrpcv1.Subscription {
	content, _ := controller.ReadJSONBody(req)
	subscription := armrpcv1.Subscription{}
	err := json.Unmarshal(content, &subscription)
	if err != nil {
		return nil
	}

	return &subscription
}
