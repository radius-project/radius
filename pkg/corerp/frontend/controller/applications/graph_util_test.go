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
	"reflect"
	"testing"

	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
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
			if got := isResourceInEnvironment(tt.args.ctx, tt.args.resource, tt.args.environmentName); got != tt.want {
				t.Errorf("isResourceInEnvironment() = %v, want %v", got, tt.want)
			}
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
			if got := isResourceInApplication(tt.args.ctx, tt.args.resource, tt.args.applicationName); got != tt.want {
				t.Errorf("isResourceInApplication() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_compute(t *testing.T) {
	gatewayID := "/planes/radius/local/resourcegroups/default/providers/Applications.Core/gateways/http-gtwy-gtwy"
	gatewayName := "http-gtwy-gtwy"
	gatewayType := "Applications.Core/gateways"

	httpRouteID := "/planes/radius/local/resourcegroups/default/providers/Applications.Core/httpRoutes/http-route-http-route"
	httpRouteName := "http-route-http-route"
	httpRouteType := "Applications.Core/httpRoutes"


	type args struct {
		applicationName      string
		applicationResources []generated.GenericResource
		environmentResources []generated.GenericResource
	}
	tests := []struct {
		name string
		args args
		want *ApplicationGraphResponse
	}{
		{
			name: "compute graph",
			args: args{
				applicationName: "myapp",
				applicationResources: []generated.GenericResource{
					{
						ID: &gatewayID,
						Properties: map[string]interface{}{
							"application": "/planes/radius/local/resourcegroups/default/providers/Applications.Core/Applications/myapp",
							"provisioningState": "Succeeded",
						},
						Name: &gatewayName,
						Type: &gatewayType,

					},
					{
						ID: &httpRouteID,
						Properties: map[string]interface{}{
							"application": "/planes/radius/local/resourcegroups/default/providers/Applications.Core/Applications/myapp",
							"provisioningState": "Succeeded",
						},
						Name: &httpRouteName,
						Type: &httpRouteType,
					},
					

				},
				environmentResources: []generated.GenericResource{},
		}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := computeGraph(tt.args.applicationName, tt.args.applicationResources, tt.args.environmentResources); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("compute() = %v, want %v", got, tt.want)
			}
		})
	}
}
