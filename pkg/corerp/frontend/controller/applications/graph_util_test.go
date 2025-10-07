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

package applications

import (
	"context"
	"testing"

	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	corerpv20231001preview "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/test/testutil"
	"github.com/stretchr/testify/require"
)

func Test_isResourceInEnvironment(t *testing.T) {
	id := "/planes/radius/local/resourceGroups/cool-group/providers/Applications.Core/Applications/myapp"

	type args struct {
		resource        generated.GenericResource
		environmentName string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "resource is in environment",
			args: args{
				resource: generated.GenericResource{
					ID: &id,
					Properties: map[string]interface{}{
						"environment": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environments/env0",
					},
				},
				environmentName: "env0",
			},
			want: true,
		},
		{
			name: "resource is not in environment",
			args: args{
				resource: generated.GenericResource{
					ID: &id,
					Properties: map[string]interface{}{
						"environment": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environments/env0",
					},
				},
				environmentName: "env",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isResourceInEnvironment(tt.args.resource, tt.args.environmentName))
		})
	}
}

func Test_isResourceInApplication(t *testing.T) {
	id := "/planes/radius/local/resourceGroups/cool-group/providers/Applications.Core/Applications/myapp"

	type args struct {
		resource        generated.GenericResource
		applicationName string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "resource is in application",
			args: args{
				resource: generated.GenericResource{
					ID: &id,
					Properties: map[string]interface{}{
						"application": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/applications/myapp",
					},
				},
				applicationName: "myapp",
			},
			want: true,
		},
		{
			name: "resource is not in application",
			args: args{
				resource: generated.GenericResource{
					ID: &id,
					Properties: map[string]interface{}{
						"application": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/applications/myapp",
					},
				},
				applicationName: "myapp2",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isResourceInApplication(tt.args.resource, tt.args.applicationName))
		})
	}
}

func Test_computeGraph(t *testing.T) {
	tests := []struct {
		name                string
		appResourceDataFile string
		envResourceDataFile string
		expectedDataFile    string
	}{
		{
			name:                "direct route",
			appResourceDataFile: "graph-app-directroute-in.json",
			envResourceDataFile: "",
			expectedDataFile:    "graph-app-directroute-out.json",
		},
		{
			name:                "with gateway route",
			appResourceDataFile: "graph-app-gw-in.json",
			envResourceDataFile: "",
			expectedDataFile:    "graph-app-gw-out.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appResource := []generated.GenericResource{}
			envResource := []generated.GenericResource{}

			if tt.appResourceDataFile != "" {
				testutil.MustUnmarshalFromFile(tt.appResourceDataFile, &appResource)
			}

			if tt.envResourceDataFile != "" {
				testutil.MustUnmarshalFromFile(tt.envResourceDataFile, &envResource)
			}

			expected := []*corerpv20231001preview.ApplicationGraphResource{}
			testutil.MustUnmarshalFromFile(tt.expectedDataFile, &expected)

			got := computeGraph(appResource, envResource)
			require.ElementsMatch(t, expected, got.Resources)
		})
	}
}

func TestFindSourceResource(t *testing.T) {
	tests := []struct {
		name             string
		source           string
		resourceDataFile string

		parsedSource string
		wantErr      error
	}{
		{
			name:             "valid source ID",
			source:           "/planes/radius/local/resourcegroups/default/providers/Applications.Datastores/sqlDatabases/sql-db",
			resourceDataFile: "graph-app-directroute-in.json",
			parsedSource:     "/planes/radius/local/resourcegroups/default/providers/Applications.Datastores/sqlDatabases/sql-db",
			wantErr:          nil,
		},
		{
			name:             "invalid source",
			source:           "invalid",
			resourceDataFile: "graph-app-directroute-in.json",
			parsedSource:     "invalid",
			wantErr:          ErrInvalidSource,
		},
		{
			name:             "direct route without scheme",
			source:           "backendapp:8080",
			resourceDataFile: "graph-app-directroute-in.json",
			parsedSource:     "/planes/radius/local/resourcegroups/default/providers/Applications.Core/containers/backendapp",
			wantErr:          nil,
		},
		{
			name:             "direct route with scheme",
			source:           "http://backendapp:8080",
			resourceDataFile: "graph-app-directroute-in.json",
			parsedSource:     "/planes/radius/local/resourcegroups/default/providers/Applications.Core/containers/backendapp",
			wantErr:          nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resources := []generated.GenericResource{}
			testutil.MustUnmarshalFromFile(tc.resourceDataFile, &resources)
			parsedSource, err := findSourceResource(tc.source, resources)
			require.Equal(t, tc.parsedSource, parsedSource)
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

// Test_getAPIVersionForResourceType_Validation tests the resource type format validation
func Test_getAPIVersionForResourceType_Validation(t *testing.T) {
	tests := []struct {
		name         string
		resourceType string
		wantErr      bool
		errContains  string
	}{
		{
			name:         "returns error for invalid resource type format - no slash",
			resourceType: "InvalidFormat",
			wantErr:      true,
			errContains:  "invalid resource type format",
		},
		{
			name:         "returns error for invalid resource type format - too many slashes",
			resourceType: "Test.Resources/postgres/extra",
			wantErr:      true,
			errContains:  "invalid resource type format",
		},
		{
			name:         "returns error for empty resource type",
			resourceType: "",
			wantErr:      true,
			errContains:  "invalid resource type format",
		},
		{
			name:         "returns error for only slash",
			resourceType: "/",
			wantErr:      true,
			errContains:  "invalid resource type format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For validation tests, we just need to check the parsing logic
			// We'll use a nil clientOptions since validation happens first
			_, err := getAPIVersionForResourceType(context.Background(), tt.resourceType, nil)

			// Verify results
			require.Error(t, err)
			if tt.errContains != "" {
				require.Contains(t, err.Error(), tt.errContains)
			}
		})
	}
}
