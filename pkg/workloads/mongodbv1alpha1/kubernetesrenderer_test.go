// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodbv1alpha1

import (
	"sort"
	"testing"

	"github.com/Azure/radius/pkg/keys"
	"github.com/Azure/radius/pkg/radrp/components"
	"github.com/Azure/radius/pkg/radrp/handlers"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_KubernetesRenderer_AllocateBindings(t *testing.T) {
	ctx := createContext(t)

	// Initialize a fake client with a secret compatible with what we expect
	builder := fake.NewClientBuilder()
	builder.WithRuntimeObjects(&corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "test-namespace",
			Labels:    keys.MakeDescriptiveLabels("test-application", "test-component"),
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			SecretKeyMongoDBAdminUsername: []byte("admin"),
			SecretKeyMongoDBAdminPassword: []byte("password"),
		},
	})

	renderer := KubernetesRenderer{
		K8s: builder.Build(),
	}

	workload := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-component",
		Namespace:   "test-namespace",
		Workload: components.GenericComponent{
			Kind: Kind,
			Name: "test-component",
			Config: map[string]interface{}{
				"managed": true,
			},
		},
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	resources := []workloads.WorkloadResourceProperties{
		{
			LocalID: outputresource.LocalIDSecret,
			Properties: map[string]string{
				handlers.KubernetesAPIVersionKey: "v1",
				handlers.KubernetesKindKey:       "Kind",
				handlers.KubernetesNameKey:       "test-secret",
				handlers.KubernetesNamespaceKey:  "test-namespace",
			},
		},
	}

	bindings, err := renderer.AllocateBindings(ctx, workload, resources)
	require.NoError(t, err)

	require.Len(t, bindings, 1)

	binding, ok := bindings[BindingMongo]
	require.True(t, ok)

	require.Equal(t, BindingMongo, binding.Binding)
	require.Equal(t, "test-component", binding.Component)
	require.Equal(t, "mongodb.com/Mongo", binding.Kind)

	expected := map[string]interface{}{
		"connectionString": "mongodb://admin:password@test-component.test-namespace.svc.cluster.local:27017/admin",
		"database":         "test-component",
	}
	require.Equal(t, expected, binding.Properties)
}

func Test_KubernetesRenderer_Render_Managed_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := KubernetesRenderer{}

	workload := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-component",
		Workload: components.GenericComponent{
			Kind: Kind,
			Name: "test-component",
			Config: map[string]interface{}{
				"managed": true,
			},
		},
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	resources, err := renderer.Render(ctx, workload)
	require.NoError(t, err)

	require.Len(t, resources, 3)
	ids := []string{}
	for _, resource := range resources {
		ids = append(ids, resource.LocalID)
	}
	sort.Strings(ids)

	// Just validating the presence of resources here, we validate the actual resources in separate tests.
	require.Equal(t, []string{outputresource.LocalIDSecret, outputresource.LocalIDService, outputresource.LocalIDStatefulSet}, ids)
}

func Test_KubernetesRenderer_Render_Unmanaged_NotSupported(t *testing.T) {
	ctx := createContext(t)
	renderer := KubernetesRenderer{}

	workload := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-component",
		Workload: components.GenericComponent{
			Kind: Kind,
			Name: "test-component",
			Config: map[string]interface{}{
				"managed": false,
			},
		},
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	resources, err := renderer.Render(ctx, workload)
	require.Empty(t, resources)
	require.Error(t, err)
	require.Equal(t, "only Radius managed resources are supported for MongoDB on Kubernetes", err.Error())
}

func Test_KubernetesRenderer_MakeSecret(t *testing.T) {
	renderer := KubernetesRenderer{}
	options := KubernetesOptions{
		DescriptiveLabels: keys.MakeDescriptiveLabels("test-application", "test-component"),
		SelectorLabels:    keys.MakeSelectorLabels("test-application", "test-component"),
		Namespace:         "test-namespace",
		Name:              "test-name",
	}
	secret := renderer.MakeSecret(options, "test-username", "test-password")

	expected := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-name",
			Namespace: "test-namespace",
			Labels:    keys.MakeDescriptiveLabels("test-application", "test-component"),
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			SecretKeyMongoDBAdminUsername: []byte("test-username"),
			SecretKeyMongoDBAdminPassword: []byte("test-password"),
		},
	}
	require.Equal(t, expected, secret)
}

func Test_KubernetesRenderer_MakeService(t *testing.T) {
	renderer := KubernetesRenderer{}
	options := KubernetesOptions{
		DescriptiveLabels: keys.MakeDescriptiveLabels("test-application", "test-component"),
		SelectorLabels:    keys.MakeSelectorLabels("test-application", "test-component"),
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
			Labels:    keys.MakeDescriptiveLabels("test-application", "test-component"),
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: corev1.ClusterIPNone,
			Selector:  keys.MakeSelectorLabels("test-application", "test-component"),
		},
	}
	require.Equal(t, expected, service)
}

func Test_KubernetesRenderer_MakeStatefulSet(t *testing.T) {
	renderer := KubernetesRenderer{}
	options := KubernetesOptions{
		DescriptiveLabels: keys.MakeDescriptiveLabels("test-application", "test-component"),
		SelectorLabels:    keys.MakeSelectorLabels("test-application", "test-component"),
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
			Labels:    keys.MakeDescriptiveLabels("test-application", "test-component"),
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: keys.MakeSelectorLabels("test-application", "test-component"),
			},
			ServiceName: "test-service",
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: keys.MakeDescriptiveLabels("test-application", "test-component"),
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
