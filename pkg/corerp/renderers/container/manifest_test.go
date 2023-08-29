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

package container

import (
	"fmt"
	"strings"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/renderers"
	"github.com/radius-project/radius/pkg/kubeutil"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/test/k8sutil"
	"github.com/radius-project/radius/test/testcontext"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	testResource = &datamodel.ContainerResource{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/test-sub-id/resourceGroups/test-group/providers/Applications.Core/containers/test-container",
				Name: "test-container",
				Type: "Applications.Core/containers",
			},
		},
	}
	testOptions = &renderers.RenderOptions{Environment: renderers.EnvironmentOptions{Namespace: "test-ns"}}
)

func TestFetchBaseManifest(t *testing.T) {
	manifestTests := []struct {
		name     string
		resource *datamodel.ContainerResource
	}{
		{
			name: "valid manifest",
			resource: &datamodel.ContainerResource{
				Properties: datamodel.ContainerProperties{
					Runtimes: &datamodel.RuntimeProperties{
						Kubernetes: &datamodel.KubernetesRuntime{
							Base: fmt.Sprintf(k8sutil.FakeDeploymentTemplate, "magpie", "", "magpie"),
						},
					},
				},
			},
		},
		{
			name: "nil runtime",
			resource: &datamodel.ContainerResource{
				Properties: datamodel.ContainerProperties{
					Runtimes: nil,
				},
			},
		},
		{
			name: "nil runtime.kubernetes",
			resource: &datamodel.ContainerResource{
				Properties: datamodel.ContainerProperties{
					Runtimes: nil,
				},
			},
		},
		{
			name: "empty runtime.kubernetes.base",
			resource: &datamodel.ContainerResource{
				Properties: datamodel.ContainerProperties{
					Runtimes: &datamodel.RuntimeProperties{
						Kubernetes: &datamodel.KubernetesRuntime{
							Base: "",
						},
					},
				},
			},
		},
	}

	for _, tc := range manifestTests {
		t.Run(tc.name, func(t *testing.T) {
			obj, err := fetchBaseManifest(tc.resource)
			require.NoError(t, err)
			require.NotNil(t, obj)
		})
	}

}

func TestGetDeploymentBase(t *testing.T) {
	deploymentTests := []struct {
		name     string
		manifest kubeutil.ObjectManifest
		expected *appsv1.Deployment
	}{
		{
			name:     "without base manifest",
			manifest: kubeutil.ObjectManifest{},
			expected: &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: "apps/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-container",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "radius-rp",
						"app.kubernetes.io/name":       "test-container",
						"app.kubernetes.io/part-of":    "test",
						"radius.dev/application":       "test",
						"radius.dev/resource":          "test-container",
						"radius.dev/resource-type":     "applications.core-containers",
					},
					Annotations: map[string]string{},
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels:      map[string]string{},
							Annotations: map[string]string{},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "test-container",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "with base manifest",
			manifest: kubeutil.ObjectManifest{
				kubeutil.DeploymentV1: []runtime.Object{
					&appsv1.Deployment{
						TypeMeta: metav1.TypeMeta{
							Kind:       "Deployment",
							APIVersion: "apps/v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-container",
							Labels: map[string]string{
								"label0": "value0",
							},
							Annotations: map[string]string{
								"annotation0": "value0",
							},
						},
						Spec: appsv1.DeploymentSpec{
							Selector: &metav1.LabelSelector{},
							Template: corev1.PodTemplateSpec{
								ObjectMeta: metav1.ObjectMeta{
									Labels:      map[string]string{},
									Annotations: map[string]string{},
								},
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Name: "sidecar",
										},
									},
								},
							},
						},
					},
				},
			},
			expected: &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: "apps/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-container",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "radius-rp",
						"app.kubernetes.io/name":       "test-container",
						"app.kubernetes.io/part-of":    "test",
						"label0":                       "value0",
						"radius.dev/application":       "test",
						"radius.dev/resource":          "test-container",
						"radius.dev/resource-type":     "applications.core-containers",
					},
					Annotations: map[string]string{
						"annotation0": "value0",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels:      map[string]string{},
							Annotations: map[string]string{},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "sidecar",
								},
								{
									Name: "test-container",
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range deploymentTests {
		t.Run(tc.name, func(t *testing.T) {
			deploy := getDeploymentBase(tc.manifest, "test", testResource, testOptions)
			require.Equal(t, tc.expected, deploy)
		})
	}
}

func TestGetServiceBase(t *testing.T) {
	serviceTests := []struct {
		name     string
		manifest kubeutil.ObjectManifest
		expected *corev1.Service
	}{
		{
			name:     "without base manifest",
			manifest: kubeutil.ObjectManifest{},
			expected: &corev1.Service{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Service",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-container",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "radius-rp",
						"app.kubernetes.io/name":       "test-container",
						"app.kubernetes.io/part-of":    "test",
						"radius.dev/application":       "test",
						"radius.dev/resource":          "test-container",
						"radius.dev/resource-type":     "applications.core-containers",
					},
					Annotations: map[string]string{},
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{},
					Type:     corev1.ServiceTypeClusterIP,
				},
			},
		},
		{
			name: "with base manifest",
			manifest: kubeutil.ObjectManifest{
				kubeutil.ServiceV1: []runtime.Object{
					&corev1.Service{
						TypeMeta: metav1.TypeMeta{
							Kind:       "Service",
							APIVersion: "v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-container",
							Labels: map[string]string{
								"label0": "value0",
							},
							Annotations: map[string]string{
								"annotation0": "value0",
							},
						},
						Spec: corev1.ServiceSpec{
							Selector: map[string]string{},
						},
					},
				},
			},
			expected: &corev1.Service{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Service",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-container",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "radius-rp",
						"app.kubernetes.io/name":       "test-container",
						"app.kubernetes.io/part-of":    "test",
						"label0":                       "value0",
						"radius.dev/application":       "test",
						"radius.dev/resource":          "test-container",
						"radius.dev/resource-type":     "applications.core-containers",
					},
					Annotations: map[string]string{
						"annotation0": "value0",
					},
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{},
				},
			},
		},
	}

	for _, tc := range serviceTests {
		t.Run(tc.name, func(t *testing.T) {
			deploy := getServiceBase(tc.manifest, "test", testResource, testOptions)
			require.Equal(t, tc.expected, deploy)
		})
	}
}

