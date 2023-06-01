/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
//
// # Function Explanation
// 
//	CreateOrUpdateSubscription creates a new controller and returns it, or returns an error if one occurs.
func NewCreateOrUpdateSubscription(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateSubscription{ctrl.NewBaseController(opts)}, nil
}

// CreateOrUpdateSubscription is triggered when the state of the user subscription is changed (setup or tear down).
// Spec: https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/subscription-lifecycle-api-reference.md#subscription-lifecycle-api-reference
//
// # Function Explanation
// 
//	CreateOrUpdateSubscription runs a check against a data store to determine if a subscription exists and, if so, validates
//	 it. If the API version is not found, a NotFoundAPIVersionResponse is returned.
func (a *CreateOrUpdateSubscription) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	// TODO: implement data store check for subscriptions
	log := ucplog.FromContextOrDiscard(ctx)
	log.Info("Within Create or Update Subscription")
	sCtx := v1.ARMRequestContextFromContext(ctx)
	switch sCtx.APIVersion {
	case v1.SubscriptionAPIVersion:
		return rest.NewOKResponse(a.Validate(req)), nil
	}
	log.Info("Exiting Create or Update Subscription")
	return rest.NewNotFoundAPIVersionResponse("Subscriptions", "Applications.Core", sCtx.APIVersion), nil
}

// # Function Explanation
// 
//	CreateOrUpdateSubscription reads the body of the request and attempts to unmarshal it into a Subscription object. If 
//	successful, it returns the Subscription object, otherwise it returns nil. Error handling is done by returning nil if an 
//	error occurs.
func (a *CreateOrUpdateSubscription) Validate(req *http.Request) *v1.Subscription {
	content, _ := ctrl.ReadJSONBody(req)
	am := v1.Subscription{}
	err := json.Unmarshal(content, &am)
	if err != nil {
		return nil
	}

	return &am
}
