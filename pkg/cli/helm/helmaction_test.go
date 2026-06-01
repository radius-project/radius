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
	"testing"

	containerderrors "github.com/containerd/containerd/remotes/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"helm.sh/helm/v3/pkg/chart"
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

	err = fmt.Errorf("%w: wrapped error", containerderrors.ErrUnexpectedStatus{})
	result = isHelmGHCR403Error(err)
	assert.False(t, result)

	err = fmt.Errorf("%w: wrapped error", containerderrors.ErrUnexpectedStatus{StatusCode: http.StatusForbidden, RequestURL: "ghcr.io/myregistry"})
	result = isHelmGHCR403Error(err)
	assert.True(t, result)

	err = containerderrors.ErrUnexpectedStatus{StatusCode: http.StatusForbidden, RequestURL: "ghcr.io/myregistry"}
	result = isHelmGHCR403Error(err)
	assert.True(t, result)

	err = containerderrors.ErrUnexpectedStatus{StatusCode: http.StatusUnauthorized, RequestURL: "ghcr.io/myregistry"}
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
