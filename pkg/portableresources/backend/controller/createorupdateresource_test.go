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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	armpolicy "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy"
	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	controllerfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/crypto/encryption"
	"github.com/radius-project/radius/pkg/portableresources"
	"github.com/radius-project/radius/pkg/portableresources/datamodel"
	"github.com/radius-project/radius/pkg/portableresources/processors"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	"github.com/radius-project/radius/pkg/recipes/controllerconfig"
	"github.com/radius-project/radius/pkg/recipes/engine"
	"github.com/radius-project/radius/pkg/resourceutil"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview/fake"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

const (
	TestResourceType  = "Applications.Test/testResources"
	TestEnvironmentID = "/planes/radius/local/resourceGroups/radius-test-rg/providers/Applications.Core/environments/test-env"
	TestApplicationID = "/planes/radius/local/resourceGroups/radius-test-rg/providers/Applications.Core/applications/test-app"
	TestResourceID    = "/planes/radius/local/resourceGroups/radius-test-rg/providers/Applications.Test/testResources/tr"
)

type TestResource struct {
	v1.BaseResource

	// ResourceMetadata represents internal DataModel properties common to all portable resource types.
	datamodel.PortableResourceMetadata

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

// ResourceMetadata returns an adapter that provides standardized access to BasicResourceProperties of the TestResource instance.
func (r *TestResource) ResourceMetadata() rpv1.BasicResourcePropertiesAdapter {
	return &r.Properties.BasicResourceProperties
}

// Recipe returns a pointer to the ResourceRecipe stored in the Properties field of the TestResource struct.
func (t *TestResource) GetRecipe() *portableresources.ResourceRecipe {
	return &t.Properties.Recipe
}

// SetRecipe allows updating the recipe in the resource.
func (t *TestResource) SetRecipe(r *portableresources.ResourceRecipe) {
	t.Properties.Recipe = *r
}

type TestResourceProperties struct {
	rpv1.BasicResourceProperties
	IsProcessed bool                             `json:"isProcessed"`
	Recipe      portableresources.ResourceRecipe `json:"recipe,omitempty"`
	Secret      any                              `json:"secret,omitempty"`
	Credentials map[string]any                   `json:"credentials,omitempty"`
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

// Delete returns no error.
func (p *SuccessProcessor) Delete(ctx context.Context, data *TestResource, options processors.Options) error {
	return nil
}

var successProcessorReference = processors.ResourceProcessor[*TestResource, TestResource](&SuccessProcessor{})

type ErrorProcessor struct {
}

// Process always returns a processorErr.
func (p *ErrorProcessor) Process(ctx context.Context, data *TestResource, options processors.Options) error {
	return errProcessor
}

func (p *ErrorProcessor) Delete(ctx context.Context, data *TestResource, options processors.Options) error {
	return nil
}

var errorProcessorReference = processors.ResourceProcessor[*TestResource, TestResource](&ErrorProcessor{})
var errProcessor = errors.New("processor error")
var errConfiguration = errors.New("configuration error")

var oldOutputResourceResourceID = "/subscriptions/test-sub/resourceGroups/test-rg/providers/Systems.Test/testResources/test1"

var newOutputResourceResourceID = "/subscriptions/test-sub/resourceGroups/test-rg/providers/Systems.Test/testResources/test2"
var newOutputResource = rpv1.OutputResource{ID: resources.MustParse(newOutputResourceResourceID)}

func TestCreateOrUpdateResource_Run(t *testing.T) {
	setupTest := func() (*database.MockClient, *engine.MockEngine, *processors.MockResourceClient, *configloader.MockConfigurationLoader) {
		mctrl := gomock.NewController(t)

		msc := database.NewMockClient(mctrl)
		eng := engine.NewMockEngine(mctrl)
		cfg := configloader.NewMockConfigurationLoader(mctrl)
		client := processors.NewMockResourceClient(mctrl)
		return msc, eng, client, cfg
	}

	cases := []struct {
		description             string
		factory                 func(recipeCfg *controllerconfig.RecipeControllerConfig, options ctrl.Options) (ctrl.Controller, error)
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
			func(recipeCfg *controllerconfig.RecipeControllerConfig, options ctrl.Options) (ctrl.Controller, error) {
				return NewCreateOrUpdateResource(options, errorProcessorReference, recipeCfg.Engine, recipeCfg.ConfigLoader)
			},
			&database.ErrNotFound{ID: TestResourceID},
			false,
			nil,
			nil,
			nil,
			nil,
			nil,
			&database.ErrNotFound{ID: TestResourceID},
		},
		{
			"get-error",
			func(recipeCfg *controllerconfig.RecipeControllerConfig, options ctrl.Options) (ctrl.Controller, error) {
				return NewCreateOrUpdateResource(options, errorProcessorReference, recipeCfg.Engine, recipeCfg.ConfigLoader)
			},
			&database.ErrInvalid{},
			false,
			nil,
			nil,
			nil,
			nil,
			nil,
			&database.ErrInvalid{},
		},
		{
			"conversion-failure",
			func(recipeCfg *controllerconfig.RecipeControllerConfig, options ctrl.Options) (ctrl.Controller, error) {
				return NewCreateOrUpdateResource(options, errorProcessorReference, recipeCfg.Engine, recipeCfg.ConfigLoader)
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
			func(recipeCfg *controllerconfig.RecipeControllerConfig, options ctrl.Options) (ctrl.Controller, error) {
				return NewCreateOrUpdateResource(options, errorProcessorReference, recipeCfg.Engine, recipeCfg.ConfigLoader)
			},
			nil,
			false,
			fmt.Errorf("could not find recipe %q in environment %q", "test-recipe", TestEnvironmentID),
			nil,
			nil,
			nil,
			nil,
			fmt.Errorf("could not find recipe %q in environment %q", "test-recipe", TestEnvironmentID),
		},
		{
			"runtime-configuration-err",
			func(recipeCfg *controllerconfig.RecipeControllerConfig, options ctrl.Options) (ctrl.Controller, error) {
				return NewCreateOrUpdateResource(options, errorProcessorReference, recipeCfg.Engine, recipeCfg.ConfigLoader)
			},
			nil,
			false,
			nil,
			errConfiguration,
			nil,
			nil,
			nil,
			errConfiguration,
		},
		{
			"processor-err",
			func(recipeCfg *controllerconfig.RecipeControllerConfig, options ctrl.Options) (ctrl.Controller, error) {
				return NewCreateOrUpdateResource(options, errorProcessorReference, recipeCfg.Engine, recipeCfg.ConfigLoader)
			},
			nil,
			false,
			nil,
			nil,
			errProcessor,
			nil,
			nil,
			errProcessor,
		},
		{
			"save-err",
			func(recipeCfg *controllerconfig.RecipeControllerConfig, options ctrl.Options) (ctrl.Controller, error) {
				return NewCreateOrUpdateResource(options, successProcessorReference, recipeCfg.Engine, recipeCfg.ConfigLoader)
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
			func(recipeCfg *controllerconfig.RecipeControllerConfig, options ctrl.Options) (ctrl.Controller, error) {
				return NewCreateOrUpdateResource(options, successProcessorReference, recipeCfg.Engine, recipeCfg.ConfigLoader)
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
			msc, eng, _, cfg := setupTest()

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
					Return(&database.Object{Data: nil}, tt.getErr).
					Times(1)
			} else if stillPassing {
				msc.EXPECT().
					Get(gomock.Any(), TestResourceID).
					Return(&database.Object{Data: data}, nil).
					Times(1)
			}

			if tt.conversionFailure {
				stillPassing = false
				data["type"] = 3 // This won't convert to our data model.
			}

			testResource := &TestResource{
				Properties: TestResourceProperties{
					BasicResourceProperties: rpv1.BasicResourceProperties{
						Application: TestApplicationID,
						Environment: TestEnvironmentID,
						Status: rpv1.ResourceStatus{
							OutputResources: []rpv1.OutputResource{
								{
									ID: resources.MustParse(oldOutputResourceResourceID),
								},
							},
						},
					},
					IsProcessed: false,
					Recipe: portableresources.ResourceRecipe{
						Name: "test-recipe",
						Parameters: map[string]any{
							"p1": "v1",
						},
					},
				},
			}

			properties, err := resourceutil.GetPropertiesFromResource(testResource)
			require.NoError(t, err)
			recipeMetadata := recipes.ResourceMetadata{
				Name:          "test-recipe",
				EnvironmentID: TestEnvironmentID,
				ApplicationID: TestApplicationID,
				ResourceID:    TestResourceID,
				Parameters: map[string]any{
					"p1": "v1",
				},
				Properties:                   properties,
				ConnectedResourcesProperties: map[string]recipes.ConnectedResource{},
			}

			prevState := []string{
				oldOutputResourceResourceID,
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
				DatabaseClient: msc,
			}

			recipeCfg := &controllerconfig.RecipeControllerConfig{
				Engine:       eng,
				ConfigLoader: cfg,
			}

			genCtrl, err := tt.factory(recipeCfg, opts)
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

func TestCreateOrUpdateResource_Run_SensitiveRedaction(t *testing.T) {
	mctrl := gomock.NewController(t)
	msc := database.NewMockClient(mctrl)
	eng := engine.NewMockEngine(mctrl)
	cfg := configloader.NewMockConfigurationLoader(mctrl)

	key, err := encryption.GenerateKey()
	require.NoError(t, err)

	provider, err := encryption.NewInMemoryKeyProvider(key)
	require.NoError(t, err)

	handler, err := encryption.NewSensitiveDataHandlerFromProvider(context.Background(), provider)
	require.NoError(t, err)

	secretValue := "top-secret"
	properties := map[string]any{
		"application":       TestApplicationID,
		"environment":       TestEnvironmentID,
		"provisioningState": "Accepted",
		"secret":            secretValue,
		"recipe": map[string]any{
			"name": "test-recipe",
			"parameters": map[string]any{
				"p1": "v1",
			},
		},
		"status": map[string]any{
			"outputResources": []map[string]any{},
		},
	}

	err = handler.EncryptSensitiveFields(properties, []string{"secret"}, TestResourceID)
	require.NoError(t, err)

	data := map[string]any{
		"name":              "tr",
		"type":              TestResourceType,
		"id":                TestResourceID,
		"location":          v1.LocationGlobal,
		"updatedApiVersion": "2024-01-01",
		"properties":        properties,
	}

	msc.EXPECT().
		Get(gomock.Any(), TestResourceID).
		Return(&database.Object{Metadata: database.Metadata{ID: TestResourceID, ETag: "etag-1"}, Data: data}, nil).
		Times(1)

	ucpClient, err := testUCPClientFactory(map[string]any{
		"properties": map[string]any{
			"secret": map[string]any{
				"type":               "string",
				"x-radius-sensitive": true,
			},
		},
	})
	require.NoError(t, err)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      encryption.DefaultEncryptionKeySecretName,
			Namespace: encryption.RadiusNamespace,
		},
		Data: map[string][]byte{
			encryption.DefaultEncryptionKeySecretKey: mustKeyStoreJSON(t, key),
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	k8sClient := controllerfake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	configuration := &recipes.Configuration{
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace:            "test-namespace",
				EnvironmentNamespace: "test-env-namespace",
			},
		},
	}

	redactionSave := msc.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, obj *database.Object, _ ...database.SaveOptions) error {
			props, err := resourceutil.GetPropertiesFromResource(obj.Data)
			require.NoError(t, err)
			require.Nil(t, props["secret"])
			obj.Metadata.ETag = "etag-2"
			return nil
		},
	)

	finalSave := msc.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, obj *database.Object, _ ...database.SaveOptions) error {
			props, err := resourceutil.GetPropertiesFromResource(obj.Data)
			require.NoError(t, err)
			require.Nil(t, props["secret"])
			return nil
		},
	)

	engExecute := eng.EXPECT().Execute(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, opts engine.ExecuteOptions) (*recipes.RecipeOutput, error) {
			require.Equal(t, secretValue, opts.BaseOptions.Recipe.Properties["secret"])
			return &recipes.RecipeOutput{}, nil
		},
	)

	gomock.InOrder(
		redactionSave,
		cfg.EXPECT().LoadConfiguration(gomock.Any(), recipes.ResourceMetadata{EnvironmentID: TestEnvironmentID, ApplicationID: TestApplicationID, ResourceID: TestResourceID}).Return(configuration, nil),
		engExecute,
		finalSave,
	)

	opts := ctrl.Options{
		DatabaseClient: msc,
		UcpClient:      ucpClient,
		KubeClient:     k8sClient,
	}

	recipeCfg := &controllerconfig.RecipeControllerConfig{
		Engine:       eng,
		ConfigLoader: cfg,
	}

	ctrlr, err := NewCreateOrUpdateResource(opts, successProcessorReference, recipeCfg.Engine, recipeCfg.ConfigLoader)
	require.NoError(t, err)

	res, err := ctrlr.Run(context.Background(), &ctrl.Request{
		OperationID:      uuid.New(),
		OperationType:    "APPLICATIONS.TEST/TESTRESOURCES|PUT",
		ResourceID:       TestResourceID,
		CorrelationID:    uuid.NewString(),
		OperationTimeout: &ctrl.DefaultAsyncOperationTimeout,
	})
	require.NoError(t, err)
	require.Equal(t, ctrl.Result{}, res)
}

