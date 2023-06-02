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
	// validate tags for ucp, de, and rp

	for _, k := range []string{"ucp", "de", "rp"} {
		o := values[k].(map[string]any)
		_, ok := o["tag"]
		assert.True(t, ok)
		assert.Equal(t, o["tag"], "imageversion")
	}

	_, ok := values["global"]
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
