package reconciler

import (
	"testing"

	v20231001preview "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

func TestGenerateDeploymentResourceName(t *testing.T) {
	tests := []struct {
		name       string
		resourceId string
		want       string
		wantErr    bool
	}{
		{
			name:       "valid resource ID",
			resourceId: "/subscriptions/123/resourceGroups/myResourceGroup/providers/Microsoft.Web/sites/mySite",
			want:       "mySite",
			wantErr:    false,
		},
		{
			name:       "invalid resource ID",
			resourceId: "invalidResourceId",
			want:       "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := generateDeploymentResourceName(tt.resourceId)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, got)
			}
		})
	}
}

func TestConvertToARMJSONParameters(t *testing.T) {
	tests := []struct {
		name       string
		parameters map[string]string
		want       map[string]map[string]string
	}{
		{
			name: "single parameter",
			parameters: map[string]string{
				"param1": "value1",
			},
			want: map[string]map[string]string{
				"param1": {
					"value": "value1",
				},
			},
		},
		{
			name: "multiple parameters",
			parameters: map[string]string{
				"param1": "value1",
				"param2": "value2",
			},
			want: map[string]map[string]string{
				"param1": {
					"value": "value1",
				},
				"param2": {
					"value": "value2",
				},
			},
		},
		{
			name:       "empty parameters",
			parameters: map[string]string{},
			want:       map[string]map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertToARMJSONParameters(tt.parameters)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestMakeKubernetesDeploymentResourceID(t *testing.T) {
	got := makeKubernetesDeploymentResourceID("current-namespace", "current-app")
	require.Equal(t, "/planes/kubernetes/local/namespaces/current-namespace/providers/apps/Deployment/current-app", got)
}

func TestContainerHasResourceReference(t *testing.T) {
	tests := []struct {
		name             string
		container        *v20231001preview.ContainerResource
		expectedResource string
		want             bool
	}{
		{
			name: "has matching resource reference",
			container: &v20231001preview.ContainerResource{
				Properties: &v20231001preview.ContainerProperties{
					Resources: []*v20231001preview.ResourceReference{{
						ID: to.Ptr("/planes/kubernetes/local/namespaces/current-namespace/providers/apps/Deployment/current-app"),
					}},
				},
			},
			expectedResource: "/planes/kubernetes/local/namespaces/current-namespace/providers/apps/Deployment/current-app",
			want:             true,
		},
		{
			name: "missing matching reference",
			container: &v20231001preview.ContainerResource{
				Properties: &v20231001preview.ContainerProperties{
					Resources: []*v20231001preview.ResourceReference{{
						ID: to.Ptr("/planes/kubernetes/local/namespaces/other-namespace/providers/apps/Deployment/other-app"),
					}},
				},
			},
			expectedResource: "/planes/kubernetes/local/namespaces/current-namespace/providers/apps/Deployment/current-app",
			want:             false,
		},
		{
			name:             "nil container",
			container:        nil,
			expectedResource: "/planes/kubernetes/local/namespaces/current-namespace/providers/apps/Deployment/current-app",
			want:             false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containerHasResourceReference(tt.container, tt.expectedResource)
			require.Equal(t, tt.want, got)
		})
	}
}
