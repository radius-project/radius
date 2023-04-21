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
	t.Skip()
	var helmChart chart.Chart
	helmChart.Values = map[string]any{}
	options := &RadiusOptions{
		AppCoreImage: "appcoreimage",
		AppCoreTag:   "appcoretag",
		DEImage:      "deimage",
		DETag:        "detag",
		UCPImage:     "ucpimage",
		UCPTag:       "ucptag",
		Values:       "global.zipkin.url=url,global.prometheus.path=path,global.prometheus.enabled=false",
	}
	err := AddRadiusValues(&helmChart, options)
	values := helmChart.Values
	require.Equal(t, err, nil)

	_, ok := values["rp"]
	assert.True(t, ok)
	rp := values["rp"].(map[string]any)
	_, ok = rp["image"]
	assert.True(t, ok)
	assert.Equal(t, rp["image"], "appcoreimage")
	_, ok = rp["tag"]
	assert.True(t, ok)
	assert.Equal(t, rp["tag"], "appcoretag")

	_, ok = values["ucp"]
	assert.True(t, ok)
	ucp := values["ucp"].(map[string]any)
	_, ok = ucp["image"]
	assert.True(t, ok)
	assert.Equal(t, ucp["image"], "ucpimage")
	_, ok = ucp["tag"]
	assert.True(t, ok)
	assert.Equal(t, ucp["tag"], "ucptag")

	_, ok = values["de"]
	assert.True(t, ok)
	de := values["de"].(map[string]any)
	_, ok = de["image"]
	assert.True(t, ok)
	assert.Equal(t, de["image"], "deimage")
	_, ok = de["tag"]
	assert.True(t, ok)
	assert.Equal(t, de["tag"], "detag")

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
