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
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func Test_getHelmChartsWithClient(t *testing.T) {
	t.Parallel()

	newHelmChart := func(namespace string, name string, targetNamespace string) *unstructured.Unstructured {
		object := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "helm.cattle.io/v1",
			"kind":       "HelmChart",
			"metadata":   map[string]any{"name": name, "namespace": namespace},
			"spec":       map[string]any{"chart": "https://%{KUBERNETES_API}%/static/charts/" + name + ".tgz"},
		}}
		if targetNamespace != "" {
			require.NoError(t, unstructured.SetNestedField(object.Object, targetNamespace, "spec", "targetNamespace"))
		}
		return object
	}
	// The status of the HelmChart traefik-crd in k3s v1.35, which stays like this while its job fails.
	installing := newHelmChart("kube-system", "traefik-crd", "")
	installing.Object["status"] = map[string]any{
		"jobName": "helm-install-traefik-crd",
		"conditions": []any{
			map[string]any{"type": "JobCreated", "status": "True"},
			map[string]any{"type": "Failed", "status": "False"},
		},
	}
	renamedJob := newHelmChart("kube-system", "traefik-crd", "")
	renamedJob.Object["status"] = map[string]any{"jobName": "helm-install-traefik-crd-2"}
	deleting := newHelmChart("kube-system", "rke2-traefik-crd", "")
	deleting.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
	deleting.SetFinalizers([]string{"wrangler.cattle.io/on-helm-chart-remove"})
	newClient := func(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
		return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{helmChartGVR: "HelmChartList"}, objects...)
	}
	failGets := func(err error) func(*dynamicfake.FakeDynamicClient) {
		return func(client *dynamicfake.FakeDynamicClient) {
			client.PrependReactor("get", "helmcharts", func(k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, err
			})
		}
	}

	tests := []struct {
		name    string
		objects []runtime.Object
		setup   func(*dynamicfake.FakeDynamicClient)
		want    []helmControllerChart
		wantErr string
	}{
		{
			name: "k3s cluster",
			objects: []runtime.Object{
				installing,
				newHelmChart("kube-system", "traefik", ""),
				newHelmChart("default", "rke2-traefik-crd", ""),
			},
			want: []helmControllerChart{{name: "traefik-crd", namespace: "kube-system", releaseNamespace: "kube-system", jobName: "helm-install-traefik-crd"}},
		},
		{
			name:    "HelmChart whose status names another job",
			objects: []runtime.Object{renamedJob},
			want:    []helmControllerChart{{name: "traefik-crd", namespace: "kube-system", releaseNamespace: "kube-system", jobName: "helm-install-traefik-crd-2"}},
		},
		{
			name:    "RKE2 cluster with a target namespace",
			objects: []runtime.Object{newHelmChart("kube-system", "rke2-traefik-crd", "traefik-system")},
			want:    []helmControllerChart{{name: "rke2-traefik-crd", namespace: "kube-system", releaseNamespace: "traefik-system"}},
		},
		{
			name:    "HelmChart that is being deleted",
			objects: []runtime.Object{deleting},
		},
		{
			name: "cluster without HelmCharts",
		},
		{
			// The API server answers like this when the cluster has no HelmChart resource type.
			name:  "cluster without the HelmChart resource type",
			setup: failGets(apierrors.NewGenericServerResponse(http.StatusNotFound, "get", schema.GroupResource{Group: "helm.cattle.io", Resource: "helmcharts"}, "traefik-crd", "", 0, true)),
		},
		{
			name:  "cluster whose RESTMapper has no HelmChart kind",
			setup: failGets(&meta.NoKindMatchError{GroupKind: schema.GroupKind{Group: "helm.cattle.io", Kind: "HelmChart"}, SearchedVersions: []string{"v1"}}),
		},
		{
			name:    "error",
			setup:   failGets(apierrors.NewForbidden(schema.GroupResource{Group: "helm.cattle.io", Resource: "helmcharts"}, "traefik-crd", errors.New("RBAC"))),
			wantErr: "forbidden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newClient(tt.objects...)
			if tt.setup != nil {
				tt.setup(client)
			}

			charts, err := getHelmChartsWithClient(t.Context(), client, helmControllerNamespace, k3sGatewayAPICRDCharts)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, charts)
		})
	}
}

