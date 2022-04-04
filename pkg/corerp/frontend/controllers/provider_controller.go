// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"

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
func (ctrl *ProviderController) CreateOrUpdateSubscription(ctx context.Context, req *http.Request) (rest.Response, error) {
	// TODO: implement!
	reqBody, _ := ioutil.ReadAll(req.Body)
	var data map[string]interface{}
	err := json.Unmarshal([]byte(reqBody), &data)
	if err != nil {
		return nil, err
	}
	return rest.NewOKResponse(data), nil
}

// TODO: Add preflight, admin related handlers.
