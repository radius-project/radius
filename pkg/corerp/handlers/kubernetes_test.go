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

	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/test/k8sutil"
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
					ResourceType: resourcemodel.ResourceType{
						Provider: resourcemodel.ProviderKubernetes,
					},
					Resource: &corev1.Secret{
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
					ResourceType: resourcemodel.ResourceType{
						Provider: resourcemodel.ProviderKubernetes,
					},
					Resource: &v1.Deployment{
						TypeMeta: metav1.TypeMeta{
							Kind:       "Deployment",
							APIVersion: "apps/v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-deployment",
							Namespace: "test-namespace",
						},
						Status: v1.DeploymentStatus{
							Conditions: []v1.DeploymentCondition{
								{
									Type:    v1.DeploymentProgressing,
									Status:  corev1.ConditionTrue,
									Reason:  "NewReplicaSetAvailable",
									Message: "Deployment has minimum availability",
								},
							},
						},
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
			ctx := context.TODO()

			handler := kubernetesHandler{
				client:              k8sutil.NewFakeKubeClient(nil),
				clientSet:           nil,
				deploymentTimeOut:   time.Duration(5) * time.Second,
				cacheResyncInterval: time.Duration(10) * time.Second,
			}

			// only deployment resources need to be watched.
			if _, ok := tc.in.Resource.Resource.(*v1.Deployment); ok {
				handler.clientSet = fake.NewSimpleClientset(tc.in.Resource.Resource.(runtime.Object))
			}

			props, err := handler.Put(ctx, tc.in)
			require.NoError(t, err)

			require.Equal(t, tc.out, props)
		})
	}
}

func TestDelete(t *testing.T) {
	ctx := context.TODO()
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

	handler := kubernetesHandler{
		client:              k8sutil.NewFakeKubeClient(nil),
		deploymentTimeOut:   time.Duration(1) * time.Second,
		cacheResyncInterval: time.Duration(10) * time.Second,
	}

	err := handler.client.Create(ctx, deployment)
	require.NoError(t, err)

	t.Run("existing resource", func(t *testing.T) {
		err := handler.Delete(ctx, &DeleteOptions{
			Resource: &rpv1.OutputResource{
				Identity: resourcemodel.ResourceIdentity{
					Data: map[string]any{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"metadata": map[string]any{
							"name":      "test-deployment",
							"namespace": "test-namespace",
						},
					},
				},
			},
		})

		require.NoError(t, err)
	})

	t.Run("existing resource", func(t *testing.T) {
		err := handler.Delete(ctx, &DeleteOptions{
			Resource: &rpv1.OutputResource{
				Identity: resourcemodel.ResourceIdentity{
					Data: map[string]any{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"metadata": map[string]any{
							"name":      "test-deployment1",
							"namespace": "test-namespace",
						},
					},
				},
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
				ResourceType: resourcemodel.ResourceType{
					Provider: resourcemodel.ProviderKubernetes,
				},
				Resource: &v1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-deployment",
						Namespace: "test-namespace",
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
				ResourceType: resourcemodel.ResourceType{
					Provider: resourcemodel.ProviderAzure,
				},
				Resource: &v1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-deployment",
						Namespace: "test-namespace",
					},
				},
			},
			err: errors.New("invalid resource type provider: azure"),
		},
		{
			name: "invalid resource",
			in: rpv1.OutputResource{
				ResourceType: resourcemodel.ResourceType{
					Provider: resourcemodel.ProviderKubernetes,
				},
				Resource: map[string]any{"invalid": "type"},
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

func TestWaitUntilDeploymentIsReady_NewResource(t *testing.T) {
	ctx := context.TODO()
	// Create first deployment that will be watched
	deployment := &v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "test-namespace",
		},
		Status: v1.DeploymentStatus{
			Conditions: []v1.DeploymentCondition{
				{
					Type:    v1.DeploymentProgressing,
					Status:  corev1.ConditionTrue,
					Reason:  "NewReplicaSetAvailable",
					Message: "Deployment has minimum availability",
				},
			},
		},
	}

	deploymentClient := fake.NewSimpleClientset(deployment)

	handler := kubernetesHandler{
		clientSet:           deploymentClient,
		deploymentTimeOut:   time.Duration(5) * time.Second,
		cacheResyncInterval: time.Duration(10) * time.Second,
	}

	err := handler.waitUntilDeploymentIsReady(ctx, deployment)
	require.NoError(t, err, "Failed to wait for deployment to be ready")
}

func TestWaitUntilDeploymentIsReady_Timeout(t *testing.T) {
	tests := []struct {
		name              string
		contextTimeout    time.Duration
		deploymentTimeout time.Duration
		expectedError     string
	}{
		{
			name:              "context timeout",
			contextTimeout:    time.Duration(1) * time.Second,
			deploymentTimeout: time.Duration(5) * time.Minute,
			expectedError:     "deployment is timed out with the status: Deadline is exceeded (ProgressDeadlineExceeded), name: test-deployment, namespace: test-namespace",
		},
		{
			name:              "deployment timeout",
			contextTimeout:    time.Duration(5) * time.Minute,
			deploymentTimeout: time.Duration(1) * time.Second,
			expectedError:     "deployment is timed out with the status: Deadline is exceeded (ProgressDeadlineExceeded), name: test-deployment, namespace: test-namespace",
		},
	}

	// Create first deployment that will be watched
	deployment := &v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "test-namespace",
		},
		Status: v1.DeploymentStatus{
			Conditions: []v1.DeploymentCondition{
				{
					Type:    v1.DeploymentProgressing,
					Status:  corev1.ConditionFalse,
					Reason:  "NewReplicaSetAvailable",
					Message: "Deployment has minimum availability",
				},
				{
					Type:    v1.DeploymentProgressing,
					Status:  corev1.ConditionFalse,
					Reason:  "ProgressDeadlineExceeded",
					Message: "Deadline is exceeded",
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), tc.contextTimeout)
			handler := kubernetesHandler{
				clientSet:           fake.NewSimpleClientset(deployment),
				deploymentTimeOut:   tc.deploymentTimeout,
				cacheResyncInterval: time.Duration(10) * time.Second,
			}
			err := handler.waitUntilDeploymentIsReady(ctx, deployment)
			require.Error(t, err)
			require.Equal(t, tc.expectedError, err.Error())
			cancel()
		})
	}
}

func TestWaitUntilDeploymentIsReady_DifferentResourceName(t *testing.T) {
	ctx := context.TODO()
	// Create first deployment that will be watched
	deployment := &v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "test-namespace",
		},
		Status: v1.DeploymentStatus{
			Conditions: []v1.DeploymentCondition{
				{
					Type:    v1.DeploymentProgressing,
					Status:  corev1.ConditionTrue,
					Reason:  "NewReplicaSetAvailable",
					Message: "Deployment has minimum availability",
				},
			},
		},
	}

	deploymentClient := fake.NewSimpleClientset(deployment)

	handler := kubernetesHandler{
		clientSet:           deploymentClient,
		deploymentTimeOut:   time.Duration(1) * time.Second,
		cacheResyncInterval: time.Duration(10) * time.Second,
	}

	err := handler.waitUntilDeploymentIsReady(ctx, &v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "not-matched-deployment",
			Namespace: "test-namespace",
		},
	})

	// It must be timed out because the name of the deployment does not match.
	require.Error(t, err)
	require.Equal(t, "deployment is timed out with the status: unknown status, name: not-matched-deployment, namespace test-namespace", err.Error())
}
