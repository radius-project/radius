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
		AppCoreImage: "image",
		Values:       "global.zipkin.url=url,global.prometheus.path=path,global.prometheus.enabled=false",
	}
	err := AddRadiusValues(&helmChart, options)
	values := helmChart.Values
	require.Equal(t, err, nil)

	_, ok := values["radius-rp"]
	assert.True(t, ok)
	rp := values["radius-rp"].(map[string]any)
	_, ok = rp["image"]
	assert.True(t, ok)
	assert.Equal(t, rp["image"], "image")

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
	_, ok = prometheus["enabled"]
	assert.True(t, ok)
	assert.Equal(t, prometheus["enabled"], false)
}
