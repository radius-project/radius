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

	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/test/k8sutil"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

func int32Ptr(i int32) *int32 { return &i }

type workqueueItem struct {
	key  string
	obj  interface{}
	meta *metav1.ObjectMeta
}

var testDeployment = &v1.Deployment{
	TypeMeta: metav1.TypeMeta{
		Kind:       "Deployment",
		APIVersion: "apps/v1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:      "test-deployment",
		Namespace: "test-namespace",
		Labels: map[string]string{
			kubernetes.LabelManagedBy: kubernetes.LabelManagedByRadiusRP,
		},
	},
	Spec: v1.DeploymentSpec{
		Replicas: int32Ptr(1),
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"app": "test",
			},
		},
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

func addReplicaSetToDeployment(t *testing.T, ctx context.Context, clientset *fake.Clientset, deployment *v1.Deployment) *v1.ReplicaSet {
	replicaSet := &v1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-replicaset-1",
			Namespace:   deployment.Namespace,
			Annotations: map[string]string{"deployment.kubernetes.io/revision": "1"},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(deployment, schema.GroupVersionKind{
					Group:   v1.SchemeGroupVersion.Group,
					Version: v1.SchemeGroupVersion.Version,
					Kind:    "Deployment",
				}),
			},
		},
	}

	// Add the ReplicaSet objects to the fake Kubernetes clientset
	_, err := clientset.AppsV1().ReplicaSets(replicaSet.Namespace).Create(ctx, replicaSet, metav1.CreateOptions{})
	require.NoError(t, err)

	_, err = clientset.AppsV1().Deployments(deployment.Namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	require.NoError(t, err)

	return replicaSet
}

func startInformers(ctx context.Context, clientSet *fake.Clientset, handler *kubernetesHandler) informers.SharedInformerFactory {
	// Create a fake replicaset informer and start
	informerFactory := informers.NewSharedInformerFactory(clientSet, 0)

	// Add informers
	informerFactory.Apps().V1().Deployments().Informer()
	informerFactory.Apps().V1().ReplicaSets().Informer()
	informerFactory.Core().V1().Pods().Informer()

	informerFactory.Start(context.Background().Done())
	informerFactory.WaitForCacheSync(ctx.Done())
	return informerFactory
}

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
					Resource: testDeployment,
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
				deploymentTimeOut:   time.Duration(50) * time.Second,
				cacheResyncInterval: time.Duration(1) * time.Second,
			}

			clientSet := fake.NewSimpleClientset(tc.in.Resource.Resource.(runtime.Object))
			handler.clientSet = clientSet

			// If the resource is a deployment, we need to add a replica set to it
			if tc.in.Resource.Resource.(runtime.Object).GetObjectKind().GroupVersionKind().Kind == "Deployment" {
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
			Labels: map[string]string{
				kubernetes.LabelManagedBy: kubernetes.LabelManagedByRadiusRP,
			},
		},
		Spec: v1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
				},
			},
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

	clientset := fake.NewSimpleClientset(deployment)

	// The deployment is not marked as ready till we find a replica set. Therefore, we need to create one.
	addReplicaSetToDeployment(t, ctx, clientset, deployment)

	handler := kubernetesHandler{
		clientSet:           clientset,
		deploymentTimeOut:   time.Duration(50) * time.Second,
		cacheResyncInterval: time.Duration(10) * time.Second,
	}

	err := handler.waitUntilDeploymentIsReady(ctx, deployment)
	require.NoError(t, err, "Failed to wait for deployment to be ready")
}