func Test_helmControllerChart_job(t *testing.T) {
	t.Parallel()

	require.Equal(t, "helm-install-traefik-crd", helmControllerChart{name: "traefik-crd"}.job())
	require.Equal(t, "helm-install-traefik-crd-2", helmControllerChart{name: "traefik-crd", jobName: "helm-install-traefik-crd-2"}.job())
}

// newHelmControllerJob returns the Job with which the k3s v1.35 helm-controller installs HelmChart traefik-crd, with
// the given status.
func newHelmControllerJob(status batchv1.JobStatus) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "helm-install-traefik-crd", Namespace: "kube-system"},
		Spec: batchv1.JobSpec{
			BackoffLimit: new(int32(1000)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Containers:    []corev1.Container{{Name: "helm", Image: "rancher/klipper-helm:v0.9.10-build20251111"}},
				},
			},
		},
		Status: status,
	}
}

func Test_getJobWithClient(t *testing.T) {
	t.Parallel()

	started := metav1.NewTime(time.Date(2026, 10, 29, 8, 56, 7, 0, time.UTC))
	finished := metav1.NewTime(started.Add(10 * time.Second))
	tests := []struct {
		name         string
		job          *batchv1.Job
		want         helmControllerJob
		wantNotFound bool
	}{
		{
			// The Job restarts the failing install in the same Pod, so it counts no failed Pod and has no conditions.
			name: "install that fails",
			job:  newHelmControllerJob(batchv1.JobStatus{Active: 1, Ready: new(int32(0)), StartTime: &started}),
			want: helmControllerJob{},
		},
		{
			name: "install that finished, as k3s v1.35 reports it",
			job:  newHelmControllerJob(batchv1.JobStatus{Succeeded: 1, StartTime: &started, CompletionTime: &finished}),
			want: helmControllerJob{complete: true},
		},
		{
			name: "install that finished, with the Complete condition",
			job: newHelmControllerJob(batchv1.JobStatus{Succeeded: 1, StartTime: &started, CompletionTime: &finished, Conditions: []batchv1.JobCondition{
				{Type: batchv1.JobSuccessCriteriaMet, Status: corev1.ConditionTrue},
				{Type: batchv1.JobComplete, Status: corev1.ConditionTrue},
			}}),
			want: helmControllerJob{complete: true},
		},
		{
			name: "job that needs more completions",
			job: func() *batchv1.Job {
				job := newHelmControllerJob(batchv1.JobStatus{Active: 1, Succeeded: 1, StartTime: &started})
				job.Spec.Completions = new(int32(2))
				return job
			}(),
			want: helmControllerJob{},
		},
		{
			name:         "job that does not exist",
			wantNotFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := kubernetesfake.NewClientset()
			if tt.job != nil {
				client = kubernetesfake.NewClientset(tt.job)
			}

			job, err := getJobWithClient(t.Context(), client, "kube-system", "helm-install-traefik-crd")
			if tt.wantNotFound {
				require.True(t, apierrors.IsNotFound(err), "%v", err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, job)
		})
	}
}

