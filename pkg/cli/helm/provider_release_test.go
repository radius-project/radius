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
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	helm "helm.sh/helm/v4/pkg/action"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage"
	"helm.sh/helm/v4/pkg/storage/driver"
)

func TestProviderReleaseHistoryPinsSourceAndOwnsRetries(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		change func(*releasev1.Release, *releasev1.Release)
		valid  bool
	}{
		{"pending owned retry", func(*releasev1.Release, *releasev1.Release) {}, true},
		{"completed owned retry", func(_, r *releasev1.Release) { r.Info.Status = releasecommon.StatusDeployed }, true},
		{"failed owned retry", func(_, r *releasev1.Release) { r.Info.Status = releasecommon.StatusFailed }, true},
		{"unlabeled pending", func(_, r *releasev1.Release) { r.Labels = nil }, false},
		{"foreign pending", func(_, r *releasev1.Release) { r.Labels[providerTransitionMarker] = uuid.NewString() }, false},
		{"target values drift", func(_, r *releasev1.Release) { r.Config["provider"] = "off" }, false},
		{"chart drift", func(_, r *releasev1.Release) { r.Chart.Values["extra"] = true }, false},
		{"source values drift", func(s, _ *releasev1.Release) { s.Config["provider"] = "off" }, false},
		{"source manifest drift", func(s, _ *releasev1.Release) { s.Manifest += "\nchanged" }, false},
		{"source revision gap", func(_, r *releasev1.Release) { r.Version++ }, false},
		{"pending rollback", func(_, r *releasev1.Release) { r.Info.Status = releasecommon.StatusPendingRollback }, false},
		{"foreign namespace", func(_, r *releasev1.Release) { r.Namespace = "other" }, false},
		{"failed source", func(s, _ *releasev1.Release) { s.Info.Status = releasecommon.StatusFailed }, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := &releasev1.Release{Name: radiusReleaseName, Namespace: RadiusSystemNamespace, Version: 7,
				Info:   &releasev1.Info{Status: releasecommon.StatusDeployed},
				Chart:  &chart.Chart{Metadata: &chart.Metadata{Name: radiusReleaseName, Version: "1.0.0"}, Values: map[string]any{}},
				Config: map[string]any{"provider": "radius"}, Manifest: "source manifest"}
			values := map[string]any{"provider": "external"}
			intent, err := newProviderHelmIntent(source, values, nil)
			require.NoError(t, err)
			plan := providerTransitionPlan{ID: uuid.NewString(), Namespace: source.Namespace, Release: source.Name, Helm: intent}
			target := &releasev1.Release{Name: source.Name, Namespace: source.Namespace, Version: 8,
				Info:  &releasev1.Info{Status: releasecommon.StatusPendingUpgrade},
				Chart: cloneChart(source.Chart), Config: copyValues(values), Labels: map[string]string{providerTransitionMarker: plan.ID}}
			test.change(source, target)
			config := &helm.Configuration{Releases: storage.Init(driver.NewMemory())}
			require.NoError(t, config.Releases.Create(source))
			require.NoError(t, config.Releases.Create(target))
			_, latest, err := providerReleaseHistory(config, plan)
			if test.valid {
				require.NoError(t, err)
				require.Equal(t, target.Version, latest.Version)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestProviderChartDigestIncludesDependencies(t *testing.T) {
	t.Parallel()
	parent := &chart.Chart{Metadata: &chart.Metadata{Name: "parent"}}
	child := &chart.Chart{Metadata: &chart.Metadata{Name: "child"}, Values: map[string]any{"enabled": true}}
	parent.AddDependency(child)
	before, err := providerChartDigest(parent)
	require.NoError(t, err)
	child.Values["enabled"] = false
	after, err := providerChartDigest(parent)
	require.NoError(t, err)
	require.NotEqual(t, before, after)
}

func TestProviderHelmTransportFencesEveryWrite(t *testing.T) {
	t.Parallel()
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	failure := errors.New("identity operation lease changed")
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions,
		http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			var checked int
			base := &http.Transport{}
			defer base.CloseIdleConnections()
			client := &http.Client{Transport: providerGuardedTransport{base: base, ctx: t.Context(),
				fence: func(context.Context) error { checked++; return failure }}}
			request, err := http.NewRequestWithContext(t.Context(), method, server.URL, nil)
			require.NoError(t, err)
			before := received.Load()
			response, err := client.Do(request)
			if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
				require.NoError(t, err)
				require.NoError(t, response.Body.Close())
				require.Zero(t, checked)
				require.Equal(t, before+1, received.Load())
			} else {
				require.ErrorIs(t, err, failure)
				require.Equal(t, 1, checked)
				require.Equal(t, before, received.Load())
			}
		})
	}
}

func TestProviderHelmTransportCancellationIncludesResponseBody(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	base := &http.Transport{}
	defer base.CloseIdleConnections()
	client := &http.Client{Transport: providerGuardedTransport{base: base, ctx: ctx}}
	response, err := client.Get(server.URL)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() {
		_, err := io.ReadAll(response.Body)
		done <- err
	}()
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("provider operation cancellation did not interrupt the Helm response body")
	}
	require.NoError(t, response.Body.Close())
}
