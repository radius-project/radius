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

package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/resourcemodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	resources_kubernetes "github.com/radius-project/radius/pkg/ucp/resources/kubernetes"
	"github.com/radius-project/radius/test/k8sutil"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestPut(t *testing.T) {
	putTests := []struct {
		name string
		in   *PutOptions
		out  map[string]string
	}{
		{
			name: "secret resource",
			in: &PutOptions{
				Resource: &rpv1.OutputResource{
					CreateResource: &rpv1.Resource{
						ResourceType: resourcemodel.ResourceType{
							Provider: resourcemodel.ProviderKubernetes,
							Type:     "core/Secret",
						},
						Data: &corev1.Secret{
							TypeMeta: metav1.TypeMeta{
								Kind:       "Secret",
								APIVersion: "core/v1",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-secret",
								Namespace: "test-namespace",
							},
						},
					},
				},
			},
			out: map[string]string{
				"kubernetesapiversion": "core/v1",
				"kuberneteskind":       "Secret",
				"kubernetesnamespace":  "test-namespace",
				"resourcename":         "test-secret",
			},
		},
		{
			name: "deploment resource",
			in: &PutOptions{
				Resource: &rpv1.OutputResource{
					CreateResource: &rpv1.Resource{
						ResourceType: resourcemodel.ResourceType{
							Provider: resourcemodel.ProviderKubernetes,
							Type:     "apps/Deployment",
						},
						Data: testDeployment,
					},
				},
			},
			out: map[string]string{
				"kubernetesapiversion": "apps/v1",
				"kuberneteskind":       "Deployment",
				"kubernetesnamespace":  "test-namespace",
				"resourcename":         "test-deployment",
			},
		},
	}

	for _, tc := range putTests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			clientSet := fake.NewSimpleClientset(tc.in.Resource.CreateResource.Data.(runtime.Object))
			handler := kubernetesHandler{
				client: k8sutil.NewFakeKubeClient(nil),
				deploymentWaiter: &deploymentWaiter{
					clientSet:           clientSet,
					deploymentTimeOut:   time.Duration(50) * time.Second,
					cacheResyncInterval: time.Duration(1) * time.Second,
				},
			}

			// If the resource is a deployment, we need to add a replica set to it
			if tc.in.Resource.CreateResource.Data.(runtime.Object).GetObjectKind().GroupVersionKind().Kind == "Deployment" {
				// The deployment is not marked as ready till we find a replica set. Therefore, we need to create one.
				addReplicaSetToDeployment(t, ctx, clientSet, testDeployment)
			}

			props, err := handler.Put(ctx, tc.in)
			require.NoError(t, err)

			require.Equal(t, tc.out, props)
		})
	}
}

func TestDelete(t *testing.T) {
	ctx := context.Background()
	// Create first deployment that will be watched
	deployment := &v1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "test-namespace",
		},
	}

	dc := &k8sutil.DiscoveryClient{
		Resources: []*metav1.APIResourceList{
			{
				GroupVersion: "apps/v1",
				APIResources: []metav1.APIResource{
					{
						Name:    "deployments",
						Version: "v1",
						Kind:    "Deployment",
					},
				},
			},
		},
	}

	handler := kubernetesHandler{
		client:             k8sutil.NewFakeKubeClient(nil),
		k8sDiscoveryClient: dc,
		deploymentWaiter: &deploymentWaiter{
			deploymentTimeOut:   time.Duration(1) * time.Second,
			cacheResyncInterval: time.Duration(10) * time.Second,
		},
	}

	err := handler.client.Create(ctx, deployment)
	require.NoError(t, err)

	t.Run("existing resource", func(t *testing.T) {
		err := handler.Delete(ctx, &DeleteOptions{
			Resource: &rpv1.OutputResource{
				ID: resources_kubernetes.IDFromParts(
					resources_kubernetes.PlaneNameTODO,
					"apps",
					"Deployment",
					"test-namespace",
					"test-deployment"),
			},
		})

		require.NoError(t, err)
	})
}

func TestConvertToUnstructured(t *testing.T) {
	convertTests := []struct {
		name string
		in   rpv1.OutputResource
		out  unstructured.Unstructured
		err  error
	}{
		{
			name: "valid resource",
			in: rpv1.OutputResource{
				CreateResource: &rpv1.Resource{
					ResourceType: resourcemodel.ResourceType{
						Provider: resourcemodel.ProviderKubernetes,
						Type:     "apps/Deployment",
					},
					Data: &v1.Deployment{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-deployment",
							Namespace: "test-namespace",
						},
					},
				},
			},
			out: unstructured.Unstructured{
				Object: map[string]any{
					"metadata": map[string]any{
						"creationTimestamp": nil,
						"name":              "test-deployment",
						"namespace":         "test-namespace",
					},
					"spec": map[string]any{
						"selector": nil,
						"strategy": map[string]any{},
						"template": map[string]any{
							"metadata": map[string]any{
								"creationTimestamp": nil,
							},
							"spec": map[string]any{
								"containers": nil,
							},
						},
					},
					"status": map[string]any{},
				},
			},
		},
		{
			name: "invalid provider",
			in: rpv1.OutputResource{
				CreateResource: &rpv1.Resource{
					ResourceType: resourcemodel.ResourceType{
						Provider: resourcemodel.ProviderAzure,
						Type:     "apps/Deployment",
					},
					Data: &v1.Deployment{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-deployment",
							Namespace: "test-namespace",
						},
					},
				},
			},
			err: errors.New("invalid resource type provider: azure"),
		},
		{
			name: "invalid resource",
			in: rpv1.OutputResource{
				CreateResource: &rpv1.Resource{
					ResourceType: resourcemodel.ResourceType{
						Provider: resourcemodel.ProviderAzure,
						Type:     "apps/Deployment",
					},
					Data: map[string]any{"invalid": "type"},
				},
			},
			err: errors.New("inner type was not a runtime.Object"),
		},
	}

	for _, tc := range convertTests {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := convertToUnstructured(tc.in)
			if tc.err != nil {
				require.Error(t, err)
				require.Equal(t, tc.err.Error(), err.Error())
				return
			}
			require.Equal(t, tc.out, actual)
		})
	}
}
