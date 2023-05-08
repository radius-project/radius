/*
------------------------------------------------------------
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
------------------------------------------------------------
*/
package portforward

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	k8stest "k8s.io/client-go/testing"
)

// Unfortunately there isn't a good way to test the Run function. watchtools.NewRetryWatcher does not
// work with fake client. Instead we're writing unit tests for all of the state transitions and trying
// to keep the main event loop simple.

func Test_DeploymentWatcher_Run_CanShutDown(t *testing.T) {
	client, _ := createPodWatchFakes()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dw := NewDeploymentWatcher(Options{Client: client}, map[string]string{}, map[string]bool{}, cancel)

	go func() { _ = dw.Run(ctx) }()
	cancel()
	dw.Wait()
}

func Test_DeploymentWatcher_Updated_HandleNewDeployment(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dw := NewDeploymentWatcher(Options{}, map[string]string{}, map[string]bool{}, cancel)
	defer stopPodWatchers(dw)

	dw.updated(ctx, createPod("p1", "rs1"))
	require.Equal(t, "p1", dw.podWatcher.Pod.Name)
	cancel()
}

func Test_DeploymentWatcher_Updated_HandleMultipleReplicas(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dw := NewDeploymentWatcher(Options{}, map[string]string{}, map[string]bool{}, cancel)
	defer stopPodWatchers(dw)

	// Step 1: Add a pod
	dw.updated(ctx, createPod("p1", "rs1"))
	require.NotNil(t, dw.podWatcher)
	require.Equal(t, dw.podWatcher.Pod.Name, "p1")
	existing := dw.podWatcher

	// Step 2: Add another pod - this won't start a new watcher
	dw.updated(ctx, createPod("p2", "rs1"))

	// Should be the same instance
	require.Same(t, existing, dw.podWatcher)
}

func Test_DeploymentWatcher_Updated_HandleStalePod(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stale := map[string]bool{
		"rs0": true,
	}

	dw := NewDeploymentWatcher(Options{}, map[string]string{}, stale, cancel)
	defer stopPodWatchers(dw)

	// Step 1: Add a pod (stale)
	dw.updated(ctx, createPod("p1", "rs0"))
	require.Nil(t, dw.podWatcher)

	// Step 2: Add another pod - this will start a watcher
	dw.updated(ctx, createPod("p2", "rs1"))

	// Should be the same instance
	require.NotNil(t, dw.podWatcher)
	require.Equal(t, "p2", dw.podWatcher.Pod.Name)
}

func Test_DeploymentWatcher_Updated_HandleDeletingStateOfWatchedPod_NoOtherReplicas(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dw := NewDeploymentWatcher(Options{}, map[string]string{}, map[string]bool{}, cancel)
	defer stopPodWatchers(dw)

	// Step 1: Add a pod
	dw.updated(ctx, createPod("p1", "rs1"))
	require.NotNil(t, dw.podWatcher)
	require.Equal(t, dw.podWatcher.Pod.Name, "p1")

	// Step 2: Update the pod to set it as deleting
	p1 := createPod("p1", "rs1")
	p1.DeletionTimestamp = &v1.Time{Time: time.Now()}
	dw.updated(ctx, p1)

	// Should be shutdown
	require.Nil(t, dw.podWatcher)
}

func Test_DeploymentWatcher_Updated_HandleDeletingStateOfWatchedPod_HasOtherReplicas(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dw := NewDeploymentWatcher(Options{}, map[string]string{}, map[string]bool{}, cancel)
	defer stopPodWatchers(dw)

	// Step 1: Add a pod
	dw.updated(ctx, createPod("p1", "rs1"))
	require.NotNil(t, dw.podWatcher)
	require.Equal(t, dw.podWatcher.Pod.Name, "p1")
	existing := dw.podWatcher

	// Step 2: Add another pod - this won't start a new watcher
	dw.updated(ctx, createPod("p2", "rs1"))

	// Should be the same instance
	require.Same(t, existing, dw.podWatcher)

	// Step 3: Update p1 to set it as deleting
	p1 := createPod("p1", "rs1")
	p1.DeletionTimestamp = &v1.Time{Time: time.Now()}
	dw.updated(ctx, p1)

	require.NotNil(t, dw.podWatcher)
	require.Equal(t, dw.podWatcher.Pod.Name, "p2")
	require.NotSame(t, existing, dw.podWatcher)

	existing.Wait()
}

