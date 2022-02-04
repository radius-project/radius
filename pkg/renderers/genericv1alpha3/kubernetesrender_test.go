// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package genericv1alpha3

import (
	"testing"

	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	applicationName = "test-app"
	resourceName    = "test-generic"
)

func Test_KubernetesRender_Render(t *testing.T) {
	ctx := createContext(t)
	renderer := KubernetesRenderer{}

	input := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"properties": map[string]interface{}{
				"host": "hello.com",
				"port": "1234",
			},
			"secrets": map[string]interface{}{
				"connectionString": "connection123",
				"password":         "password123",
			},
		},
	}
	output, err := renderer.Render(ctx, renderers.RenderOptions{
		Resource:     input,
		Dependencies: map[string]renderers.RendererDependency{},
	})
	require.NoError(t, err)

	expected := renderers.RendererOutput{
		Resources: []outputresource.OutputResource{
			{
				LocalID:      outputresource.LocalIDGeneric,
				ResourceKind: resourcekinds.Kubernetes,
				Resource: &corev1.ConfigMap{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ConfigMap",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: applicationName,
						Labels:    kubernetes.MakeDescriptiveLabels(applicationName, resourceName),
					},
					Data: map[string]string{},
				},
			},
		},
		ComputedValues: map[string]renderers.ComputedValueReference{
			"host": {
				Value: "hello.com",
			},
			"port": {
				Value: "1234",
			},
		},
		SecretValues: map[string]renderers.SecretValueReference{
			"password": {
				LocalID:       outputresource.LocalIDScrapedSecret,
				ValueSelector: "password",
				Value:         "",
			},
			"connectionString": {
				LocalID:       outputresource.LocalIDScrapedSecret,
				ValueSelector: "connectionString",
				Value:         "",
			},
		},
	}
	assert.DeepEqual(t, expected, output)
}
