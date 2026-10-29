/*
Copyright 2026 The Radius Authors.

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
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/radius-project/radius/pkg/cli/clierrors"
	clikubernetes "github.com/radius-project/radius/pkg/cli/kubernetes"
	"github.com/radius-project/radius/pkg/cli/output"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	"helm.sh/helm/v4/pkg/storage/driver"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/dynamic"
)

// helmControllerNamespace is the namespace of the HelmCharts with which k3s and RKE2 install their bundled components.
const helmControllerNamespace = "kube-system"

var (
	// k3sCRDWaitInterval and k3sCRDWaitTimeout bound the wait for k3s to install the Traefik CRDs.
	k3sCRDWaitInterval = 2 * time.Second
	k3sCRDWaitTimeout  = 3 * time.Minute
)

// k3sGatewayAPICRDCharts are the HelmCharts with which the helm-controller of k3s and RKE2 installs the Gateway API
// CRDs of Traefik. The helm-controller installs each HelmChart as a Helm release with the same name.
var k3sGatewayAPICRDCharts = []string{"traefik-crd", "rke2-traefik-crd"}

var helmChartGVR = schema.GroupVersionResource{Group: "helm.cattle.io", Version: "v1", Resource: "helmcharts"}

// helmControllerChart is a HelmChart of the helm-controller of k3s and RKE2.
type helmControllerChart struct {
	name      string
	namespace string

	// releaseNamespace is the namespace of the Helm release that the helm-controller installs the chart as.
	releaseNamespace string
}

// contourClusterReader reads the cluster state that the Contour checks use. Tests replace it so that they never
// contact a cluster.
type contourClusterReader interface {
	// CRDs reads the CRDs with the given names, skipping the ones that do not exist.
	CRDs(ctx context.Context, kubeContext string, names []string) ([]crdInfo, error)

	// HelmCharts reads the helm-controller HelmCharts with the given names in namespace. It skips the HelmCharts that
	// do not exist or are being deleted, and returns none when the cluster has no HelmChart resource type.
	HelmCharts(ctx context.Context, kubeContext string, namespace string, names []string) ([]helmControllerChart, error)
}

// liveContourCluster reads the cluster that kubeContext selects.
type liveContourCluster struct{}

func (liveContourCluster) CRDs(ctx context.Context, kubeContext string, names []string) ([]crdInfo, error) {
	return getClusterCRDs(ctx, kubeContext, names)
}

func (liveContourCluster) HelmCharts(ctx context.Context, kubeContext string, namespace string, names []string) ([]helmControllerChart, error) {
	client, err := clikubernetes.NewDynamicClient(kubeContext)
	if err != nil {
		return nil, err
	}

	return getHelmChartsWithClient(ctx, client, namespace, names)
}

// getHelmChartsWithClient reads the helm-controller HelmCharts with the given names in namespace. It skips the
// HelmCharts that do not exist or are being deleted, and returns none when the cluster has no HelmChart resource type.
func getHelmChartsWithClient(ctx context.Context, client dynamic.Interface, namespace string, names []string) ([]helmControllerChart, error) {
	var charts []helmControllerChart
	for _, name := range names {
		object, err := client.Resource(helmChartGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		// The API server also answers NotFound when the cluster has no HelmChart resource type.
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		if object.GetDeletionTimestamp() != nil {
			continue
		}

		releaseNamespace, _, _ := unstructured.NestedString(object.Object, "spec", "targetNamespace")
		if releaseNamespace == "" {
			releaseNamespace = namespace
		}
		charts = append(charts, helmControllerChart{name: name, namespace: namespace, releaseNamespace: releaseNamespace})
	}

	return charts, nil
}

// waitForK3sGatewayAPICRDs waits until the helm-controller of k3s or RKE2 has installed the Gateway API CRDs of
// Traefik. k3s installs them in the background after the cluster starts. If Radius read the CRDs before that, Contour
// would install its own Gateway API CRDs at the same time as k3s, and one of the two installs would fail. It returns
// at once on clusters without these HelmCharts, and when it cannot read them.
func (i *Impl) waitForK3sGatewayAPICRDs(ctx context.Context, kubeContext string) error {
	charts, err := i.cluster().HelmCharts(ctx, kubeContext, helmControllerNamespace, k3sGatewayAPICRDCharts)
	if apierrors.IsForbidden(err) {
		logK3sCheckSkipped(err)
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to check whether k3s is installing Gateway API CRDs, err: %w", err)
	}
	if len(charts) == 0 {
		return nil
	}

	waiting := false
	var installing helmControllerChart
	err = wait.PollUntilContextTimeout(ctx, k3sCRDWaitInterval, k3sCRDWaitTimeout, true, func(context.Context) (bool, error) {
		for _, chart := range charts {
			deployed, err := i.helmControllerChartDeployed(kubeContext, chart)
			if err != nil {
				return false, err
			}
			if deployed {
				continue
			}

			installing = chart
			if !waiting {
				output.LogInfo("Waiting for k3s to finish installing the Traefik CRDs...")
				waiting = true
			}
			return false, nil
		}
		return true, nil
	})
	switch {
	case err == nil:
		return nil
	case ctx.Err() != nil:
		return fmt.Errorf("stopped waiting for k3s to install the Traefik CRDs, err: %w", ctx.Err())
	case wait.Interrupted(err):
		return k3sCRDTimeoutError(installing, kubeContext, k3sCRDWaitTimeout)
	default:
		return err
	}
}

// helmControllerChartDeployed reports whether the latest revision of the Helm release of a helm-controller HelmChart
// is deployed. It returns a friendly error when that revision failed, and reports a release that it cannot read as
// deployed.
func (i *Impl) helmControllerChartDeployed(kubeContext string, chart helmControllerChart) (bool, error) {
	flags := genericclioptions.ConfigFlags{
		Namespace: &chart.releaseNamespace,
		Context:   &kubeContext,
	}
	helmConf, err := initHelmConfig(&flags)
	if err != nil {
		return false, fmt.Errorf("failed to get Helm config, err: %w", err)
	}

	latest, err := i.Helm.RunHelmGet(helmConf, chart.name)
	if errors.Is(err, driver.ErrReleaseNotFound) {
		return false, nil
	} else if apierrors.IsForbidden(err) {
		logK3sCheckSkipped(err)
		return true, nil
	} else if err != nil {
		return false, fmt.Errorf("failed to get Helm release %q in namespace %q, err: %w", chart.name, chart.releaseNamespace, err)
	}

	switch revisionStatus(latest) {
	case releasecommon.StatusDeployed:
		return true, nil
	case releasecommon.StatusFailed:
		return false, clierrors.Message("Radius stopped before changing the cluster because k3s failed to install the Traefik CRDs: revision %d of Helm release %q in namespace %q, which HelmChart %s/%s installs, failed. "+
			"Radius needs to know which Gateway API CRDs the cluster has before it installs or upgrades Contour. "+
			"Find the cause in the logs of the job that installs the chart, for example with %s, fix it, and then run the command again. "+
			"Or create the cluster without Traefik, for example with k3d cluster create --k3s-arg \"--disable=traefik@server:*\".",
			latest.Version, chart.name, chart.releaseNamespace, chart.namespace, chart.name, kubectlCommand(kubeContext, chart.namespace, "logs job/helm-install-"+chart.name))
	default:
		return false, nil
	}
}

// logK3sCheckSkipped warns that Radius cannot check whether k3s is installing the Traefik CRDs, and continues.
func logK3sCheckSkipped(err error) {
	output.LogInfo("Warning: Radius cannot check whether k3s is still installing the Traefik CRDs, so it continues without waiting: %v", err)
}
func k3sCRDTimeoutError(chart helmControllerChart, kubeContext string, timeout time.Duration) error {
	return clierrors.Message("Radius stopped before changing the cluster because k3s has not finished installing the Traefik CRDs after %s: HelmChart %s/%s has not deployed Helm release %q in namespace %q yet. "+
		"Radius needs to know which Gateway API CRDs the cluster has before it installs or upgrades Contour. "+
		"Wait until k3s finishes, for example with %s, and then run the command again. "+
		"Or create the cluster without Traefik, for example with k3d cluster create --k3s-arg \"--disable=traefik@server:*\".",
		timeout, chart.namespace, chart.name, chart.name, chart.releaseNamespace,
		kubectlCommand(kubeContext, chart.namespace, "wait --for=condition=complete job/helm-install-"+chart.name+" --timeout=5m"))
}

// kubectlCommand returns a kubectl command for the cluster that kubeContext selects.
func kubectlCommand(kubeContext string, namespace string, arguments string) string {
	command := "kubectl"
	if kubeContext != "" {
		command += " --context " + QuoteFlagValue(kubeContext)
	}

	return command + " -n " + namespace + " " + arguments
}
