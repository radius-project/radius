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
		Values: "de.tag=de-tag,ucp.tag=ucp-tag,rp.image=image",
	}
	err := AddRadiusValues(&helmChart, options)
	values := helmChart.Values
	require.Equal(t, err, nil)

	_, ok := values["rp"]
	assert.True(t, ok)
	rp := values["rp"].(map[string]any)
	_, ok = rp["image"]
	assert.True(t, ok)
	assert.Equal(t, rp["image"], "image")

	_, ok = values["ucp"]
	assert.True(t, ok)
	ucp := values["ucp"].(map[string]any)
	_, ok = ucp["tag"]
	assert.True(t, ok)
	assert.Equal(t, ucp["tag"], "ucp-tag")

	_, ok = values["de"]
	assert.True(t, ok)
	de := values["de"].(map[string]any)
	_, ok = de["tag"]
	assert.True(t, ok)
	assert.Equal(t, de["tag"], "de-tag")
}
