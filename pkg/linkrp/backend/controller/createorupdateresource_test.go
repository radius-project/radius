// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/stretchr/testify/require"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/processors"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/recipes/engine"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/store"
)

const (
	TestResourceType  = "Applications.Test/testResources"
	TestEnvironmentID = "/planes/radius/local/resourceGroups/radius-test-rg/providers/Applications.Core/environments/test-env"
	TestApplicationID = "/planes/radius/local/resourceGroups/radius-test-rg/providers/Applications.Core/applications/test-app"
	TestResourceID    = "/planes/radius/local/resourceGroups/radius-test-rg/providers/Applications.Test/testResources/tr"
)

type TestResource struct {
	v1.BaseResource

	// LinkMetadata represents internal DataModel properties common to all link types.
	datamodel.LinkMetadata

	// Properties is the properties of the resource.
	Properties TestResourceProperties `json:"properties"`
}

func (r *TestResource) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	r.Properties.Status.OutputResources = do.DeployedOutputResources
	r.ComputedValues = do.ComputedValues
	r.SecretValues = do.SecretValues

	return nil
}

func (r *TestResource) OutputResources() []rpv1.OutputResource {
	return r.Properties.Status.OutputResources
}

func (r *TestResource) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

func (t *TestResource) Recipe() *linkrp.LinkRecipe {
	return &t.Properties.Recipe
}

type TestResourceProperties struct {
	rpv1.BasicResourceProperties
	IsProcessed bool              `json:"isProcessed"`
	Recipe      linkrp.LinkRecipe `json:"recipe,omitempty"`
}

type SuccessProcessor struct {
}