func TestWaitUntilDeploymentIsReady_Timeout(t *testing.T) {
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
					Status:  corev1.ConditionFalse,
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

	err := handler.waitUntilDeploymentIsReady(ctx, deployment)
	require.Error(t, err)
	require.Equal(t, "deployment timed out, name: test-deployment, namespace test-namespace, status: Deployment has minimum availability, reason: NewReplicaSetAvailable", err.Error())
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

	clientset := fake.NewSimpleClientset(deployment)

	handler := kubernetesHandler{
		clientSet:           clientset,
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
	require.Equal(t, "deployment timed out, name: not-matched-deployment, namespace test-namespace, error occured while fetching latest status: deployments.apps \"not-matched-deployment\" not found", err.Error())
}

func TestGetPodsInDeployment(t *testing.T) {
	// Create a fake Kubernetes clientset
	fakeClient := fake.NewSimpleClientset()

	// Create a Deployment object
	deployment := &v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "test-namespace",
		},
		Spec: v1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test-app",
				},
			},
		},
	}

	// Create a ReplicaSet object
	replicaset := &v1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-replicaset",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "test-app",
			},
		},
	}

	// Create a Pod object
	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod1",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "test-app",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "ReplicaSet",
					Name:       replicaset.Name,
					Controller: to.Ptr(true),
				},
			},
		},
	}

	// Create a Pod object
	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod2",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "doesnotmatch",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "ReplicaSet",
					Name:       "xyz",
					Controller: to.Ptr(true),
				},
			},
		},
	}

	// Add the Pod object to the fake Kubernetes clientset
	_, err := fakeClient.CoreV1().Pods(pod1.Namespace).Create(context.Background(), pod1, metav1.CreateOptions{})
	require.NoError(t, err, "Failed to create Pod: %v", err)

	_, err = fakeClient.CoreV1().Pods(pod2.Namespace).Create(context.Background(), pod2, metav1.CreateOptions{})
	require.NoError(t, err, "Failed to create Pod: %v", err)

	// Create a KubernetesHandler object with the fake clientset
	handler := &kubernetesHandler{
		clientSet: fakeClient,
	}

	ctx := context.Background()
	informerFactory := startInformers(ctx, fakeClient, handler)

	// Call the getPodsInDeployment function
	pods, err := handler.getPodsInDeployment(ctx, informerFactory, deployment, replicaset.Name)
	require.NoError(t, err)
	require.Equal(t, 1, len(pods))
	require.Equal(t, pod1.Name, pods[0].Name)
}

func TestGetReplicaSetName(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "ReplicaSet",
					Name: "my-replicaset",
				},
			},
		},
	}

	handler := &kubernetesHandler{}
	replicaSetName := handler.getReplicaSetName(pod)

	require.Equal(t, "my-replicaset", replicaSetName)
}

func TestGetNewestReplicaSetForDeployment(t *testing.T) {
	// Create a fake Kubernetes clientset
	fakeClient := fake.NewSimpleClientset()

	// Create a Deployment object
	deployment := &v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "test-namespace",
		},
	}

	// Create a ReplicaSet object with a higher revision than the other ReplicaSet
	replicaSet1 := &v1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-replicaset-1",
			Namespace:   "test-namespace",
			Annotations: map[string]string{"deployment.kubernetes.io/revision": "1"},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(deployment, schema.GroupVersionKind{
					Group:   v1.SchemeGroupVersion.Group,
					Version: v1.SchemeGroupVersion.Version,
					Kind:    "Deployment",
				}),
			},
		},
	}
	// Create another ReplicaSet object with a lower revision than the other ReplicaSet
	replicaSet2 := &v1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-replicaset-2",
			Namespace:   "test-namespace",
			Annotations: map[string]string{"deployment.kubernetes.io/revision": "0"},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(deployment, schema.GroupVersionKind{
					Group:   v1.SchemeGroupVersion.Group,
					Version: v1.SchemeGroupVersion.Version,
					Kind:    "Deployment",
				}),
			},
		},
	}

	// Add the ReplicaSet objects to the fake Kubernetes clientset
	_, err := fakeClient.AppsV1().ReplicaSets(replicaSet1.Namespace).Create(context.Background(), replicaSet1, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = fakeClient.AppsV1().ReplicaSets(replicaSet2.Namespace).Create(context.Background(), replicaSet2, metav1.CreateOptions{})
	require.NoError(t, err)

	// Add the Deployment object to the fake Kubernetes clientset
	_, err = fakeClient.AppsV1().Deployments(deployment.Namespace).Create(context.Background(), deployment, metav1.CreateOptions{})
	require.NoError(t, err)

	// Create a KubernetesHandler object with the fake clientset
	handler := &kubernetesHandler{
		clientSet: fakeClient,
	}

	ctx := context.Background()
	informerFactory := startInformers(ctx, fakeClient, handler)

	// Call the getNewestReplicaSetForDeployment function
	replicaSetName := handler.getNewestReplicaSetForDeployment(ctx, informerFactory, deployment)
	require.Equal(t, replicaSet1.Name, replicaSetName)
}

