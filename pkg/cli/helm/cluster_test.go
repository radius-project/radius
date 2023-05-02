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
			Reinstall: true,
		},
	}
	clusterOptions := PopulateDefaultClusterOptions(cliOptions)

	require.Equal(t, "chartpath", clusterOptions.Radius.ChartPath)
	require.Equal(t, true, clusterOptions.Radius.Reinstall)

}

func Test_DefaultsToHelmChartVersionValue(t *testing.T) {
	clusterOptions := PopulateDefaultClusterOptions(CLIClusterOptions{})

	// Not checking other values due to potential failures on release builds, the chart version
	// is primarily the mail regression we see.
	require.Equal(t, version.ChartVersion(), clusterOptions.Radius.ChartVersion)
}
