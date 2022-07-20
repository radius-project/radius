// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprsecretstores

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/renderers"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	applicationName        = "test-app"
	applicationID          = "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/test-app"
	environmentID          = "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/test-env"
	resourceName           = "test-secret-store"
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

	resource := datamodel.DaprSecretStore{
		TrackedResource: v1.TrackedResource{
			ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Connector/daprSecretStores/test-secret-store",
			Name: resourceName,
			Type: "Applications.Connector/daprSecretStores",
		},
		Properties: datamodel.DaprSecretStoreProperties{
			Application: applicationID,
			Environment: environmentID,
			Type:        ResourceType,
			Kind:        "azure.keyvault",
		},
	}

	_, err := renderer.Render(ctx, &resource, renderers.RenderOptions{})
	require.Error(t, err)
	require.Equal(t, fmt.Sprintf("azure.keyvault is not supported. Supported kind values: %s", getAlphabeticallySortedKeys(SupportedSecretStoreKindValues)), err.Error())
}

func Test_Render_Generic_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedSecretStoreKindValues}
	resource := datamodel.DaprSecretStore{
		TrackedResource: v1.TrackedResource{
			ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Connector/daprSecretStores/test-secret-store",
			Name: resourceName,
			Type: "Applications.Connector/daprSecretStores",
		},
		Properties: datamodel.DaprSecretStoreProperties{
			Application: applicationID,
			Environment: environmentID,
			Type:        ResourceType,
			Kind:        resourcekinds.DaprGeneric,
			Version:     daprSecretStoreVersion,
			Metadata: map[string]interface{}{
				"foo": "bar",
			},
		},
	}

	result, err := renderer.Render(ctx, &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.NoError(t, err)

	require.Len(t, result.Resources, 1)
	outputResource := result.Resources[0]

	require.Equal(t, outputresource.LocalIDDaprComponent, outputResource.LocalID)
	require.Equal(t, resourcekinds.DaprComponent, outputResource.ResourceType.Type)
	expectedComputedValues := map[string]renderers.ComputedValueReference{
		"secretStoreName": {
			Value: "test-app-test-secret-store",
		},
	}
	require.Equal(t, expectedComputedValues, result.ComputedValues)

	expected := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": daprVersion,
			"kind":       k8sKind,
			"metadata": map[string]interface{}{
				"namespace": "radius-test",
				"name":      kubernetes.MakeResourceName(applicationName, resourceName),
				"labels":    kubernetes.MakeDescriptiveLabels(applicationName, resourceName),
			},
			"spec": map[string]interface{}{
				"type":    "Applications.Connector/daprSecretStores",
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
	require.Equal(t, &expected, outputResource.Resource)
}

func Test_Render_Generic_MissingMetadata(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedSecretStoreKindValues}
	resource := datamodel.DaprSecretStore{
		TrackedResource: v1.TrackedResource{
			ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Connector/daprSecretStores/test-secret-store",
			Name: resourceName,
			Type: "Applications.Connector/daprSecretStores",
		},
		Properties: datamodel.DaprSecretStoreProperties{
			Application: applicationID,
			Environment: environmentID,
			Type:        "secretstores.kubernetes",
			Kind:        resourcekinds.DaprGeneric,
			Version:     daprSecretStoreVersion,
			Metadata:    map[string]interface{}{},
		},
	}
	_, err := renderer.Render(ctx, &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.Error(t, err)
	require.Equal(t, "No metadata specified for Dapr component of type secretstores.kubernetes", err.Error())
}

func Test_Render_Generic_MissingType(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedSecretStoreKindValues}
	resource := datamodel.DaprSecretStore{
		TrackedResource: v1.TrackedResource{
			ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Connector/daprSecretStores/test-secret-store",
			Name: resourceName,
			Type: "Applications.Connector/daprSecretStores",
		},
		Properties: datamodel.DaprSecretStoreProperties{
			Application: applicationID,
			Environment: environmentID,
			Kind:        resourcekinds.DaprGeneric,
			Version:     daprSecretStoreVersion,
			Metadata: map[string]interface{}{
				"foo": "bar",
			},
		},
	}

	_, err := renderer.Render(ctx, &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.Error(t, err)
	require.Equal(t, "No type specified for generic Dapr component", err.Error())
}

func Test_Render_Generic_MissingVersion(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedSecretStoreKindValues}
	resource := datamodel.DaprSecretStore{
		TrackedResource: v1.TrackedResource{
			ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Connector/daprSecretStores/test-secret-store",
			Name: resourceName,
			Type: "Applications.Connector/daprSecretStores",
		},
		Properties: datamodel.DaprSecretStoreProperties{
			Application: applicationID,
			Environment: environmentID,
			Type:        "secretstores.kubernetes",
			Kind:        resourcekinds.DaprGeneric,
			Metadata: map[string]interface{}{
				"foo": "bar",
			},
		},
	}

	_, err := renderer.Render(ctx, &resource, renderers.RenderOptions{Namespace: "radius-test"})

	require.Error(t, err)
	require.Equal(t, "No Dapr component version specified for generic Dapr component", err.Error())
}
