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
package portforward

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	k8stest "k8s.io/client-go/testing"
)

// Unfortunately there isn't a good way to test the Run function. watchtools.NewRetryWatcher does not
// work with fake client. Instead we're writing unit tests for all of the state transitions and trying
// to keep the main event loop simple.

func Test_ApplicationWatcher_Run_CanShutDown(t *testing.T) {
	client, _ := createDeploymentWatchFakes()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	aw := NewApplicationWatcher(Options{ApplicationName: "test", Namespace: "default", Client: client})

	go func() { _ = aw.Run(ctx) }()
	cancel()
	aw.Wait()
}

func Test_ApplicationWatcher_Updated_HandleNewDeployment(t *testing.T) {
	client, _ := createDeploymentWatchFakes()

	aw := NewApplicationWatcher(Options{Client: client})
	defer stopDeploymentWatchers(aw)

	aw.updated(context.Background(), createDeployment("test", "1"))
	require.Contains(t, aw.deploymentWatchers, "test")
}

func Test_ApplicationWatcher_Updated_HandleUnchangedDeployment(t *testing.T) {
	client, _ := createDeploymentWatchFakes()

	aw := NewApplicationWatcher(Options{Client: client})
	defer stopDeploymentWatchers(aw)

	// Step 1: Add a deployment
	aw.updated(context.Background(), createDeployment("test", "1"))
	require.Contains(t, aw.deploymentWatchers, "test")
	existing := aw.deploymentWatchers["test"]

	// Step 2: Update the deployment but don't change the selector
	aw.updated(context.Background(), createDeployment("test", "1"))
	require.Contains(t, aw.deploymentWatchers, "test")
	updated := aw.deploymentWatchers["test"]

	// Should be the same instance
	require.Same(t, existing, updated)
}

func Test_ApplicationWatcher_Updated_HandleChangedDeployment(t *testing.T) {
	client, _ := createDeploymentWatchFakes()

	aw := NewApplicationWatcher(Options{Client: client})
	defer stopDeploymentWatchers(aw)

	// Step 1: Add a deployment
	aw.updated(context.Background(), createDeployment("test", "1"))
	require.Contains(t, aw.deploymentWatchers, "test")
	existing := aw.deploymentWatchers["test"]

	// Step 2: Update the deployment and change the selector
	aw.updated(context.Background(), createDeployment("test", "2"))

	require.Contains(t, aw.deploymentWatchers, "test")
	updated := aw.deploymentWatchers["test"]

	// Should not be the same instance
	require.NotSame(t, existing, updated)

	// first watcher should have been canceled
	existing.Wait()
}

func Test_ApplicationWatcher_Deleted(t *testing.T) {
	client, _ := createDeploymentWatchFakes()

	aw := NewApplicationWatcher(Options{Client: client})
	defer stopDeploymentWatchers(aw)

	// Step 1: Add a deployment
	aw.updated(context.Background(), createDeployment("test", "1"))
	require.Contains(t, aw.deploymentWatchers, "test")
	existing := aw.deploymentWatchers["test"]

	// Step 2: Delete the deployment
	aw.deleted(context.Background(), createDeployment("test", "1"))
	require.NotContains(t, aw.deploymentWatchers, "test")

	// watcher should have been canceled
	existing.Wait()
}

func stopDeploymentWatchers(aw *applicationWatcher) {
	for _, entry := range aw.deploymentWatchers {
		entry.Cancel()
		entry.Wait()
	}
}

func createDeployment(name string, value string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name: name,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{
					"a": value,
				},
			},
		},
	}
}

func createDeploymentWatchFakes(objects ...runtime.Object) (*fake.Clientset, *watch.FakeWatcher) {
	client := fake.NewSimpleClientset(objects...)
	watcher := watch.NewFake()
	client.PrependWatchReactor("deployments", k8stest.DefaultWatchReactor(watcher, nil))

	return client, watcher
}
