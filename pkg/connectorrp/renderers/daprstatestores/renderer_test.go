// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestores

import (
	"context"
	"fmt"
	"testing"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/handlers"
	"github.com/project-radius/radius/pkg/connectorrp/renderers"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/rp/outputresource"
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
			BasicResourceProperties: v1.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Kind: datamodel.DaprStateStoreKindAzureTableStorage,
			DaprStateStoreAzureTableStorage: datamodel.DaprStateStoreAzureTableStorageResourceProperties{
				Resource: "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Storage/storageAccounts/test-account/tableServices/default/tables/mytable",
			},
		},
	}
	renderer.StateStores = SupportedStateStoreKindValues
	result, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.NoError(t, err)

	require.Len(t, result.Resources, 1)
	output := result.Resources[0]

	require.Equal(t, outputresource.LocalIDDaprStateStoreAzureStorage, output.LocalID)
	require.Equal(t, resourcekinds.DaprStateStoreAzureStorage, output.ResourceType.Type)
	require.Equal(t, kubernetes.MakeResourceName(applicationName, resourceName), result.ComputedValues[renderers.ComponentNameKey].Value)

	expected := map[string]string{
		handlers.KubernetesNameKey:       "test-state-store",
		handlers.KubernetesNamespaceKey:  "radius-test",
		handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
		handlers.KubernetesKindKey:       "Component",
		handlers.ApplicationName:         applicationName,
		handlers.ResourceIDKey:           "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Storage/storageAccounts/test-account/tableServices/default/tables/mytable",
		handlers.StorageAccountNameKey:   "test-account",
		handlers.ResourceName:            "test-state-store",
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
			BasicResourceProperties: v1.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Kind: "state.azure.tablestorage",
			DaprStateStoreAzureTableStorage: datamodel.DaprStateStoreAzureTableStorageResourceProperties{
				Resource: "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.SomethingElse/test-storageAccounts/test-account",
			},
		},
	}
	renderer.StateStores = SupportedStateStoreKindValues
	_, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*conv.ErrClientRP).Code)
	require.Equal(t, "the 'resource' field must refer to a Storage Table", err.(*conv.ErrClientRP).Message)
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
			BasicResourceProperties: v1.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Kind:                            "state.azure.tablestorage",
			DaprStateStoreAzureTableStorage: datamodel.DaprStateStoreAzureTableStorageResourceProperties{},
		},
	}
	renderer.StateStores = SupportedStateStoreKindValues
	_, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*conv.ErrClientRP).Code)
	require.Equal(t, renderers.ErrResourceMissingForResource.Error(), err.(*conv.ErrClientRP).Message)
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
			BasicResourceProperties: v1.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Kind: "state.azure.cosmosdb",
			DaprStateStoreAzureTableStorage: datamodel.DaprStateStoreAzureTableStorageResourceProperties{
				Resource: "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.SomethingElse/test-storageAccounts/test-account",
			},
		},
	}
	renderer.StateStores = SupportedStateStoreKindValues
	_, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*conv.ErrClientRP).Code)
	require.Equal(t, fmt.Sprintf("state.azure.cosmosdb is not supported. Supported kind values: %s", getAlphabeticallySortedKeys(SupportedStateStoreKindValues)), err.(*conv.ErrClientRP).Message)
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
			BasicResourceProperties: v1.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Kind: datamodel.DaprStateStoreKindGeneric,
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
	result, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.NoError(t, err)
	require.Len(t, result.Resources, 1)
	output := result.Resources[0]

	require.Equal(t, outputresource.LocalIDDaprComponent, output.LocalID)
	require.Equal(t, resourcekinds.DaprComponent, output.ResourceType.Type)
	require.Equal(t, kubernetes.MakeResourceName(applicationName, resourceName), result.ComputedValues[renderers.ComponentNameKey].Value)

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
			BasicResourceProperties: v1.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Kind: "generic",
			DaprStateStoreGeneric: datamodel.DaprStateStoreGenericResourceProperties{
				Type:    stateStoreType,
				Version: daprStateStoreVersion,
			},
		},
	}
	renderer.StateStores = SupportedStateStoreKindValues
	_, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*conv.ErrClientRP).Code)
	require.Equal(t, "No metadata specified for Dapr component of type state.zookeeper", err.(*conv.ErrClientRP).Message)
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
			BasicResourceProperties: v1.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Kind: "generic",
			DaprStateStoreGeneric: datamodel.DaprStateStoreGenericResourceProperties{
				Metadata: map[string]interface{}{
					"foo": "bar",
				},
				Version: daprStateStoreVersion,
			},
		},
	}
	renderer.StateStores = SupportedStateStoreKindValues
	_, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*conv.ErrClientRP).Code)
	require.Equal(t, "No type specified for generic Dapr component", err.(*conv.ErrClientRP).Message)
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
			BasicResourceProperties: v1.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Kind: "generic",
			DaprStateStoreGeneric: datamodel.DaprStateStoreGenericResourceProperties{
				Metadata: map[string]interface{}{
					"foo": "bar",
				},
				Type: stateStoreType,
			},
		},
	}
	renderer.StateStores = SupportedStateStoreKindValues
	_, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})

	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*conv.ErrClientRP).Code)
	require.Equal(t, "No Dapr component version specified for generic Dapr component", err.(*conv.ErrClientRP).Message)
}

func Test_Render_InvalidApplicationID(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprStateStore{
		TrackedResource: v1.TrackedResource{
			ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Connector/daprStateStores/test-state-store",
			Name: resourceName,
			Type: "Applications.Connector/daprStateStores",
		},
		Properties: datamodel.DaprStateStoreProperties{
			BasicResourceProperties: v1.BasicResourceProperties{
				Application: "invalid-app-id",
				Environment: environmentID,
			},
			Kind: datamodel.DaprStateStoreKindAzureTableStorage,
			DaprStateStoreAzureTableStorage: datamodel.DaprStateStoreAzureTableStorageResourceProperties{
				Resource: "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Storage/storageAccounts/test-account/tableServices/default/tables/mytable",
			},
		},
	}
	renderer.StateStores = SupportedStateStoreKindValues
	_, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*conv.ErrClientRP).Code)
	require.Equal(t, "failed to parse application from the property: 'invalid-app-id' is not a valid resource id", err.(*conv.ErrClientRP).Message)
}

func Test_Render_EmptyApplicationID(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprStateStore{
		TrackedResource: v1.TrackedResource{
			ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Connector/daprStateStores/test-state-store",
			Name: resourceName,
			Type: "Applications.Connector/daprStateStores",
		},
		Properties: datamodel.DaprStateStoreProperties{
			BasicResourceProperties: v1.BasicResourceProperties{
				Environment: environmentID,
			},
			Kind: datamodel.DaprStateStoreKindAzureTableStorage,
			DaprStateStoreAzureTableStorage: datamodel.DaprStateStoreAzureTableStorageResourceProperties{
				Resource: "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Storage/storageAccounts/test-account/tableServices/default/tables/mytable",
			},
		},
	}
	renderer.StateStores = SupportedStateStoreKindValues
	_, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.NoError(t, err)
}
