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

package embedded

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/portableresources/backend/controller"
	"github.com/radius-project/radius/pkg/portableresources/processors"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	"github.com/radius-project/radius/pkg/recipes/engine"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/frontend/controller/resourcegroups"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

var _ ctrl.Controller = (*Controller)(nil)

// Controller is the async operation controller to perform background processing on tracked resources.
type Controller struct {
	ctrl.BaseController

	opts         ctrl.Options
	engine       engine.Engine
	client       processors.ResourceClient
	configLoader configloader.ConfigurationLoader
}

// NewController creates a new Controller controller which is used to process resources asynchronously.
func NewController(opts ctrl.Options, engine engine.Engine, client processors.ResourceClient, configLoader configloader.ConfigurationLoader) (ctrl.Controller, error) {
	return &Controller{
		BaseController: ctrl.NewBaseAsyncController(opts),

		opts:         opts,
		engine:       engine,
		client:       client,
		configLoader: configLoader,
	}, nil
}

// Run implements the async operation controller to process resources asynchronously.
func (c *Controller) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	id, err := resources.ParseResource(request.ResourceID)
	if err != nil {
		return ctrl.Result{}, err
	}

	provider, err := resourcegroups.ValidateResourceProvider(ctx, c.StorageClient(), id)
	if errors.Is(err, &resourcegroups.NotFoundError{}) {
		e := v1.ErrorDetails{
			Code:    v1.CodeInvalid,
			Message: fmt.Sprintf("Resource provider %q was not found.", id.ProviderNamespace()),
			Target:  request.ResourceID,
		}
		return ctrl.NewFailedResult(e), nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	resourceType, _, _, err := resourcegroups.ValidateResourceType(id, v1.LocationGlobal, provider)
	if errors.Is(err, &resourcegroups.NotFoundError{}) {
		e := v1.ErrorDetails{
			Code:    v1.CodeInvalid,
			Message: fmt.Sprintf("Resource type %q was not found.", id.Type()),
			Target:  request.ResourceID,
		}
		return ctrl.NewFailedResult(e), nil
	}

	operationType, _ := v1.ParseOperationType(request.OperationType)
	switch operationType.Method {
	case http.MethodPut:
		return c.processPut(ctx, request, provider, resourceType)
	case http.MethodDelete:
		return c.processDelete(ctx, request, provider, resourceType)
	default:
		e := v1.ErrorDetails{
			Code:    v1.CodeInvalid,
			Message: fmt.Sprintf("Invalid operation type: %q", operationType),
			Target:  request.ResourceID,
		}
		return ctrl.NewFailedResult(e), nil
	}
}

func (c *Controller) processDelete(ctx context.Context, request *ctrl.Request, provider *datamodel.ResourceProvider, resourceType *datamodel.ResourceType) (ctrl.Result, error) {
	p := &dynamicProcessor{
		APIVersion:       request.APIVersion,
		ResourceProvider: provider,
		ResourceType:     resourceType,
	}

	inner, err := controller.NewDeleteResource(c.opts, p, c.engine, c.configLoader)
	if err != nil {
		return ctrl.Result{}, err
	}

	result, err := inner.Run(ctx, request)
	if err != nil {
		return ctrl.Result{}, err
	}

	return result, nil
}

func (c *Controller) processPut(ctx context.Context, request *ctrl.Request, provider *datamodel.ResourceProvider, resourceType *datamodel.ResourceType) (ctrl.Result, error) {
	p := &dynamicProcessor{
		APIVersion:       request.APIVersion,
		ResourceProvider: provider,
		ResourceType:     resourceType,
	}

	inner, err := controller.NewCreateOrUpdateResource(c.opts, p, c.engine, c.client, c.configLoader)
	if err != nil {
		return ctrl.Result{}, err
	}

	result, err := inner.Run(ctx, request)
	if err != nil {
		return ctrl.Result{}, err
	}

	return result, nil
}

var _ processors.ResourceProcessor[*datamodel.DynamicResource, datamodel.DynamicResource] = (*dynamicProcessor)(nil)

type dynamicProcessor struct {
	ResourceProvider *datamodel.ResourceProvider
	ResourceType     *datamodel.ResourceType
	APIVersion       string
}

func (d *dynamicProcessor) Delete(ctx context.Context, resource *datamodel.DynamicResource, options processors.Options) error {
	return nil
}

func (d *dynamicProcessor) Process(ctx context.Context, resource *datamodel.DynamicResource, options processors.Options) error {
	computedValues := map[string]any{}
	secretValues := map[string]rpv1.SecretValueReference{}
	outputResources := []rpv1.OutputResource{}
	status := rpv1.RecipeStatus{}

	validator := processors.NewValidator(&computedValues, &secretValues, &outputResources, &status)

	// TODO: loop over schema and add to validator - right now this bypasses validation.
	for key, value := range options.RecipeOutput.Values {
		value := value
		validator.AddOptionalAnyField(key, &value)
	}
	for key, value := range options.RecipeOutput.Secrets {
		value := value.(string)
		validator.AddOptionalSecretField(key, &value)
	}

	err := validator.SetAndValidate(options.RecipeOutput)
	if err != nil {
		return err
	}

	err = resource.ApplyDeploymentOutput(rpv1.DeploymentOutput{DeployedOutputResources: outputResources, ComputedValues: computedValues, SecretValues: secretValues})
	if err != nil {
		return err
	}

	resource.SetRecipeStatus(status)

	return nil
}