func Test_getJobPodsWithClient(t *testing.T) {
	t.Parallel()

	newPod := func(namespace string, name string, job string, status corev1.PodStatus) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				// The labels that the Job controller sets on the Pods of a Job.
				Labels: map[string]string{"job-name": job, "batch.kubernetes.io/job-name": job},
			},
			Status: status,
		}
	}
	// The Pod of the job helm-install-traefik-crd in k3s v1.35 while the install fails.
	crashLooping := corev1.PodStatus{
		Phase: corev1.PodRunning,
		ContainerStatuses: []corev1.ContainerStatus{{
			Name:                 "helm",
			RestartCount:         4,
			State:                corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
			LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 1, Reason: "Error"}},
		}},
	}
	pullingImage := corev1.PodStatus{
		Phase: corev1.PodPending,
		ContainerStatuses: []corev1.ContainerStatus{{
			Name:  "helm",
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}},
		}},
	}

	tests := []struct {
		name    string
		objects []runtime.Object
		want    helmControllerPods
	}{
		{
			name:    "pod of an install that fails",
			objects: []runtime.Object{newPod("kube-system", "helm-install-traefik-crd-abcde", "helm-install-traefik-crd", crashLooping)},
			want:    helmControllerPods{count: 1, restarts: 4, waitingReasons: []string{"CrashLoopBackOff"}},
		},
		{
			name: "pods of the job and of other jobs",
			objects: []runtime.Object{
				newPod("kube-system", "helm-install-traefik-crd-abcde", "helm-install-traefik-crd", crashLooping),
				newPod("kube-system", "helm-install-traefik-crd-fghij", "helm-install-traefik-crd", pullingImage),
				newPod("kube-system", "helm-install-traefik-fghij", "helm-install-traefik", crashLooping),
				newPod("default", "helm-install-traefik-crd-klmno", "helm-install-traefik-crd", crashLooping),
			},
			want: helmControllerPods{count: 2, restarts: 4, waitingReasons: []string{"CrashLoopBackOff", "ImagePullBackOff"}},
		},
		{
			name: "job without pods",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pods, err := getJobPodsWithClient(t.Context(), kubernetesfake.NewClientset(tt.objects...), "kube-system", "helm-install-traefik-crd")
			require.NoError(t, err)
			require.Equal(t, tt.want, pods)
		})
	}
}

// Test_k3sGatewayAPICRDs verifies that Radius checks the Gateway API CRDs that both the Traefik CRD chart of k3s and the
// Contour chart include, because the helm-controller cannot install the chart while another owner has one of them.
func Test_k3sGatewayAPICRDs(t *testing.T) {
	t.Parallel()

	var both []string
	for _, crd := range traefikGatewayAPICRDs() {
		if _, found := findCRD(contourBundleCRDs(), crd.name); found {
			both = append(both, crd.name)
		}
	}
	require.ElementsMatch(t, both, k3sGatewayAPICRDs)
}

// getResult is what RunHelmGet returns for one poll.
type getResult struct {
	release *releasev1.Release
	err     error
}

