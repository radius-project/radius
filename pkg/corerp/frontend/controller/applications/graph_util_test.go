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
		ctx             context.Context
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
				ctx: context.Background(),
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
				ctx: context.Background(),
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
			require.Equal(t, tt.want, isResourceInEnvironment(tt.args.ctx, tt.args.resource, tt.args.environmentName))
		})
	}
}

func Test_isResourceInApplication(t *testing.T) {
	id := "/planes/radius/local/resourceGroups/cool-group/providers/Applications.Core/Applications/myapp"

	type args struct {
		ctx             context.Context
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
				ctx: context.Background(),
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
				ctx: context.Background(),
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
			require.Equal(t, tt.want, isResourceInApplication(tt.args.ctx, tt.args.resource, tt.args.applicationName))
		})
	}
}

func Test_computeGraph(t *testing.T) {
	tests := []struct {
		name                string
		applicationName     string
		appResourceDataFile string
		envResourceDataFile string
		expectedDataFile    string
	}{
		{
			name:                "using httproute",
			applicationName:     "myapp",
			appResourceDataFile: "graph-app-httproute-in.json",
			envResourceDataFile: "",
			expectedDataFile:    "graph-app-httproute-out.json",
		},
		{
			name:                "direct route",
			applicationName:     "myapp",
			appResourceDataFile: "graph-app-directroute-in.json",
			envResourceDataFile: "",
			expectedDataFile:    "graph-app-directroute-out.json",
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

			got := computeGraph(tt.applicationName, appResource, envResource)
			require.ElementsMatch(t, expected, got.Resources)
		})
	}
}

func TestParseSource(t *testing.T) {
	tests := []struct {
		name     string
		parentID string
		source   string

		parsedSource string
		ok           bool
	}{
		{
			name:         "valid source ID",
			parentID:     "/planes/radius/local/resourcegroups/default/providers/Applications.Core/containers/sql-app-ctnr",
			source:       "/planes/radius/local/resourcegroups/default/providers/Applications.Datastores/sqlDatabases/sql-db",
			parsedSource: "/planes/radius/local/resourcegroups/default/providers/Applications.Datastores/sqlDatabases/sql-db",
			ok:           true,
		},
		{
			name:         "invalid source",
			parentID:     "/planes/radius/local/resourcegroups/default/providers/Applications.Core/containers/sql-app-ctnr",
			source:       "invalid",
			parsedSource: "",
			ok:           false,
		},
		{
			name:         "direct route",
			parentID:     "/planes/radius/local/resourcegroups/default/providers/Applications.Core/containers/sql-app-ctnr",
			source:       "http://backend:8080",
			parsedSource: "/planes/radius/local/resourcegroups/default/providers/Applications.Core/containers/backend",
			ok:           true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parsedSource, ok := parseSource(tc.parentID, tc.source)
			require.Equal(t, tc.parsedSource, parsedSource)
			require.Equal(t, tc.ok, ok)
		})
	}
}
