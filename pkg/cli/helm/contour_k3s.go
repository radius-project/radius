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
	"slices"
	"strings"
	"time"

	"github.com/radius-project/radius/pkg/cli/clierrors"
	clikubernetes "github.com/radius-project/radius/pkg/cli/kubernetes"
	helm "helm.sh/helm/v4/pkg/action"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage/driver"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
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

// k3sGatewayAPICRDs are the Gateway API CRDs that both the Traefik CRD charts of k3s and the Contour chart include.
// Helm does not install a chart whose resources belong to something else, so the helm-controller cannot install the
// Traefik CRDs while one of these CRDs exists and belongs to something else than their release.
var k3sGatewayAPICRDs = []string{
	"backendtlspolicies." + gatewayAPIGroup,
	"gatewayclasses." + gatewayAPIGroup,
	"gateways." + gatewayAPIGroup,
	"grpcroutes." + gatewayAPIGroup,
	"httproutes." + gatewayAPIGroup,
	"referencegrants." + gatewayAPIGroup,
}

var helmChartGVR = schema.GroupVersionResource{Group: "helm.cattle.io", Version: "v1", Resource: "helmcharts"}

// helmControllerChart is a HelmChart of the helm-controller of k3s and RKE2.
type helmControllerChart struct {
	name      string
	namespace string

	// releaseNamespace is the namespace of the Helm release that the helm-controller installs the chart as.
	releaseNamespace string

	// jobName is the name of the Job that installs the chart from the status of the HelmChart, or empty when the
	// status does not name one.
	jobName string
}

// job returns the name of the Job with which the helm-controller installs the chart in the namespace of the HelmChart.
func (c helmControllerChart) job() string {
	if c.jobName != "" {
		return c.jobName
	}

	return "helm-install-" + c.name
}

// helmControllerJob is the state of a Job with which the helm-controller installs a HelmChart.
type helmControllerJob struct {
	// complete is true when the Job finished successfully.
	complete bool
}

// helmControllerPods describes the Pods of a Job with which the helm-controller installs a HelmChart. The Job restarts
// a failing install in the same Pod rather than creating a failed Pod, so the restarts show that the install fails.
type helmControllerPods struct {
	count    int
	restarts int32

	// waitingReasons are the reasons why containers of the Pods wait, such as CrashLoopBackOff.
	waitingReasons []string
}

// contourClusterReader reads the cluster state that the Contour checks use. Tests replace it so that they never
// contact a cluster.
type contourClusterReader interface {
	// CRDs reads the CRDs with the given names, skipping the ones that do not exist.
	CRDs(ctx context.Context, kubeContext string, names []string) ([]crdInfo, error)

	// HelmCharts reads the helm-controller HelmCharts with the given names in namespace. It skips the HelmCharts that
	// do not exist or are being deleted, and returns none when the cluster has no HelmChart resource type.
	HelmCharts(ctx context.Context, kubeContext string, namespace string, names []string) ([]helmControllerChart, error)

	// Job reads the Job with the given name in namespace. It returns the error of the API server, for example NotFound
	// when the Job does not exist.
	Job(ctx context.Context, kubeContext string, namespace string, name string) (helmControllerJob, error)

	// JobPods reads the Pods of the Job with the given name in namespace.
	JobPods(ctx context.Context, kubeContext string, namespace string, job string) (helmControllerPods, error)

	// RenderConfiguration returns the Helm configuration with which the Contour checks render a chart as Helm renders
	// it in the cluster: with the capabilities of the cluster, and with template lookups that read it. A dry run with
	// the configuration never changes the cluster.
	RenderConfiguration(kubeContext string, namespace string) (*helm.Configuration, error)
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

func (liveContourCluster) Job(ctx context.Context, kubeContext string, namespace string, name string) (helmControllerJob, error) {
	client, _, err := clikubernetes.NewClientset(kubeContext)
	if err != nil {
		return helmControllerJob{}, err
	}

	return getJobWithClient(ctx, client, namespace, name)
}

func (liveContourCluster) JobPods(ctx context.Context, kubeContext string, namespace string, job string) (helmControllerPods, error) {
	client, _, err := clikubernetes.NewClientset(kubeContext)
	if err != nil {
		return helmControllerPods{}, err
	}

	return getJobPodsWithClient(ctx, client, namespace, job)
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
		jobName, _, _ := unstructured.NestedString(object.Object, "status", "jobName")
		charts = append(charts, helmControllerChart{name: name, namespace: namespace, releaseNamespace: releaseNamespace, jobName: jobName})
	}

	return charts, nil
}

