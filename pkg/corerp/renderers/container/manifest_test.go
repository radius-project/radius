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
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/kubeutil"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const validManifest = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
  namespace: app-scoped
  labels:
    app: nginx
spec:
  replicas: 3
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:1.14.2
        ports:
        - containerPort: 80
`

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
							Base: validManifest,
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
			name:     "nil manifest",
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
			name: "base manifest",
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
			name:     "nil manifest",
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
			name: "base manifest",
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

}

func TestPopulateAllBaseResources(t *testing.T) {

}