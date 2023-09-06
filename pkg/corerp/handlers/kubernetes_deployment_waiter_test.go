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
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/kubernetes"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/test/k8sutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

var testDeployment = &v1.Deployment{
	TypeMeta: metav1.TypeMeta{
		Kind:       "Deployment",
		APIVersion: "apps/v1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:        "test-deployment",
		Namespace:   "test-namespace",
		Annotations: map[string]string{"deployment.kubernetes.io/revision": "1"},
	},
	Spec: v1.DeploymentSpec{
		Replicas: to.Ptr(int32(1)),
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

func TestWaitUntilReady_NewResource(t *testing.T) {
	ctx := context.Background()

	// Create first deployment that will be watched
	deployment := &v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "test-namespace",
			Labels: map[string]string{
				kubernetes.LabelManagedBy: kubernetes.LabelManagedByRadiusRP,
			},
			Annotations: map[string]string{"deployment.kubernetes.io/revision": "1"},
		},
		Spec: v1.DeploymentSpec{
			Replicas: to.Ptr(int32(1)),
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
		client: k8sutil.NewFakeKubeClient(nil),
		deploymentWaiter: &deploymentWaiter{
			clientSet:           clientset,
			deploymentTimeOut:   time.Duration(50) * time.Second,
			cacheResyncInterval: time.Duration(10) * time.Second,
		},
	}

	err := handler.deploymentWaiter.waitUntilReady(ctx, deployment)
	require.NoError(t, err, "Failed to wait for deployment to be ready")
}

func TestWaitUntilReady_Timeout(t *testing.T) {
	ctx := context.Background()
	// Create first deployment that will be watched
	deployment := &v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-deployment",
			Namespace:   "test-namespace",
			Annotations: map[string]string{"deployment.kubernetes.io/revision": "1"},
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
		client: k8sutil.NewFakeKubeClient(nil),
		deploymentWaiter: &deploymentWaiter{
			clientSet:           deploymentClient,
			deploymentTimeOut:   time.Duration(1) * time.Second,
			cacheResyncInterval: time.Duration(10) * time.Second,
		},
	}

	err := handler.deploymentWaiter.waitUntilReady(ctx, deployment)
	require.Error(t, err)
	require.Equal(t, "deployment timed out, name: test-deployment, namespace test-namespace, status: Deployment has minimum availability, reason: NewReplicaSetAvailable", err.Error())
}

