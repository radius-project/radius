/*
Copyright 2023 The Radius Authors.
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