func TestCreateOrUpdateResource_Run_SensitiveMissingKey(t *testing.T) {
	mctrl := gomock.NewController(t)
	msc := database.NewMockClient(mctrl)
	eng := engine.NewMockEngine(mctrl)
	cfg := configloader.NewMockConfigurationLoader(mctrl)

	data := map[string]any{
		"name":              "tr",
		"type":              TestResourceType,
		"id":                TestResourceID,
		"location":          v1.LocationGlobal,
		"updatedApiVersion": "2024-01-01",
		"properties": map[string]any{
			"application":       TestApplicationID,
			"environment":       TestEnvironmentID,
			"provisioningState": "Accepted",
			"secret": map[string]any{
				"encrypted": "not-real",
				"nonce":     "not-real",
			},
			"recipe": map[string]any{
				"name": "test-recipe",
			},
		},
	}

	msc.EXPECT().
		Get(gomock.Any(), TestResourceID).
		Return(&database.Object{Metadata: database.Metadata{ID: TestResourceID, ETag: "etag-1"}, Data: data}, nil).
		Times(1)

	ucpClient, err := testUCPClientFactory(map[string]any{
		"properties": map[string]any{
			"secret": map[string]any{
				"type":               "string",
				"x-radius-sensitive": true,
			},
		},
	})
	require.NoError(t, err)

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	k8sClient := controllerfake.NewClientBuilder().WithScheme(scheme).Build()

	cfg.EXPECT().LoadConfiguration(gomock.Any(), gomock.Any()).Times(0)
	eng.EXPECT().Execute(gomock.Any(), gomock.Any()).Times(0)
	msc.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	opts := ctrl.Options{
		DatabaseClient: msc,
		UcpClient:      ucpClient,
		KubeClient:     k8sClient,
	}

	recipeCfg := &controllerconfig.RecipeControllerConfig{
		Engine:       eng,
		ConfigLoader: cfg,
	}

	ctrlr, err := NewCreateOrUpdateResource(opts, successProcessorReference, recipeCfg.Engine, recipeCfg.ConfigLoader)
	require.NoError(t, err)

	res, err := ctrlr.Run(context.Background(), &ctrl.Request{
		OperationID:      uuid.New(),
		OperationType:    "APPLICATIONS.TEST/TESTRESOURCES|PUT",
		ResourceID:       TestResourceID,
		CorrelationID:    uuid.NewString(),
		OperationTimeout: &ctrl.DefaultAsyncOperationTimeout,
	})
	require.Error(t, err)
	require.Equal(t, v1.ProvisioningStateFailed, res.ProvisioningState())
	require.False(t, res.Requeue)
}