func TestWaitUntilReady_DifferentResourceName(t *testing.T) {
	ctx := context.Background()
	// Create first deployment that will be watched
	deployment := &v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-deployment",
			Namespace:   "test-namespace",
			Annotations: map[string]string{"deployment.kubernetes.io/revision": "1"},
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
		deploymentWaiter: &deploymentWaiter{
			clientSet:           clientset,
			deploymentTimeOut:   time.Duration(1) * time.Second,
			cacheResyncInterval: time.Duration(10) * time.Second,
		},
	}

	err := handler.deploymentWaiter.waitUntilReady(ctx, &v1.Deployment{
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
			Name:        "test-deployment",
			Namespace:   "test-namespace",
			Annotations: map[string]string{"deployment.kubernetes.io/revision": "1"},
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
			Annotations: map[string]string{"deployment.kubernetes.io/revision": "1"},
			UID:         "1234",
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
					UID:        "1234",
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
					UID:        "1234",
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
	deploymentWaiter := &deploymentWaiter{
		clientSet: fakeClient,
	}
	handler := &kubernetesHandler{
		deploymentWaiter: deploymentWaiter,
	}

	ctx := context.Background()
	informerFactory := startInformers(ctx, fakeClient, handler)

	// Call the getPodsInDeployment function
	pods, err := deploymentWaiter.getPodsInDeployment(ctx, informerFactory, deployment, replicaset)
	require.NoError(t, err)
	require.Equal(t, 1, len(pods))
	require.Equal(t, pod1.Name, pods[0].Name)
}

func TestGetCurrentReplicaSetForDeployment(t *testing.T) {
	// Create a fake Kubernetes clientset
	fakeClient := fake.NewSimpleClientset()

	// Create a Deployment object
	deployment := &v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-deployment",
			Namespace:   "test-namespace",
			Annotations: map[string]string{"deployment.kubernetes.io/revision": "1"},
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

	// Create another ReplicaSet object with a higher revision than the other ReplicaSet
	replicaSet3 := &v1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-replicaset-3",
			Namespace:   "test-namespace",
			Annotations: map[string]string{"deployment.kubernetes.io/revision": "3"},
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
	_, err = fakeClient.AppsV1().ReplicaSets(replicaSet2.Namespace).Create(context.Background(), replicaSet3, metav1.CreateOptions{})
	require.NoError(t, err)

	// Add the Deployment object to the fake Kubernetes clientset
	_, err = fakeClient.AppsV1().Deployments(deployment.Namespace).Create(context.Background(), deployment, metav1.CreateOptions{})
	require.NoError(t, err)

	// Create a KubernetesHandler object with the fake clientset
	deploymentWaiter := &deploymentWaiter{
		clientSet: fakeClient,
	}
	handler := &kubernetesHandler{
		deploymentWaiter: deploymentWaiter,
	}

	ctx := context.Background()
	informerFactory := startInformers(ctx, fakeClient, handler)

	// Call the getNewestReplicaSetForDeployment function
	rs := deploymentWaiter.getCurrentReplicaSetForDeployment(ctx, informerFactory, deployment)
	require.Equal(t, replicaSet1.Name, rs.Name)
}

func TestCheckPodStatus(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
		},
		Status: corev1.PodStatus{},
	}

	podTests := []struct {
		podCondition    []corev1.PodCondition
		containerStatus []corev1.ContainerStatus
		isReady         bool
		expectedError   string
	}{
		{
			// Container is in Terminated state
			podCondition: nil,
			containerStatus: []corev1.ContainerStatus{
				{
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:  "Error",
							Message: "Container terminated due to an error",
						},
					},
				},
			},
			isReady:       false,
			expectedError: "Container state is 'Terminated' Reason: Error, Message: Container terminated due to an error",
		},
		{
			// Container is in CrashLoopBackOff state
			podCondition: nil,
			containerStatus: []corev1.ContainerStatus{
				{
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CrashLoopBackOff",
							Message: "Back-off 5m0s restarting failed container=test-container pod=test-pod",
						},
					},
				},
			},
			isReady:       false,
			expectedError: "Container state is 'Waiting' Reason: CrashLoopBackOff, Message: Back-off 5m0s restarting failed container=test-container pod=test-pod",
		},
		{
			// Container is in ErrImagePull state
			podCondition: nil,
			containerStatus: []corev1.ContainerStatus{
				{
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "ErrImagePull",
							Message: "Cannot pull image",
						},
					},
				},
			},
			isReady:       false,
			expectedError: "Container state is 'Waiting' Reason: ErrImagePull, Message: Cannot pull image",
		},
		{
			// Container is in ImagePullBackOff state
			podCondition: nil,
			containerStatus: []corev1.ContainerStatus{
				{
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "ImagePullBackOff",
							Message: "ImagePullBackOff",
						},
					},
				},
			},
			isReady:       false,
			expectedError: "Container state is 'Waiting' Reason: ImagePullBackOff, Message: ImagePullBackOff",
		},
		{
			// No container statuses available
			isReady:       false,
			expectedError: "",
		},
		{
			// Container is in Waiting state but not a terminally failed state
			podCondition: nil,
			containerStatus: []corev1.ContainerStatus{
				{
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "ContainerCreating",
							Message: "Container is being created",
						},
					},
					Ready: false,
				},
			},
			isReady:       false,
			expectedError: "",
		},
		{
			// Container's Running state is nil
			podCondition: nil,
			containerStatus: []corev1.ContainerStatus{
				{
					State: corev1.ContainerState{
						Running: nil,
					},
					Ready: false,
				},
			},
			isReady:       false,
			expectedError: "",
		},
		{
			// Readiness check is not yet passed
			podCondition: nil,
			containerStatus: []corev1.ContainerStatus{
				{
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
					Ready: false,
				},
			},
			isReady:       false,
			expectedError: "",
		},
		{
			// Container is in Ready state
			podCondition: nil,
			containerStatus: []corev1.ContainerStatus{
				{
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
					Ready: true,
				},
			},
			isReady:       true,
			expectedError: "",
		},
	}

	ctx := context.Background()
	deploymentWaiter := NewDeploymentWaiter(fake.NewSimpleClientset())
	for _, tc := range podTests {
		pod.Status.Conditions = tc.podCondition
		pod.Status.ContainerStatuses = tc.containerStatus
		isReady, err := deploymentWaiter.checkPodStatus(ctx, pod)
		if tc.expectedError != "" {
			require.Error(t, err)
			require.Equal(t, tc.expectedError, err.Error())
		} else {
			require.NoError(t, err)
		}
		require.Equal(t, tc.isReady, isReady)
	}
}

