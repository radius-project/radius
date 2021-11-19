// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/kubernetes"
	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/resourcemodel"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	Namespace       = "test-namespace"
	ApplicationName = "test-application"
	ResourceName    = "test-resource"
)

var resourceID = azresources.MakeID(
	"kubernetes",
	Namespace,
	azresources.ResourceType{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radiusv3"},
	azresources.ResourceType{Type: "Application", Name: ApplicationName},
	azresources.ResourceType{Type: "ContainerComponent", Name: ResourceName})

func Test_GetRenderDependency(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(radiusv1alpha3.AddToScheme(scheme))

	// Simulate a *rendered* component
	resource := radiusv1alpha3.ContainerComponent{
		TypeMeta: v1.TypeMeta{
			APIVersion: radiusv1alpha3.GroupVersion.String(),
			Kind:       "ContainerComponent",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      kubernetes.MakeResourceName(ApplicationName, ResourceName),
			Namespace: Namespace,
		},
		Spec: radiusv1alpha3.ResourceSpec{
			Template: rawOrPanic(map[string]interface{}{
				"properties": map[string]interface{}{
					"definition-property": "definition-value",
				},
			}),
		},
		Status: radiusv1alpha3.ResourceStatus{
			Resources: map[string]*radiusv1alpha3.OutputResource{
				"SecretLocalID": &radiusv1alpha3.OutputResource{
					Resource: corev1.ObjectReference{
						Namespace:  Namespace,
						Name:       "some-secret",
						Kind:       "Secret",
						APIVersion: "v1",
					},
				},
			},
			ComputedValues: rawOrPanic(map[string]renderers.ComputedValueReference{
				"computed-property": {
					Value: "computed-value",
				},
			}),
			SecretValues: rawOrPanic(map[string]renderers.SecretValueReference{
				"secret-property": {
					LocalID:       "SecretLocalID",
					ValueSelector: "secret-key",
				},
			}),
		},
	}

	// And the secret that holds the secret values
	secret := corev1.Secret{
		TypeMeta: v1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "some-secret",
			Namespace: Namespace,
		},
		Data: map[string][]byte{
			"secret-key": []byte("secret-value"),
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&resource, &secret).Build()

	controller := ResourceReconciler{
		Client: c,
	}

	id, err := azresources.Parse(resourceID)
	require.NoError(t, err)

	dependency, err := controller.GetRenderDependency(context.Background(), Namespace, id)
	require.NoError(t, err)
	require.NotNil(t, dependency)

	expected := renderers.RendererDependency{
		ResourceID: id,
		Definition: map[string]interface{}{
			"definition-property": "definition-value",
		},
		ComputedValues: map[string]interface{}{
			"computed-property": "computed-value",
			"secret-property":   "secret-value",
		},
		OutputResources: map[string]resourcemodel.ResourceIdentity{
			"SecretLocalID": {
				Kind: resourcemodel.IdentityKindKubernetes,
				Data: resourcemodel.KubernetesIdentity{
					Namespace:  Namespace,
					Name:       "some-secret",
					Kind:       "Secret",
					APIVersion: "v1",
				},
			},
		},
	}

	require.Equal(t, expected, *dependency)
}

func rawOrPanic(obj interface{}) *runtime.RawExtension {
	b, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}

	return &runtime.RawExtension{Raw: b}
}
