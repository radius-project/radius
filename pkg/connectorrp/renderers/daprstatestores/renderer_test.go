// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestores

import (
	"context"
	"fmt"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	connectorrprenderer "github.com/project-radius/radius/pkg/connectorrp/renderers"
	"github.com/project-radius/radius/pkg/handlers"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	applicationName       = "test-app"
	applicationID         = "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/test-app"
	environmentID         = "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/test-env"
	resourceName          = "test-state-store"
	daprVersion           = "dapr.io/v1alpha1"
	k8sKind               = "Component"
	stateStoreType        = "state.zookeeper"
	daprStateStoreVersion = "v1"
)

func Test_Render_Success(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprStateStore{
		TrackedResource: v1.TrackedResource{
			ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Connector/daprStateStores/test-state-store",
			Name: resourceName,
			Type: "Applications.Connector/daprStateStores",
		},
		Properties: datamodel.DaprStateStoreProperties{
			Application: applicationID,
			Environment: environmentID,
			Kind:        datamodel.DaprStateStoreKindAzureTableStorage,
			DaprStateStoreAzureTableStorage: datamodel.DaprStateStoreAzureTableStorageResourceProperties{
				Resource: "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Storage/storageAccounts/test-account/tableServices/default/tables/mytable",
			},
		},
	}
	renderer.StateStores = SupportedStateStoreKindValues
	result, err := renderer.Render(context.Background(), &resource, connectorrprenderer.RenderOptions{Namespace: "radius-test"})
	require.NoError(t, err)

	require.Len(t, result.Resources, 1)
	output := result.Resources[0]

	require.Equal(t, outputresource.LocalIDDaprStateStoreAzureStorage, output.LocalID)
	require.Equal(t, resourcekinds.DaprStateStoreAzureStorage, output.ResourceType.Type)

	expected := map[string]string{
		handlers.KubernetesNameKey:       "test-state-store",
		handlers.KubernetesNamespaceKey:  "radius-test",
		handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
		handlers.KubernetesKindKey:       "Component",
		handlers.ApplicationName:         applicationName,
		handlers.ResourceIDKey:           "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Storage/storageAccounts/test-account/tableServices/default/tables/mytable",
		handlers.StorageAccountNameKey:   "test-account",
		handlers.ResourceName:            "mytable",
	}
	require.Equal(t, expected, output.Resource)
}

func Test_Render_InvalidResourceType(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprStateStore{
		TrackedResource: v1.TrackedResource{
			ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Connector/daprStateStores/test-state-store",
			Name: resourceName,
			Type: "Applications.Connector/daprStateStores",
		},
		Properties: datamodel.DaprStateStoreProperties{
			Application: applicationID,
			Environment: environmentID,
			Kind:        "state.azure.tablestorage",
			DaprStateStoreAzureTableStorage: datamodel.DaprStateStoreAzureTableStorageResourceProperties{
				Resource: "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.SomethingElse/test-storageAccounts/test-account",
			},
		},
	}
	renderer.StateStores = SupportedStateStoreKindValues
	_, err := renderer.Render(context.Background(), &resource, connectorrprenderer.RenderOptions{Namespace: "radius-test"})
	require.Error(t, err)
	require.Equal(t, "the 'resource' field must refer to a Storage Table", err.Error())
}

func Test_Render_SpecifiesUmanagedWithoutResource(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprStateStore{
		TrackedResource: v1.TrackedResource{
			ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Connector/daprStateStores/test-state-store",
			Name: resourceName,
			Type: "Applications.Connector/daprStateStores",
		},
		Properties: datamodel.DaprStateStoreProperties{
			Application:                     applicationID,
			Environment:                     environmentID,
			Kind:                            "state.azure.tablestorage",
			DaprStateStoreAzureTableStorage: datamodel.DaprStateStoreAzureTableStorageResourceProperties{},
		},
	}
	renderer.StateStores = SupportedStateStoreKindValues
	_, err := renderer.Render(context.Background(), &resource, connectorrprenderer.RenderOptions{Namespace: "radius-test"})
	require.Error(t, err)
	require.Equal(t, renderers.ErrResourceMissingForResource.Error(), err.Error())
}