func TestCheckAllPodsReady_Success(t *testing.T) {
	// Create a fake Kubernetes clientset
	clientset := fake.NewSimpleClientset()

	ctx := context.Background()

	_, err := clientset.AppsV1().Deployments("test-namespace").Create(ctx, testDeployment, metav1.CreateOptions{})
	require.NoError(t, err)

	replicaSet := addReplicaSetToDeployment(t, ctx, clientset, testDeployment)

	// Create a pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "test",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "test-container",
					Image: "test-image",
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "test-container",
					Ready: true,
				},
			},
		},
	}
	_, err = clientset.CoreV1().Pods("test-namespace").Create(context.Background(), pod, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Create an informer factory and add the deployment and replica set to the cache
	informerFactory := informers.NewSharedInformerFactory(clientset, 0)
	addTestObjects(t, clientset, informerFactory, testDeployment, replicaSet, pod)

	// Create a done channel
	doneCh := make(chan error)

	// Create a handler with the fake clientset
	deploymentWaiter := &deploymentWaiter{
		clientSet: clientset,
	}

	// Call the checkAllPodsReady function
	allReady := deploymentWaiter.checkAllPodsReady(ctx, informerFactory, testDeployment, replicaSet, doneCh)

	// Check that all pods are ready
	require.True(t, allReady)
}

func TestCheckAllPodsReady_Fail(t *testing.T) {
	// Create a fake Kubernetes clientset
	clientset := fake.NewSimpleClientset()

	ctx := context.Background()

	_, err := clientset.AppsV1().Deployments("test-namespace").Create(ctx, testDeployment, metav1.CreateOptions{})
	require.NoError(t, err)

	replicaSet := addReplicaSetToDeployment(t, ctx, clientset, testDeployment)

	// Create a pod
	pod := &corev1.Pod{
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
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "test-container",
					Image: "test-image",
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "test-container",
					Ready: false,
				},
			},
		},
	}
	_, err = clientset.CoreV1().Pods(pod.Namespace).Create(context.Background(), pod, metav1.CreateOptions{})
	require.NoError(t, err)

	// Create an informer factory and add the deployment and replica set to the cache
	informerFactory := informers.NewSharedInformerFactory(clientset, 0)
	addTestObjects(t, clientset, informerFactory, testDeployment, replicaSet, pod)

	// Create a done channel
	doneCh := make(chan error)

	// Create a handler with the fake clientset
	deploymentWaiter := &deploymentWaiter{
		clientSet: clientset,
	}

	// Call the checkAllPodsReady function
	allReady := deploymentWaiter.checkAllPodsReady(ctx, informerFactory, testDeployment, replicaSet, doneCh)

	// Check that all pods are ready
	require.False(t, allReady)
}