func Test_waitForK3sGatewayAPICRDs(t *testing.T) {
	t.Parallel()

	revision := func(version int, status releasecommon.Status) getResult {
		return getResult{release: &releasev1.Release{Name: "traefik-crd", Version: version, Info: &releasev1.Info{Status: status}}}
	}
	notFound := getResult{err: driver.ErrReleaseNotFound}
	forbidden := apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "", errors.New("RBAC"))
	traefikCRD := helmControllerChart{name: "traefik-crd", namespace: "kube-system", releaseNamespace: "kube-system", jobName: "helm-install-traefik-crd"}
	const waitingLog = "Waiting for k3s to finish installing the Traefik CRDs..."
	// The job that installs the chart: k3s restarts a failing install in the same Pod, and the job finishes only when the
	// install succeeds.
	runningJob := map[string]helmControllerJob{"kube-system/helm-install-traefik-crd": {}}
	finishedJob := map[string]helmControllerJob{"kube-system/helm-install-traefik-crd": {complete: true}}
	crashLooping := map[string]helmControllerPods{"kube-system/helm-install-traefik-crd": {count: 1, restarts: 4, waitingReasons: []string{"CrashLoopBackOff"}}}
	timeoutMessage := `Radius stopped before changing the cluster because k3s has not finished installing the Traefik CRDs after 3m0s: HelmChart kube-system/traefik-crd has not deployed its Helm release. Radius observed that `
	commands := `for example with kubectl --context "test-context" -n kube-system get pods -l job-name=helm-install-traefik-crd and kubectl --context "test-context" -n kube-system logs job/helm-install-traefik-crd, fix the cause`

	tests := []struct {
		name       string
		helmCharts []helmControllerChart
		chartsErr  error
		// crds are the CRDs in the cluster, and laterCRDs, when set, are the CRDs from the second read on.
		crds      []crdInfo
		laterCRDs []crdInfo
		jobs      map[string]helmControllerJob
		jobErr    error
		jobPods   map[string]helmControllerPods
		// polls are the results of the release reads, and the last one repeats.
		polls []getResult
		// wantPolls is the number of release reads, or -1 when it does not matter.
		wantPolls    int
		wantCRDReads int32
		wantElapsed  time.Duration
		// wantLogs has a part of each message that Radius logs, in order.
		wantLogs     []string
		wantErr      []string
		wantFriendly bool
	}{
		{
			name: "cluster without k3s HelmCharts",
		},
		{
			name:      "HelmCharts that Radius cannot read",
			chartsErr: apierrors.NewForbidden(schema.GroupResource{Group: "helm.cattle.io", Resource: "helmcharts"}, "traefik-crd", errors.New("RBAC")),
			wantLogs:  []string{"Warning: Radius cannot check whether k3s is still installing the Traefik CRDs, so it continues without waiting"},
		},
		{
			name:      "error reading the HelmCharts",
			chartsErr: errors.New("connection refused"),
			wantErr:   []string{"failed to check whether k3s is installing Gateway API CRDs", "connection refused"},
		},
		{
			name:       "CRDs that k3s installed",
			helmCharts: []helmControllerChart{traefikCRD},
			polls:      []getResult{revision(1, releasecommon.StatusDeployed)},
			wantPolls:  1,
		},
		{
			name:         "CRDs that k3s installs while Radius waits",
			helmCharts:   []helmControllerChart{traefikCRD},
			jobs:         runningJob,
			polls:        []getResult{notFound, revision(1, releasecommon.StatusPendingInstall), revision(1, releasecommon.StatusDeployed)},
			wantPolls:    3,
			wantCRDReads: 1,
			wantElapsed:  2 * k3sCRDWaitInterval,
			wantLogs:     []string{waitingLog},
		},
		{
			name:        "CRDs that k3s upgrades",
			helmCharts:  []helmControllerChart{traefikCRD},
			polls:       []getResult{revision(2, releasecommon.StatusPendingUpgrade), revision(2, releasecommon.StatusDeployed)},
			wantPolls:   2,
			wantElapsed: k3sCRDWaitInterval,
			wantLogs:    []string{waitingLog},
		},
		{
			name:       "CRDs that k3s failed to install",
			helmCharts: []helmControllerChart{traefikCRD},
			polls:      []getResult{revision(1, releasecommon.StatusFailed)},
			wantPolls:  1,
			wantErr: []string{
				`Radius stopped before changing the cluster because k3s failed to install the Traefik CRDs: revision 1 of Helm release "traefik-crd" in namespace "kube-system", which HelmChart kube-system/traefik-crd installs, failed.`,
				`kubectl --context "test-context" -n kube-system logs job/helm-install-traefik-crd`,
				`--k3s-arg "--disable=traefik@server:*"`,
			},
			wantFriendly: true,
		},
		{
			// An install of v0.62.0 left Contour with the Gateway API CRDs, so the job of k3s fails on every retry before
			// Helm records the release.
			name:         "Gateway API CRDs that Contour owns",
			helmCharts:   []helmControllerChart{traefikCRD},
			crds:         contourBundleCRDs(),
			jobs:         runningJob,
			jobPods:      crashLooping,
			polls:        []getResult{notFound},
			wantPolls:    1,
			wantCRDReads: 1,
			wantLogs: []string{`Warning: k3s cannot install the Traefik CRDs of HelmChart kube-system/traefik-crd because CRD backendtlspolicies.gateway.networking.k8s.io belongs to Helm release "contour" in namespace "radius-system", so Radius continues without waiting for k3s. ` +
				`To see why k3s does not install them, run kubectl --context "test-context" -n kube-system logs job/helm-install-traefik-crd.`},
		},
		{
			name:         "Gateway API CRDs that were installed without Helm",
			helmCharts:   []helmControllerChart{traefikCRD},
			crds:         unmanagedCRDs(traefikGatewayAPICRDs()),
			jobs:         runningJob,
			polls:        []getResult{notFound},
			wantPolls:    1,
			wantCRDReads: 1,
			wantLogs:     []string{"because CRD backendtlspolicies.gateway.networking.k8s.io belongs to an installer other than Helm, so Radius continues without waiting for k3s."},
		},
		{
			// helm uninstall traefik-crd: the CRDs of the chart have the keep annotation, so they stay with the ownership
			// metadata of the release, and the job that installed the chart does not run again.
			name:         "release of the Traefik CRDs that was uninstalled after the job finished",
			helmCharts:   []helmControllerChart{traefikCRD},
			crds:         keptCRDs(traefikGatewayAPICRDs()),
			jobs:         finishedJob,
			polls:        []getResult{notFound},
			wantPolls:    1,
			wantCRDReads: 1,
			wantLogs: []string{`Warning: k3s does not install the Traefik CRDs of HelmChart kube-system/traefik-crd again: job helm-install-traefik-crd finished, ` +
				`but Helm release "traefik-crd" in namespace "kube-system" does not exist. Radius continues without waiting for k3s.`},
		},
		{
			name:         "release of the Traefik CRDs that was uninstalled with its history kept after the job finished",
			helmCharts:   []helmControllerChart{traefikCRD},
			jobs:         finishedJob,
			polls:        []getResult{revision(2, releasecommon.StatusUninstalled)},
			wantPolls:    1,
			wantCRDReads: 1,
			wantLogs:     []string{`job helm-install-traefik-crd finished, but revision 2 of Helm release "traefik-crd" in namespace "kube-system" is uninstalled.`},
		},
		{
			name:         "job that the status of the HelmChart names",
			helmCharts:   []helmControllerChart{{name: "traefik-crd", namespace: "kube-system", releaseNamespace: "kube-system", jobName: "helm-install-traefik-crd-2"}},
			jobs:         map[string]helmControllerJob{"kube-system/helm-install-traefik-crd-2": {complete: true}},
			polls:        []getResult{notFound},
			wantPolls:    1,
			wantCRDReads: 1,
			wantLogs:     []string{"again: job helm-install-traefik-crd-2 finished"},
		},
		{
			// The poll and the timeout can end at the same time, so the number of reads varies by one.
			name:         "install that k3s retries until Radius stops waiting",
			helmCharts:   []helmControllerChart{traefikCRD},
			jobs:         runningJob,
			jobPods:      crashLooping,
			polls:        []getResult{notFound},
			wantPolls:    -1,
			wantCRDReads: 2,
			wantElapsed:  k3sCRDWaitTimeout,
			wantLogs:     []string{waitingLog},
			wantErr: []string{
				timeoutMessage + `Helm release "traefik-crd" in namespace "kube-system" does not exist, and the pod of job helm-install-traefik-crd restarted 4 times and is in CrashLoopBackOff. `,
				commands,
				`Or create the cluster without Traefik, for example with k3d cluster create --k3s-arg "--disable=traefik@server:*".`,
			},
			wantFriendly: true,
		},
		{
			// The CRDs carry the ownership metadata of the release, so the helm-controller can still install it.
			name:         "release that does not exist while its release owns the CRDs",
			helmCharts:   []helmControllerChart{traefikCRD},
			crds:         keptCRDs(traefikGatewayAPICRDs()),
			jobs:         runningJob,
			polls:        []getResult{notFound},
			wantPolls:    -1,
			wantCRDReads: 2,
			wantElapsed:  k3sCRDWaitTimeout,
			wantLogs:     []string{waitingLog},
			wantErr:      []string{timeoutMessage + `Helm release "traefik-crd" in namespace "kube-system" does not exist, and job helm-install-traefik-crd has not finished. `},
			wantFriendly: true,
		},
		{
			name:         "install that does not finish",
			helmCharts:   []helmControllerChart{traefikCRD},
			jobs:         runningJob,
			polls:        []getResult{revision(1, releasecommon.StatusPendingInstall)},
			wantPolls:    -1,
			wantElapsed:  k3sCRDWaitTimeout,
			wantLogs:     []string{waitingLog},
			wantErr:      []string{timeoutMessage + `revision 1 of Helm release "traefik-crd" in namespace "kube-system" is pending-install, and job helm-install-traefik-crd has not finished. `},
			wantFriendly: true,
		},
		{
			name:         "job that does not exist",
			helmCharts:   []helmControllerChart{traefikCRD},
			polls:        []getResult{notFound},
			wantPolls:    -1,
			wantCRDReads: 2,
			wantElapsed:  k3sCRDWaitTimeout,
			wantLogs:     []string{waitingLog},
			wantErr:      []string{timeoutMessage + `Helm release "traefik-crd" in namespace "kube-system" does not exist, and job helm-install-traefik-crd does not exist. `},
			wantFriendly: true,
		},
		{
			name:         "job that Radius cannot read",
			helmCharts:   []helmControllerChart{traefikCRD},
			jobErr:       apierrors.NewForbidden(schema.GroupResource{Group: "batch", Resource: "jobs"}, "helm-install-traefik-crd", errors.New("RBAC")),
			polls:        []getResult{notFound},
			wantPolls:    -1,
			wantCRDReads: 2,
			wantElapsed:  k3sCRDWaitTimeout,
			wantLogs:     []string{waitingLog},
			wantErr:      []string{timeoutMessage + `Helm release "traefik-crd" in namespace "kube-system" does not exist. Radius needs`},
			wantFriendly: true,
		},
		{
			name:         "error reading the job",
			helmCharts:   []helmControllerChart{traefikCRD},
			jobErr:       errors.New("connection refused"),
			polls:        []getResult{notFound},
			wantPolls:    1,
			wantCRDReads: 1,
			wantErr:      []string{`failed to get job "helm-install-traefik-crd" in namespace "kube-system"`, "connection refused"},
		},
		{
			// Radius reads the CRDs again before it reports that k3s did not finish.
			name:         "Gateway API CRDs that another installer adds while Radius waits",
			helmCharts:   []helmControllerChart{traefikCRD},
			laterCRDs:    contourBundleCRDs(),
			jobs:         runningJob,
			polls:        []getResult{notFound},
			wantPolls:    -1,
			wantCRDReads: 2,
			wantElapsed:  k3sCRDWaitTimeout,
			wantLogs:     []string{waitingLog, "Warning: k3s cannot install the Traefik CRDs of HelmChart kube-system/traefik-crd because CRD backendtlspolicies.gateway.networking.k8s.io belongs to Helm release \"contour\""},
		},
		{
			name:       "error reading the release",
			helmCharts: []helmControllerChart{traefikCRD},
			polls:      []getResult{{err: errors.New("connection refused")}},
			wantPolls:  1,
			wantErr:    []string{`failed to get Helm release "traefik-crd" in namespace "kube-system"`, "connection refused"},
		},
		{
			name:       "release that Radius cannot read",
			helmCharts: []helmControllerChart{traefikCRD},
			polls:      []getResult{{err: fmt.Errorf("query: failed to query with labels: %w", forbidden)}},
			wantPolls:  1,
			wantLogs:   []string{"Warning: Radius cannot check whether k3s is still installing the Traefik CRDs, so it continues without waiting"},
		},
		{
			name: "k3s and RKE2 HelmCharts",
			helmCharts: []helmControllerChart{
				traefikCRD,
				{name: "rke2-traefik-crd", namespace: "kube-system", releaseNamespace: "traefik-system"},
			},
			polls:     []getResult{revision(1, releasecommon.StatusDeployed)},
			wantPolls: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				client := NewMockHelmClient(gomock.NewController(t))
				polls := 0
				call := client.EXPECT().RunHelmGet(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ *helm.Configuration, releaseName string) (*releasev1.Release, error) {
						result := tt.polls[min(polls, len(tt.polls)-1)]
						polls++
						return result.release, result.err
					})
				if tt.wantPolls >= 0 {
					call.Times(tt.wantPolls)
				} else {
					call.MinTimes(int(k3sCRDWaitTimeout / k3sCRDWaitInterval))
				}
				cluster := &fakeContourCluster{
					helmCharts:    tt.helmCharts,
					helmChartsErr: tt.chartsErr,
					jobs:          tt.jobs,
					jobErr:        tt.jobErr,
					jobPods:       tt.jobPods,
				}
				cluster.readCRDs = func(ctx context.Context, _ string, names []string) ([]crdInfo, error) {
					crds := tt.crds
					if cluster.crdReads.Load() > 1 && tt.laterCRDs != nil {
						crds = tt.laterCRDs
					}
					return crdLookup(crds)(ctx, names)
				}
				var logs []string
				impl := &Impl{
					Helm:           client,
					contourCluster: cluster,
					logger:         func(format string, v ...any) { logs = append(logs, fmt.Sprintf(format, v...)) },
				}

				start := time.Now()
				err := impl.waitForK3sGatewayAPICRDs(t.Context(), "test-context")
				require.Equal(t, tt.wantElapsed, time.Since(start))
				require.Equal(t, tt.wantCRDReads, cluster.crdReads.Load(), "Radius reads the CRDs when the wait starts and before it reports a timeout")
				require.Len(t, logs, len(tt.wantLogs), "%q", logs)
				for i, want := range tt.wantLogs {
					require.Contains(t, logs[i], want)
				}
				if len(tt.wantErr) == 0 {
					require.NoError(t, err)
					return
				}

				require.Error(t, err)
				require.Equal(t, tt.wantFriendly, clierrors.IsFriendlyError(err), "%v", err)
				for _, want := range tt.wantErr {
					require.Contains(t, err.Error(), want)
				}
			})
		})
	}
}

