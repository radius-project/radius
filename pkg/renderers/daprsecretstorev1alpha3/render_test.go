// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprsecretstorev1alpha3

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	appName                = "test-app"
	resourceName           = "test-resource"
	daprVersion            = "dapr.io/v1alpha1"
	k8sKind                = "Component"
	daprSecretStoreVersion = "v1"
)

func createContext(t *testing.T) context.Context {
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
}

func Test_Render_UnsupportedKind(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedSecretStoreKindValues}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"kind": "azure.keyvault",
		},
	}

	_, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.Error(t, err)
	require.Equal(t, fmt.Sprintf("azure.keyvault is not supported. Supported kind values: %s", getAlphabeticallySortedKeys(SupportedSecretStoreKindValues)), err.Error())
}

func Test_Render_Generic_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedSecretStoreKindValues}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: appName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"kind":    resourcekinds.DaprGeneric,
			"type":    "secretstores.kubernetes",
			"version": "v1",
			"metadata": map[string]interface{}{
				"foo": "bar",
			},
		},
	}

	result, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.NoError(t, err)

	require.Len(t, result.Resources, 1)
	output := result.Resources[0]

	require.Equal(t, outputresource.LocalIDDaprComponent, output.LocalID)
	require.Equal(t, resourcekinds.DaprComponent, output.ResourceType.Type)

	expected := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": daprVersion,
			"kind":       k8sKind,
			"metadata": map[string]interface{}{
				"namespace": appName,
				"name":      resourceName,
				"labels":    kubernetes.MakeDescriptiveLabels(appName, resourceName),
			},
			"spec": map[string]interface{}{
				"type":    "secretstores.kubernetes",
				"version": "v1",
				"metadata": []map[string]interface{}{
					{
						"name":  "foo",
						"value": "bar",
					},
				},
			},
		},
	}
	require.Equal(t, &expected, output.Resource)
}

func Test_Render_Generic_MissingMetadata(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedSecretStoreKindValues}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: appName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"kind":     resourcekinds.DaprGeneric,
			"type":     "secretstores.kubernetes",
			"version":  daprSecretStoreVersion,
			"metadata": map[string]interface{}{},
		},
	}

	_, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.Error(t, err)
	require.Equal(t, "No metadata specified for Dapr component of type secretstores.kubernetes", err.Error())
}

func Test_Render_Generic_MissingType(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedSecretStoreKindValues}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: appName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"kind":    resourcekinds.DaprGeneric,
			"version": "v1",
			"metadata": map[string]interface{}{
				"foo": "bar",
			},
		},
	}

	_, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.Error(t, err)
	require.Equal(t, "No type specified for generic Dapr component", err.Error())
}

func Test_Render_Generic_MissingVersion(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedSecretStoreKindValues}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: appName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"kind": resourcekinds.DaprGeneric,
			"type": "secretstores.kubernetes",
			"metadata": map[string]interface{}{
				"foo": "bar",
			},
		},
	}

	_, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: dependencies})

	require.Error(t, err)
	require.Equal(t, "No Dapr component version specified for generic Dapr component", err.Error())
}
