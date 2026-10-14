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
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	goretry "github.com/sethvargo/go-retry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	helm "helm.sh/helm/v4/pkg/action"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	chartutil "helm.sh/helm/v4/pkg/chart/v2/util"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage/driver"
	"oras.land/oras-go/v2/registry/remote/errcode"
)

func Test_isTransientHelmChartError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{"nil", nil, false},
		{"HTTP 500", errors.New("failed to fetch https://example.com/contour.tgz : 500 Internal Server Error"), true},
		{"wrapped HTTP 503", fmt.Errorf("index: %w", errors.New("failed to fetch https://example.com/index.yaml : 503 Service Unavailable")), true},
		{"HTTP 408", errors.New("failed to fetch http://example.com/chart.tgz : 408 Request Timeout"), true},
		{"HTTP 429", errors.New("failed to fetch https://example.com/chart.tgz : 429 Too Many Requests"), true},
		{"HTTP 502", errors.New("failed to fetch https://example.com/chart.tgz : 502 Bad Gateway"), true},
		{"HTTP 504", errors.New("failed to fetch https://example.com/chart.tgz : 504 Gateway Timeout"), true},
		{"HTTP 401", errors.New("failed to fetch https://example.com/chart.tgz : 401 Unauthorized"), false},
		{"HTTP 403", errors.New("failed to fetch https://example.com/chart.tgz : 403 Forbidden"), false},
		{"HTTP 404", errors.New("failed to fetch https://example.com/chart.tgz : 404 Not Found"), false},
		{"HTTP 501", errors.New("failed to fetch https://example.com/chart.tgz : 501 Not Implemented"), false},
		{"status in URL", errors.New("failed to fetch https://example.com/500 : 404 Not Found"), false},
		{"unrelated error", errors.New("chart version 500 not found"), false},
		{"canceled", fmt.Errorf("pull: %w", context.Canceled), false},
		{"permanent DNS", &net.DNSError{IsNotFound: true}, false},
		{"temporary DNS", &net.DNSError{IsTemporary: true}, true},
		{"timeout", &url.Error{Op: "Get", URL: "https://example.com", Err: context.DeadlineExceeded}, true},
		{"reset", fmt.Errorf("read: %w", syscall.ECONNRESET), true},
		{"refused", fmt.Errorf("dial: %w", syscall.ECONNREFUSED), true},
		{"broken pipe", fmt.Errorf("read: %w", syscall.EPIPE), true},
		{"short response", fmt.Errorf("read: %w", io.ErrUnexpectedEOF), true},
		{"local file", os.ErrPermission, false},
	}
	for _, status := range []int{408, 429, 500, 502, 503, 504, 401, 403, 404, 501} {
		tests = append(tests, struct {
			name      string
			err       error
			retryable bool
		}{
			fmt.Sprintf("OCI %d", status),
			fmt.Errorf("pull: %w", &errcode.ErrorResponse{StatusCode: status}),
			status == 408 || status == 429 || status == 500 || status == 502 || status == 503 || status == 504,
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.retryable, isTransientHelmChartError(tt.err))
		})
	}
}

