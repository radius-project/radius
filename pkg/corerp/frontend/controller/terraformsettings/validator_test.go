/*
Copyright 2026 The Radius Authors.

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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/frontend/defaultoperation"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/datamodel/converter"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestValidateRequest(t *testing.T) {
	for _, backend := range []datamodel.TerraformBackend{
		{Type: "s3", Bucket: "states", Region: "us-west-2"},
		{Type: "azurerm", StorageAccountName: "states", ContainerName: "radius"},
	} {
		t.Run(backend.Type, func(t *testing.T) {
			old := &datamodel.TerraformSettings{Properties: datamodel.TerraformSettingsResourceProperties{Backend: &backend}}
			before, err := json.Marshal(old)
			require.NoError(t, err)
			cases := []struct {
				name   string
				update func(*datamodel.TerraformSettings)
				reject bool
			}{
				{"same", func(*datamodel.TerraformSettings) {}, false},
				{"explicit default", func(r *datamodel.TerraformSettings) { r.Properties.Backend.KeyPrefix = new("radius") }, false},
				{"unrelated env", func(r *datamodel.TerraformSettings) { r.Properties.Env = map[string]string{"HELLO": "world"} }, false},
				{"prefix", func(r *datamodel.TerraformSettings) { r.Properties.Backend.KeyPrefix = new("other") }, true},
				{"type", func(r *datamodel.TerraformSettings) { r.Properties.Backend.Type = "other" }, true},
				{"bucket", func(r *datamodel.TerraformSettings) { r.Properties.Backend.Bucket = "other" }, true},
				{"region", func(r *datamodel.TerraformSettings) { r.Properties.Backend.Region = "other" }, true},
				{"account", func(r *datamodel.TerraformSettings) { r.Properties.Backend.StorageAccountName = "other" }, true},
				{"container", func(r *datamodel.TerraformSettings) { r.Properties.Backend.ContainerName = "other" }, true},
				{"PUT removal", func(r *datamodel.TerraformSettings) { r.Properties.Backend = nil }, true},
			}

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					var next datamodel.TerraformSettings
					require.NoError(t, json.Unmarshal(before, &next))
					tc.update(&next)
					resp, err := ValidateRequest(t.Context(), &next, old, nil)
					require.NoError(t, err)
					if tc.reject {
						require.Contains(t, resp.(*rest.BadRequestResponse).Body.Error.Message, "include the unchanged backend")
					} else {
						require.Nil(t, resp)
					}
					after, err := json.Marshal(old)
					require.NoError(t, err)
					require.JSONEq(t, string(before), string(after))
				})
			}
			// PATCH deliberately uses replacement semantics: omission and explicit null both remove the field.
			for _, body := range []string{`{"properties":{"env":{"HELLO":"world"}}}`, `{"properties":{"backend":null}}`} {
				var patch datamodel.TerraformSettings
				require.NoError(t, json.Unmarshal([]byte(body), &patch))
				resp, err := ValidateRequest(t.Context(), &patch, old, nil)
				require.NoError(t, err)
				require.Contains(t, resp.(*rest.BadRequestResponse).Body.Error.Message, "backend location cannot change")
				after, err := json.Marshal(old)
				require.NoError(t, err)
				require.JSONEq(t, string(before), string(after))
			}
			for _, prior := range []*datamodel.TerraformSettings{nil, {}} {
				resp, err := ValidateRequest(t.Context(), old, prior, nil)
				require.NoError(t, err)
				require.Nil(t, resp)
			}
		})
	}
	resp, err := ValidateRequest(t.Context(), &datamodel.TerraformSettings{
		Properties: datamodel.TerraformSettingsResourceProperties{Backend: &datamodel.TerraformBackend{Type: "unknown"}},
	}, nil, nil)
	require.NoError(t, err)
	require.IsType(t, &rest.BadRequestResponse{}, resp)
}

func TestRejectedReplacementDoesNotSave(t *testing.T) {
	for _, method := range []string{http.MethodPut, http.MethodPatch} {
		for _, body := range []string{
			`{"tags":{"owner":"new"}}`,
			`{"properties":null}`,
			`{"properties":{"env":{"HELLO":"world"}}}`,
			`{"properties":{"backend":null}}`,
			`{"properties":{"backend":{"type":"s3","bucket":"other","region":"us-west-2"}}}`,
		} {
			t.Run(method+"/"+body, func(t *testing.T) {
				const id = "/planes/radius/local/resourceGroups/test-rg/providers/Radius.Core/terraformSettings/states"
				old := &datamodel.TerraformSettings{
					Properties: datamodel.TerraformSettingsResourceProperties{
						Backend: &datamodel.TerraformBackend{Type: "s3", Bucket: "states", Region: "us-west-2"},
						Env:     map[string]string{"ORIGINAL": "value"},
					},
				}
				old.SetProvisioningState(v1.ProvisioningStateSucceeded)
				before, err := json.Marshal(old)
				require.NoError(t, err)
				db := database.NewMockClient(gomock.NewController(t))
				db.EXPECT().Get(gomock.Any(), id).Return(&database.Object{ID: id, Data: old, ETag: "original"}, nil)
				// No Save expectation: a rejected request must not touch persisted state.
				ctl, err := defaultoperation.NewDefaultSyncPut[*datamodel.TerraformSettings](
					controller.Options{DatabaseClient: db},
					controller.ResourceOptions[datamodel.TerraformSettings]{
						RequestConverter:  converter.TerraformSettingsDataModelFromVersioned,
						ResponseConverter: converter.TerraformSettingsDataModelToVersioned,
						UpdateFilters:     []controller.UpdateFilter[datamodel.TerraformSettings]{ValidateRequest},
					})
				require.NoError(t, err)
				req, err := rpctest.NewHTTPRequestWithContent(t.Context(), method, "http://localhost"+id+"?api-version=2025-08-01-preview", []byte(body))
				require.NoError(t, err)
				ctx := rpctest.NewARMRequestContext(req)
				recorder := httptest.NewRecorder()
				response, err := ctl.Run(ctx, recorder, req)
				require.NoError(t, err)
				require.NoError(t, response.Apply(ctx, recorder, req))
				require.Equal(t, http.StatusBadRequest, recorder.Code)
				require.Contains(t, recorder.Body.String(), "include the unchanged backend")
				after, err := json.Marshal(old)
				require.NoError(t, err)
				require.JSONEq(t, string(before), string(after))
			})
		}
	}
}