func (p *SuccessProcessor) Process(ctx context.Context, data *TestResource, output *recipes.RecipeOutput) error {
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

func (p *ErrorProcessor) Process(ctx context.Context, data *TestResource, output *recipes.RecipeOutput) error {
	return processorErr
}

var errorProcessorReference = processors.ResourceProcessor[*TestResource, TestResource](&ErrorProcessor{})
var processorErr = errors.New("OH NO!")

var oldOutputResourceResourceID = "/subscriptions/test-sub/resourceGroups/test-rg/providers/System.Test/testResources/test1"
var oldOutputResource = rpv1.OutputResource{
	Identity: resourcemodel.NewARMIdentity(&resourcemodel.ResourceType{
		Type:     "System.Test/testResources",
		Provider: resourcemodel.ProviderAzure,
	}, oldOutputResourceResourceID, "2022-01-01"),
}

var newOutputResourceResourceID = "/subscriptions/test-sub/resourceGroups/test-rg/providers/System.Test/testResources/test2"
var newOutputResource = rpv1.OutputResource{
	Identity: resourcemodel.NewARMIdentity(&resourcemodel.ResourceType{
		Type:     "System.Test/testResources",
		Provider: resourcemodel.ProviderAzure,
	}, newOutputResourceResourceID, "2022-01-01"),
}

func TestCreateOrUpdateResource_Run(t *testing.T) {
	setupTest := func(tb testing.TB) (*store.MockStorageClient, *engine.MockEngine, *processors.MockResourceClient) {
		mctrl := gomock.NewController(t)

		msc := store.NewMockStorageClient(mctrl)
		eng := engine.NewMockEngine(mctrl)
		client := processors.NewMockResourceClient(mctrl)
		return msc, eng, client
	}

	cases := []struct {
		description       string
		factory           func(eng engine.Engine, client processors.ResourceClient, options ctrl.Options) (ctrl.Controller, error)
		getErr            error
		conversionFailure bool
		recipeErr         error
		processorErr      error
		resourceClientErr error
		saveErr           error
		expectedErr       error
	}{
		{
			"get-not-found",
			func(eng engine.Engine, client processors.ResourceClient, options ctrl.Options) (ctrl.Controller, error) {
				return NewCreateOrUpdateResource(successProcessorReference, eng, client, options)
			},
			&store.ErrNotFound{},
			false,
			nil,
			nil,
			nil,
			nil,
			&store.ErrNotFound{},
		},
		{
			"get-error",
			func(eng engine.Engine, client processors.ResourceClient, options ctrl.Options) (ctrl.Controller, error) {
				return NewCreateOrUpdateResource(successProcessorReference, eng, client, options)
			},
			&store.ErrInvalid{},
			false,
			nil,
			nil,
			nil,
			nil,
			&store.ErrInvalid{},
		},
		{
			"conversion-failure",
			func(eng engine.Engine, client processors.ResourceClient, options ctrl.Options) (ctrl.Controller, error) {
				return NewCreateOrUpdateResource(successProcessorReference, eng, client, options)
			},
			nil,
			true,
			nil,
			nil,
			nil,
			nil,
			&mapstructure.Error{Errors: []string{"'type' expected type 'string', got unconvertible type 'int', value: '3'"}},
		},
		{
			"recipe-err",
			func(eng engine.Engine, client processors.ResourceClient, options ctrl.Options) (ctrl.Controller, error) {
				return NewCreateOrUpdateResource(successProcessorReference, eng, client, options)
			},
			nil,
			false,
			&recipes.ErrRecipeNotFound{},
			nil,
			nil,
			nil,
			&recipes.ErrRecipeNotFound{},
		},
		{
			"processor-err",
			func(eng engine.Engine, client processors.ResourceClient, options ctrl.Options) (ctrl.Controller, error) {
				return NewCreateOrUpdateResource(errorProcessorReference, eng, client, options)
			},
			nil,
			false,
			nil,
			processorErr,
			nil,
			nil,
			processorErr,
		},
		{
			"resourceclient-err",
			func(eng engine.Engine, client processors.ResourceClient, options ctrl.Options) (ctrl.Controller, error) {
				return NewCreateOrUpdateResource(successProcessorReference, eng, client, options)
			},
			nil,
			false,
			nil,
			nil,
			errors.New("resource client failed"),
			nil,
			errors.New("resource client failed"),
		},
		{
			"save-err",
			func(eng engine.Engine, client processors.ResourceClient, options ctrl.Options) (ctrl.Controller, error) {
				return NewCreateOrUpdateResource(successProcessorReference, eng, client, options)
			},
			nil,
			false,
			nil,
			nil,
			nil,
			errors.New("resource save failed"),
			errors.New("resource save failed"),
		},
		{
			"success",
			func(eng engine.Engine, client processors.ResourceClient, options ctrl.Options) (ctrl.Controller, error) {
				return NewCreateOrUpdateResource(successProcessorReference, eng, client, options)
			},
			nil,
			false,
			nil,
			nil,
			nil,
			nil,
			nil,
		},
	}

	for _, tt := range cases {
		t.Run(tt.description, func(t *testing.T) {
			msc, eng, client := setupTest(t)

			req := &ctrl.Request{
				OperationID:      uuid.New(),
				OperationType:    "APPLICATIONS.TEST/TESTRESOURCES|PUT", // Operation does not affect the behavior of the controller.
				ResourceID:       TestResourceID,
				CorrelationID:    uuid.NewString(),
				OperationTimeout: &ctrl.DefaultAsyncOperationTimeout,
			}

			// Set up an output resource so we can cover resource deletion.
			status := rpv1.ResourceStatus{
				OutputResources: []rpv1.OutputResource{
					oldOutputResource,
				},
			}
			sb, err := json.Marshal(&status)
			require.NoError(t, err)

			sm := map[string]interface{}{}
			err = json.Unmarshal(sb, &sm)
			require.NoError(t, err)

			data := map[string]any{
				"name":     "tr",
				"type":     "Applications.Test/testResources",
				"id":       TestResourceID,
				"location": v1.LocationGlobal,
				"properties": map[string]any{
					"application":       TestApplicationID,
					"environment":       TestEnvironmentID,
					"provisioningState": "Accepted",
					"status":            sm,
					"recipe": map[string]any{
						"name": "test-recipe",
						"parameters": map[string]any{
							"p1": "v1",
						},
					},
				},
			}
			if tt.conversionFailure {
				data["type"] = 3 // This won't convert to our data model.
			}

			if tt.getErr != nil {
				msc.EXPECT().
					Get(gomock.Any(), TestResourceID).
					Return(&store.Object{Data: nil}, tt.getErr).
					Times(1)
			} else {
				msc.EXPECT().
					Get(gomock.Any(), TestResourceID).
					Return(&store.Object{Data: data}, nil).
					Times(1)
			}

			recipeMetadata := recipes.Metadata{
				Name:          "test-recipe",
				EnvironmentID: TestEnvironmentID,
				ApplicationID: TestApplicationID,
				ResourceID:    TestResourceID,
				Parameters: map[string]any{
					"p1": "v1",
				},
			}

			if tt.getErr == nil && !tt.conversionFailure && tt.recipeErr != nil {
				eng.EXPECT().
					Execute(gomock.Any(), recipeMetadata).
					Return(&recipes.RecipeOutput{}, tt.recipeErr).
					Times(1)
			} else if tt.getErr == nil && !tt.conversionFailure {
				eng.EXPECT().
					Execute(gomock.Any(), recipeMetadata).
					Return(&recipes.RecipeOutput{}, nil).
					Times(1)
			}

			if tt.getErr == nil && !tt.conversionFailure && tt.recipeErr == nil && tt.processorErr == nil && tt.resourceClientErr != nil {
				client.EXPECT().
					Delete(gomock.Any(), oldOutputResourceResourceID).
					Return(tt.resourceClientErr).
					Times(1)
			} else if tt.getErr == nil && !tt.conversionFailure && tt.recipeErr == nil && tt.processorErr == nil {
				client.EXPECT().
					Delete(gomock.Any(), oldOutputResourceResourceID).
					Return(nil).
					Times(1)
			}

			if tt.getErr == nil && !tt.conversionFailure && tt.recipeErr == nil && tt.processorErr == nil && tt.resourceClientErr == nil && tt.saveErr != nil {
				msc.EXPECT().
					Save(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(tt.saveErr).
					Times(1)
			} else if tt.getErr == nil && !tt.conversionFailure && tt.recipeErr == nil && tt.processorErr == nil && tt.resourceClientErr == nil {
				msc.EXPECT().
					Save(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).
					Times(1)
			}

			opts := ctrl.Options{
				StorageClient: msc,
			}

			genCtrl, err := tt.factory(eng, client, opts)
			require.NoError(t, err)

			res, err := genCtrl.Run(context.Background(), req)
			if tt.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, tt.expectedErr, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, ctrl.Result{}, res)
			}
		})
	}
}