func Test_waitForK3sGatewayAPICRDs_Cancellation(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		client := NewMockHelmClient(gomock.NewController(t))
		client.EXPECT().RunHelmGet(gomock.Any(), "traefik-crd").
			DoAndReturn(func(*helm.Configuration, string) (*releasev1.Release, error) {
				// The user stops the command while Radius waits.
				time.AfterFunc(time.Second, cancel)
				return nil, driver.ErrReleaseNotFound
			}).Times(1)
		impl := &Impl{
			Helm:           client,
			contourCluster: &fakeContourCluster{helmCharts: []helmControllerChart{{name: "traefik-crd", namespace: "kube-system", releaseNamespace: "kube-system"}}},
			logger:         func(string, ...any) {},
		}

		start := time.Now()
		err := impl.waitForK3sGatewayAPICRDs(ctx, "")
		require.ErrorIs(t, err, context.Canceled)
		require.ErrorContains(t, err, "stopped waiting for k3s to install the Traefik CRDs")
		require.Equal(t, time.Second, time.Since(start))
	})
}

func Test_describeK3sPods(t *testing.T) {
	t.Parallel()

	tests := []struct {
		pods helmControllerPods
		want string
	}{
		{pods: helmControllerPods{count: 1, restarts: 4, waitingReasons: []string{"CrashLoopBackOff"}}, want: "the pod of job helm-install-traefik-crd restarted 4 times and is in CrashLoopBackOff"},
		{pods: helmControllerPods{count: 1, restarts: 1}, want: "the pod of job helm-install-traefik-crd restarted once"},
		{pods: helmControllerPods{count: 1, waitingReasons: []string{"ImagePullBackOff"}}, want: "the pod of job helm-install-traefik-crd is in ImagePullBackOff"},
		{pods: helmControllerPods{count: 2, restarts: 7, waitingReasons: []string{"CrashLoopBackOff", "ImagePullBackOff"}}, want: "the pods of job helm-install-traefik-crd restarted 7 times and are in CrashLoopBackOff and ImagePullBackOff"},
		{pods: helmControllerPods{count: 1}, want: ""},
		{pods: helmControllerPods{}, want: ""},
	}

	for _, tt := range tests {
		require.Equal(t, tt.want, describeK3sPods("helm-install-traefik-crd", tt.pods))
	}
}