// getJobWithClient reads the Job with the given name in namespace.
func getJobWithClient(ctx context.Context, client kubernetes.Interface, namespace string, name string) (helmControllerJob, error) {
	job, err := client.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return helmControllerJob{}, err
	}

	completions := int32(1)
	if job.Spec.Completions != nil {
		completions = max(*job.Spec.Completions, 1)
	}
	state := helmControllerJob{complete: job.Status.Succeeded >= completions}
	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobComplete && condition.Status == corev1.ConditionTrue {
			state.complete = true
		}
	}

	return state, nil
}

// getJobPodsWithClient reads the Pods of the Job with the given name in namespace.
func getJobPodsWithClient(ctx context.Context, client kubernetes.Interface, namespace string, job string) (helmControllerPods, error) {
	list, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: "job-name=" + job})
	if err != nil {
		return helmControllerPods{}, err
	}

	pods := helmControllerPods{count: len(list.Items)}
	for _, pod := range list.Items {
		for _, status := range slices.Concat(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses) {
			pods.restarts += status.RestartCount
			if waiting := status.State.Waiting; waiting != nil && waiting.Reason != "" && !slices.Contains(pods.waitingReasons, waiting.Reason) {
				pods.waitingReasons = append(pods.waitingReasons, waiting.Reason)
			}
		}
	}
	slices.Sort(pods.waitingReasons)

	return pods, nil
}

// k3sChartWait is what Radius observed about a HelmChart while it waits for the helm-controller to install it.
type k3sChartWait struct {
	chart helmControllerChart

	// done is true when Radius no longer waits for the chart.
	done bool

	// release is the latest revision of the Helm release of the chart, or nil when the release does not exist.
	release *releasev1.Release

	// job is the Job that installs the chart, or nil when Radius did not read it or could not. jobMissing is true when
	// the Job does not exist.
	job        *helmControllerJob
	jobMissing bool

	// pods describes the Pods of the Job, or is nil when Radius did not read them or could not. Radius reads them only
	// for the message that reports that k3s did not finish.
	pods *helmControllerPods

	// crdsChecked is true when Radius checked whether a CRD stops the helm-controller from installing the chart. The
	// CRDs are large, so Radius does not read them on every poll.
	crdsChecked bool
}

// waitForK3sGatewayAPICRDs waits until the helm-controller of k3s or RKE2 has installed the Gateway API CRDs of
// Traefik. k3s installs them in the background after the cluster starts. If Radius read the CRDs before that, Contour
// would install its own Gateway API CRDs at the same time as k3s, and one of the two installs would fail. It returns
// at once on clusters without these HelmCharts, and when it cannot read them. It does not wait when the
// helm-controller cannot install the CRDs or will not try again. It waits once for each Kubernetes context.
func (i *Impl) waitForK3sGatewayAPICRDs(ctx context.Context, kubeContext string) error {
	i.k3sWaitedLock.Lock()
	waited := i.k3sWaited[kubeContext]
	i.k3sWaitedLock.Unlock()
	if waited {
		return nil
	}

	if err := i.awaitK3sGatewayAPICRDs(ctx, kubeContext); err != nil {
		return err
	}

	i.k3sWaitedLock.Lock()
	defer i.k3sWaitedLock.Unlock()
	if i.k3sWaited == nil {
		i.k3sWaited = map[string]bool{}
	}
	i.k3sWaited[kubeContext] = true

	return nil
}

// awaitK3sGatewayAPICRDs waits for the helm-controller as waitForK3sGatewayAPICRDs describes.
func (i *Impl) awaitK3sGatewayAPICRDs(ctx context.Context, kubeContext string) error {
	charts, err := i.cluster().HelmCharts(ctx, kubeContext, helmControllerNamespace, k3sGatewayAPICRDCharts)
	if apierrors.IsForbidden(err) {
		i.logK3sCheckSkipped(err)
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to check whether k3s is installing Gateway API CRDs, err: %w", err)
	}
	if len(charts) == 0 {
		return nil
	}

	waits := make([]*k3sChartWait, 0, len(charts))
	for _, chart := range charts {
		waits = append(waits, &k3sChartWait{chart: chart})
	}
	waiting := false
	err = wait.PollUntilContextTimeout(ctx, k3sCRDWaitInterval, k3sCRDWaitTimeout, true, func(ctx context.Context) (bool, error) {
		done, err := i.k3sChartsInstalled(ctx, kubeContext, waits, false)
		if err == nil && !done && !waiting {
			i.logInfo("Waiting for k3s to finish installing the Traefik CRDs...")
			waiting = true
		}
		return done, err
	})
	switch {
	case err == nil:
		return nil
	case ctx.Err() != nil:
		return fmt.Errorf("stopped waiting for k3s to install the Traefik CRDs, err: %w", ctx.Err())
	case !wait.Interrupted(err):
		return err
	}

	// Look once more, and read the CRDs again, before reporting that k3s did not finish.
	done, err := i.k3sChartsInstalled(ctx, kubeContext, waits, true)
	if err != nil || done {
		return err
	}
	for _, w := range waits {
		if !w.done {
			return k3sCRDTimeoutError(w, kubeContext, k3sCRDWaitTimeout)
		}
	}

	return nil
}

