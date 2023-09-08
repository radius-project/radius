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

package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/stretchr/testify/require"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/portableresources"
	"github.com/radius-project/radius/pkg/portableresources/datamodel"
	"github.com/radius-project/radius/pkg/portableresources/processors"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	"github.com/radius-project/radius/pkg/recipes/engine"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/store"
)

const (
	TestResourceType  = "Applications.Test/testResources"
	TestEnvironmentID = "/planes/radius/local/resourceGroups/radius-test-rg/providers/Applications.Core/environments/test-env"
	TestApplicationID = "/planes/radius/local/resourceGroups/radius-test-rg/providers/Applications.Core/applications/test-app"
	TestResourceID    = "/planes/radius/local/resourceGroups/radius-test-rg/providers/Applications.Test/testResources/tr"
)

type TestResource struct {
	v1.BaseResource

	// LinkMetadata represents internal DataModel properties common to all portable resource types.
	datamodel.LinkMetadata

	// Properties is the properties of the resource.
	Properties TestResourceProperties `json:"properties"`
}

// ApplyDeploymentOutput updates the status of the TestResource instance with the DeploymentOutput values.
func (r *TestResource) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	r.Properties.Status.OutputResources = do.DeployedOutputResources
	r.ComputedValues = do.ComputedValues
	r.SecretValues = do.SecretValues
	return nil
}

// OutputResources returns the OutputResources from the Status field of the Properties field of the TestResource instance.
func (r *TestResource) OutputResources() []rpv1.OutputResource {
	return r.Properties.Status.OutputResources
}

// ResourceMetadata returns the BasicResourceProperties of the TestResource instance.
func (r *TestResource) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

// Recipe returns a pointer to the LinkRecipe stored in the Properties field of the TestResource struct.
func (t *TestResource) Recipe() *portableresources.LinkRecipe {
	return &t.Properties.Recipe
}

type TestResourceProperties struct {
	rpv1.BasicResourceProperties
	IsProcessed bool                         `json:"isProcessed"`
	Recipe      portableresources.LinkRecipe `json:"recipe,omitempty"`
}

type SuccessProcessor struct {
}

// Process sets a computed value and adds an output resource to the TestResource object, and returns no error.
func (p *SuccessProcessor) Process(ctx context.Context, data *TestResource, options processors.Options) error {
	// Simulate setting a computed value and adding an output resource.
	data.Properties.IsProcessed = true
	data.Properties.Status.OutputResources = []rpv1.OutputResource{
		newOutputResource,
	}
	return nil
}

var successProcessorReference = processors.ResourceProcessor[*TestResource, TestResource](&SuccessProcessor{})

type ErrorProcessor struct {
}

// Process always returns a processorErr.
func (p *ErrorProcessor) Process(ctx context.Context, data *TestResource, options processors.Options) error {
	return processorErr
}

var errorProcessorReference = processors.ResourceProcessor[*TestResource, TestResource](&ErrorProcessor{})
var processorErr = errors.New("processor error")
var configurationErr = errors.New("configuration error")

var oldOutputResourceResourceID = "/subscriptions/test-sub/resourceGroups/test-rg/providers/System.Test/testResources/test1"

var newOutputResourceResourceID = "/subscriptions/test-sub/resourceGroups/test-rg/providers/System.Test/testResources/test2"
var newOutputResource = rpv1.OutputResource{ID: resources.MustParse(newOutputResourceResourceID)}

