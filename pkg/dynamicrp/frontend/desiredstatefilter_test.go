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

package frontend

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	armpolicy "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy"
	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	"github.com/radius-project/radius/pkg/schema"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type desiredStateTestTransport func(*http.Request) (*http.Response, error)

func (transport desiredStateTestTransport) Do(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestDesiredStateAdmissionPreservesSensitiveInputs(t *testing.T) {
	t.Parallel()
	const environment = "/planes/radius/local/resourceGroups/test-group/providers/Radius.Core/environments/env"
	const targetID = "/planes/radius/local/resourceGroups/test-group/providers/Radius.Compute/containers/target"
	for _, definition := range []*recipes.EnvironmentDefinition{
		{Driver: "delegated", DelegatesTo: "Radius.Compute/containers"},
		{Driver: "bicep", AcceptsDelegationFrom: []string{"Radius.Compute/routes"}},
	} {
		t.Run(definition.Driver, func(t *testing.T) {
			t.Parallel()
			source := map[string]any{"type": "object", "properties": map[string]any{
				"environment": map[string]any{"type": "string"},
				"password":    map[string]any{"type": "string", "x-radius-sensitive": true},
				"image":       map[string]any{"type": "string"},
				"target":      map[string]any{"type": "string", schema.ResourceReferenceAnnotation: "Radius.Compute/containers"},
			}}
			client, err := v20231001preview.NewClientFactory(&aztoken.AnonymousCredential{}, &armpolicy.ClientOptions{
				Transport: desiredStateTestTransport(func(request *http.Request) (*http.Response, error) {
					response := map[string]any{"properties": map[string]any{}}
					if strings.Contains(strings.ToLower(request.URL.Path), "/apiversions/") {
						response["properties"] = map[string]any{"schema": source}
					}
					data, err := json.Marshal(response)
					require.NoError(t, err)
					return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(data))), Request: request}, nil
				}),
			})
			require.NoError(t, err)
			loader := configloader.NewMockConfigurationLoader(gomock.NewController(t))
			loader.EXPECT().LoadRecipe(gomock.Any(), gomock.Any()).Return(definition, nil)
			store := database.NewMockClient(gomock.NewController(t))
			if definition.Driver == "delegated" {
				target := &datamodel.DynamicResource{Properties: map[string]any{"environment": environment}}
				target.ID = targetID
				target.Type = "Radius.Compute/containers"
				store.EXPECT().Get(gomock.Any(), targetID).Return(&database.Object{ID: targetID, Data: target}, nil)
				loader.EXPECT().LoadRecipe(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, metadata *recipes.ResourceMetadata) (*recipes.EnvironmentDefinition, error) {
						require.Equal(t, targetID, metadata.ResourceID)
						return &recipes.EnvironmentDefinition{Driver: "bicep", AcceptsDelegationFrom: []string{"Applications.Test/testResources"}}, nil
					})
			}
			handler := createTestHandler(t)
			ctx := createTestContext(t)
			resource := &datamodel.DynamicResource{Properties: map[string]any{
				"environment": environment, "target": targetID,
				"password": "private-input", "image": "saved-image",
			}}
			old := &datamodel.DynamicResource{DelegationTargetID: targetID}
			response, err := makeDesiredStateFilter(loader, client, handler)(ctx, resource, old, &controller.Options{DatabaseClient: store})
			require.NoError(t, err)
			require.Nil(t, response)
			require.NotNil(t, resource.DesiredState)
			require.Equal(t, old.DelegationTargetID, resource.DelegationTargetID)
			encoded, err := json.Marshal(resource.DesiredState)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "private-input")
			require.NoError(t, handler.EncryptSensitiveFields(resource.Properties, []string{"password"}, testResourceID))
			schema.RedactFields(resource.Properties, []string{"password"})
			materialized, _, err := resource.DesiredState.Materialize(ctx, handler)
			require.NoError(t, err)
			require.Equal(t, "private-input", materialized.Properties["password"])
			require.Equal(t, "saved-image", materialized.Properties["image"])
		})
	}
}

func TestDesiredStateAdmissionLeavesLegacyResourcesUnchanged(t *testing.T) {
	t.Parallel()
	resource := &datamodel.DynamicResource{Properties: map[string]any{
		"environment": "/planes/radius/local/resourceGroups/rg/providers/Applications.Core/environments/env",
	}}
	response, err := makeDesiredStateFilter(nil, nil, nil)(createTestContext(t), resource, nil, nil)
	require.NoError(t, err)
	require.Nil(t, response)
	require.Nil(t, resource.DesiredState)
}
