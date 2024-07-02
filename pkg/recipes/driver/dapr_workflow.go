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

package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	daprclient "github.com/dapr/go-sdk/client"
	"github.com/go-logr/logr"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/recipecontext"
	recipes_util "github.com/radius-project/radius/pkg/recipes/util"
)

var _ Driver = (*daprWorkflowDriver)(nil)

func NewDaprWorkflowDriver(client daprclient.Client, options DaprWorkflowOptions) Driver {
	return &daprWorkflowDriver{
		client:  client,
		options: options,
	}
}

type DaprWorkflowOptions struct {
}

type daprWorkflowDriver struct {
	client  daprclient.Client
	options DaprWorkflowOptions
}

// GetRecipeMetadata implements Driver.
func (d *daprWorkflowDriver) GetRecipeMetadata(ctx context.Context, opts BaseOptions) (map[string]any, error) {
	metadata := map[string]any{
		"parameters": map[string]any{},
	}

	return metadata, nil
}

// Execute implements Driver.
func (d *daprWorkflowDriver) Execute(ctx context.Context, opts ExecuteOptions) (*recipes.RecipeOutput, error) {
	logger := logr.FromContextOrDiscard(ctx)
	logger = logger.WithValues("recipe", opts.Definition.Name, "appId", opts.Definition.AppID, "workflow", opts.Definition.PutWorkflow)
	logger.Info("Deploying Dapr workflow recipe")

	recipeContext, err := recipecontext.New(&opts.Recipe, &opts.Configuration)
	if err != nil {
		return nil, recipes.NewRecipeError(recipes.RecipeDeploymentFailed, err.Error(), recipes_util.RecipeSetupError, recipes.GetErrorDetails(err))
	}

	port, ok := os.LookupEnv("DAPR_HTTP_PORT")
	if !ok {
		port = "3500"
	}
	url := fmt.Sprintf("http://localhost:%s/workflows", port)

	request := workflowRequest{
		Name:  opts.Definition.PutWorkflow,
		Input: recipeContext,
	}

	response, err := d.startWorkflow(ctx, url, opts.Definition.AppID, &request)
	if err != nil {
		return nil, err
	}

	logger = logger.WithValues("workflow.id", response.ID)
	logger.Info("Dapr workflow started")

	result, err := d.waitForWorkflowCompletion(ctx, opts.Definition.AppID, response.ID)
	if err != nil {
		return nil, err
	}

	if result.RuntimeStatus == StatusCompleted {
		output := recipes.RecipeOutput{}
		err = json.Unmarshal([]byte(result.SerializedOutput), &output)
		if err != nil {
			return nil, err
		}

		return &output, nil
	}

	return nil, result.AsRecipeError()
}

// Delete implements Driver.
func (d *daprWorkflowDriver) Delete(ctx context.Context, opts DeleteOptions) error {
	logger := logr.FromContextOrDiscard(ctx)
	logger.WithValues("recipe", opts.Definition.Name, "appId", opts.Definition.AppID, "workflow", opts.Definition.DeleteWorkflow)
	logger.Info("Deleting Dapr workflow recipe")

	recipeContext, err := recipecontext.New(&opts.Recipe, &opts.Configuration)
	if err != nil {
		return recipes.NewRecipeError(recipes.RecipeDeploymentFailed, err.Error(), recipes_util.RecipeSetupError, recipes.GetErrorDetails(err))
	}

	port, ok := os.LookupEnv("DAPR_HTTP_PORT")
	if !ok {
		port = "3500"
	}
	url := fmt.Sprintf("http://localhost:%s/workflows", port)

	request := workflowRequest{
		Name:  opts.Definition.DeleteWorkflow,
		Input: recipeContext,
	}

	response, err := d.startWorkflow(ctx, url, opts.Definition.AppID, &request)
	if err != nil {
		return err
	}

	logger = logger.WithValues("workflow.id", response.ID)
	logger.Info("Dapr workflow started")

	result, err := d.waitForWorkflowCompletion(ctx, opts.Definition.AppID, response.ID)
	if err != nil {
		return err
	}

	if result.RuntimeStatus == StatusCompleted {
		return nil
	}

	return result.AsRecipeError()
}

