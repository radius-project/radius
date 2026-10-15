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
	"time"

	"helm.sh/helm/v4/pkg/kube"
	"k8s.io/cli-runtime/pkg/resource"
)

// dashboardDeploymentName is the name of the Radius dashboard Deployment in the Radius chart.
const dashboardDeploymentName = "dashboard"

// skipWaitKubeClient wraps a Helm kube client so that install and upgrade waits ignore
// selected resources. Resources are still created and updated normally, and hook waits
// (WatchUntilReady) and delete waits are unaffected.
type skipWaitKubeClient struct {
	kube.Interface
	skip func(kind, namespace, name string) bool
}

var _ kube.InterfaceWaitOptions = (*skipWaitKubeClient)(nil)

// newSkipDashboardWaitKubeClient returns a kube client whose install and upgrade waits
// skip the Radius dashboard Deployment in namespace. The dashboard is not needed to use
// Radius and its image is much larger than the core control-plane images, so waiting for
// it would add significant time to `rad install` and `rad upgrade`.
func newSkipDashboardWaitKubeClient(client kube.Interface, namespace string) kube.Interface {
	return &skipWaitKubeClient{
		Interface: client,
		skip: func(kind, ns, name string) bool {
			return kind == "Deployment" && ns == namespace && name == dashboardDeploymentName
		},
	}
}

func (c *skipWaitKubeClient) GetWaiter(strategy kube.WaitStrategy) (kube.Waiter, error) {
	waiter, err := c.Interface.GetWaiter(strategy)
	if err != nil {
		return nil, err
	}
	return &skipWaiter{Waiter: waiter, skip: c.skip}, nil
}

func (c *skipWaitKubeClient) GetWaiterWithOptions(strategy kube.WaitStrategy, opts ...kube.WaitOption) (kube.Waiter, error) {
	inner, ok := c.Interface.(kube.InterfaceWaitOptions)
	if !ok {
		return c.GetWaiter(strategy)
	}
	waiter, err := inner.GetWaiterWithOptions(strategy, opts...)
	if err != nil {
		return nil, err
	}
	return &skipWaiter{Waiter: waiter, skip: c.skip}, nil
}

type skipWaiter struct {
	kube.Waiter
	skip func(kind, namespace, name string) bool
}

func (w *skipWaiter) Wait(resources kube.ResourceList, timeout time.Duration) error {
	return w.Waiter.Wait(w.filter(resources), timeout)
}

func (w *skipWaiter) WaitWithJobs(resources kube.ResourceList, timeout time.Duration) error {
	return w.Waiter.WaitWithJobs(w.filter(resources), timeout)
}

func (w *skipWaiter) filter(resources kube.ResourceList) kube.ResourceList {
	return resources.Filter(func(info *resource.Info) bool {
		kind := ""
		if info.Mapping != nil {
			kind = info.Mapping.GroupVersionKind.Kind
		} else if info.Object != nil {
			kind = info.Object.GetObjectKind().GroupVersionKind().Kind
		}
		return !w.skip(kind, info.Namespace, info.Name)
	})
}
