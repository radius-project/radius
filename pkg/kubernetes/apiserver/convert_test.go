// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package apiserver

import (
	"encoding/json"
	"testing"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/kubernetes"
	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	"github.com/Azure/radius/pkg/radrp/frontend/resourceprovider"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/Azure/radius/pkg/resourcemodel"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
)

func Test_ConvertApplication_RoundTrips(t *testing.T) {
	namespace := "default"
	id, err := azresources.Parse("/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/frontend-backend")
	require.NoError(t, err)
	var raw *runtime.RawExtension
	properties := map[string]interface{}{}
	properties["status"] = rest.ApplicationStatus{}
	template := map[string]interface{}{
		"body": map[string]interface{}{
			"properties": properties,
		},
	}
	b, err := json.Marshal(template)
	require.NoError(t, err)
	raw = &runtime.RawExtension{Raw: b}
	original := radiusv1alpha3.Application{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "radius.dev/v1alpha3",
			Kind:       "Application",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "frontend-backend",
			Namespace: namespace,
			Annotations: map[string]string{
				kubernetes.LabelRadiusApplication: "frontend-backend",
			},
		},
		Spec: radiusv1alpha3.ApplicationSpec{
			Application: "frontend-backend",
			Template:    raw,
		},
	}

	res, err := NewRestApplicationResource(id, original, rest.ApplicationStatus{})
	require.NoError(t, err)

	final, err := NewKubernetesApplicationResource(id, res, namespace)
	require.NoError(t, err)

	require.Equal(t, original, final)
}

func Test_ConvertResource_RoundTrips(t *testing.T) {
	namespace := "default"
	id, err := azresources.Parse("/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/frontend-backend/Container/frontend")
	require.NoError(t, err)
	original := radiusv1alpha3.Resource{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "radius.dev/v1alpha3",
			Kind:       "Container",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        "frontend-backend-frontend",
			Namespace:   namespace,
			Annotations: kubernetes.MakeResourceCRDLabels("frontend-backend", "Container", "frontend"),
			Labels:      kubernetes.MakeResourceCRDLabels("frontend-backend", "Container", "frontend"),
		},
		Spec: radiusv1alpha3.ResourceSpec{
			Application: "frontend-backend",
			Resource:    "frontend",
			Template: &runtime.RawExtension{
				Raw: marshalJSONIgnoreErr(map[string]interface{}{
					"name": "kata-container",
					"id":   id.ID,
					"type": "Container",
					"body": map[string]interface{}{
						"properties": map[string]string{
							"image": "the-best",
						},
					},
				}),
			},
		},
	}

	unstMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&original)
	require.NoError(t, err)
	unst := unstructured.Unstructured{Object: unstMap}

	res, err := NewRestRadiusResourceFromUnstructured(unst)
	require.NoError(t, err)

	gvk := k8sschema.GroupVersionKind{
		Group:   "radius.dev",
		Version: "v1alpha3",
		Kind:    "Container",
	}

	final, err := NewKubernetesRadiusResource(id, res, namespace, gvk)
	require.NoError(t, err)

	// Unstructured comparison causes a comparison between interface{} and a string
	// so we need to convert to JSON
	expectedUns, err := json.Marshal(unst)

	require.NoError(t, err)

	actualUns, err := json.Marshal(final)
	require.NoError(t, err)

	require.JSONEq(t, string(expectedUns), string(actualUns))
}

