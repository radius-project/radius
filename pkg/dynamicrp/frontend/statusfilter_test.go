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

package frontend

import (
	"strings"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/stretchr/testify/require"
)

func TestStatusFilter(t *testing.T) {
	const (
		envA = "/planes/radius/local/resourceGroups/test-group/providers/Radius.Core/environments/env-a"
		envB = "/planes/radius/local/resourceGroups/test-group/providers/Radius.Core/environments/env-b"
		appA = "/planes/radius/local/resourceGroups/test-group/providers/Radius.Core/applications/app-a"
	)
	clientStatus := map[string]any{
		"outputResources": []any{map[string]any{"id": "/planes/kubernetes/local/namespaces/kube-system/providers/core/ConfigMap/client"}},
	}
	storedStatus := map[string]any{
		"outputResources": []any{map[string]any{"id": "/planes/kubernetes/local/namespaces/app/providers/core/ConfigMap/stored", "radiusManaged": true}},
		"recipe":          map[string]any{"templatePath": "test-path"},
	}
	storedStatusWithoutOutputs := map[string]any{"recipe": map[string]any{"templatePath": "test-path"}}
	resource := func(environment, application string, status map[string]any) *datamodel.DynamicResource {
		properties := map[string]any{"environment": environment}
		if application != "" {
			properties["application"] = application
		}
		if status != nil {
			properties["status"] = status
		}
		return &datamodel.DynamicResource{Properties: properties}
	}

	tests := []struct {
		name       string
		newRes     *datamodel.DynamicResource
		oldRes     *datamodel.DynamicResource
		wantStatus map[string]any
		wantReject bool
	}{
		{
			name:   "create drops client status",
			newRes: resource(envA, "", clientStatus),
		},
		{
			name:   "create without properties",
			newRes: &datamodel.DynamicResource{},
		},
		{
			name:       "update keeps stored status instead of client status",
			newRes:     resource(envA, "", clientStatus),
			oldRes:     resource(envA, "", storedStatus),
			wantStatus: storedStatus,
		},
		{
			name:   "update drops client status when nothing is stored",
			newRes: resource(envA, "", clientStatus),
			oldRes: resource(envA, "", nil),
		},
		{
			name:       "update without properties keeps stored status",
			newRes:     &datamodel.DynamicResource{},
			oldRes:     &datamodel.DynamicResource{Properties: map[string]any{"status": storedStatusWithoutOutputs}},
			wantStatus: storedStatusWithoutOutputs,
		},
		{
			name:       "environment change with output resources is rejected",
			newRes:     resource(envB, "", nil),
			oldRes:     resource(envA, "", storedStatus),
			wantReject: true,
		},
		{
			name:       "application change with output resources is rejected",
			newRes:     resource(envA, appA, nil),
			oldRes:     resource(envA, "", storedStatus),
			wantReject: true,
		},
		{
			name:       "environment change without output resources is allowed",
			newRes:     resource(envB, "", nil),
			oldRes:     resource(envA, "", storedStatusWithoutOutputs),
			wantStatus: storedStatusWithoutOutputs,
		},
		{
			name:       "environment ID differing only in case is the same scope",
			newRes:     resource(strings.ToLower(envA), "", nil),
			oldRes:     resource(envA, "", storedStatus),
			wantStatus: storedStatus,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := makeStatusFilter()(createTestContext(t), tt.newRes, tt.oldRes, nil)
			require.NoError(t, err)

			if tt.wantReject {
				badRequest, ok := response.(*rest.BadRequestResponse)
				require.True(t, ok, "expected a bad request response, got %T", response)
				require.Equal(t, v1.CodeInvalid, badRequest.Body.Error.Code)
				return
			}

			require.Nil(t, response)
			status, found := tt.newRes.Properties["status"]
			if tt.wantStatus == nil {
				require.False(t, found, "unexpected status: %v", status)
				return
			}
			require.Equal(t, tt.wantStatus, status)
		})
	}
}

func TestMakeUpdateFilters_ScopeCheckUsesDefaults(t *testing.T) {
	const application = "/planes/radius/local/resourceGroups/test-group/providers/Radius.Core/applications/default-app"
	ucpClient, err := createFakeUCPClientFactory(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"application": map[string]any{"type": "string", "default": application},
		},
	})
	require.NoError(t, err)

	storedStatus := map[string]any{
		"outputResources": []any{map[string]any{"id": "/planes/kubernetes/local/namespaces/app/providers/core/ConfigMap/stored"}},
	}
	oldResource := &datamodel.DynamicResource{Properties: map[string]any{"application": application, "status": storedStatus}}
	newResource := &datamodel.DynamicResource{Properties: map[string]any{}}

	filters := makeUpdateFilters(
		makeDefaultsFilter(ucpClient),
		makeStatusFilter(),
		makeValidationFilter(ucpClient),
		makeEncryptionFilter(ucpClient, createTestHandler(t)),
	)
	for _, filter := range filters {
		response, err := filter(createTestContext(t), newResource, oldResource, nil)
		require.NoError(t, err)
		require.Nil(t, response)
	}

	require.Equal(t, application, newResource.Properties["application"])
	require.Equal(t, storedStatus, newResource.Properties["status"])
}