func TestCheckDeploymentStatus_AllReady(t *testing.T) {
	// Create a fake Kubernetes fakeClient
	fakeClient := fake.NewSimpleClientset()

	ctx := context.Background()
	_, err := fakeClient.AppsV1().Deployments("test-namespace").Create(ctx, testDeployment, metav1.CreateOptions{})
	require.NoError(t, err)
	replicaSet := addReplicaSetToDeployment(t, ctx, fakeClient, testDeployment)

	// Create a Pod object
	pod := &corev1.Pod{
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
	_, err = fakeClient.CoreV1().Pods(pod.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	require.NoError(t, err, "Failed to create Pod: %v", err)

	// Create an informer factory and add the deployment to the cache
	informerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
	addTestObjects(t, fakeClient, informerFactory, testDeployment, replicaSet, pod)

	// Create a fake item and object
	item := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":      "test-deployment",
				"namespace": "test-namespace",
			},
		},
	}

	// Create a done channel
	doneCh := make(chan error, 1)

	// Call the checkDeploymentStatus function
	deploymentWaiter := &deploymentWaiter{
		clientSet: fakeClient,
	}

	deploymentWaiter.checkDeploymentStatus(ctx, informerFactory, item, doneCh)

	err = <-doneCh

	// Check that the deployment readiness was checked
	require.Nil(t, err)
}

func TestCheckDeploymentStatus_NoReplicaSetsFound(t *testing.T) {
	// Create a fake Kubernetes fakeClient
	fakeClient := fake.NewSimpleClientset()

	ctx := context.Background()
	_, err := fakeClient.AppsV1().Deployments("test-namespace").Create(ctx, testDeployment, metav1.CreateOptions{})
	require.NoError(t, err)

	// Create a Pod object
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod1",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "test",
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
	_, err = fakeClient.CoreV1().Pods(pod.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	require.NoError(t, err, "Failed to create Pod: %v", err)

	// Create an informer factory and add the deployment to the cache
	informerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
	err = informerFactory.Apps().V1().Deployments().Informer().GetIndexer().Add(testDeployment)
	require.NoError(t, err, "Failed to add deployment to informer cache")
	err = informerFactory.Core().V1().Pods().Informer().GetIndexer().Add(pod)
	require.NoError(t, err, "Failed to add pod to informer cache")
	// Note: No replica set added

	// Create a fake item and object
	item := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":      "test-deployment",
				"namespace": "test-namespace",
			},
		},
	}

	// Create a done channel
	doneCh := make(chan error, 1)

	// Call the checkDeploymentStatus function
	deploymentWaiter := &deploymentWaiter{
		clientSet: fakeClient,
	}

	allReady := deploymentWaiter.checkDeploymentStatus(ctx, informerFactory, item, doneCh)

	// Check that the deployment readiness was checked
	require.False(t, allReady)
}

func TestCheckDeploymentStatus_PodsNotReady(t *testing.T) {
	// Create a fake Kubernetes fakeClient
	fakeClient := fake.NewSimpleClientset()

	ctx := context.Background()
	_, err := fakeClient.AppsV1().Deployments("test-namespace").Create(ctx, testDeployment, metav1.CreateOptions{})
	require.NoError(t, err)
	replicaSet := addReplicaSetToDeployment(t, ctx, fakeClient, testDeployment)

	// Create a Pod object
	pod := &corev1.Pod{
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
	_, err = fakeClient.CoreV1().Pods(pod.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	require.NoError(t, err, "Failed to create Pod: %v", err)

	// Create an informer factory and add the deployment to the cache
	informerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
	addTestObjects(t, fakeClient, informerFactory, testDeployment, replicaSet, pod)

	// Create a fake item and object
	item := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":      "test-deployment",
				"namespace": "test-namespace",
			},
		},
	}

	// Create a done channel
	doneCh := make(chan error, 1)

	// Call the checkDeploymentStatus function
	deploymentWaiter := &deploymentWaiter{
		clientSet: fakeClient,
	}

	go deploymentWaiter.checkDeploymentStatus(ctx, informerFactory, item, doneCh)
	err = <-doneCh

	// Check that the deployment readiness was checked
	require.Error(t, err)
	require.Equal(t, err.Error(), "Container state is 'Terminated' Reason: Error, Message: Container terminated due to an error")
}

