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

package resourceproviders

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	resourcetypeslearn "github.com/radius-project/radius/pkg/resourcetypes/learn"
)

var _ armrpc_controller.Controller = (*ResourceTypeLearnController)(nil)

// ResourceTypeLearnController handles generation of resource type definitions from Terraform modules.
type ResourceTypeLearnController struct {
	armrpc_controller.BaseController
}

// ResourceTypeLearnRequest captures the input payload for learning resource types.
type ResourceTypeLearnRequest struct {
	GitURL    string `json:"gitUrl"`
	Namespace string `json:"namespace,omitempty"`
	TypeName  string `json:"typeName,omitempty"`
}

// ResourceTypeLearnResponse captures the generated resource type definition and metadata.
type ResourceTypeLearnResponse struct {
	Namespace         string `json:"namespace"`
	TypeName          string `json:"typeName"`
	YAML              string `json:"yaml"`
	VariableCount     int    `json:"variableCount"`
	GeneratedTypeName bool   `json:"generatedTypeName"`
	InferredNamespace bool   `json:"inferredNamespace"`
}

// NewResourceTypeLearnController creates a new controller instance for learning resource types.
func NewResourceTypeLearnController(opts armrpc_controller.Options) (armrpc_controller.Controller, error) {
	return &ResourceTypeLearnController{
		BaseController: armrpc_controller.NewBaseController(opts),
	}, nil
}

// Run executes the learn flow and returns the generated definition.
func (c *ResourceTypeLearnController) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	defer req.Body.Close()

	var payload ResourceTypeLearnRequest
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		return armrpc_rest.NewBadRequestResponse("invalid request payload"), nil
	}

	if payload.GitURL == "" {
		return armrpc_rest.NewBadRequestResponse("gitUrl is required"), nil
	}

	result, err := resourcetypeslearn.Run(ctx, resourcetypeslearn.Options{
		GitURL:    payload.GitURL,
		Namespace: payload.Namespace,
		TypeName:  payload.TypeName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to learn resource type: %w", err)
	}

	response := ResourceTypeLearnResponse{
		Namespace:         result.Namespace,
		TypeName:          result.TypeName,
		YAML:              string(result.YAML),
		VariableCount:     result.VariableCount,
		GeneratedTypeName: result.GeneratedTypeName,
		InferredNamespace: result.InferredNamespace,
	}

	return armrpc_rest.NewOKResponse(response), nil
}
