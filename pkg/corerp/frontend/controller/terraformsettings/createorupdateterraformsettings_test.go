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

package terraformsettings

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

func TestNewCreateOrUpdateTerraformSettings(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	databaseClient := database.NewMockClient(mctrl)
	opts := ctrl.Options{
		DatabaseClient: databaseClient,
	}

	controller, err := NewCreateOrUpdateTerraformSettings(opts)
	require.NoError(t, err)
	require.NotNil(t, controller)
}

func TestCreateOrUpdateTerraformSettingsRun_CreateNew(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	databaseClient := database.NewMockClient(mctrl)

	terraformSettingsInput, terraformSettingsDataModel, expectedOutput := getTestModels()
	w := httptest.NewRecorder()

	jsonPayload, err := json.Marshal(terraformSettingsInput)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPut, "/planes/radius/local/resourceGroups/default/providers/Radius.Core/terraformSettings/test-settings?api-version=2025-08-01-preview", strings.NewReader(string(jsonPayload)))
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
			obj.Data = terraformSettingsDataModel
			return nil
		})

	opts := ctrl.Options{
		DatabaseClient: databaseClient,
	}

	ctl, err := NewCreateOrUpdateTerraformSettings(opts)
	require.NoError(t, err)
	resp, err := ctl.Run(ctx, w, req)
	require.NoError(t, err)
	_ = resp.Apply(ctx, w, req)
	require.Equal(t, 200, w.Result().StatusCode)

	actualOutput := &v20250801preview.TerraformSettingsResource{}
	_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
	require.Equal(t, expectedOutput.Properties.Backend.Type, actualOutput.Properties.Backend.Type)
	require.Equal(t, v20250801preview.ProvisioningStateSucceeded, *actualOutput.Properties.ProvisioningState)
}

func TestCreateOrUpdateTerraformSettingsRun_UpdateExisting(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	databaseClient := database.NewMockClient(mctrl)
	terraformSettingsInput, terraformSettingsDataModel, expectedOutput := getTestModels()
	w := httptest.NewRecorder()

	jsonPayload, err := json.Marshal(terraformSettingsInput)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPut, "/planes/radius/local/resourceGroups/default/providers/Radius.Core/terraformSettings/test-settings?api-version=2025-08-01-preview", strings.NewReader(string(jsonPayload)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	ctx := rpctest.NewARMRequestContext(req)

	databaseClient.
		EXPECT().
		Get(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
			return &database.Object{
				Data: terraformSettingsDataModel,
			}, nil
		})

	databaseClient.
		EXPECT().
		Save(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, obj *database.Object, opts ...database.SaveOptions) error {
			obj.Data = terraformSettingsDataModel
			return nil
		})

	opts := ctrl.Options{
		DatabaseClient: databaseClient,
	}

	ctl, err := NewCreateOrUpdateTerraformSettings(opts)
	require.NoError(t, err)
	resp, err := ctl.Run(ctx, w, req)
	require.NoError(t, err)
	_ = resp.Apply(ctx, w, req)
	require.Equal(t, 200, w.Result().StatusCode)

	actualOutput := &v20250801preview.TerraformSettingsResource{}
	_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
	require.Equal(t, expectedOutput.Properties.Backend.Type, actualOutput.Properties.Backend.Type)
	require.Equal(t, v20250801preview.ProvisioningStateSucceeded, *actualOutput.Properties.ProvisioningState)
}

func getTestModels() (*v20250801preview.TerraformSettingsResource, *datamodel.TerraformSettings_v20250801preview, *v20250801preview.TerraformSettingsResource) {
	resourceID := "/planes/radius/local/resourceGroups/default/providers/Radius.Core/terraformSettings/test-settings"
	resourceName := "test-settings"
	location := "global"

	terraformSettingsInput := &v20250801preview.TerraformSettingsResource{
		Location: &location,
		Properties: &v20250801preview.TerraformSettingsProperties{
			Terraformrc: &v20250801preview.TerraformCliConfiguration{
				ProviderInstallation: &v20250801preview.TerraformProviderInstallationConfiguration{
					NetworkMirror: &v20250801preview.TerraformNetworkMirrorConfiguration{
						URL:     to.Ptr("https://mirror.example.com/"),
						Include: []*string{to.Ptr("*")},
					},
				},
			},
			Backend: &v20250801preview.TerraformBackendConfiguration{
				Type: to.Ptr("kubernetes"),
				Config: map[string]*string{
					"namespace": to.Ptr("radius-system"),
				},
			},
			Env: map[string]*string{
				"TF_LOG": to.Ptr("DEBUG"),
			},
		},
	}

	terraformSettingsDataModel := &datamodel.TerraformSettings_v20250801preview{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       resourceID,
				Name:     resourceName,
				Type:     datamodel.TerraformSettingsResourceType_v20250801preview,
				Location: location,
			},
		},
		Properties: datamodel.TerraformSettingsProperties_v20250801preview{
			TerraformRC: &datamodel.TerraformCliConfiguration{
				ProviderInstallation: &datamodel.TerraformProviderInstallationConfiguration{
					NetworkMirror: &datamodel.TerraformNetworkMirrorConfiguration{
						URL:     "https://mirror.example.com/",
						Include: []string{"*"},
					},
				},
			},
			Backend: &datamodel.TerraformBackendConfiguration{
				Type: "kubernetes",
				Config: map[string]string{
					"namespace": "radius-system",
				},
			},
			Env: map[string]string{
				"TF_LOG": "DEBUG",
			},
		},
	}

	expectedOutput := &v20250801preview.TerraformSettingsResource{
		ID:       &resourceID,
		Name:     &resourceName,
		Type:     to.Ptr(datamodel.TerraformSettingsResourceType_v20250801preview),
		Location: &location,
		Properties: &v20250801preview.TerraformSettingsProperties{
			ProvisioningState: to.Ptr(v20250801preview.ProvisioningStateSucceeded),
			Terraformrc: &v20250801preview.TerraformCliConfiguration{
				ProviderInstallation: &v20250801preview.TerraformProviderInstallationConfiguration{
					NetworkMirror: &v20250801preview.TerraformNetworkMirrorConfiguration{
						URL:     to.Ptr("https://mirror.example.com/"),
						Include: []*string{to.Ptr("*")},
					},
				},
			},
			Backend: &v20250801preview.TerraformBackendConfiguration{
				Type: to.Ptr("kubernetes"),
				Config: map[string]*string{
					"namespace": to.Ptr("radius-system"),
				},
			},
			Env: map[string]*string{
				"TF_LOG": to.Ptr("DEBUG"),
			},
		},
	}

	return terraformSettingsInput, terraformSettingsDataModel, expectedOutput
}