// k3sChartsInstalled reports whether Radius can stop waiting for the helm-controller to install the charts of waits.
// final is true for the last look before Radius reports that k3s did not finish.
func (i *Impl) k3sChartsInstalled(ctx context.Context, kubeContext string, waits []*k3sChartWait, final bool) (bool, error) {
	for _, w := range waits {
		if w.done {
			continue
		}

		done, err := i.k3sChartInstalled(ctx, kubeContext, w, final)
		if err != nil || !done {
			return false, err
		}
		w.done = true
	}

	return true, nil
}

// k3sChartInstalled reports whether Radius can stop waiting for the helm-controller to install the chart of w: the
// latest revision of its Helm release is deployed, or the helm-controller cannot install the chart or will not try
// again. It returns a friendly error when that revision failed, and does not wait for a release that it cannot read.
// It reads the CRDs the first time that the helm-controller is not installing the chart, and again when final is true.
func (i *Impl) k3sChartInstalled(ctx context.Context, kubeContext string, w *k3sChartWait, final bool) (bool, error) {
	chart := w.chart
	flags := genericclioptions.ConfigFlags{
		Namespace: &chart.releaseNamespace,
		Context:   &kubeContext,
	}
	helmConf, err := initHelmConfig(&flags)
	if err != nil {
		return false, fmt.Errorf("failed to get Helm config, err: %w", err)
	}

	latest, err := i.Helm.RunHelmGet(helmConf, chart.name)
	switch {
	case errors.Is(err, driver.ErrReleaseNotFound):
		latest = nil
	case apierrors.IsForbidden(err):
		i.logK3sCheckSkipped(err)
		return true, nil
	case err != nil:
		return false, fmt.Errorf("failed to get Helm release %q in namespace %q, err: %w", chart.name, chart.releaseNamespace, err)
	}
	w.release = latest

	installing := false
	if latest != nil {
		switch status := revisionStatus(latest); {
		case status == releasecommon.StatusDeployed:
			return true, nil
		case status == releasecommon.StatusFailed:
			return false, k3sChartFailedError(chart, latest, kubeContext)
		case status.IsPending():
			installing = true
		}
	}

	// Otherwise the release does not exist, was uninstalled, or has another status. Helm checks the ownership of the
	// resources of a chart before it records a release, so the helm-controller cannot install the chart while one of
	// its CRDs belongs to something else, and the release never appears.
	if !installing && (final || !w.crdsChecked) {
		w.crdsChecked = true
		crd, blocked, err := i.k3sBlockingCRD(ctx, kubeContext, chart)
		if err != nil {
			return false, err
		}
		if blocked {
			i.logInfo("Warning: k3s cannot install the Traefik CRDs of HelmChart %s/%s because CRD %s belongs to %s, so Radius continues without waiting for k3s. To see why k3s does not install them, run %s.",
				chart.namespace, chart.name, crd.name, crd.owner(), kubectlCommand(kubeContext, chart.namespace, "logs job/"+chart.job()))
			return true, nil
		}
	}
	if installing && !final {
		return false, nil
	}

	// The helm-controller does not run a Job again once it completed.
	if err := i.readK3sJob(ctx, kubeContext, w); err != nil {
		return false, err
	}
	if !installing && w.job != nil && w.job.complete {
		i.logInfo("Warning: k3s does not install the Traefik CRDs of HelmChart %s/%s again: job %s finished, but %s. Radius continues without waiting for k3s.",
			chart.namespace, chart.name, chart.job(), describeK3sRelease(chart, latest))
		return true, nil
	}
	if final && w.job != nil && !w.job.complete {
		// The Pods only make the message more useful, so Radius reports the timeout even when it cannot read them.
		if pods, err := i.cluster().JobPods(ctx, kubeContext, chart.namespace, chart.job()); err == nil {
			w.pods = &pods
		}
	}

	return false, nil
}

// k3sBlockingCRD returns a CRD that the Traefik CRD chart includes and that exists and belongs to something else than
// the Helm release of chart.
func (i *Impl) k3sBlockingCRD(ctx context.Context, kubeContext string, chart helmControllerChart) (crdInfo, bool, error) {
	crds, err := i.cluster().CRDs(ctx, kubeContext, k3sGatewayAPICRDs)
	if err != nil {
		return crdInfo{}, false, fmt.Errorf("failed to read CRDs, err: %w", err)
	}
	for _, name := range k3sGatewayAPICRDs {
		if crd, found := findCRD(crds, name); found && !crd.ownedBy(chart.name, chart.releaseNamespace) {
			return crd, true, nil
		}
	}

	return crdInfo{}, false, nil
}