func testUCPClientFactory(schema map[string]any) (*v20231001preview.ClientFactory, error) {
	apiVersionsServer := fake.APIVersionsServer{
		Get: func(ctx context.Context, planeName string, resourceProviderName string, resourceTypeName string, apiVersionName string, options *v20231001preview.APIVersionsClientGetOptions) (resp azfake.Responder[v20231001preview.APIVersionsClientGetResponse], errResp azfake.ErrorResponder) {
			response := v20231001preview.APIVersionsClientGetResponse{
				APIVersionResource: v20231001preview.APIVersionResource{
					Properties: &v20231001preview.APIVersionProperties{
						Schema: schema,
					},
				},
			}
			resp.SetResponse(http.StatusOK, response, nil)
			return
		},
	}

	return v20231001preview.NewClientFactory(&aztoken.AnonymousCredential{}, &armpolicy.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Transport: fake.NewAPIVersionsServerTransport(&apiVersionsServer),
		},
	})
}

func mustKeyStoreJSON(t *testing.T, key []byte) []byte {
	keyStore := encryption.KeyStore{
		CurrentVersion: 1,
		Keys: map[string]encryption.KeyData{
			"1": {
				Key:       base64.StdEncoding.EncodeToString(key),
				Version:   1,
				CreatedAt: "2026-01-01T00:00:00Z",
				ExpiresAt: "2026-12-31T00:00:00Z",
			},
		},
	}

	bytes, err := json.Marshal(keyStore)
	require.NoError(t, err)
	return bytes
}

