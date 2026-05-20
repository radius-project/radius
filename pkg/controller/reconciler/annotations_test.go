/*
Copyright 2023.

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

package reconciler

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_readAnnotations(t *testing.T) {
	testDeploymentStatus := &deploymentStatus{
		Scope:       "/planes/radius/local/resourceGroups/controller-test",
		Application: "test-application",
		Environment: "test-environment",
		Container:   "/planes/radius/local/resourceGroups/controller-test/providers/Applications.Core/containers/test-container",
		Operation:   nil,
		Phrase:      deploymentPhraseReady,
	}

	dsm, err := json.Marshal(testDeploymentStatus)
	require.NoError(t, err)

	// invalidDeploymentStatus is missing a curly brace at the end of the JSON
	// so that an unmarshaling error can be triggered.
	invalidDeploymentStatus := []byte(`{"invalid": "json"`)

	_, invalidContainerIDErr := resources.ParseResource("not-a-resource-id")
	_, invalidScopeIDErr := resources.ParseScope("not-a-scope")

	tests := []struct {
		name        string
		deployment  *appsv1.Deployment
		annotations deploymentAnnotations
		err         error
	}{
		{
			name: "radius-disabled-with-annotation",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationRadiusEnabled: "false",
					},
				},
			},
			annotations: deploymentAnnotations{
				Configuration:     nil,
				ConfigurationHash: "",
				Status:            nil,
			},
			err: nil,
		},
		{
			name: "radius-disabled-empty-annotation-map",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			annotations: deploymentAnnotations{
				Configuration:     nil,
				ConfigurationHash: "",
				Status:            nil,
			},
			err: nil,
		},
		{
			name: "radius-disabled-no-annotations",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{},
			},
			annotations: deploymentAnnotations{
				Configuration:     nil,
				ConfigurationHash: "",
				Status:            nil,
			},
			err: nil,
		},
		{
			name: "radius-was-enabled-now-disabled-with-annotations",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationRadiusEnabled:           "false",
						AnnotationRadiusConfigurationHash: "configuration-hash",
						AnnotationRadiusStatus:            string(dsm),
					},
				},
			},
			annotations: deploymentAnnotations{
				Configuration:     nil,
				ConfigurationHash: "configuration-hash",
				Status:            testDeploymentStatus,
			},
			err: nil,
		},
		{
			name: "radius-enabled-with-annotations",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationRadiusEnabled:                              "true",
						AnnotationRadiusConfigurationHash:                    "configuration-hash",
						AnnotationRadiusStatus:                               string(dsm),
						AnnotationRadiusApplication:                          "test-application",
						AnnotationRadiusEnvironment:                          "test-environment",
						AnnotationRadiusConnectionPrefix + "test-connection": "test-connection-value",
					},
				},
			},
			annotations: deploymentAnnotations{
				Configuration: &deploymentConfiguration{
					Environment: "test-environment",
					Application: "test-application",
					Connections: map[string]string{
						"test-connection": "test-connection-value",
					},
				},
				ConfigurationHash: "configuration-hash",
				Status:            testDeploymentStatus,
			},
			err: nil,
		},
		{
			name: "status-unmarshal-error",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationRadiusEnabled:                              "true",
						AnnotationRadiusConfigurationHash:                    "configuration-hash",
						AnnotationRadiusStatus:                               string(invalidDeploymentStatus),
						AnnotationRadiusApplication:                          "test-application",
						AnnotationRadiusEnvironment:                          "test-environment",
						AnnotationRadiusConnectionPrefix + "test-connection": "test-connection-value",
					},
				},
			},
			annotations: deploymentAnnotations{
				ConfigurationHash: "configuration-hash",
			},
			err: fmt.Errorf("failed to unmarshal status annotation: %w",
				json.Unmarshal(invalidDeploymentStatus, &deploymentStatus{})),
		},
		{
			name: "status-invalid-container-id",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationRadiusStatus: `{"scope":"/planes/radius/local/resourceGroups/controller-test","container":"not-a-resource-id"}`,
					},
				},
			},
			annotations: deploymentAnnotations{ConfigurationHash: ""},
			err:         fmt.Errorf("invalid status annotation: invalid status.container: %w", invalidContainerIDErr),
		},
		{
			name: "status-scope-container-mismatch-allowed",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationRadiusStatus: `{"scope":"/planes/radius/local/resourceGroups/controller-test","container":"/planes/radius/local/resourceGroups/other/providers/Applications.Core/containers/test-container"}`,
					},
				},
			},
			// The reconciler intentionally produces this transitional state when the environment or
			// application changes: status.scope advances to the new scope while status.container still
			// references the previous container until it is deleted.
			annotations: deploymentAnnotations{
				ConfigurationHash: "",
				Status: &deploymentStatus{
					Scope:     "/planes/radius/local/resourceGroups/controller-test",
					Container: "/planes/radius/local/resourceGroups/other/providers/Applications.Core/containers/test-container",
				},
			},
			err: nil,
		},
		{
			name: "status-container-wrong-resource-type",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationRadiusStatus: `{"scope":"/planes/radius/local/resourceGroups/controller-test","container":"/planes/radius/local/resourceGroups/controller-test/providers/Applications.Core/applications/test-app"}`,
					},
				},
			},
			annotations: deploymentAnnotations{ConfigurationHash: ""},
			err:         fmt.Errorf("invalid status annotation: status.container type %q is not %q", "Applications.Core/applications", applicationsCoreContainersResourceType),
		},
		{
			name: "status-only-scope-set",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationRadiusStatus: `{"scope":"/planes/radius/local/resourceGroups/controller-test"}`,
					},
				},
			},
			annotations: deploymentAnnotations{
				ConfigurationHash: "",
				Status: &deploymentStatus{
					Scope: "/planes/radius/local/resourceGroups/controller-test",
				},
			},
			err: nil,
		},
		{
			name: "status-only-container-set",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationRadiusStatus: `{"container":"/planes/radius/local/resourceGroups/controller-test/providers/Applications.Core/containers/test-container"}`,
					},
				},
			},
			annotations: deploymentAnnotations{ConfigurationHash: ""},
			err:         fmt.Errorf("invalid status annotation: status.scope must be set when status.container is set"),
		},
		{
			name: "status-invalid-scope-only",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationRadiusStatus: `{"scope":"not-a-scope"}`,
					},
				},
			},
			annotations: deploymentAnnotations{ConfigurationHash: ""},
			err:         fmt.Errorf("invalid status annotation: invalid status.scope: %w", invalidScopeIDErr),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotations, err := readAnnotations(tt.deployment)
			if tt.err == nil {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tt.err.Error())
			}
			require.Equal(t, tt.annotations, annotations)
		})
	}
}
