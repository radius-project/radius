/*
Copyright 2025 The Radius Authors.

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

package helm

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/kube"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/resource"
)

type recordingWaiter struct {
	kube.Waiter
	waited      kube.ResourceList
	waitedJobs  kube.ResourceList
	watched     kube.ResourceList
	waitTimeout time.Duration
}

func (w *recordingWaiter) Wait(resources kube.ResourceList, timeout time.Duration) error {
	w.waited, w.waitTimeout = resources, timeout
	return nil
}

func (w *recordingWaiter) WaitWithJobs(resources kube.ResourceList, timeout time.Duration) error {
	w.waitedJobs = resources
	return nil
}

func (w *recordingWaiter) WatchUntilReady(resources kube.ResourceList, timeout time.Duration) error {
	w.watched = resources
	return nil
}

type fakeWaitKubeClient struct {
	kube.Interface
	waiter          *recordingWaiter
	withOptionsUsed bool
}

func (c *fakeWaitKubeClient) GetWaiter(kube.WaitStrategy) (kube.Waiter, error) {
	return c.waiter, nil
}

func (c *fakeWaitKubeClient) GetWaiterWithOptions(kube.WaitStrategy, ...kube.WaitOption) (kube.Waiter, error) {
	c.withOptionsUsed = true
	return c.waiter, nil
}

func testInfo(kind, namespace, name string) *resource.Info {
	return &resource.Info{
		Namespace: namespace,
		Name:      name,
		Mapping:   &meta.RESTMapping{GroupVersionKind: schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kind}},
	}
}

func Test_SkipDashboardWaitKubeClient(t *testing.T) {
	ucp := testInfo("Deployment", "radius-system", "ucp")
	dashboard := testInfo("Deployment", "radius-system", "dashboard")
	dashboardService := testInfo("Service", "radius-system", "dashboard")
	otherDashboard := testInfo("Deployment", "other", "dashboard")
	all := kube.ResourceList{ucp, dashboard, dashboardService, otherDashboard}
	expected := kube.ResourceList{ucp, dashboardService, otherDashboard}

	inner := &fakeWaitKubeClient{waiter: &recordingWaiter{}}
	client := newSkipDashboardWaitKubeClient(inner, "radius-system")

	withOptions, ok := client.(kube.InterfaceWaitOptions)
	require.True(t, ok, "Helm only passes wait options to clients implementing InterfaceWaitOptions")
	waiter, err := withOptions.GetWaiterWithOptions(kube.LegacyStrategy)
	require.NoError(t, err)
	require.True(t, inner.withOptionsUsed)

	require.NoError(t, waiter.Wait(all, time.Minute))
	require.Equal(t, expected, inner.waiter.waited)
	require.Equal(t, time.Minute, inner.waiter.waitTimeout)

	require.NoError(t, waiter.WaitWithJobs(all, time.Minute))
	require.Equal(t, expected, inner.waiter.waitedJobs)

	// Hook waits are not filtered.
	require.NoError(t, waiter.WatchUntilReady(all, time.Minute))
	require.Equal(t, all, inner.waiter.watched)

	waiter, err = client.GetWaiter(kube.LegacyStrategy)
	require.NoError(t, err)
	require.NoError(t, waiter.Wait(all, time.Minute))
	require.Equal(t, expected, inner.waiter.waited)
}