func Test_HelmChartFromContainerRegistry_HTTPRetry(t *testing.T) {
	originalBackoff := helmChartPullBackoff
	helmChartPullBackoff = func() goretry.Backoff {
		return goretry.WithMaxRetries(2, goretry.NewConstant(time.Millisecond))
	}
	t.Cleanup(func() { helmChartPullBackoff = originalBackoff })
	t.Setenv("HELM_CACHE_HOME", t.TempDir())
	t.Setenv("HELM_CONFIG_HOME", t.TempDir())
	t.Setenv("HELM_DATA_HOME", t.TempDir())

	archive, err := chartutil.Save(&chart.Chart{Metadata: &chart.Metadata{
		APIVersion: "v2",
		Name:       "contour",
		Version:    "0.1.0",
	}}, t.TempDir())
	require.NoError(t, err)
	chartBytes, err := os.ReadFile(archive)
	require.NoError(t, err)

	tests := []struct {
		name         string
		failPath     string
		status       int
		failures     int
		wantRequests int
		wantError    bool
	}{
		{"first attempt succeeds", "", 0, 0, 0, false},
		{"chart recovers from 500", "/contour-0.1.0.tgz", 500, 1, 2, false},
		{"index recovers from 503", "/index.yaml", 503, 1, 2, false},
		{"rate limit recovers", "/contour-0.1.0.tgz", 429, 2, 3, false},
		{"chart retry limit", "/contour-0.1.0.tgz", 500, 10, 3, true},
		{"index retry limit", "/index.yaml", 503, 10, 3, true},
		{"missing chart", "/contour-0.1.0.tgz", 404, 10, 1, true},
		{"index unauthorized", "/index.yaml", 401, 10, 1, true},
		{"chart forbidden", "/contour-0.1.0.tgz", 403, 10, 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == tt.failPath {
					if requests.Add(1) <= int32(tt.failures) {
						http.Error(w, http.StatusText(tt.status), tt.status)
						return
					}
				}
				switch r.URL.Path {
				case "/index.yaml":
					_, _ = fmt.Fprint(w, "apiVersion: v1\nentries:\n  contour:\n  - name: contour\n    version: 0.1.0\n    apiVersion: v2\n    urls:\n    - contour-0.1.0.tgz\n")
				case "/contour-0.1.0.tgz":
					_, _ = w.Write(chartBytes)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			action := NewHelmAction(NewHelmClient())
			got, err := action.HelmChartFromContainerRegistry("0.1.0", &helm.Configuration{}, server.URL, "contour")
			if tt.wantError {
				require.Error(t, err)
				assert.Nil(t, got)
				assert.Contains(t, err.Error(), fmt.Sprintf("%d %s", tt.status, http.StatusText(tt.status)))
				assert.Contains(t, err.Error(), "version: 0.1.0, release name: contour")
			} else {
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, "contour", got.Metadata.Name)
			}
			assert.EqualValues(t, tt.wantRequests, requests.Load())
		})
	}
}

func Test_HelmChartFromContainerRegistry_LoadFailure(t *testing.T) {
	client := NewMockHelmClient(gomock.NewController(t))
	client.EXPECT().RunHelmPull(gomock.Any(), "contour").
		DoAndReturn(func(opts []helm.PullOpt, ref string) (string, error) {
			pull := helm.NewPull(opts...)
			require.NoError(t, os.WriteFile(filepath.Join(pull.DestDir, "chart.tgz"), []byte("invalid chart"), 0600))
			return "", nil
		}).Times(1)
	loadErr := io.ErrUnexpectedEOF
	client.EXPECT().LoadChart(gomock.Any()).Return(nil, loadErr).Times(1)
	got, err := NewHelmAction(client).HelmChartFromContainerRegistry("0.1.0", &helm.Configuration{}, "https://example.com", "contour")
	assert.Nil(t, got)
	assert.ErrorIs(t, err, loadErr)
}

func Test_HelmChartFromContainerRegistry_GHCRForbidden(t *testing.T) {
	client := NewMockHelmClient(gomock.NewController(t))
	client.EXPECT().RunHelmPull(gomock.Any(), "oci://ghcr.io/radius-project/helm-chart/radius").
		Return("", &errcode.ErrorResponse{StatusCode: http.StatusForbidden, URL: &url.URL{Host: "ghcr.io"}}).
		Times(1)
	got, err := NewHelmAction(client).HelmChartFromContainerRegistry("0.1.0", &helm.Configuration{}, "oci://ghcr.io/radius-project/helm-chart", "radius")
	assert.Nil(t, got)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "docker logout ghcr.io")
}

func Test_isHelmGHCR403Error(t *testing.T) {
	var err error
	var result bool

	err = errors.New("error")
	result = isHelmGHCR403Error(err)
	assert.False(t, result)

	err = fmt.Errorf("%w: wrapped error", errors.New("error"))
	result = isHelmGHCR403Error(err)
	assert.False(t, result)

	err = fmt.Errorf("%w: wrapped error", &errcode.ErrorResponse{})
	result = isHelmGHCR403Error(err)
	assert.False(t, result)

	err = fmt.Errorf("%w: wrapped error", &errcode.ErrorResponse{StatusCode: http.StatusForbidden, URL: &url.URL{Host: "ghcr.io", Path: "/myregistry"}})
	result = isHelmGHCR403Error(err)
	assert.True(t, result)

	err = &errcode.ErrorResponse{StatusCode: http.StatusForbidden, URL: &url.URL{Host: "ghcr.io", Path: "/myregistry"}}
	result = isHelmGHCR403Error(err)
	assert.True(t, result)

	err = &errcode.ErrorResponse{StatusCode: http.StatusUnauthorized, URL: &url.URL{Host: "ghcr.io", Path: "/myregistry"}}
	result = isHelmGHCR403Error(err)
	assert.False(t, result)
}