func Test_DeploymentWatcher_Deleted_NoOtherReplicas(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dw := NewDeploymentWatcher(Options{}, map[string]string{}, map[string]bool{}, cancel)
	defer stopPodWatchers(dw)

	// Step 1: Add a pod
	dw.updated(context.Background(), createPod("p1", "rs1"))
	require.NotNil(t, dw.podWatcher)
	require.Equal(t, dw.podWatcher.Pod.Name, "p1")
	existing := dw.podWatcher

	// Step 2: Delete the pod
	dw.deleted(ctx, createPod("p1", "rs1"))

	// Should be shutdown
	require.Nil(t, dw.podWatcher)

	existing.Wait()
}

func Test_DeploymentWatcher_Deleted_HasOtherReplicas(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dw := NewDeploymentWatcher(Options{}, map[string]string{}, map[string]bool{}, cancel)
	defer stopPodWatchers(dw)

	// Step 1: Add a pod
	dw.updated(ctx, createPod("p1", "rs1"))
	require.NotNil(t, dw.podWatcher)
	require.Equal(t, dw.podWatcher.Pod.Name, "p1")
	existing := dw.podWatcher

	// Step 2: Add another pod - this won't start a new watcher
	dw.updated(ctx, createPod("p2", "rs1"))

	// Should be the same instance
	require.Same(t, existing, dw.podWatcher)

	// Step 3: Delete the pod
	dw.deleted(ctx, createPod("p1", "rs1"))

	require.NotNil(t, dw.podWatcher)
	require.Equal(t, dw.podWatcher.Pod.Name, "p2")
	require.NotSame(t, existing, dw.podWatcher)

	existing.Wait()
}

func Test_DeploymentWatcher_SelectBestPod(t *testing.T) {
	dw := NewDeploymentWatcher(Options{}, map[string]string{}, map[string]bool{}, func() {})

	// The best pod is chosen based on the newest creation date, with name as a tiebreaker
	dw.pods = map[string]*corev1.Pod{
		"a": &corev1.Pod{ // Oldest
			ObjectMeta: v1.ObjectMeta{
				Name:              "a",
				CreationTimestamp: v1.NewTime(time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)),
			},
		},
		"b": &corev1.Pod{ // Newest - chosen based on name
			ObjectMeta: v1.ObjectMeta{
				Name:              "b",
				CreationTimestamp: v1.NewTime(time.Date(2022, 2, 1, 0, 0, 0, 0, time.UTC)),
			},
		},
		"c": &corev1.Pod{ // Newest - not chosen based on name
			ObjectMeta: v1.ObjectMeta{
				Name:              "c",
				CreationTimestamp: v1.NewTime(time.Date(2022, 2, 1, 0, 0, 0, 0, time.UTC)),
			},
		},
	}
	actual := dw.selectBestPod()
	require.Same(t, dw.pods["b"], actual)
}

func stopPodWatchers(dw *deploymentWatcher) {
	if dw.podWatcher == nil {
		return
	}

	dw.podWatcher.Cancel()
	dw.podWatcher.Wait()
}

func createPod(name string, replicaSetName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name: name,
			OwnerReferences: []v1.OwnerReference{
				{
					APIVersion: "v1",
					Kind:       "ReplicaSet",
					Name:       replicaSetName,
				},
			},
		},
	}
}

func createPodWatchFakes(objects ...runtime.Object) (*fake.Clientset, *watch.FakeWatcher) {
	client := fake.NewSimpleClientset(objects...)
	watcher := watch.NewFake()
	client.PrependWatchReactor("pods", k8stest.DefaultWatchReactor(watcher, nil))

	return client, watcher
}
