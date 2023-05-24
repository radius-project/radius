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

package daprsecretstores

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
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
	logger, err := ucplog.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
}

func Test_Render_UnsupportedMode(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedSecretStoreModes}

	resource := datamodel.DaprSecretStore{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/daprSecretStores/test-secret-store",
				Name: resourceName,
				Type: linkrp.DaprSecretStoresResourceType,
			},
		},
		Properties: datamodel.DaprSecretStoreProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Type: linkrp.DaprSecretStoresResourceType,
			Mode: "invalid",
		},
	}

	_, err := renderer.Render(ctx, &resource, renderers.RenderOptions{})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, fmt.Sprintf("invalid secret store mode, Supported mode values: %s", getAlphabeticallySortedKeys(SupportedSecretStoreModes)), err.(*v1.ErrClientRP).Message)
}

func Test_Render_Generic_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedSecretStoreModes}
	resource := datamodel.DaprSecretStore{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/daprSecretStores/test-secret-store",
				Name: resourceName,
				Type: linkrp.DaprSecretStoresResourceType,
			},
		},
		Properties: datamodel.DaprSecretStoreProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Type:    linkrp.DaprSecretStoresResourceType,
			Mode:    datamodel.LinkModeValues,
			Version: daprSecretStoreVersion,
			Metadata: map[string]any{
				"foo": "bar",
			},
		},
	}

	result, err := renderer.Render(ctx, &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.NoError(t, err)

	require.Len(t, result.Resources, 1)
	outputResource := result.Resources[0]

	require.Equal(t, rpv1.LocalIDDaprComponent, outputResource.LocalID)
	require.Equal(t, resourcekinds.DaprComponent, outputResource.ResourceType.Type)
	expectedComputedValues := map[string]renderers.ComputedValueReference{
		renderers.ComponentNameKey: {
			Value: "test-secret-store",
		},
	}
	require.Equal(t, expectedComputedValues, result.ComputedValues)

	expected := unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": daprVersion,
			"kind":       k8sKind,
			"metadata": map[string]any{
				"namespace": "radius-test",
				"name":      kubernetes.NormalizeDaprResourceName(resourceName),
				"labels":    kubernetes.MakeDescriptiveDaprLabels(applicationName, resourceName, linkrp.DaprSecretStoresResourceType),
			},
			"spec": map[string]any{
				"type":    linkrp.DaprSecretStoresResourceType,
				"version": "v1",
				"metadata": []any{
					map[string]any{
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
	renderer := Renderer{SupportedSecretStoreModes}
	resource := datamodel.DaprSecretStore{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/daprSecretStores/test-secret-store",
				Name: resourceName,
				Type: linkrp.DaprSecretStoresResourceType,
			},
		},
		Properties: datamodel.DaprSecretStoreProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Mode:    datamodel.LinkModeValues,
			Type:    "secretstores.kubernetes",
			Version: daprSecretStoreVersion,
		},
	}
	_, err := renderer.Render(ctx, &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, "No metadata specified for Dapr component of type secretstores.kubernetes", err.(*v1.ErrClientRP).Message)
}

func Test_Render_Generic_MissingType(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedSecretStoreModes}
	resource := datamodel.DaprSecretStore{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/daprSecretStores/test-secret-store",
				Name: resourceName,
				Type: linkrp.DaprSecretStoresResourceType,
			},
		},
		Properties: datamodel.DaprSecretStoreProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Mode:    datamodel.LinkModeValues,
			Version: daprSecretStoreVersion,
			Metadata: map[string]any{
				"foo": "bar",
			},
		},
	}

	_, err := renderer.Render(ctx, &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, "No type specified for generic Dapr component", err.(*v1.ErrClientRP).Message)
}

func Test_Render_Generic_MissingVersion(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedSecretStoreModes}
	resource := datamodel.DaprSecretStore{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/daprSecretStores/test-secret-store",
				Name: resourceName,
				Type: linkrp.DaprSecretStoresResourceType,
			},
		},
		Properties: datamodel.DaprSecretStoreProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Mode: datamodel.LinkModeValues,
			Type: "secretstores.kubernetes",
			Metadata: map[string]any{
				"foo": "bar",
			},
		},
	}

	_, err := renderer.Render(ctx, &resource, renderers.RenderOptions{Namespace: "radius-test"})

	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, "No Dapr component version specified for generic Dapr component", err.(*v1.ErrClientRP).Message)
}

func Test_Render_InvalidApplicationID(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedSecretStoreModes}
	resource := datamodel.DaprSecretStore{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/daprSecretStores/test-secret-store",
				Name: resourceName,
				Type: linkrp.DaprSecretStoresResourceType,
			},
		},
		Properties: datamodel.DaprSecretStoreProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: "invalid-app-id",
				Environment: environmentID,
			},
			Mode:    datamodel.LinkModeValues,
			Type:    linkrp.DaprSecretStoresResourceType,
			Version: daprSecretStoreVersion,
			Metadata: map[string]any{
				"foo": "bar",
			},
		},
	}

	_, err := renderer.Render(ctx, &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, "failed to parse application from the property: 'invalid-app-id' is not a valid resource id", err.(*v1.ErrClientRP).Message)
}

func Test_Render_EmptyApplicationID(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedSecretStoreModes}
	resource := datamodel.DaprSecretStore{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/daprSecretStores/test-secret-store",
				Name: resourceName,
				Type: linkrp.DaprSecretStoresResourceType,
			},
		},
		Properties: datamodel.DaprSecretStoreProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: environmentID,
			},
			Mode:    datamodel.LinkModeValues,
			Type:    linkrp.DaprSecretStoresResourceType,
			Version: daprSecretStoreVersion,
			Metadata: map[string]any{
				"foo": "bar",
			},
		},
	}

	rendererOutput, err := renderer.Render(ctx, &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.NoError(t, err)
	require.Equal(t, kubernetes.NormalizeDaprResourceName("test-secret-store"), rendererOutput.ComputedValues[renderers.ComponentNameKey].Value)
}
