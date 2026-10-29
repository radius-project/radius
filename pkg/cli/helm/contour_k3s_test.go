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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
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
				newHelmChart("kube-system", "traefik-crd", ""),
				newHelmChart("kube-system", "traefik", ""),
				newHelmChart("default", "rke2-traefik-crd", ""),
			},
			want: []helmControllerChart{{name: "traefik-crd", namespace: "kube-system", releaseNamespace: "kube-system"}},
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
	traefikCRD := helmControllerChart{name: "traefik-crd", namespace: "kube-system", releaseNamespace: "kube-system"}

	tests := []struct {
		name       string
		helmCharts []helmControllerChart
		chartsErr  error
		// polls are the results of the release reads, and the last one repeats.
		polls []getResult
		// wantPolls is the number of release reads, or -1 when it does not matter.
		wantPolls    int
		wantElapsed  time.Duration
		wantErr      []string
		wantFriendly bool
	}{
		{
			name: "cluster without k3s HelmCharts",
		},
		{
			name:      "HelmCharts that Radius cannot read",
			chartsErr: apierrors.NewForbidden(schema.GroupResource{Group: "helm.cattle.io", Resource: "helmcharts"}, "traefik-crd", errors.New("RBAC")),
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
			name:        "CRDs that k3s installs while Radius waits",
			helmCharts:  []helmControllerChart{traefikCRD},
			polls:       []getResult{notFound, revision(1, releasecommon.StatusPendingInstall), revision(1, releasecommon.StatusDeployed)},
			wantPolls:   3,
			wantElapsed: 2 * k3sCRDWaitInterval,
		},
		{
			name:        "CRDs that k3s upgrades",
			helmCharts:  []helmControllerChart{traefikCRD},
			polls:       []getResult{revision(2, releasecommon.StatusPendingUpgrade), revision(2, releasecommon.StatusDeployed)},
			wantPolls:   2,
			wantElapsed: k3sCRDWaitInterval,
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
			// The poll and the timeout can end at the same time, so the number of reads varies by one.
			name:        "CRDs that k3s does not install in time",
			helmCharts:  []helmControllerChart{traefikCRD},
			polls:       []getResult{notFound},
			wantPolls:   -1,
			wantElapsed: k3sCRDWaitTimeout,
			wantErr: []string{
				`Radius stopped before changing the cluster because k3s has not finished installing the Traefik CRDs after 3m0s: HelmChart kube-system/traefik-crd has not deployed Helm release "traefik-crd" in namespace "kube-system" yet.`,
				`kubectl --context "test-context" -n kube-system wait --for=condition=complete job/helm-install-traefik-crd --timeout=5m`,
				`--k3s-arg "--disable=traefik@server:*"`,
			},
			wantFriendly: true,
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
				impl := &Impl{Helm: client, contourCluster: &fakeContourCluster{helmCharts: tt.helmCharts, helmChartsErr: tt.chartsErr}}

				start := time.Now()
				err := impl.waitForK3sGatewayAPICRDs(t.Context(), "test-context")
				require.Equal(t, tt.wantElapsed, time.Since(start))
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
		}

		start := time.Now()
		err := impl.waitForK3sGatewayAPICRDs(ctx, "")
		require.ErrorIs(t, err, context.Canceled)
		require.ErrorContains(t, err, "stopped waiting for k3s to install the Traefik CRDs")
		require.Equal(t, time.Second, time.Since(start))
	})
}
func Test_kubectlCommand(t *testing.T) {
	t.Parallel()

	require.Equal(t, "kubectl -n kube-system get pods", kubectlCommand("", "kube-system", "get pods"))
	require.Equal(t, `kubectl --context "k3d-demo" -n kube-system get pods`, kubectlCommand("k3d-demo", "kube-system", "get pods"))
}