func Test_Render_UnsupportedKind(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprStateStore{
		TrackedResource: v1.TrackedResource{
			ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Connector/daprStateStores/test-state-store",
			Name: resourceName,
			Type: "Applications.Connector/daprStateStores",
		},
		Properties: datamodel.DaprStateStoreProperties{
			Application: applicationID,
			Environment: environmentID,
			Kind:        "state.azure.cosmosdb",
			DaprStateStoreAzureTableStorage: datamodel.DaprStateStoreAzureTableStorageResourceProperties{
				Resource: "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.SomethingElse/test-storageAccounts/test-account",
			},
		},
	}
	renderer.StateStores = SupportedStateStoreKindValues
	_, err := renderer.Render(context.Background(), &resource, connectorrprenderer.RenderOptions{Namespace: "radius-test"})
	require.Error(t, err)
	require.Equal(t, fmt.Sprintf("state.azure.cosmosdb is not supported. Supported kind values: %s", getAlphabeticallySortedKeys(SupportedStateStoreKindValues)), err.Error())
}

func Test_Render_Generic_Success(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprStateStore{
		TrackedResource: v1.TrackedResource{
			ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Connector/daprStateStores/test-state-store",
			Name: resourceName,
			Type: "Applications.Connector/daprStateStores",
		},
		Properties: datamodel.DaprStateStoreProperties{
			Application: applicationID,
			Environment: environmentID,
			Kind:        datamodel.DaprStateStoreKindGeneric,
			DaprStateStoreGeneric: datamodel.DaprStateStoreGenericResourceProperties{
				Type:    stateStoreType,
				Version: daprStateStoreVersion,
				Metadata: map[string]interface{}{
					"foo": "bar",
				},
			},
		},
	}
	renderer.StateStores = SupportedStateStoreKindValues
	result, err := renderer.Render(context.Background(), &resource, connectorrprenderer.RenderOptions{Namespace: "radius-test"})
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
				"namespace": "radius-test",
				"name":      kubernetes.MakeResourceName(applicationName, resourceName),
				"labels":    kubernetes.MakeDescriptiveLabels(applicationName, resourceName),
			},
			"spec": map[string]interface{}{
				"type":    stateStoreType,
				"version": daprStateStoreVersion,
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
	renderer := Renderer{}
	resource := datamodel.DaprStateStore{
		TrackedResource: v1.TrackedResource{
			ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Connector/daprStateStores/test-state-store",
			Name: resourceName,
			Type: "Applications.Connector/daprStateStores",
		},
		Properties: datamodel.DaprStateStoreProperties{
			Application: applicationID,
			Environment: environmentID,
			Kind:        "generic",
			DaprStateStoreGeneric: datamodel.DaprStateStoreGenericResourceProperties{
				Type:     stateStoreType,
				Metadata: map[string]interface{}{},
				Version:  daprStateStoreVersion,
			},
		},
	}
	renderer.StateStores = SupportedStateStoreKindValues
	_, err := renderer.Render(context.Background(), &resource, connectorrprenderer.RenderOptions{Namespace: "radius-test"})
	require.Error(t, err)
	require.Equal(t, "No metadata specified for Dapr component of type state.zookeeper", err.Error())
}

func Test_Render_Generic_MissingType(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprStateStore{
		TrackedResource: v1.TrackedResource{
			ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Connector/daprStateStores/test-state-store",
			Name: resourceName,
			Type: "Applications.Connector/daprStateStores",
		},
		Properties: datamodel.DaprStateStoreProperties{
			Application: applicationID,
			Environment: environmentID,
			Kind:        "generic",
			DaprStateStoreGeneric: datamodel.DaprStateStoreGenericResourceProperties{
				Metadata: map[string]interface{}{
					"foo": "bar",
				},
				Version: daprStateStoreVersion,
			},
		},
	}
	renderer.StateStores = SupportedStateStoreKindValues
	_, err := renderer.Render(context.Background(), &resource, connectorrprenderer.RenderOptions{Namespace: "radius-test"})
	require.Error(t, err)
	require.Equal(t, "No type specified for generic Dapr component", err.Error())
}

func Test_Render_Generic_MissingVersion(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprStateStore{
		TrackedResource: v1.TrackedResource{
			ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Connector/daprStateStores/test-state-store",
			Name: resourceName,
			Type: "Applications.Connector/daprStateStores",
		},
		Properties: datamodel.DaprStateStoreProperties{
			Application: applicationID,
			Environment: environmentID,
			Kind:        "generic",
			DaprStateStoreGeneric: datamodel.DaprStateStoreGenericResourceProperties{
				Metadata: map[string]interface{}{
					"foo": "bar",
				},
				Type: stateStoreType,
			},
		},
	}
	renderer.StateStores = SupportedStateStoreKindValues
	_, err := renderer.Render(context.Background(), &resource, connectorrprenderer.RenderOptions{Namespace: "radius-test"})

	require.Error(t, err)
	require.Equal(t, "No Dapr component version specified for generic Dapr component", err.Error())
}
