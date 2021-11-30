// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rad

import (
	"os"
	"path"
	"testing"

	"github.com/Azure/radius/pkg/cli/bicep"
	"github.com/Azure/radius/pkg/cli/radyaml"
	"github.com/Azure/radius/test/radcli"
	"github.com/Azure/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

func Test_AppInit(t *testing.T) {
	ctx, cancel := testcontext.GetContext(t)
	defer cancel()

	tempDir := t.TempDir()

	cli := radcli.NewCLI(t, "" /* Don't need an environment for these tests */)
	cli.WorkingDirectory = tempDir

	err := cli.ApplicationInit(ctx, "cool-test")
	require.NoError(t, err)

	t.Run("rad.yaml", func(t *testing.T) {
		require.FileExists(t, path.Join(tempDir, "rad", "rad.yaml"))

		file, err := os.Open(path.Join(tempDir, "rad", "rad.yaml"))
		require.NoError(t, err)
		defer file.Close()

		parsed, err := radyaml.Parse(file)
		require.NoError(t, err)
		require.Equal(t, "cool-test", parsed.Name)
	})

	t.Run("infra.bicep", func(t *testing.T) {
		require.FileExists(t, path.Join(tempDir, "rad", "infra.bicep"))

		// No deep validation here just that it builds.
		_, err := bicep.Build(path.Join(tempDir, "rad", "infra.bicep"))
		require.NoError(t, err)
	})

	t.Run("app.bicep", func(t *testing.T) {
		require.FileExists(t, path.Join(tempDir, "rad", "app.bicep"))

		// No deep validation here just that it builds.
		_, err := bicep.Build(path.Join(tempDir, "rad", "app.bicep"))
		require.NoError(t, err)
	})
}
