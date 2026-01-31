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

package bicepsettings

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/to"
)

func TestNewCreateOrUpdateBicepSettings(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	databaseClient := database.NewMockClient(mctrl)
	opts := ctrl.Options{
		DatabaseClient: databaseClient,
	}

	controller, err := NewCreateOrUpdateBicepSettings(opts)
	require.NoError(t, err)
	require.NotNil(t, controller)
}

func TestCreateOrUpdateBicepSettingsRun_CreateNew(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	databaseClient := database.NewMockClient(mctrl)

	bicepSettingsInput, bicepSettingsDataModel, _ := getTestModels()
	w := httptest.NewRecorder()

	jsonPayload, err := json.Marshal(bicepSettingsInput)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPut, "/planes/radius/local/resourceGroups/default/providers/Radius.Core/bicepSettings/test-settings?api-version=2025-08-01-preview", strings.NewReader(string(jsonPayload)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	ctx := rpctest.NewARMRequestContext(req)

	databaseClient.
		EXPECT().
		Get(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
			return nil, &database.ErrNotFound{ID: id}
		})

	databaseClient.
		EXPECT().
		Save(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, obj *database.Object, opts ...database.SaveOptions) error {
			obj.ETag = "new-resource-etag"
			obj.Data = bicepSettingsDataModel
			return nil
		})

	opts := ctrl.Options{
		DatabaseClient: databaseClient,
	}

	ctl, err := NewCreateOrUpdateBicepSettings(opts)
	require.NoError(t, err)
	resp, err := ctl.Run(ctx, w, req)
	require.NoError(t, err)
	_ = resp.Apply(ctx, w, req)
	require.Equal(t, 200, w.Result().StatusCode)

	actualOutput := &v20250801preview.BicepSettingsResource{}
	_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
	require.Equal(t, v20250801preview.ProvisioningStateSucceeded, *actualOutput.Properties.ProvisioningState)
}

func TestCreateOrUpdateBicepSettingsRun_UpdateExisting(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	databaseClient := database.NewMockClient(mctrl)
	bicepSettingsInput, bicepSettingsDataModel, _ := getTestModels()
	w := httptest.NewRecorder()

	jsonPayload, err := json.Marshal(bicepSettingsInput)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPut, "/planes/radius/local/resourceGroups/default/providers/Radius.Core/bicepSettings/test-settings?api-version=2025-08-01-preview", strings.NewReader(string(jsonPayload)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	ctx := rpctest.NewARMRequestContext(req)

	databaseClient.
		EXPECT().
		Get(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
			return &database.Object{
				Data: bicepSettingsDataModel,
			}, nil
		})

	databaseClient.
		EXPECT().
		Save(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, obj *database.Object, opts ...database.SaveOptions) error {
			obj.Data = bicepSettingsDataModel
			return nil
		})

	opts := ctrl.Options{
		DatabaseClient: databaseClient,
	}

	ctl, err := NewCreateOrUpdateBicepSettings(opts)
	require.NoError(t, err)
	resp, err := ctl.Run(ctx, w, req)
	require.NoError(t, err)
	_ = resp.Apply(ctx, w, req)
	require.Equal(t, 200, w.Result().StatusCode)

	actualOutput := &v20250801preview.BicepSettingsResource{}
	_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
	require.Equal(t, v20250801preview.ProvisioningStateSucceeded, *actualOutput.Properties.ProvisioningState)
}

func getTestModels() (*v20250801preview.BicepSettingsResource, *datamodel.BicepSettings_v20250801preview, *v20250801preview.BicepSettingsResource) {
	resourceID := "/planes/radius/local/resourceGroups/default/providers/Radius.Core/bicepSettings/test-settings"
	resourceName := "test-settings"
	location := "global"

	bicepSettingsInput := &v20250801preview.BicepSettingsResource{
		Location: &location,
		Properties: &v20250801preview.BicepSettingsProperties{
			Authentication: &v20250801preview.BicepAuthenticationConfiguration{
				Registries: map[string]*v20250801preview.BicepRegistryAuthentication{
					"myregistry.azurecr.io": {
						Basic: &v20250801preview.BicepBasicAuthentication{
							Username: to.Ptr("admin"),
							Password: &v20250801preview.SecretReference{
								SecretID: to.Ptr("/planes/radius/local/providers/Radius.Security/secrets/acr-password"),
								Key:      to.Ptr("password"),
							},
						},
					},
				},
			},
		},
	}

	bicepSettingsDataModel := &datamodel.BicepSettings_v20250801preview{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       resourceID,
				Name:     resourceName,
				Type:     datamodel.BicepSettingsResourceType_v20250801preview,
				Location: location,
			},
		},
		Properties: datamodel.BicepSettingsProperties_v20250801preview{
			Authentication: &datamodel.BicepAuthenticationConfiguration{
				Registries: map[string]*datamodel.BicepRegistryAuthentication{
					"myregistry.azurecr.io": {
						Basic: &datamodel.BicepBasicAuthentication{
							Username: "admin",
							Password: &datamodel.SecretRef{
								SecretID: "/planes/radius/local/providers/Radius.Security/secrets/acr-password",
								Key:      "password",
							},
						},
					},
				},
			},
		},
	}

	expectedOutput := &v20250801preview.BicepSettingsResource{
		ID:       &resourceID,
		Name:     &resourceName,
		Type:     to.Ptr(datamodel.BicepSettingsResourceType_v20250801preview),
		Location: &location,
		Properties: &v20250801preview.BicepSettingsProperties{
			ProvisioningState: to.Ptr(v20250801preview.ProvisioningStateSucceeded),
			Authentication: &v20250801preview.BicepAuthenticationConfiguration{
				Registries: map[string]*v20250801preview.BicepRegistryAuthentication{
					"myregistry.azurecr.io": {
						Basic: &v20250801preview.BicepBasicAuthentication{
							Username: to.Ptr("admin"),
							Password: &v20250801preview.SecretReference{
								SecretID: to.Ptr("/planes/radius/local/providers/Radius.Security/secrets/acr-password"),
								Key:      to.Ptr("password"),
							},
						},
					},
				},
			},
		},
	}

	return bicepSettingsInput, bicepSettingsDataModel, expectedOutput
}
