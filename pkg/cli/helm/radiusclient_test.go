// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart"
)

func Test_AddRadiusValues(t *testing.T) {
	var helmChart chart.Chart
	helmChart.Values = map[string]any{}
	options := &RadiusOptions{
		ImageVersion: "imageversion",
		Values:       "global.zipkin.url=url,global.prometheus.path=path",
	}

	err := AddRadiusValues(&helmChart, options)
	require.Equal(t, err, nil)

	values := helmChart.Values
	_, ok := values["rp"]

	assert.True(t, ok)

	rp := values["rp"].(map[string]any)
	_, ok = rp["tag"]
	assert.True(t, ok)
	assert.Equal(t, rp["tag"], "imageversion")

	ucp := values["ucp"].(map[string]any)
	_, ok = ucp["tag"]
	assert.True(t, ok)
	assert.Equal(t, ucp["tag"], "imageversion")

	de := values["de"].(map[string]any)
	_, ok = de["tag"]
	assert.True(t, ok)
	assert.Equal(t, de["tag"], "imageversion")

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
