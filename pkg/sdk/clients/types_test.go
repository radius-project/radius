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

package clients

import "testing"

func TestDeploymentEngineURL(t *testing.T) {
	type args struct {
		baseURI    string
		resourceID string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "baseURI ends with /",
			args: args{
				baseURI:    "https://management.azure.com/",
				resourceID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/rg/providers/Microsoft.ContainerService/managedClusters/cluster",
			},
			want: "https://management.azure.com/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/rg/providers/Microsoft.ContainerService/managedClusters/cluster",
		},
		{
			name: "baseURI does not end with /",
			args: args{
				baseURI:    "https://management.azure.com",
				resourceID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/rg/providers/Microsoft.ContainerService/managedClusters/cluster",
			},
			want: "https://management.azure.com/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/rg/providers/Microsoft.ContainerService/managedClusters/cluster",
		},
		{
			name: "resourceID starts with /",
			args: args{
				baseURI:    "https://management.azure.com/",
				resourceID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/rg/providers/Microsoft.ContainerService/managedClusters/cluster",
			},
			want: "https://management.azure.com/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/rg/providers/Microsoft.ContainerService/managedClusters/cluster",
		},
		{
			name: "resourceID does not start with /",
			args: args{
				baseURI:    "https://management.azure.com/",
				resourceID: "subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/rg/providers/Microsoft.ContainerService/managedClusters/cluster",
			},
			want: "https://management.azure.com/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/rg/providers/Microsoft.ContainerService/managedClusters/cluster",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DeploymentEngineURL(tt.args.baseURI, tt.args.resourceID); got != tt.want {
				t.Errorf("DeploymentEngineURL() = %v, want %v", got, tt.want)
			}
		})
	}
}