func Test_parseUserValuesFromCLI(t *testing.T) {
	options := &RadiusChartOptions{
		SetArgs:     []string{"global.zipkin.url=url,global.prometheus.path=path"},
		SetFileArgs: []string{"global.rootCA.cert=./testdata/fake-ca-cert.crt"},
	}

	values, err := parseUserValuesFromCLI(options)
	require.NoError(t, err)

	_, ok := values["global"]
	assert.True(t, ok)
	global := values["global"].(map[string]any)
	_, ok = global["zipkin"]
	assert.True(t, ok)
	zipkin := global["zipkin"].(map[string]any)
	_, ok = zipkin["url"]
	assert.True(t, ok)
	assert.Equal(t, zipkin["url"], "url")
	rootCA := global["rootCA"].(map[string]any)
	_, ok = rootCA["cert"]
	assert.True(t, ok)
	assert.Contains(t, rootCA["cert"], "-----BEGIN CERTIFICATE-----")

	_, ok = global["prometheus"]
	assert.True(t, ok)
	prometheus := global["prometheus"].(map[string]any)
	_, ok = prometheus["path"]
	assert.True(t, ok)
	assert.Equal(t, prometheus["path"], "path")
}

func Test_prepareRadiusChart_DoesNotMutateChartValues(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	helmChart := &chart.Chart{
		Values: map[string]any{
			"global": map[string]any{
				"existing": "untouched",
			},
		},
	}
	mockHelmClient := NewMockHelmClient(ctrl)
	mockHelmClient.EXPECT().LoadChart("test-chart").Return(helmChart, nil).Times(1)
	helmAction := NewHelmAction(mockHelmClient)

	options := &RadiusChartOptions{
		Namespace: "radius-system",
		ChartPath: "test-chart",
		SetArgs:   []string{"global.zipkin.url=url"},
	}

	_, _, values, err := prepareRadiusChart(helmAction, *options, "")
	require.NoError(t, err)

	global, ok := values["global"].(map[string]any)
	require.True(t, ok)
	zipkin, ok := global["zipkin"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "url", zipkin["url"])

	chartGlobal, ok := helmChart.Values["global"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "untouched", chartGlobal["existing"])
	_, hasZipkin := chartGlobal["zipkin"]
	assert.False(t, hasZipkin, "prepareRadiusChart must not mutate helmChart.Values")
}

func Test_parseUserValuesFromCLI_InvalidSetArg(t *testing.T) {
	options := &RadiusChartOptions{
		SetArgs: []string{"invalid_no_equals"},
	}

	_, err := parseUserValuesFromCLI(options)
	require.Error(t, err)
}

func Test_parseUserValuesFromCLI_InvalidSetFileArg(t *testing.T) {
	options := &RadiusChartOptions{
		SetFileArgs: []string{"key=./testdata/nonexistent-file.txt"},
	}

	_, err := parseUserValuesFromCLI(options)
	require.Error(t, err)
}

func Test_parseUserValuesFromCLI_EmptyArgs(t *testing.T) {
	options := &RadiusChartOptions{
		ChartOptions: ChartOptions{},
	}

	values, err := parseUserValuesFromCLI(options)
	require.NoError(t, err)
	assert.Empty(t, values)
}

func Test_prepareRadiusChart_LoadChartError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	mockHelmClient.EXPECT().LoadChart("bad-chart").Return(nil, errors.New("chart not found")).Times(1)
	helmAction := NewHelmAction(mockHelmClient)

	options := &RadiusChartOptions{
		Namespace: "radius-system",
		ChartPath: "bad-chart",
	}

	_, _, _, err := prepareRadiusChart(helmAction, *options, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load Helm chart")
}

func Test_prepareRadiusChart_ParseValuesError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	mockHelmClient.EXPECT().LoadChart("test-chart").Return(&chart.Chart{}, nil).Times(1)
	helmAction := NewHelmAction(mockHelmClient)

	options := &RadiusChartOptions{
		Namespace: "radius-system",
		ChartPath: "test-chart",
		SetArgs:   []string{"invalid_no_equals"},
	}

	_, _, _, err := prepareRadiusChart(helmAction, *options, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse Radius values")
}