func TestCheckDeploymentStatus_ObservedGenerationMismatch(t *testing.T) {
	// Modify testDeployment to have a different generation than the observed generation
	generationMismatchDeployment := testDeployment.DeepCopy()
	generationMismatchDeployment.Generation = 2

	// Create a fake Kubernetes fakeClient
	fakeClient := fake.NewSimpleClientset()

	ctx := context.Background()
	_, err := fakeClient.AppsV1().Deployments("test-namespace").Create(ctx, generationMismatchDeployment, metav1.CreateOptions{})
	require.NoError(t, err)
	replicaSet := addReplicaSetToDeployment(t, ctx, fakeClient, generationMismatchDeployment)

	// Create a Pod object
	pod := &corev1.Pod{
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
	_, err = fakeClient.CoreV1().Pods(pod.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	require.NoError(t, err, "Failed to create Pod: %v", err)

	// Create an informer factory and add the deployment to the cache
	informerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
	addTestObjects(t, fakeClient, informerFactory, generationMismatchDeployment, replicaSet, pod)

	// Create a fake item and object
	item := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":      "test-deployment",
				"namespace": "test-namespace",
			},
		},
	}

	// Create a done channel
	doneCh := make(chan error, 1)

	// Call the checkDeploymentStatus function
	deploymentWaiter := &deploymentWaiter{
		clientSet: fakeClient,
	}

	deploymentWaiter.checkDeploymentStatus(ctx, informerFactory, item, doneCh)

	// Check that the deployment readiness was checked
	require.Zero(t, len(doneCh))
}

func TestCheckDeploymentStatus_DeploymentNotProgressing(t *testing.T) {
	// Create a fake Kubernetes fakeClient
	fakeClient := fake.NewSimpleClientset()

	deploymentNotProgressing := testDeployment.DeepCopy()

	ctx := context.Background()
	_, err := fakeClient.AppsV1().Deployments("test-namespace").Create(ctx, deploymentNotProgressing, metav1.CreateOptions{})
	require.NoError(t, err)
	replicaSet := addReplicaSetToDeployment(t, ctx, fakeClient, deploymentNotProgressing)

	// Create a Pod object
	pod := &corev1.Pod{
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
	_, err = fakeClient.CoreV1().Pods(pod.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	require.NoError(t, err, "Failed to create Pod: %v", err)

	// Create an informer factory and add the deployment to the cache
	informerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
	addTestObjects(t, fakeClient, informerFactory, deploymentNotProgressing, replicaSet, pod)

	deploymentNotProgressing.Status = v1.DeploymentStatus{
		Conditions: []v1.DeploymentCondition{
			{
				Type:    v1.DeploymentProgressing,
				Status:  corev1.ConditionFalse,
				Reason:  "NewReplicaSetAvailable",
				Message: "Deployment has minimum availability",
			},
		},
	}

	// Create a fake item and object
	item := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":      "test-deployment",
				"namespace": "test-namespace",
			},
		},
	}

	// Create a done channel
	doneCh := make(chan error, 1)

	// Call the checkDeploymentStatus function
	deploymentWaiter := &deploymentWaiter{
		clientSet: fakeClient,
	}

	ready := deploymentWaiter.checkDeploymentStatus(ctx, informerFactory, item, doneCh)
	require.False(t, ready)
}

func addTestObjects(t *testing.T, fakeClient *fake.Clientset, informerFactory informers.SharedInformerFactory, deployment *v1.Deployment, replicaSet *v1.ReplicaSet, pod *corev1.Pod) {
	err := informerFactory.Apps().V1().Deployments().Informer().GetIndexer().Add(deployment)
	require.NoError(t, err, "Failed to add deployment to informer cache")
	err = informerFactory.Apps().V1().ReplicaSets().Informer().GetIndexer().Add(replicaSet)
	require.NoError(t, err, "Failed to add replica set to informer cache")
	err = informerFactory.Core().V1().Pods().Informer().GetIndexer().Add(pod)
	require.NoError(t, err, "Failed to add pod to informer cache")
}
