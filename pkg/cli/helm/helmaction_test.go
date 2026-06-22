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
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	helm "helm.sh/helm/v4/pkg/action"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage/driver"
	"oras.land/oras-go/v2/registry/remote/errcode"
)

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
		ChartOptions: ChartOptions{
			SetArgs:     []string{"global.zipkin.url=url,global.prometheus.path=path"},
			SetFileArgs: []string{"global.rootCA.cert=./testdata/fake-ca-cert.crt"},
		},
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
		ChartOptions: ChartOptions{
			Namespace: "radius-system",
			ChartPath: "test-chart",
			SetArgs:   []string{"global.zipkin.url=url"},
		},
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
		ChartOptions: ChartOptions{
			SetArgs: []string{"invalid_no_equals"},
		},
	}

	_, err := parseUserValuesFromCLI(options)
	require.Error(t, err)
}

func Test_parseUserValuesFromCLI_InvalidSetFileArg(t *testing.T) {
	options := &RadiusChartOptions{
		ChartOptions: ChartOptions{
			SetFileArgs: []string{"key=./testdata/nonexistent-file.txt"},
		},
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
		ChartOptions: ChartOptions{
			Namespace: "radius-system",
			ChartPath: "bad-chart",
		},
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
		ChartOptions: ChartOptions{
			Namespace: "radius-system",
			ChartPath: "test-chart",
			SetArgs:   []string{"invalid_no_equals"},
		},
	}

	_, _, _, err := prepareRadiusChart(helmAction, *options, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse Radius values")
}

func Test_AddRadiusValuesOverrideWithSet(t *testing.T) {
	options := &RadiusChartOptions{
		ChartOptions: ChartOptions{
			SetArgs: []string{"rp.image=ghcr.io/radius-project/applications-rp,rp.tag=latest", "global.zipkin.url=url,global.prometheus.path=path"},
		},
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
		RunHelmInstall(gomock.AssignableToTypeOf(&helm.Configuration{}), helmChart, vals, "myrelease", "myns", true).
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
		RunHelmUpgrade(gomock.AssignableToTypeOf(&helm.Configuration{}), helmChart, vals, "myrelease", "myns", true, true).
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
		RunHelmUpgrade(gomock.AssignableToTypeOf(&helm.Configuration{}), helmChart, vals, "myrelease", "myns", true, true).
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
