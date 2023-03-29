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
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/clientv2"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/handlers"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
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
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/daprStateStores/test-state-store",
				Name: resourceName,
				Type: linkrp.DaprStateStoresResourceType,
			},
		},
		Properties: datamodel.DaprStateStoreProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Mode:     datamodel.LinkModeResource,
			Resource: "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Storage/storageAccounts/test-account/tableServices/default/tables/mytable",
		},
	}
	renderer.StateStores = SupportedStateStoreModes
	result, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.NoError(t, err)

	require.Len(t, result.Resources, 1)
	output := result.Resources[0]

	require.Equal(t, rpv1.LocalIDDaprStateStoreAzureStorage, output.LocalID)
	require.Equal(t, resourcekinds.DaprStateStoreAzureStorage, output.ResourceType.Type)
	require.Equal(t, kubernetes.NormalizeDaprResourceName(resourceName), result.ComputedValues[renderers.ComponentNameKey].Value)

	expected := map[string]string{
		handlers.KubernetesNameKey:       "test-state-store",
		handlers.KubernetesNamespaceKey:  "radius-test",
		handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
		handlers.KubernetesKindKey:       "Component",
		handlers.ApplicationName:         applicationName,
		handlers.ResourceName:            "test-state-store",
	}
	require.Equal(t, expected, output.Resource)
}

func Test_Render_InvalidResourceType(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprStateStore{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/daprStateStores/test-state-store",
				Name: resourceName,
				Type: linkrp.DaprStateStoresResourceType,
			},
		},
		Properties: datamodel.DaprStateStoreProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Mode:     datamodel.LinkModeResource,
			Resource: "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.SomethingElse/test-storageAccounts/test-account",
		},
	}
	renderer.StateStores = SupportedStateStoreModes
	_, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, "the 'resource' field must refer to a Storage Table", err.(*v1.ErrClientRP).Message)
}

func Test_Render_UnsupportedMode(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprStateStore{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/daprStateStores/test-state-store",
				Name: resourceName,
				Type: linkrp.DaprStateStoresResourceType,
			},
		},
		Properties: datamodel.DaprStateStoreProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Mode:     "invalid",
			Resource: "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.SomethingElse/test-storageAccounts/test-account",
		},
	}
	renderer.StateStores = SupportedStateStoreModes
	_, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, fmt.Sprintf("invalid state store mode, Supported mode values: %s", getAlphabeticallySortedKeys(SupportedStateStoreModes)), err.(*v1.ErrClientRP).Message)
}
func Test_Render_SpecifiesUmanagedWithoutResource(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprStateStore{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/daprStateStores/test-state-store",
				Name: resourceName,
				Type: linkrp.DaprStateStoresResourceType,
			},
		},
		Properties: datamodel.DaprStateStoreProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Mode: datamodel.LinkModeResource,
		},
	}
	renderer.StateStores = SupportedStateStoreModes
	_, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, renderers.ErrResourceMissingForResource.Error(), err.(*v1.ErrClientRP).Message)
}

