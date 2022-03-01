// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprsecretstorev1alpha3

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/handlers"
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

func Test_Render_Azure_Generic_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedAzureSecretStoreKindValues}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"kind":    "generic",
			"type":    "secretstores.azure.keyvault",
			"version": "v1",
			"metadata": map[string]interface{}{
				"vaultName": "testVault",
			},
		},
	}

	result, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.NoError(t, err)

	require.Len(t, result.Resources, 1)
	output := result.Resources[0]

	require.Equal(t, outputresource.LocalIDDaprSecretStoreGeneric, output.LocalID)
	require.Equal(t, resourcekinds.DaprSecretStoreGeneric, output.ResourceKind)

	metadata := map[string]interface{}{
		"vaultName": "testVault",
	}
	metadataSerialized, err := json.Marshal(metadata)
	require.NoError(t, err, "Could not serialize metadata")

	expected := map[string]string{
		handlers.KubernetesNameKey:       "test-resource",
		handlers.KubernetesNamespaceKey:  "test-app",
		handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
		handlers.KubernetesKindKey:       "Component",
		handlers.ResourceName:            "test-resource",
		handlers.GenericDaprTypeKey:      "secretstores.azure.keyvault",
		handlers.GenericDaprVersionKey:   "v1",
		handlers.GenericDaprMetadataKey:  string(metadataSerialized),
	}
	require.Equal(t, expected, output.Resource)
}

func Test_Render_UnsupportedKind(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedAzureSecretStoreKindValues}

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
	require.Equal(t, fmt.Sprintf("azure.keyvault is not supported. Supported kind values: %s", getAlphabeticallySortedKeys(SupportedAzureSecretStoreKindValues)), err.Error())
}

func Test_Render_Azure_Generic_MissingMetadata(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedAzureSecretStoreKindValues}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"kind":     "generic",
			"type":     "azure.keyvault",
			"version":  "v1",
			"metadata": map[string]interface{}{},
		},
	}

	_, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.Error(t, err)
	require.Equal(t, "No metadata specified for Dapr component of type azure.keyvault", err.Error())
}

func Test_Render_Azure_Generic_MissingType(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedAzureSecretStoreKindValues}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"kind":    "generic",
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

func Test_Render_Azure_Generic_MissingVersion(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedAzureSecretStoreKindValues}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"kind": "generic",
			"type": "azure.keyvault",
			"metadata": map[string]interface{}{
				"foo": "bar",
			},
		},
	}

	_, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.Error(t, err)
	require.Equal(t, "No Dapr component version specified for generic Dapr component", err.Error())
}

func Test_Render_Kubernetes_Generic_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedKubernetesSecretStoreKindValues}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: appName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"kind":    "generic",
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

	require.Equal(t, outputresource.LocalIDDaprSecretStoreGeneric, output.LocalID)
	require.Equal(t, resourcekinds.Kubernetes, output.ResourceKind)

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

func Test_Render_Kubernetes_Generic_MissingMetadata(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedKubernetesSecretStoreKindValues}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: appName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"kind":     "generic",
			"type":     "secretstores.kubernetes",
			"version":  daprSecretStoreVersion,
			"metadata": map[string]interface{}{},
		},
	}

	_, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.Error(t, err)
	require.Equal(t, "No metadata specified for Dapr component of type secretstores.kubernetes", err.Error())
}

func Test_Render_Kubernetes_Generic_MissingType(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedKubernetesSecretStoreKindValues}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: appName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"kind":    "generic",
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

func Test_Render_Kubernetes_Generic_MissingVersion(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedKubernetesSecretStoreKindValues}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: appName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"kind": "generic",
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