// readK3sJob reads the Job that installs the chart of w. A Job that does not exist or that Radius cannot read tells
// nothing.
func (i *Impl) readK3sJob(ctx context.Context, kubeContext string, w *k3sChartWait) error {
	job, err := i.cluster().Job(ctx, kubeContext, w.chart.namespace, w.chart.job())
	w.job, w.jobMissing = nil, apierrors.IsNotFound(err)
	if apierrors.IsNotFound(err) || apierrors.IsForbidden(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get job %q in namespace %q, err: %w", w.chart.job(), w.chart.namespace, err)
	}
	w.job = &job

	return nil
}

// describeK3sRelease describes the Helm release of chart, whose latest revision is latest, or nil when the release does
// not exist.
func describeK3sRelease(chart helmControllerChart, latest *releasev1.Release) string {
	if latest == nil {
		return fmt.Sprintf("Helm release %q in namespace %q does not exist", chart.name, chart.releaseNamespace)
	}

	status := revisionStatus(latest)
	if status == "" {
		status = releasecommon.StatusUnknown
	}
	return fmt.Sprintf("revision %d of Helm release %q in namespace %q is %s", latest.Version, chart.name, chart.releaseNamespace, status)
}

// logK3sCheckSkipped warns that Radius cannot check whether k3s is installing the Traefik CRDs, and continues.
func (i *Impl) logK3sCheckSkipped(err error) {
	i.logInfo("Warning: Radius cannot check whether k3s is still installing the Traefik CRDs, so it continues without waiting: %v", err)
}

func k3sChartFailedError(chart helmControllerChart, latest *releasev1.Release, kubeContext string) error {
	return clierrors.Message("Radius stopped before changing the cluster because k3s failed to install the Traefik CRDs: revision %d of Helm release %q in namespace %q, which HelmChart %s/%s installs, failed. "+
		"Radius needs to know which Gateway API CRDs the cluster has before it installs or upgrades Contour. "+
		"Find the cause in the logs of the job that installs the chart, for example with %s, fix it, and then run the command again. "+
		"Or create the cluster without Traefik, for example with k3d cluster create --k3s-arg \"--disable=traefik@server:*\".",
		latest.Version, chart.name, chart.releaseNamespace, chart.namespace, chart.name, kubectlCommand(kubeContext, chart.namespace, "logs job/"+chart.job()))
}

// k3sCRDTimeoutError reports that the helm-controller did not install the chart of w in time, with what Radius
// observed last.
func k3sCRDTimeoutError(w *k3sChartWait, kubeContext string, timeout time.Duration) error {
	chart := w.chart
	observed := describeK3sRelease(chart, w.release)
	switch {
	case w.jobMissing:
		observed += fmt.Sprintf(", and job %s does not exist", chart.job())
	case w.job != nil && w.job.complete:
		observed += fmt.Sprintf(", and job %s finished", chart.job())
	case w.pods != nil && describeK3sPods(chart.job(), *w.pods) != "":
		observed += ", and " + describeK3sPods(chart.job(), *w.pods)
	case w.job != nil:
		observed += fmt.Sprintf(", and job %s has not finished", chart.job())
	}

	return clierrors.Message("Radius stopped before changing the cluster because k3s has not finished installing the Traefik CRDs after %s: HelmChart %s/%s has not deployed its Helm release. "+
		"Radius observed that %s. "+
		"Radius needs to know which Gateway API CRDs the cluster has before it installs or upgrades Contour. "+
		"Find out why k3s does not finish, for example with %s and %s, fix the cause, and then run the command again. "+
		"Or create the cluster without Traefik, for example with k3d cluster create --k3s-arg \"--disable=traefik@server:*\".",
		timeout, chart.namespace, chart.name, observed,
		kubectlCommand(kubeContext, chart.namespace, "get pods -l job-name="+chart.job()), kubectlCommand(kubeContext, chart.namespace, "logs job/"+chart.job()))
}

// describeK3sPods describes the restarts and waiting containers of the Pods of a job, or returns an empty string when
// they have neither.
func describeK3sPods(job string, pods helmControllerPods) string {
	subject, verb := "the pod of job "+job, "is"
	if pods.count > 1 {
		subject, verb = "the pods of job "+job, "are"
	}

	var parts []string
	switch {
	case pods.restarts == 1:
		parts = append(parts, "restarted once")
	case pods.restarts > 1:
		parts = append(parts, fmt.Sprintf("restarted %d times", pods.restarts))
	}
	if len(pods.waitingReasons) > 0 {
		parts = append(parts, verb+" in "+strings.Join(pods.waitingReasons, " and "))
	}
	if len(parts) == 0 {
		return ""
	}

	return subject + " " + strings.Join(parts, " and ")
}