func Test_Render_Generic_Success(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprStateStore{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/daprStateStores/test-state-store",
				Name: resourceName,
				Type: linkrp.DaprStateStoresResourceType,
			},
		},
		Properties: datamodel.DaprStateStoreProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Mode:    datamodel.LinkModeValues,
			Type:    stateStoreType,
			Version: daprStateStoreVersion,
			Metadata: map[string]any{
				"foo": "bar",
			},
		},
	}
	renderer.StateStores = SupportedStateStoreModes
	result, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.NoError(t, err)
	require.Len(t, result.Resources, 1)
	output := result.Resources[0]

	require.Equal(t, rpv1.LocalIDDaprComponent, output.LocalID)
	require.Equal(t, resourcekinds.DaprComponent, output.ResourceType.Type)
	require.Equal(t, kubernetes.NormalizeDaprResourceName(resourceName), result.ComputedValues[renderers.ComponentNameKey].Value)

	expected := unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": daprVersion,
			"kind":       k8sKind,
			"metadata": map[string]any{
				"namespace": "radius-test",
				"name":      kubernetes.NormalizeDaprResourceName(resourceName),
				"labels":    kubernetes.MakeDescriptiveDaprLabels(applicationName, resourceName, linkrp.DaprStateStoresResourceType),
			},
			"spec": map[string]any{
				"type":    stateStoreType,
				"version": daprStateStoreVersion,
				"metadata": []map[string]any{
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
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/daprStateStores/test-state-store",
				Name: resourceName,
				Type: linkrp.DaprStateStoresResourceType,
			},
		},
		Properties: datamodel.DaprStateStoreProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Mode:    datamodel.LinkModeValues,
			Type:    stateStoreType,
			Version: daprStateStoreVersion,
		},
	}
	renderer.StateStores = SupportedStateStoreModes
	_, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, "No metadata specified for Dapr component of type state.zookeeper", err.(*v1.ErrClientRP).Message)
}

func Test_Render_Generic_MissingType(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprStateStore{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/daprStateStores/test-state-store",
				Name: resourceName,
				Type: linkrp.DaprStateStoresResourceType,
			},
		},
		Properties: datamodel.DaprStateStoreProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Mode: datamodel.LinkModeValues,
			Metadata: map[string]any{
				"foo": "bar",
			},
			Version: daprStateStoreVersion,
		},
	}
	renderer.StateStores = SupportedStateStoreModes
	_, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, "No type specified for generic Dapr component", err.(*v1.ErrClientRP).Message)
}

func Test_Render_Generic_MissingVersion(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprStateStore{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/daprStateStores/test-state-store",
				Name: resourceName,
				Type: linkrp.DaprStateStoresResourceType,
			},
		},
		Properties: datamodel.DaprStateStoreProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Mode: datamodel.LinkModeValues,
			Metadata: map[string]any{
				"foo": "bar",
			},
			Type: stateStoreType,
		},
	}
	renderer.StateStores = SupportedStateStoreModes
	_, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})

	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, "No Dapr component version specified for generic Dapr component", err.(*v1.ErrClientRP).Message)
}

func Test_Render_InvalidApplicationID(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprStateStore{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/daprStateStores/test-state-store",
				Name: resourceName,
				Type: linkrp.DaprStateStoresResourceType,
			},
		},
		Properties: datamodel.DaprStateStoreProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: "invalid-app-id",
				Environment: environmentID,
			},
			Mode:     datamodel.LinkModeResource,
			Resource: "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Storage/storageAccounts/test-account/tableServices/default/tables/mytable",
		},
	}
	renderer.StateStores = SupportedStateStoreModes
	_, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, "failed to parse application from the property: 'invalid-app-id' is not a valid resource id", err.(*v1.ErrClientRP).Message)
}

func Test_Render_EmptyApplicationID(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprStateStore{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/daprStateStores/test-state-store",
				Name: resourceName,
				Type: linkrp.DaprStateStoresResourceType,
			},
		},
		Properties: datamodel.DaprStateStoreProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: environmentID,
			},
			Mode:     datamodel.LinkModeResource,
			Resource: "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Storage/storageAccounts/test-account/tableServices/default/tables/mytable",
		},
	}
	renderer.StateStores = SupportedStateStoreModes
	_, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.NoError(t, err)
}

