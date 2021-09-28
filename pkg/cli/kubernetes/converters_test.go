// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"encoding/json"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/Azure/radius/pkg/azure/radclientv3"
	"github.com/Azure/radius/pkg/kubernetes"
	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	"github.com/stretchr/testify/require"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func Test_ConvertK8sApplicationToARM(t *testing.T) {
	original := radiusv1alpha3.Application{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "radius.dev/v1alpha1",
			Kind:       "Application",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "frontend-backend",
			Namespace: "default",
			Annotations: map[string]string{
				kubernetes.AnnotationsApplication: "frontend-backend",
			},
		},
		Spec: radiusv1alpha3.ApplicationSpec{},
	}

	expected := &radclient.ApplicationResource{
		TrackedResource: radclient.TrackedResource{
			Resource: radclient.Resource{
				Name: to.StringPtr("frontend-backend"),
			},
		},
		Properties: &radclient.ApplicationProperties{},
	}

	actual, err := ConvertK8sApplicationToARM(original)
	require.NoError(t, err, "failed to convert application")

	require.Equal(t, expected, actual)
}

func Test_ConvertK8sApplicationToARMV3(t *testing.T) {
	original := radiusv1alpha3.Application{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "radius.dev/v1alpha1",
			Kind:       "Application",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "frontend-backend",
			Namespace: "default",
			Annotations: map[string]string{
				kubernetes.AnnotationsApplication: "frontend-backend",
			},
		},
		Spec: radiusv1alpha3.ApplicationSpec{},
	}

	expected := &radclientv3.ApplicationResource{
		TrackedResource: radclientv3.TrackedResource{
			Resource: radclientv3.Resource{
				Name: to.StringPtr("frontend-backend"),
			},
		},
		Properties: &radclientv3.ApplicationProperties{},
	}

	actual, err := ConvertK8sApplicationToARMV3(original)
	require.NoError(t, err, "failed to convert application")

	require.Equal(t, expected, actual)
}

func Test_ConvertK8sResourceToARMV3(t *testing.T) {

	for _, tc := range []struct {
		name        string
		original    interface{}
		expected    *radclientv3.RadiusResource
		expectedErr string
	}{{
		name: "has all fields",
		original: &radiusv1alpha3.ContainerComponent{
			Spec: radiusv1alpha3.ResourceSpec{
				Template: &runtime.RawExtension{
					Raw: marshalJSONIgnoreErr(map[string]interface{}{
						"name": "kata-container",
						"id":   "/very/long/path/container-01",
						"type": "/very/long/path/radius.dev/ContainerComponent",
						"body": map[string]interface{}{
							"properties": map[string]string{
								"image": "the-best",
							},
						},
					}),
				},
			},
		},
		expected: &radclientv3.RadiusResource{
			ProxyResource: radclientv3.ProxyResource{
				Resource: radclientv3.Resource{
					Name: to.StringPtr("kata-container"),
					ID:   to.StringPtr("/very/long/path/container-01"),
					Type: to.StringPtr("/very/long/path/radius.dev/ContainerComponent"),
				},
			},
			Properties: map[string]interface{}{
				"image": "the-best",
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
		},
		expected: &radclientv3.RadiusResource{
			ProxyResource: radclientv3.ProxyResource{
				Resource: radclientv3.Resource{
					Name: to.StringPtr("route-42"),
					ID:   to.StringPtr("/the/long/and/winding/route"),
					Type: to.StringPtr("/very/long/path/radius.dev/HttpRoute"),
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
			actual, err := ConvertK8sResourceToARMV3(input)
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