func Test_ConvertK8sResourceToARM(t *testing.T) {

	for _, tc := range []struct {
		name        string
		original    interface{}
		expected    resourceprovider.RadiusResource
		expectedErr string
	}{{
		name: "has all fields",
		original: &radiusv1alpha3.Container{
			Spec: radiusv1alpha3.ResourceSpec{
				Template: &runtime.RawExtension{
					Raw: marshalJSONIgnoreErr(map[string]interface{}{
						"name": "kata-container",
						"id":   "/very/long/path/container-01",
						"type": "/very/long/path/radius.dev/Container",
						"body": map[string]interface{}{
							"properties": map[string]string{
								"image": "the-best",
							},
						},
					}),
				},
			},
			Status: radiusv1alpha3.ResourceStatus{
				Resources: map[string]*radiusv1alpha3.OutputResource{
					"Deployment": {
						Status: radiusv1alpha3.OutputResourceStatus{
							ProvisioningState: "Provisioned",
							HealthState:       healthcontract.HealthStateHealthy,
						},
					},
				},
			},
		},
		expected: resourceprovider.RadiusResource{
			Name: "kata-container",
			ID:   "/very/long/path/container-01",
			Type: "/very/long/path/radius.dev/Container",
			Properties: map[string]interface{}{
				"image": "the-best",
				"status": rest.ResourceStatus{
					ProvisioningState: "Provisioned",
					HealthState:       healthcontract.HealthStateHealthy,
					OutputResources: []rest.OutputResource{
						{
							LocalID:            "Deployment",
							OutputResourceType: string(resourcemodel.IdentityKindKubernetes),
							Status: rest.OutputResourceStatus{
								ProvisioningState: "Provisioned",
								HealthState:       healthcontract.HealthStateHealthy,
							},
						},
					},
				},
			},
		},
	}, {
		name: "no body",
		original: &radiusv1alpha3.HttpRoute{
			Spec: radiusv1alpha3.ResourceSpec{
				Template: &runtime.RawExtension{
					Raw: marshalJSONIgnoreErr(map[string]interface{}{
						"name": "/app/route-42",
						"id":   "/the/long/and/winding/route",
						"type": "/very/long/path/radius.dev/HttpRoute",
					}),
				},
			},
			Status: radiusv1alpha3.ResourceStatus{
				Resources: map[string]*radiusv1alpha3.OutputResource{
					"Deployment": {
						Status: radiusv1alpha3.OutputResourceStatus{
							ProvisioningState: "Provisioned",
							HealthState:       healthcontract.HealthStateHealthy,
						},
					},
				},
			},
		},
		expected: resourceprovider.RadiusResource{
			Name: "route-42",
			ID:   "/the/long/and/winding/route",
			Type: "/very/long/path/radius.dev/HttpRoute",
			Properties: map[string]interface{}{
				"status": rest.ResourceStatus{
					ProvisioningState: "Provisioned",
					HealthState:       healthcontract.HealthStateHealthy,
					OutputResources: []rest.OutputResource{
						{
							LocalID:            "Deployment",
							OutputResourceType: string(resourcemodel.IdentityKindKubernetes),
							Status: rest.OutputResourceStatus{
								ProvisioningState: "Provisioned",
								HealthState:       healthcontract.HealthStateHealthy,
							},
						},
					},
				},
			},
		},
	}, {
		name: "no name",
		original: &radiusv1alpha3.HttpRoute{
			Spec: radiusv1alpha3.ResourceSpec{
				Template: &runtime.RawExtension{
					Raw: marshalJSONIgnoreErr(map[string]interface{}{
						"id": "stark/arya",
					}),
				},
			},
		},
		expectedErr: "cannot find name",
	}, {
		name:        "no spec",
		original:    &radiusv1alpha3.HttpRoute{},
		expectedErr: "cannot find spec",
	}, {
		name: "no template",
		original: &radiusv1alpha3.HttpRoute{
			Spec: radiusv1alpha3.ResourceSpec{},
		},
		expectedErr: "cannot find spec.template",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			input := unstructured.Unstructured{}
			j, _ := json.MarshalIndent(tc.original, "", "  ")
			_ = json.Unmarshal(j, &input.Object)
			actual, err := NewRestRadiusResourceFromUnstructured(input)
			if tc.expectedErr == "" {
				require.NoError(t, err)
			} else {
				require.Contains(t, err.Error(), tc.expectedErr)
			}

			require.Equal(t, tc.expected, actual)
		})
	}
}

func marshalJSONIgnoreErr(foo interface{}) []byte {
	b, _ := json.MarshalIndent(foo, "  ", "")
	return b
}