func Test_Render_Recipe_Success(t *testing.T) {
	renderer := Renderer{}

	resource := datamodel.DaprStateStore{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/daprStateStores/test-state-store",
				Name: resourceName,
				Type: "Applications.Link/daprStateStores",
			},
		},
		Properties: datamodel.DaprStateStoreProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Mode: datamodel.LinkModeRecipe,
			Recipe: linkrp.LinkRecipe{
				Name: "daprstatestores",
			},
		},
	}

	expectedOutputResources := []rpv1.OutputResource{
		{
			LocalID: rpv1.LocalIDDaprStateStoreAzureStorage,
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.DaprStateStoreAzureStorage,
				Provider: resourcemodel.ProviderAzure,
			},
			ProviderResourceType: azresources.StorageStorageAccounts,
			Resource: map[string]string{
				handlers.KubernetesNameKey:       "test-state-store",
				handlers.KubernetesNamespaceKey:  "radius-test",
				handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
				handlers.KubernetesKindKey:       "Component",
				handlers.ApplicationName:         applicationName,
				handlers.ResourceName:            "test-state-store",
			},
			RadiusManaged: to.Ptr(true),
		},
		{
			LocalID: rpv1.LocalIDAzureStorageTableService,
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.DaprStateStoreAzureTableService,
				Provider: resourcemodel.ProviderAzure,
			},
			ProviderResourceType: azresources.StorageStorageAccounts + "/" + azresources.StorageStorageTableServices,
			RadiusManaged:        to.Ptr(false),
			Dependencies:         []rpv1.Dependency{{LocalID: rpv1.LocalIDDaprStateStoreAzureStorage}},
		},
		{
			LocalID: rpv1.LocalIDAzureStorageTable,
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.DaprStateStoreAzureTable,
				Provider: resourcemodel.ProviderAzure,
			},
			ProviderResourceType: azresources.StorageStorageAccounts + "/" + azresources.StorageStorageTableServices + "/" + azresources.StorageStorageAccountsTables,
			RadiusManaged:        to.Ptr(false),
			Dependencies:         []rpv1.Dependency{{LocalID: rpv1.LocalIDAzureStorageTableService}},
		},
	}

	renderer.StateStores = SupportedStateStoreModes
	result, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{
		Namespace: "radius-test",
		RecipeProperties: linkrp.RecipeProperties{
			LinkRecipe: linkrp.LinkRecipe{
				Name: "daprstatestores",
			},
			TemplatePath: "testpublicrecipe.azurecr.io/bicep/modules/daprstatestores:v1",
			LinkType:     "Applications.Link/daprStateStores",
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Resources, 3)
	require.Equal(t, expectedOutputResources, result.Resources)
	require.Equal(t, kubernetes.NormalizeDaprResourceName(resourceName), result.ComputedValues[renderers.ComponentNameKey].Value)
	require.Equal(t, resource.Properties.Recipe.Name, result.RecipeData.Name)
	require.Equal(t, clientv2.StateStoreClientAPIVersion, result.RecipeData.APIVersion)
	require.Equal(t, "testpublicrecipe.azurecr.io/bicep/modules/daprstatestores:v1", result.RecipeData.TemplatePath)
}

func Test_Render_Recipe_InvalidLinkType(t *testing.T) {
	renderer := Renderer{}

	resource := datamodel.DaprStateStore{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/daprStateStores/test-state-store",
				Name: resourceName,
				Type: "Applications.Link/daprStateStores",
			},
		},
		Properties: datamodel.DaprStateStoreProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Mode: datamodel.LinkModeRecipe,
			Recipe: linkrp.LinkRecipe{
				Name: "daprstatestores",
			},
		},
	}
	renderer.StateStores = SupportedStateStoreModes
	_, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{
		Namespace: "radius-test",
		RecipeProperties: linkrp.RecipeProperties{
			LinkRecipe: linkrp.LinkRecipe{
				Name: "daprstatestores",
			},
			TemplatePath: "testpublicrecipe.azurecr.io/bicep/modules/daprstatestores:v1",
			LinkType:     "Applications.Link/redisCaches",
		},
	})

	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, "link type \"Applications.Link/redisCaches\" of provided recipe \"daprstatestores\" is incompatible with \"Applications.Link/daprStateStores\" resource type. Recipe link type must match link resource type.", err.(*v1.ErrClientRP).Message)
}