func Test_AddRadiusValuesOverrideWithSet(t *testing.T) {
	options := &RadiusChartOptions{
		SetArgs: []string{"rp.image=ghcr.io/radius-project/applications-rp,rp.tag=latest", "global.zipkin.url=url,global.prometheus.path=path"},
	}

	values, err := parseUserValuesFromCLI(options)
	require.NoError(t, err)

	// validate image, tag for rp should have been overridden with latest
	o := values["rp"].(map[string]any)
	_, ok := o["tag"]
	assert.True(t, ok)
	assert.Equal(t, o["tag"], "latest")
	_, ok = o["image"]
	assert.True(t, ok)
	assert.Equal(t, o["image"], "ghcr.io/radius-project/applications-rp")

	_, ok = values["global"]
	assert.True(t, ok)
	global := values["global"].(map[string]any)
	_, ok = global["zipkin"]
	assert.True(t, ok)
	zipkin := global["zipkin"].(map[string]any)
	_, ok = zipkin["url"]
	assert.True(t, ok)
	assert.Equal(t, zipkin["url"], "url")

	_, ok = global["prometheus"]
	assert.True(t, ok)
	prometheus := global["prometheus"].(map[string]any)
	_, ok = prometheus["path"]
	assert.True(t, ok)
	assert.Equal(t, prometheus["path"], "path")
}

// Test_ApplyHelmChart_TimeoutPropagation verifies the caller-configured readiness timeout
// (`rad install kubernetes --timeout`) reaches the Helm client for both the install and the
// reinstall/upgrade path. See https://github.com/radius-project/radius/issues/10236.
func Test_ApplyHelmChart_TimeoutPropagation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		reinstall bool
		timeout   time.Duration
	}{
		{
			name:      "install propagates an explicit timeout",
			reinstall: false,
			timeout:   42 * time.Minute,
		},
		{
			name:      "install propagates the unset timeout unchanged",
			reinstall: false,
			timeout:   0,
		},
		{
			name:      "reinstall propagates an explicit timeout",
			reinstall: true,
			timeout:   42 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockHelmClient := NewMockHelmClient(ctrl)
			helmAction := NewHelmAction(mockHelmClient)

			helmConf := &helm.Configuration{}
			helmChart := &chart.Chart{}
			vals := map[string]any{"key": "value"}

			existingRelease := &releasev1.Release{
				Name:  "myrelease",
				Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.1.0"}},
				Info:  &releasev1.Info{Status: releasecommon.StatusDeployed},
			}

			if tt.reinstall {
				mockHelmClient.EXPECT().
					RunHelmGet(gomock.AssignableToTypeOf(&helm.Configuration{}), "myrelease").
					Return(existingRelease, nil).Times(1)
				mockHelmClient.EXPECT().
					RunHelmUpgrade(gomock.AssignableToTypeOf(&helm.Configuration{}), helmChart, vals, "myrelease", "myns", true, true, tt.timeout).
					Return(existingRelease, nil).Times(1)
			} else {
				mockHelmClient.EXPECT().
					RunHelmGet(gomock.AssignableToTypeOf(&helm.Configuration{}), "myrelease").
					Return(nil, driver.ErrReleaseNotFound).Times(1)
				mockHelmClient.EXPECT().
					RunHelmInstall(gomock.AssignableToTypeOf(&helm.Configuration{}), helmChart, vals, "myrelease", "myns", true, tt.timeout).
					Return(existingRelease, nil).Times(1)
			}

			err := helmAction.ApplyHelmChart("", helmChart, helmConf, ChartOptions{
				ReleaseName: "myrelease",
				Namespace:   "myns",
				Wait:        true,
				Reinstall:   tt.reinstall,
				Timeout:     tt.timeout,
			}, vals)
			require.NoError(t, err)
		})
	}
}

func Test_ApplyHelmChart_InstallError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	helmAction := NewHelmAction(mockHelmClient)

	helmConf := &helm.Configuration{}
	helmChart := &chart.Chart{}
	vals := map[string]any{"key": "value"}

	// QueryRelease: not installed
	mockHelmClient.EXPECT().
		RunHelmGet(gomock.AssignableToTypeOf(&helm.Configuration{}), "myrelease").
		Return(nil, driver.ErrReleaseNotFound).Times(1)

	// Install returns an error
	mockHelmClient.EXPECT().
		RunHelmInstall(gomock.AssignableToTypeOf(&helm.Configuration{}), helmChart, vals, "myrelease", "myns", true, gomock.Any()).
		Return(nil, errors.New("install failed")).Times(1)

	err := helmAction.ApplyHelmChart("", helmChart, helmConf, ChartOptions{
		ReleaseName: "myrelease",
		Namespace:   "myns",
		Wait:        true,
	}, vals)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to run Helm install")
}

