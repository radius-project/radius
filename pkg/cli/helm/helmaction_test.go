package helm

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	containerderrors "github.com/containerd/containerd/remotes/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func Test_addArgsFromCLI(t *testing.T) {
	var helmChart chart.Chart
	helmChart.Values = map[string]any{}
	options := &RadiusChartOptions{
		ChartOptions: ChartOptions{
			SetArgs:     []string{"global.zipkin.url=url,global.prometheus.path=path"},
			SetFileArgs: []string{"global.rootCA.cert=./testdata/fake-ca-cert.crt"},
		},
	}

	err := addArgsFromCLI(&helmChart, options)
	require.Equal(t, err, nil)

	values := helmChart.Values

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

func Test_AddRadiusValuesOverrideWithSet(t *testing.T) {
	var helmChart chart.Chart
	helmChart.Values = map[string]any{}
	options := &RadiusChartOptions{
		ChartOptions: ChartOptions{
			SetArgs: []string{"rp.image=ghcr.io/radius-project/applications-rp,rp.tag=latest", "global.zipkin.url=url,global.prometheus.path=path"},
		},
	}

	err := addArgsFromCLI(&helmChart, options)
	require.Equal(t, err, nil)

	values := helmChart.Values

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
