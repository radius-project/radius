// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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