func Test_ApplyHelmChart_ReinstallPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	helmAction := NewHelmAction(mockHelmClient)

	helmConf := &helm.Configuration{}
	helmChart := &chart.Chart{}
	vals := map[string]any{"key": "value"}

	existingRelease := &releasev1.Release{
		Name:  "myrelease",
		Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.1.0"}},
		Info:  &releasev1.Info{Status: releasecommon.StatusDeployed},
	}

	// QueryRelease: already installed
	mockHelmClient.EXPECT().
		RunHelmGet(gomock.AssignableToTypeOf(&helm.Configuration{}), "myrelease").
		Return(existingRelease, nil).Times(1)

	// Reinstall triggers upgrade with reuseValues=true
	mockHelmClient.EXPECT().
		RunHelmUpgrade(gomock.AssignableToTypeOf(&helm.Configuration{}), helmChart, vals, "myrelease", "myns", true, true, gomock.Any()).
		Return(existingRelease, nil).Times(1)

	err := helmAction.ApplyHelmChart("", helmChart, helmConf, ChartOptions{
		ReleaseName: "myrelease",
		Namespace:   "myns",
		Wait:        true,
		Reinstall:   true,
	}, vals)
	require.NoError(t, err)
}

func Test_ApplyHelmChart_ReinstallError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	helmAction := NewHelmAction(mockHelmClient)

	helmConf := &helm.Configuration{}
	helmChart := &chart.Chart{}
	vals := map[string]any{"key": "value"}

	existingRelease := &releasev1.Release{
		Name:  "myrelease",
		Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.1.0"}},
		Info:  &releasev1.Info{Status: releasecommon.StatusDeployed},
	}

	// QueryRelease: already installed
	mockHelmClient.EXPECT().
		RunHelmGet(gomock.AssignableToTypeOf(&helm.Configuration{}), "myrelease").
		Return(existingRelease, nil).Times(1)

	// Reinstall triggers upgrade but fails
	mockHelmClient.EXPECT().
		RunHelmUpgrade(gomock.AssignableToTypeOf(&helm.Configuration{}), helmChart, vals, "myrelease", "myns", true, true, gomock.Any()).
		Return(nil, errors.New("upgrade failed")).Times(1)

	err := helmAction.ApplyHelmChart("", helmChart, helmConf, ChartOptions{
		ReleaseName: "myrelease",
		Namespace:   "myns",
		Wait:        true,
		Reinstall:   true,
	}, vals)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to run Helm upgrade")
}

func Test_ApplyHelmChart_AlreadyInstalled_NoReinstall(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	helmAction := NewHelmAction(mockHelmClient)

	helmConf := &helm.Configuration{}
	helmChart := &chart.Chart{}
	vals := map[string]any{"key": "value"}

	existingRelease := &releasev1.Release{
		Name:  "myrelease",
		Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.1.0"}},
		Info:  &releasev1.Info{Status: releasecommon.StatusDeployed},
	}

	// QueryRelease: already installed
	mockHelmClient.EXPECT().
		RunHelmGet(gomock.AssignableToTypeOf(&helm.Configuration{}), "myrelease").
		Return(existingRelease, nil).Times(1)

	// No install or upgrade should be called
	err := helmAction.ApplyHelmChart("", helmChart, helmConf, ChartOptions{
		ReleaseName: "myrelease",
		Namespace:   "myns",
		Wait:        true,
		Reinstall:   false,
	}, vals)
	require.NoError(t, err)
}

func Test_ApplyHelmChart_QueryReleaseError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	helmAction := NewHelmAction(mockHelmClient)

	helmConf := &helm.Configuration{}
	helmChart := &chart.Chart{}
	vals := map[string]any{}

	// QueryRelease returns an error (not ErrReleaseNotFound)
	mockHelmClient.EXPECT().
		RunHelmGet(gomock.AssignableToTypeOf(&helm.Configuration{}), "myrelease").
		Return(nil, errors.New("connection refused")).Times(1)

	err := helmAction.ApplyHelmChart("", helmChart, helmConf, ChartOptions{
		ReleaseName: "myrelease",
		Namespace:   "myns",
	}, vals)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to query Helm release")
}
