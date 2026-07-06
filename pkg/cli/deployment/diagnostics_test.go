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

package deployment

import (
	"context"
	"errors"
	"testing"

	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
)

func Test_findNamespaceOfContainerPreview(t *testing.T) {
	const resourceName = "myapp-frontend"
	deploymentID := "/planes/kubernetes/local/namespaces/my-namespace/providers/apps/Deployment/myapp-frontend"

	containerWith := func(properties map[string]any) generated.GenericResource {
		return generated.GenericResource{Properties: properties}
	}

	tests := []struct {
		name      string
		resource  generated.GenericResource
		getErr    error
		expected  string
		expectErr bool
	}{
		{
			name: "resolves namespace from output resource deployment id",
			resource: containerWith(map[string]any{
				"status": map[string]any{
					"outputResources": []any{
						map[string]any{"id": deploymentID},
					},
				},
			}),
			expected: "my-namespace",
		},
		{
			name: "skips non-namespaced output resources and resolves the namespaced one",
			resource: containerWith(map[string]any{
				"status": map[string]any{
					"outputResources": []any{
						map[string]any{"id": "/planes/radius/local/resourceGroups/default/providers/Applications.Core/secretStores/secret"},
						map[string]any{"id": deploymentID},
					},
				},
			}),
			expected: "my-namespace",
		},
		{
			name:      "errors when GetResource fails",
			getErr:    errors.New("not found"),
			expectErr: true,
		},
		{
			name:      "errors when status is missing",
			resource:  containerWith(map[string]any{}),
			expectErr: true,
		},
		{
			name: "errors when outputResources is missing",
			resource: containerWith(map[string]any{
				"status": map[string]any{},
			}),
			expectErr: true,
		},
		{
			name: "errors when no output resource encodes a namespace",
			resource: containerWith(map[string]any{
				"status": map[string]any{
					"outputResources": []any{
						map[string]any{"id": "/planes/radius/local/resourceGroups/default/providers/Applications.Core/secretStores/secret"},
					},
				},
			}),
			expectErr: true,
		},
		{
			name: "errors when output resource id is malformed",
			resource: containerWith(map[string]any{
				"status": map[string]any{
					"outputResources": []any{
						map[string]any{"id": "not-a-valid-resource-id"},
					},
				},
			}),
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			managementClient := clients.NewMockApplicationsManagementClient(ctrl)
			managementClient.EXPECT().
				GetResource(gomock.Any(), previewContainerResourceType, resourceName).
				Return(tt.resource, tt.getErr)

			dc := &ARMDiagnosticsClient{Preview: true, ManagementClient: managementClient}
			namespace, err := dc.findNamespaceOfContainerPreview(context.Background(), resourceName)

			if tt.expectErr {
				require.Error(t, err)
				require.Empty(t, namespace)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expected, namespace)
		})
	}
}

func Test_defaultContainerName(t *testing.T) {
	podWithContainers := func(names ...string) *corev1.Pod {
		pod := &corev1.Pod{}
		for _, name := range names {
			pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{Name: name})
		}
		return pod
	}

	t.Run("preview pod with a single container uses that container name", func(t *testing.T) {
		dc := &ARMDiagnosticsClient{Preview: true}
		require.Equal(t, "main", defaultContainerName(dc, podWithContainers("main")))
	})

	t.Run("preview pod with multiple containers returns empty so caller requires --container", func(t *testing.T) {
		dc := &ARMDiagnosticsClient{Preview: true}
		require.Equal(t, "", defaultContainerName(dc, podWithContainers("main", "sidecar")))
	})

	t.Run("preview pod with no containers returns empty", func(t *testing.T) {
		dc := &ARMDiagnosticsClient{Preview: true}
		require.Equal(t, "", defaultContainerName(dc, podWithContainers()))
	})

	t.Run("legacy pod uses the radius resource label as the container name", func(t *testing.T) {
		dc := &ARMDiagnosticsClient{Preview: false}
		pod := podWithContainers("anything")
		pod.Labels = map[string]string{"radapp.io/resource": "frontend"}
		require.Equal(t, "frontend", defaultContainerName(dc, pod))
	})
}