func TestCreateOrUpdateResource_Run_SensitiveNilKubeClient(t *testing.T) {
	// When sensitive fields are detected but KubeClient is nil, the controller
	// must fail immediately. Per design: no retry (Requeue: false), and recipe
	// execution must not proceed.
	mctrl := gomock.NewController(t)
	msc := database.NewMockClient(mctrl)
	eng := engine.NewMockEngine(mctrl)
	cfg := configloader.NewMockConfigurationLoader(mctrl)

	data := map[string]any{
		"name":              "tr",
		"type":              TestResourceType,
		"id":                TestResourceID,
		"location":          v1.LocationGlobal,
		"updatedApiVersion": "2024-01-01",
		"properties": map[string]any{
			"application":       TestApplicationID,
			"environment":       TestEnvironmentID,
			"provisioningState": "Accepted",
			"secret":            "value-does-not-matter",
			"recipe": map[string]any{
				"name": "test-recipe",
			},
		},
	}

	msc.EXPECT().
		Get(gomock.Any(), TestResourceID).
		Return(&database.Object{Metadata: database.Metadata{ID: TestResourceID, ETag: "etag-1"}, Data: data}, nil).
		Times(1)

	ucpClient, err := testUCPClientFactory(map[string]any{
		"properties": map[string]any{
			"secret": map[string]any{
				"type":               "string",
				"x-radius-sensitive": true,
			},
		},
	})
	require.NoError(t, err)

	// Nothing downstream should execute
	cfg.EXPECT().LoadConfiguration(gomock.Any(), gomock.Any()).Times(0)
	eng.EXPECT().Execute(gomock.Any(), gomock.Any()).Times(0)
	msc.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	opts := ctrl.Options{
		DatabaseClient: msc,
		UcpClient:      ucpClient,
		KubeClient:     nil, // deliberately nil
	}

	recipeCfg := &controllerconfig.RecipeControllerConfig{
		Engine:       eng,
		ConfigLoader: cfg,
	}

	ctrlr, err := NewCreateOrUpdateResource(opts, successProcessorReference, recipeCfg.Engine, recipeCfg.ConfigLoader)
	require.NoError(t, err)

	res, err := ctrlr.Run(context.Background(), &ctrl.Request{
		OperationID:      uuid.New(),
		OperationType:    "APPLICATIONS.TEST/TESTRESOURCES|PUT",
		ResourceID:       TestResourceID,
		CorrelationID:    uuid.NewString(),
		OperationTimeout: &ctrl.DefaultAsyncOperationTimeout,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "kubernetes client not configured")
	require.Equal(t, v1.ProvisioningStateFailed, res.ProvisioningState())
	require.False(t, res.Requeue)
}

func TestCreateOrUpdateResource_Run_SensitiveRedactionSaveFails(t *testing.T) {
	// When the redaction save fails, the controller must return a non-retryable
	// failed result and must NOT proceed to recipe execution.  Per design:
	// users must resubmit with fresh credentials.
	mctrl := gomock.NewController(t)
	msc := database.NewMockClient(mctrl)
	eng := engine.NewMockEngine(mctrl)
	cfg := configloader.NewMockConfigurationLoader(mctrl)

	key, err := encryption.GenerateKey()
	require.NoError(t, err)

	provider, err := encryption.NewInMemoryKeyProvider(key)
	require.NoError(t, err)

	handler, err := encryption.NewSensitiveDataHandlerFromProvider(context.Background(), provider)
	require.NoError(t, err)

	properties := map[string]any{
		"application":       TestApplicationID,
		"environment":       TestEnvironmentID,
		"provisioningState": "Accepted",
		"secret":            "save-failure-secret",
		"recipe": map[string]any{
			"name": "test-recipe",
		},
		"status": map[string]any{
			"outputResources": []map[string]any{},
		},
	}

	err = handler.EncryptSensitiveFields(properties, []string{"secret"}, TestResourceID)
	require.NoError(t, err)

	data := map[string]any{
		"name":              "tr",
		"type":              TestResourceType,
		"id":                TestResourceID,
		"location":          v1.LocationGlobal,
		"updatedApiVersion": "2024-01-01",
		"properties":        properties,
	}

	msc.EXPECT().
		Get(gomock.Any(), TestResourceID).
		Return(&database.Object{Metadata: database.Metadata{ID: TestResourceID, ETag: "etag-1"}, Data: data}, nil).
		Times(1)

	ucpClient, err := testUCPClientFactory(map[string]any{
		"properties": map[string]any{
			"secret": map[string]any{
				"type":               "string",
				"x-radius-sensitive": true,
			},
		},
	})
	require.NoError(t, err)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      encryption.DefaultEncryptionKeySecretName,
			Namespace: encryption.RadiusNamespace,
		},
		Data: map[string][]byte{
			encryption.DefaultEncryptionKeySecretKey: mustKeyStoreJSON(t, key),
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	k8sClient := controllerfake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	saveErr := errors.New("database unavailable")
	// The single Save call is the redaction save — verify the payload has nil
	// for the sensitive field (no plaintext leaked) before returning the error.
	msc.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, obj *database.Object, _ ...database.SaveOptions) error {
			props, err := resourceutil.GetPropertiesFromResource(obj.Data)
			require.NoError(t, err)
			require.Nil(t, props["secret"], "plaintext must never appear in the save payload")
			return saveErr
		},
	).Times(1)

	// Recipe and config must never execute
	cfg.EXPECT().LoadConfiguration(gomock.Any(), gomock.Any()).Times(0)
	eng.EXPECT().Execute(gomock.Any(), gomock.Any()).Times(0)

	opts := ctrl.Options{
		DatabaseClient: msc,
		UcpClient:      ucpClient,
		KubeClient:     k8sClient,
	}

	recipeCfg := &controllerconfig.RecipeControllerConfig{
		Engine:       eng,
		ConfigLoader: cfg,
	}

	ctrlr, err := NewCreateOrUpdateResource(opts, successProcessorReference, recipeCfg.Engine, recipeCfg.ConfigLoader)
	require.NoError(t, err)

	res, err := ctrlr.Run(context.Background(), &ctrl.Request{
		OperationID:      uuid.New(),
		OperationType:    "APPLICATIONS.TEST/TESTRESOURCES|PUT",
		ResourceID:       TestResourceID,
		CorrelationID:    uuid.NewString(),
		OperationTimeout: &ctrl.DefaultAsyncOperationTimeout,
	})
	require.Error(t, err)
	require.Equal(t, v1.ProvisioningStateFailed, res.ProvisioningState())
	require.False(t, res.Requeue)
}