func (d *daprWorkflowDriver) startWorkflow(ctx context.Context, url string, appID string, request *workflowRequest) (*workflowResponse, error) {
	bs, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	client := http.DefaultClient
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(bs))
	if err != nil {
		return nil, err
	}
	req.Header.Set("dapr-app-id", appID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode > 299 || resp.StatusCode < 200 {
		return nil, fmt.Errorf("failed to start Dapr workflow with status %d: %s", resp.StatusCode, body)
	}

	response := workflowResponse{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

func (d *daprWorkflowDriver) waitForWorkflowCompletion(ctx context.Context, appID string, id string) (*workflowMetadata, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute*5)
	defer cancel()

	port, ok := os.LookupEnv("DAPR_HTTP_PORT")
	if !ok {
		port = "3500"
	}
	url := fmt.Sprintf("http://localhost:%s/workflows/%s", port, id)

	for {
		client := http.DefaultClient
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("dapr-app-id", appID)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode > 299 || resp.StatusCode < 200 {
			return nil, fmt.Errorf("failed to poll Dapr workflow with status %d: %s", resp.StatusCode, body)
		}

		response := workflowMetadata{}
		err = json.Unmarshal(body, &response)
		if err != nil {
			return nil, err
		}

		if response.IsTerminal() {
			return &response, nil
		}

		time.Sleep(time.Second * 5)
	}
}

type workflowRequest struct {
	Name  string `json:"name"`
	Input any    `json:"input"`
	ID    string `json:"id,omitempty"`
}

type workflowResponse struct {
	ID string `json:"id"`
}

type workflowMetadata struct {
	InstanceID             string          `json:"id"`
	Name                   string          `json:"name"`
	RuntimeStatus          Status          `json:"status"`
	CreatedAt              time.Time       `json:"createdAt"`
	LastUpdatedAt          time.Time       `json:"lastUpdatedAt"`
	SerializedInput        string          `json:"serializedInput"`
	SerializedOutput       string          `json:"serializedOutput"`
	SerializedCustomStatus string          `json:"serializedCustomStatus"`
	FailureDetails         *FailureDetails `json:"failureDetails"`
}

type FailureDetails struct {
	Type           string          `json:"type"`
	Message        string          `json:"message"`
	StackTrace     string          `json:"stackTrace"`
	InnerFailure   *FailureDetails `json:"innerFailure"`
	IsNonRetriable bool            `json:"IsNonRetriable"`
}

type Status int

const (
	StatusRunning Status = iota
	StatusCompleted
	StatusContinuedAsNew
	StatusFailed
	StatusCanceled
	StatusTerminated
	StatusPending
	StatusSuspended
	StatusUnknown
)

// String returns the runtime status as a string.
func (s Status) String() string {
	status := [...]string{
		"RUNNING",
		"COMPLETED",
		"CONTINUED_AS_NEW",
		"FAILED",
		"CANCELED",
		"TERMINATED",
		"PENDING",
		"SUSPENDED",
	}
	if s > StatusSuspended || s < StatusRunning {
		return "UNKNOWN"
	}
	return status[s]
}

func (w *workflowMetadata) IsTerminal() bool {
	switch w.RuntimeStatus {
	case StatusCompleted, StatusFailed, StatusCanceled, StatusTerminated:
		return true
	default:
		return false
	}
}

func (w *workflowMetadata) AsRecipeError() error {
	if !w.IsTerminal() {
		return nil
	}

	if w.RuntimeStatus == StatusCompleted {
		return nil
	}

	message := fmt.Sprintf("Dapr workflow completed with status %s", w.RuntimeStatus.String())
	details := v1.ErrorDetails{
		AdditionalInfo: []v1.ErrorAdditionalInfo{},
	}
	return recipes.NewRecipeError(recipes.RecipeDeploymentFailed, message, recipes_util.ExecutionError, &details)

}
