// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package helm

import (
	"testing"

	"github.com/project-radius/radius/pkg/version"
	"github.com/stretchr/testify/require"
)

func Test_CanSetCLIOptions(t *testing.T) {
	cliOptions := CLIClusterOptions{
		Radius: RadiusOptions{
			ChartPath: "chartpath",
			Image:     "image",
			Tag:       "tag",
		},
	}
	clusterOptions := PopulateDefaultClusterOptions(cliOptions)

	require.Equal(t, "chartpath", clusterOptions.Radius.ChartPath)
	require.Equal(t, "image", clusterOptions.Radius.Image)
	require.Equal(t, "tag", clusterOptions.Radius.Tag)
}

func Test_DefaultTags(t *testing.T) {
	clusterOptions := NewDefaultClusterOptions()
	tag := version.Channel()
	if version.IsEdgeChannel() {
		tag = "latest"
	}

	require.Equal(t, tag, clusterOptions.Radius.Tag)
	require.Equal(t, tag, clusterOptions.Radius.AppCoreTag)
	require.Equal(t, tag, clusterOptions.Radius.UCPTag)
	require.Equal(t, tag, clusterOptions.Radius.DETag)

}

func Test_DefaultsToHelmChartVersionValue(t *testing.T) {
	clusterOptions := PopulateDefaultClusterOptions(CLIClusterOptions{})

	// Not checking other values due to potential failures on release builds, the chart version
	// is primarily the mail regression we see.
	require.Equal(t, version.ChartVersion(), clusterOptions.Radius.ChartVersion)
}
