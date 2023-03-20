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
		Image:  "image",
		Values: "global.de.tag=de-tag,global.ucp.tag=ucp-tag,",
	}
	err := AddRadiusValues(&helmChart, options)
	values := helmChart.Values
	require.Equal(t, err, nil)

	_, ok := values["global"]
	assert.True(t, ok)
	global := values["global"].(map[string]any)
	_, ok = global["rp"]
	assert.True(t, ok)
	rp := global["rp"].(map[string]any)
	_, ok = rp["container"]
	assert.True(t, ok)
	assert.Equal(t, rp["container"], "image")

	_, ok = values["global"]
	assert.True(t, ok)
	global = values["global"].(map[string]any)
	_, ok = global["de"]
	assert.True(t, ok)
	de := global["de"].(map[string]any)
	_, ok = de["tag"]
	assert.True(t, ok)
	assert.Equal(t, de["tag"], "de-tag")

	_, ok = global["ucp"]
	assert.True(t, ok)
	ucp := global["ucp"].(map[string]any)
	_, ok = ucp["tag"]
	assert.True(t, ok)
	assert.Equal(t, ucp["tag"], "ucp-tag")

}