func TestCreateOrUpdateResource_Run_SensitiveMultipleFields(t *testing.T) {
	// Two sensitive fields at different nesting depths: a top-level "secret" and
	// a nested "credentials.password".  Validates:
	//   - Both fields are nil in both the redaction save and the final save
	//   - The recipe engine receives both decrypted values
	mctrl := gomock.NewController(t)
	msc := database.NewMockClient(mctrl)
	eng := engine.NewMockEngine(mctrl)
	cfg := configloader.NewMockConfigurationLoader(mctrl)

	key, err := encryption.GenerateKey()
	require.NoError(t, err)

	provider, err := encryption.NewInMemoryKeyProvider(key)
	require.NoError(t, err)

	handler, err := encryption.NewSensitiveDataHandlerFromProvider(context.Background(), provider)
	require.NoError(t, err)

	properties := map[string]any{
		"application":       TestApplicationID,
		"environment":       TestEnvironmentID,
		"provisioningState": "Accepted",
		"secret":            "secret-value",
		"credentials": map[string]any{
			"password": "password-value",
		},
		"recipe": map[string]any{
			"name": "test-recipe",
			"parameters": map[string]any{
				"p1": "v1",
			},
		},
		"status": map[string]any{
			"outputResources": []map[string]any{},
		},
	}

	err = handler.EncryptSensitiveFields(properties, []string{"secret", "credentials.password"}, TestResourceID)
	require.NoError(t, err)

	data := map[string]any{
		"name":              "tr",
		"type":              TestResourceType,
		"id":                TestResourceID,
		"location":          v1.LocationGlobal,
		"updatedApiVersion": "2024-01-01",
		"properties":        properties,
	}

	msc.EXPECT().
		Get(gomock.Any(), TestResourceID).
		Return(&database.Object{Metadata: database.Metadata{ID: TestResourceID, ETag: "etag-1"}, Data: data}, nil).
		Times(1)

	ucpClient, err := testUCPClientFactory(map[string]any{
		"properties": map[string]any{
			"secret": map[string]any{
				"type":               "string",
				"x-radius-sensitive": true,
			},
			"credentials": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"password": map[string]any{
						"type":               "string",
						"x-radius-sensitive": true,
					},
				},
			},
		},
	})
	require.NoError(t, err)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      encryption.DefaultEncryptionKeySecretName,
			Namespace: encryption.RadiusNamespace,
		},
		Data: map[string][]byte{
			encryption.DefaultEncryptionKeySecretKey: mustKeyStoreJSON(t, key),
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	k8sClient := controllerfake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	configuration := &recipes.Configuration{
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace:            "test-namespace",
				EnvironmentNamespace: "test-env-namespace",
			},
		},
	}

	assertBothFieldsRedacted := func(obj *database.Object) {
		props, err := resourceutil.GetPropertiesFromResource(obj.Data)
		require.NoError(t, err)
		require.Nil(t, props["secret"], "secret must be nil in DB")
		creds, ok := props["credentials"].(map[string]any)
		require.True(t, ok, "credentials must be a map")
		require.Nil(t, creds["password"], "credentials.password must be nil in DB")
	}

	redactionSave := msc.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, obj *database.Object, _ ...database.SaveOptions) error {
			assertBothFieldsRedacted(obj)
			obj.Metadata.ETag = "etag-2"
			return nil
		},
	)

	finalSave := msc.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, obj *database.Object, _ ...database.SaveOptions) error {
			assertBothFieldsRedacted(obj)
			return nil
		},
	)

	engExecute := eng.EXPECT().Execute(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, opts engine.ExecuteOptions) (*recipes.RecipeOutput, error) {
			// Recipe must receive ALL decrypted values
			require.Equal(t, "secret-value", opts.BaseOptions.Recipe.Properties["secret"])
			creds, ok := opts.BaseOptions.Recipe.Properties["credentials"].(map[string]any)
			require.True(t, ok, "recipe must receive credentials map")
			require.Equal(t, "password-value", creds["password"])
			return &recipes.RecipeOutput{}, nil
		},
	)

	gomock.InOrder(
		redactionSave,
		cfg.EXPECT().LoadConfiguration(gomock.Any(), recipes.ResourceMetadata{EnvironmentID: TestEnvironmentID, ApplicationID: TestApplicationID, ResourceID: TestResourceID}).Return(configuration, nil),
		engExecute,
		finalSave,
	)

	opts := ctrl.Options{
		DatabaseClient: msc,
		UcpClient:      ucpClient,
		KubeClient:     k8sClient,
	}

	recipeCfg := &controllerconfig.RecipeControllerConfig{
		Engine:       eng,
		ConfigLoader: cfg,
	}

	ctrlr, err := NewCreateOrUpdateResource(opts, successProcessorReference, recipeCfg.Engine, recipeCfg.ConfigLoader)
	require.NoError(t, err)

	res, err := ctrlr.Run(context.Background(), &ctrl.Request{
		OperationID:      uuid.New(),
		OperationType:    "APPLICATIONS.TEST/TESTRESOURCES|PUT",
		ResourceID:       TestResourceID,
		CorrelationID:    uuid.NewString(),
		OperationTimeout: &ctrl.DefaultAsyncOperationTimeout,
	})
	require.NoError(t, err)
	require.Equal(t, ctrl.Result{}, res)
}
