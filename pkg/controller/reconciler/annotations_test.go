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

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_readAnnotations(t *testing.T) {
	testDeploymentStatus := &deploymentStatus{
		Scope:       "/planes/radius/local/resourceGroups/controller-test",
		Application: "test-application",
		Environment: "test-environment",
		Container:   "test-container",
		Operation:   nil,
		Phrase:      deploymentPhraseReady,
	}

	dsm, err := json.Marshal(testDeploymentStatus)
	require.NoError(t, err)

	// invalidDeploymentStatus is missing a curly brace at the end of the JSON
	// so that an unmarshaling error can be triggered.
	invalidDeploymentStatus := []byte(`{"invalid": "json"`)

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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotations, err := readAnnotations(tt.deployment)
			require.Equal(t, tt.err, err)
			require.Equal(t, tt.annotations, annotations)
		})
	}
}