func TestCheckPodStatus(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{
					Type:    corev1.PodScheduled,
					Status:  corev1.ConditionFalse,
					Reason:  "Unschedulable",
					Message: "0/1 nodes are available: 1 Insufficient cpu.",
				},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "test-container",
					Ready: true,
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
				},
			},
		},
	}
	podTests := []struct {
		podCondition    corev1.PodCondition
		containerStatus corev1.ContainerStatus
		isReady         bool
		expectedError   string
	}{
		{
			podCondition: corev1.PodCondition{
				Type:    corev1.PodScheduled,
				Status:  corev1.ConditionFalse,
				Reason:  "Unschedulable",
				Message: "0/1 nodes are available: 1 Insufficient cpu.",
			},
			isReady:       false,
			expectedError: "Pod test-pod in namespace test-namespace is not scheduled. Reason: Unschedulable, Message: 0/1 nodes are available: 1 Insufficient cpu.",
		},
		{
			containerStatus: corev1.ContainerStatus{
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						Reason:  "Error",
						Message: "Container terminated due to an error",
					},
				},
			},
			isReady:       false,
			expectedError: "Container state is 'Terminated' Reason: Error, Message: Container terminated due to an error",
		},
		{
			containerStatus: corev1.ContainerStatus{
				State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{
						Reason:  "CrashLoopBackOff",
						Message: "Back-off 5m0s restarting failed container=test-container pod=test-pod",
					},
				},
			},
			isReady:       false,
			expectedError: "Container state is 'Waiting' Reason: CrashLoopBackOff, Message: Back-off 5m0s restarting failed container=test-container pod=test-pod",
		},
		{
			containerStatus: corev1.ContainerStatus{
				State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{},
				},
				Ready: false,
			},
			isReady:       false,
			expectedError: "",
		},
		{
			containerStatus: corev1.ContainerStatus{
				State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{},
				},
				Ready: true,
			},
			isReady:       true,
			expectedError: "",
		},
	}

	ctx := context.Background()
	handler := &kubernetesHandler{}
	for _, tc := range podTests {
		pod.Status.Conditions[0] = tc.podCondition
		pod.Status.ContainerStatuses[0] = tc.containerStatus
		isReady, err := handler.checkPodStatus(ctx, pod)
		if tc.expectedError != "" {
			require.Error(t, err)
			require.Equal(t, tc.expectedError, err.Error())
		} else {
			require.NoError(t, err)
		}
		require.Equal(t, tc.isReady, isReady)
	}
}

