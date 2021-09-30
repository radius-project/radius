// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodbv1alpha3

import (
	"sort"
	"testing"

	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/renderers/cosmosdbmongov1alpha3"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_KubernetesRenderer_Render_Managed_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := KubernetesRenderer{}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"managed": true,
		},
	}

	output, err := renderer.Render(ctx, resource, map[string]renderers.RendererDependency{})
	require.NoError(t, err)
	require.NoError(t, err)

	require.Len(t, output.Resources, 3)
	ids := []string{}
	for _, resource := range output.Resources {
		ids = append(ids, resource.LocalID)
	}
	sort.Strings(ids)

	// Just validating the presence of resources here, we validate the actual resources in separate tests.
	require.Equal(t, []string{outputresource.LocalIDSecret, outputresource.LocalIDService, outputresource.LocalIDStatefulSet}, ids)

	expectedComputedValues := map[string]renderers.ComputedValueReference{
		"database": {
			Value: resource.ResourceName,
		},
	}
	require.Equal(t, expectedComputedValues, output.ComputedValues)

	expectedSecretValues := map[string]renderers.SecretValueReference{
		cosmosdbmongov1alpha3.ConnectionStringValue: {
			LocalID:       outputresource.LocalIDSecret,
			ValueSelector: "/data/MONGO_CONNECTIONSTRING",
		},
	}
	require.Equal(t, expectedSecretValues, output.SecretValues)
}

func Test_KubernetesRenderer_Render_Unmanaged_NotSupported(t *testing.T) {
	ctx := createContext(t)
	renderer := KubernetesRenderer{}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"managed": false,
		},
	}

	output, err := renderer.Render(ctx, resource, map[string]renderers.RendererDependency{})
	require.Error(t, err)
	require.Empty(t, output.Resources)
	require.Equal(t, "only Radius managed resources are supported for MongoDB on Kubernetes", err.Error())
}

func Test_KubernetesRenderer_MakeSecret(t *testing.T) {
	renderer := KubernetesRenderer{}
	options := KubernetesOptions{
		DescriptiveLabels: kubernetes.MakeDescriptiveLabels("test-application", "test-component"),
		SelectorLabels:    kubernetes.MakeSelectorLabels("test-application", "test-component"),
		Namespace:         "test-namespace",
		Name:              "test-name",
	}
	secret := renderer.MakeSecret(options, resourceName, "test-username", "test-password")

	expected := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-name",
			Namespace: "test-namespace",
			Labels:    kubernetes.MakeDescriptiveLabels("test-application", "test-component"),
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			SecretKeyMongoDBAdminUsername:    []byte("test-username"),
			SecretKeyMongoDBAdminPassword:    []byte("test-password"),
			SecretKeyMongoDBConnectionString: []byte("mongodb://test-username:test-password@test-db:27017/admin"),
		},
	}

	require.Equal(t, expected, secret)
}

func Test_KubernetesRenderer_MakeService(t *testing.T) {
	renderer := KubernetesRenderer{}
	options := KubernetesOptions{
		DescriptiveLabels: kubernetes.MakeDescriptiveLabels("test-application", "test-component"),
		SelectorLabels:    kubernetes.MakeSelectorLabels("test-application", "test-component"),
		Namespace:         "test-namespace",
		Name:              "test-name",
	}
	service := renderer.MakeService(options)

	expected := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-name",
			Namespace: "test-namespace",
			Labels:    kubernetes.MakeDescriptiveLabels("test-application", "test-component"),
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: corev1.ClusterIPNone,
			Selector:  kubernetes.MakeSelectorLabels("test-application", "test-component"),
		},
	}
	require.Equal(t, expected, service)
}

func Test_KubernetesRenderer_MakeStatefulSet(t *testing.T) {
	renderer := KubernetesRenderer{}
	options := KubernetesOptions{
		DescriptiveLabels: kubernetes.MakeDescriptiveLabels("test-application", "test-component"),
		SelectorLabels:    kubernetes.MakeSelectorLabels("test-application", "test-component"),
		Namespace:         "test-namespace",
		Name:              "test-name",
	}
	set := renderer.MakeStatefulSet(options, "test-service", "test-secret")

	expected := &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "StatefulSet",
			APIVersion: appsv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-name",
			Namespace: "test-namespace",
			Labels:    kubernetes.MakeDescriptiveLabels("test-application", "test-component"),
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: kubernetes.MakeSelectorLabels("test-application", "test-component"),
			},
			ServiceName: "test-service",
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: kubernetes.MakeDescriptiveLabels("test-application", "test-component"),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "mongo",
							Image: "mongo:5",
							Env: []corev1.EnvVar{
								{
									Name: "MONGO_INITDB_ROOT_USERNAME",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "test-secret",
											},
											Key: SecretKeyMongoDBAdminUsername,
										},
									},
								},
								{
									Name: "MONGO_INITDB_ROOT_PASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "test-secret",
											},
											Key: SecretKeyMongoDBAdminPassword,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	require.Equal(t, expected, set)
}