func TestCreateOrUpdateResource_Run(t *testing.T) {
	setupTest := func(tb testing.TB) (*store.MockStorageClient, *engine.MockEngine, *processors.MockResourceClient, *configloader.MockConfigurationLoader) {
		mctrl := gomock.NewController(t)

		msc := store.NewMockStorageClient(mctrl)
		eng := engine.NewMockEngine(mctrl)
		cfg := configloader.NewMockConfigurationLoader(mctrl)
		client := processors.NewMockResourceClient(mctrl)
		return msc, eng, client, cfg
	}

	cases := []struct {
		description             string
		factory                 func(eng engine.Engine, client processors.ResourceClient, cfg configloader.ConfigurationLoader, options ctrl.Options) (ctrl.Controller, error)
		getErr                  error
		conversionFailure       bool
		recipeErr               error
		runtimeConfigurationErr error
		processorErr            error
		resourceClientErr       error
		saveErr                 error
		expectedErr             error
	}{
		{
			"get-not-found",
			func(eng engine.Engine, client processors.ResourceClient, cfg configloader.ConfigurationLoader, options ctrl.Options) (ctrl.Controller, error) {
				return NewCreateOrUpdateResource(errorProcessorReference, eng, client, cfg, options)
			},
			&store.ErrNotFound{ID: TestResourceID},
			false,
			nil,
			nil,
			nil,
			nil,
			nil,
			&store.ErrNotFound{ID: TestResourceID},
		},
		{
			"get-error",
			func(eng engine.Engine, client processors.ResourceClient, cfg configloader.ConfigurationLoader, options ctrl.Options) (ctrl.Controller, error) {
				return NewCreateOrUpdateResource(errorProcessorReference, eng, client, cfg, options)
			},
			&store.ErrInvalid{},
			false,
			nil,
			nil,
			nil,
			nil,
			nil,
			&store.ErrInvalid{},
		},
		{
			"conversion-failure",
			func(eng engine.Engine, client processors.ResourceClient, cfg configloader.ConfigurationLoader, options ctrl.Options) (ctrl.Controller, error) {
				return NewCreateOrUpdateResource(errorProcessorReference, eng, client, cfg, options)
			},
			nil,
			true,
			nil,
			nil,
			nil,
			nil,
			nil,
			&mapstructure.Error{Errors: []string{"'type' expected type 'string', got unconvertible type 'int', value: '3'"}},
		},
		{
			"recipe-err",
			func(eng engine.Engine, client processors.ResourceClient, cfg configloader.ConfigurationLoader, options ctrl.Options) (ctrl.Controller, error) {
				return NewCreateOrUpdateResource(errorProcessorReference, eng, client, cfg, options)
			},
			nil,
			false,
			&recipes.ErrRecipeNotFound{},
			nil,
			nil,
			nil,
			nil,
			&recipes.ErrRecipeNotFound{},
		},
		{
			"runtime-configuration-err",
			func(eng engine.Engine, client processors.ResourceClient, cfg configloader.ConfigurationLoader, options ctrl.Options) (ctrl.Controller, error) {
				return NewCreateOrUpdateResource(errorProcessorReference, eng, client, cfg, options)
			},
			nil,
			false,
			nil,
			configurationErr,
			nil,
			nil,
			nil,
			configurationErr,
		},
		{
			"processor-err",
			func(eng engine.Engine, client processors.ResourceClient, cfg configloader.ConfigurationLoader, options ctrl.Options) (ctrl.Controller, error) {
				return NewCreateOrUpdateResource(errorProcessorReference, eng, client, cfg, options)
			},
			nil,
			false,
			nil,
			nil,
			processorErr,
			nil,
			nil,
			processorErr,
		},
		{
			"save-err",
			func(eng engine.Engine, client processors.ResourceClient, cfg configloader.ConfigurationLoader, options ctrl.Options) (ctrl.Controller, error) {
				return NewCreateOrUpdateResource(successProcessorReference, eng, client, cfg, options)
			},
			nil,
			false,
			nil,
			nil,
			nil,
			nil,
			errors.New("resource save failed"),
			errors.New("resource save failed"),
		},
		{
			"success",
			func(eng engine.Engine, client processors.ResourceClient, cfg configloader.ConfigurationLoader, options ctrl.Options) (ctrl.Controller, error) {
				return NewCreateOrUpdateResource(successProcessorReference, eng, client, cfg, options)
			},
			nil,
			false,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		},
	}

	for _, tt := range cases {
		t.Run(tt.description, func(t *testing.T) {
			msc, eng, client, cfg := setupTest(t)

			req := &ctrl.Request{
				OperationID:      uuid.New(),
				OperationType:    "APPLICATIONS.TEST/TESTRESOURCES|PUT", // Operation does not affect the behavior of the controller.
				ResourceID:       TestResourceID,
				CorrelationID:    uuid.NewString(),
				OperationTimeout: &ctrl.DefaultAsyncOperationTimeout,
			}

			data := map[string]any{
				"name":     "tr",
				"type":     "Applications.Test/testResources",
				"id":       TestResourceID,
				"location": v1.LocationGlobal,
				"properties": map[string]any{
					"application":       TestApplicationID,
					"environment":       TestEnvironmentID,
					"provisioningState": "Accepted",
					"status": map[string]any{
						"outputResources": []map[string]any{
							{
								"id": oldOutputResourceResourceID,
							},
						},
					},
					"recipe": map[string]any{
						"name": "test-recipe",
						"parameters": map[string]any{
							"p1": "v1",
						},
					},
				},
			}

			// Note: this test walks through the mock setup in same order as the controller
			// performs these steps. That makes it easier to reason about what to configure
			// for the desired test case.
			//
			// This flag is used to track whether the "current" test will reach the current
			// control flow.
			stillPassing := true

			if stillPassing && tt.getErr != nil {
				stillPassing = false
				msc.EXPECT().
					Get(gomock.Any(), TestResourceID).
					Return(&store.Object{Data: nil}, tt.getErr).
					Times(1)
			} else if stillPassing {
				msc.EXPECT().
					Get(gomock.Any(), TestResourceID).
					Return(&store.Object{Data: data}, nil).
					Times(1)
			}

			if tt.conversionFailure {
				stillPassing = false
				data["type"] = 3 // This won't convert to our data model.
			}

			recipeMetadata := recipes.ResourceMetadata{
				Name:          "test-recipe",
				EnvironmentID: TestEnvironmentID,
				ApplicationID: TestApplicationID,
				ResourceID:    TestResourceID,
				Parameters: map[string]any{
					"p1": "v1",
				},
			}
			prevState := []string{
				oldOutputResourceResourceID,
			}

			if stillPassing && tt.recipeErr != nil {
				stillPassing = false
				eng.EXPECT().
					Execute(gomock.Any(), engine.ExecuteOptions{
						BaseOptions: engine.BaseOptions{
							Recipe: recipeMetadata,
						},
						PreviousState: prevState,
					}).
					Return(&recipes.RecipeOutput{}, tt.recipeErr).
					Times(1)
			} else if stillPassing {
				eng.EXPECT().
					Execute(gomock.Any(), engine.ExecuteOptions{
						BaseOptions: engine.BaseOptions{
							Recipe: recipeMetadata,
						},
						PreviousState: prevState,
					}).
					Return(&recipes.RecipeOutput{}, nil).
					Times(1)
			}

			if stillPassing && tt.runtimeConfigurationErr != nil {
				stillPassing = false
				cfg.EXPECT().
					LoadConfiguration(gomock.Any(), recipes.ResourceMetadata{
						EnvironmentID: TestEnvironmentID,
						ApplicationID: TestApplicationID,
						ResourceID:    TestResourceID,
					}).
					Return(nil, tt.runtimeConfigurationErr).
					Times(1)
			} else if stillPassing {
				configuration := &recipes.Configuration{
					Runtime: recipes.RuntimeConfiguration{
						Kubernetes: &recipes.KubernetesRuntime{
							Namespace:            "test-namespace",
							EnvironmentNamespace: "test-env-namespace",
						},
					},
				}
				cfg.EXPECT().
					LoadConfiguration(gomock.Any(), recipes.ResourceMetadata{
						EnvironmentID: TestEnvironmentID,
						ApplicationID: TestApplicationID,
						ResourceID:    TestResourceID,
					}).
					Return(configuration, nil).
					Times(1)
			}

			// No mock for the processor...
			if stillPassing && tt.processorErr != nil {
				stillPassing = false
			}

			if stillPassing && tt.saveErr != nil {
				stillPassing = false
				msc.EXPECT().
					Save(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(tt.saveErr).
					Times(1)
			} else if stillPassing && tt.resourceClientErr == nil {
				msc.EXPECT().
					Save(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).
					Times(1)
			}

			opts := ctrl.Options{
				StorageClient: msc,
			}

			genCtrl, err := tt.factory(eng, client, cfg, opts)
			require.NoError(t, err)

			res, err := genCtrl.Run(context.Background(), req)
			if tt.expectedErr != nil {
				require.False(t, stillPassing)
				require.Error(t, err)
				require.Equal(t, tt.expectedErr, err)
			} else {
				require.True(t, stillPassing)
				require.NoError(t, err)
				require.Equal(t, ctrl.Result{}, res)
			}
		})
	}
}