func TestGetServiceAccountBase(t *testing.T) {
	accountTests := []struct {
		name     string
		manifest kubeutil.ObjectManifest
		expected *corev1.ServiceAccount
	}{
		{
			name:     "without base manifest",
			manifest: kubeutil.ObjectManifest{},
			expected: &corev1.ServiceAccount{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ServiceAccount",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-container",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "radius-rp",
						"app.kubernetes.io/name":       "test-container",
						"app.kubernetes.io/part-of":    "test",
						"radius.dev/application":       "test",
						"radius.dev/resource":          "test-container",
						"radius.dev/resource-type":     "applications.core-containers",
					},
					Annotations: map[string]string{},
				},
			},
		},
		{
			name: "with base manifest",
			manifest: kubeutil.ObjectManifest{
				kubeutil.ServiceAccountV1: []runtime.Object{
					&corev1.ServiceAccount{
						TypeMeta: metav1.TypeMeta{
							Kind:       "ServiceAccount",
							APIVersion: "v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-container",
							Labels: map[string]string{
								"label0": "value0",
							},
							Annotations: map[string]string{
								"annotation0": "value0",
							},
						},
					},
				},
			},
			expected: &corev1.ServiceAccount{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ServiceAccount",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-container",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "radius-rp",
						"app.kubernetes.io/name":       "test-container",
						"app.kubernetes.io/part-of":    "test",
						"label0":                       "value0",
						"radius.dev/application":       "test",
						"radius.dev/resource":          "test-container",
						"radius.dev/resource-type":     "applications.core-containers",
					},
					Annotations: map[string]string{
						"annotation0": "value0",
					},
				},
			},
		},
	}

	for _, tc := range accountTests {
		t.Run(tc.name, func(t *testing.T) {
			deploy := getServiceAccountBase(tc.manifest, "test", testResource, testOptions)
			require.Equal(t, tc.expected, deploy)
		})
	}
}

func TestPopulateAllBaseResources(t *testing.T) {
	fakeDeployment := fmt.Sprintf(k8sutil.FakeDeploymentTemplate, "magpie", "", "magpie")

	ctx := testcontext.New(t)

	t.Run("deployment resource is not in outputResources", func(t *testing.T) {
		manifest, err := kubeutil.ParseManifest([]byte(fakeDeployment))
		require.NoError(t, err)
		outputResources := []rpv1.OutputResource{}
		require.Panics(t, func() {
			populateAllBaseResources(ctx, manifest, outputResources, *testOptions)
		})
	})

	t.Run("populate secret and configmap into outputResource", func(t *testing.T) {
		fakeService := fmt.Sprintf(k8sutil.FakeServiceTemplate, "magpie", "")
		fakeServiceAccount := fmt.Sprintf(k8sutil.FakeServiceAccountTemplate, "magpie")
		fakeSecret0 := fmt.Sprintf(k8sutil.FakeSecretTemplate, "secret0")
		fakeSecret1 := fmt.Sprintf(k8sutil.FakeSecretTemplate, "secret1")
		fakeConfigMap0 := fmt.Sprintf(k8sutil.FakeConfigMapTemplate, "configmap0")
		fakeConfigMap1 := fmt.Sprintf(k8sutil.FakeConfigMapTemplate, "configmap1")

		baseString := strings.Join([]string{fakeDeployment, fakeService, fakeServiceAccount, fakeSecret0, fakeConfigMap0, fakeSecret1, fakeConfigMap1}, k8sutil.YAMLSeparater)
		manifest, err := kubeutil.ParseManifest([]byte(baseString))
		require.NoError(t, err)

		outputResources := []rpv1.OutputResource{
			{
				LocalID:        rpv1.LocalIDDeployment,
				CreateResource: &rpv1.Resource{},
			},
		}

		newOutput := populateAllBaseResources(ctx, manifest, outputResources, *testOptions)
		require.Len(t, newOutput, 5)
		outLocalIDs := []string{}
		for _, o := range newOutput {
			outLocalIDs = append(outLocalIDs, o.LocalID)
		}
		require.ElementsMatch(t, []string{"Deployment", "Secret-dtl+8w==", "Secret-ddl9YA==", "ConfigMap-6BU8tQ==", "ConfigMap-5xU7Ig=="}, outLocalIDs)
		require.Len(t, outputResources[0].CreateResource.Dependencies, 4)
		require.ElementsMatch(t, []string{"Secret-dtl+8w==", "Secret-ddl9YA==", "ConfigMap-6BU8tQ==", "ConfigMap-5xU7Ig=="}, outputResources[0].CreateResource.Dependencies)
	})
}