func TestCheckDeploymentStatus_AllReady(t *testing.T) {
	// Create a fake Kubernetes fakeClient
	fakeClient := fake.NewSimpleClientset()

	ctx := context.Background()
	_, err := fakeClient.AppsV1().Deployments("test-namespace").Create(ctx, testDeployment, metav1.CreateOptions{})
	require.NoError(t, err)
	replicaSet := addReplicaSetToDeployment(t, ctx, fakeClient, testDeployment)

	// Create a Pod object
	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod1",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "test",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "ReplicaSet",
					Name:       replicaSet.Name,
					Controller: to.Ptr(true),
				},
			},
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodScheduled,
					Status: corev1.ConditionTrue,
				},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "test-container",
					Ready: true,
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
				},
			},
		},
	}

	// Add the Pod object to the fake Kubernetes clientset
	_, err = fakeClient.CoreV1().Pods(pod1.Namespace).Create(ctx, pod1, metav1.CreateOptions{})
	require.NoError(t, err, "Failed to create Pod: %v", err)

	// Create an informer factory and add the deployment to the cache
	informerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
	informerFactory.Apps().V1().Deployments().Informer().GetIndexer().Add(testDeployment)
	informerFactory.Apps().V1().ReplicaSets().Informer().GetIndexer().Add(replicaSet)
	informerFactory.Core().V1().Pods().Informer().GetIndexer().Add(pod1)

	// Create a fake item and object
	item := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":      "test-deployment",
				"namespace": "test-namespace",
			},
		},
	}
	obj := &workqueueItem{
		key: item.GetName(),
		obj: testDeployment,
		meta: &metav1.ObjectMeta{
			Name:      item.GetName(),
			Namespace: item.GetNamespace(),
		},
	}

	// Create a done channel
	doneCh := make(chan error, 1)

	// Call the checkDeploymentStatus function
	handler := &kubernetesHandler{
		clientSet: fakeClient,
	}
	startInformers(ctx, fakeClient, handler)

	go handler.checkDeploymentStatus(ctx, informerFactory, item, obj, doneCh)

	err = <-doneCh

	// Check that the deployment readiness was checked
	require.NoError(t, err)
}

func TestCheckDeploymentStatus_PodsNotReady(t *testing.T) {
	// Create a fake Kubernetes fakeClient
	fakeClient := fake.NewSimpleClientset()

	ctx := context.Background()
	_, err := fakeClient.AppsV1().Deployments("test-namespace").Create(ctx, testDeployment, metav1.CreateOptions{})
	require.NoError(t, err)
	replicaSet := addReplicaSetToDeployment(t, ctx, fakeClient, testDeployment)

	// Create a Pod object
	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod1",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "test",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "ReplicaSet",
					Name:       replicaSet.Name,
					Controller: to.Ptr(true),
				},
			},
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodScheduled,
					Status: corev1.ConditionTrue,
				},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "test-container",
					Ready: true,
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:  "Error",
							Message: "Container terminated due to an error",
						},
					},
				},
			},
		},
	}

	// Add the Pod object to the fake Kubernetes clientset
	_, err = fakeClient.CoreV1().Pods(pod1.Namespace).Create(ctx, pod1, metav1.CreateOptions{})
	require.NoError(t, err, "Failed to create Pod: %v", err)

	// Create an informer factory and add the deployment to the cache
	informerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
	informerFactory.Apps().V1().Deployments().Informer().GetIndexer().Add(testDeployment)
	informerFactory.Apps().V1().ReplicaSets().Informer().GetIndexer().Add(replicaSet)
	informerFactory.Core().V1().Pods().Informer().GetIndexer().Add(pod1)

	// Create a fake item and object
	item := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":      "test-deployment",
				"namespace": "test-namespace",
			},
		},
	}
	obj := &workqueueItem{
		key: item.GetName(),
		obj: testDeployment,
		meta: &metav1.ObjectMeta{
			Name:      item.GetName(),
			Namespace: item.GetNamespace(),
		},
	}

	// Create a done channel
	doneCh := make(chan error, 1)

	// Call the checkDeploymentStatus function
	handler := &kubernetesHandler{
		clientSet: fakeClient,
	}
	startInformers(ctx, fakeClient, handler)

	go handler.checkDeploymentStatus(ctx, informerFactory, item, obj, doneCh)
	err = <-doneCh

	// Check that the deployment readiness was checked
	require.Error(t, err)
	require.Equal(t, err.Error(), "Container state is 'Terminated' Reason: Error, Message: Container terminated due to an error")
}
