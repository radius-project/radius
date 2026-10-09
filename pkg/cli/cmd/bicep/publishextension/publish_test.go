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

package publishextension

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/test/radcli"
	"github.com/stretchr/testify/require"
)

const publishHelperEnv = "RADIUS_PUBLISH_EXTENSION_TEST_HELPER"

func TestMain(m *testing.M) {
	if os.Getenv(publishHelperEnv) != "" {
		// Bicep's fixed arguments cannot select a helper test themselves.
		if err := flag.Set("test.run", "^TestPublishHelper$"); err != nil {
			panic(err)
		}
	}
	os.Exit(m.Run()) //nolint:forbidigo // Return the test suite's exit status.
}

func TestPublishHelper(t *testing.T) {
	if os.Getenv(publishHelperEnv) == "" {
		return
	}
	target := os.Getenv("RADIUS_PUBLISH_TARGET")
	require.GreaterOrEqual(t, len(os.Args), 5)
	args := []string{"publish-extension", os.Args[2], "--target", target}
	if os.Getenv("RADIUS_PUBLISH_FORCE") == "true" {
		args = append(args, "--force")
	}
	require.Equal(t, args, os.Args[1:])
	require.True(t, filepath.IsAbs(os.Args[2]))
	require.Equal(t, "index.json", filepath.Base(os.Args[2]))
	require.FileExists(t, os.Args[2])
	directory, err := os.Getwd()
	require.NoError(t, err)
	require.Equal(t, os.Getenv(publishHelperEnv), directory)
	config, err := os.ReadFile("bicepconfig.json")
	require.NoError(t, err)
	require.Equal(t, os.Getenv("RADIUS_PUBLISH_CONFIG"), string(config))
	require.Equal(t, ".docker", os.Getenv("DOCKER_CONFIG"))
	if os.Getenv("RADIUS_PUBLISH_FAIL") == "true" {
		os.Exit(23) //nolint:forbidigo // Distinguish a Bicep failure from a failed helper assertion.
	}
	if !strings.HasPrefix(target, "br:") {
		require.NoError(t, os.WriteFile(target, []byte("published extension"), 0600))
	}
}

func TestRunner_Validate(t *testing.T) {
	tests := []radcli.ValidateInput{
		{
			Name:          "Valid",
			Input:         []string{"--from-file", "testdata/valid.yaml", "--target", "./output.tgz"},
			ExpectedValid: true,
		},
		{
			Name:          "Valid with force flag",
			Input:         []string{"--from-file", "testdata/valid.yaml", "--target", "./output.tgz", "--force"},
			ExpectedValid: true,
		},
		{
			Name:          "Valid registry target",
			Input:         []string{"--from-file", "testdata/valid.yaml", "--target", "br:ghcr.io/example/extension:v1"},
			ExpectedValid: true,
		},
		{
			Name:          "Invalid: invalid manifest",
			Input:         []string{"--from-file", "testdata/invalid.yaml", "--target", "./output.tgz"},
			ExpectedValid: false,
		},
		{
			Name:          "Invalid: missing required options",
			Input:         []string{"--from-file", "testdata/valid.yaml"},
			ExpectedValid: false,
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, tests)
}

func TestRunner_Run(t *testing.T) {
	manifest, err := filepath.Abs("testdata/valid.yaml")
	require.NoError(t, err)
	executable, err := os.Executable()
	require.NoError(t, err)

	for _, tt := range []struct {
		name, target, config  string
		force, fail, canceled bool
	}{
		{name: "local archive", target: "./output.tgz", config: `{}`},
		{name: "OCI true and force", target: "br:ghcr.io/example/extension:v1", force: true, config: `{"experimentalFeaturesEnabled":{"ociEnabled":true}}`},
		{name: "OCI false", target: "br:ghcr.io/example/extension:v1", config: `{"experimentalFeaturesEnabled":{"ociEnabled":false}}`},
		{name: "OCI unset", target: "br:ghcr.io/example/extension:v1", config: `{}`},
		{name: "subprocess failure", target: "br:ghcr.io/example/extension:v1", config: `{}`, fail: true},
		{name: "cancellation", target: "./output.tgz", config: `{}`, canceled: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			caller, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			temporary := t.TempDir()
			t.Chdir(caller)
			for name, value := range map[string]string{
				bicep.BicepEnvVar:       executable,
				publishHelperEnv:        caller,
				"RADIUS_PUBLISH_TARGET": tt.target,
				"RADIUS_PUBLISH_CONFIG": tt.config,
				"RADIUS_PUBLISH_FORCE":  fmt.Sprint(tt.force),
				"RADIUS_PUBLISH_FAIL":   fmt.Sprint(tt.fail),
				"DOCKER_CONFIG":         ".docker",
				"TMPDIR":                temporary, "TMP": temporary, "TEMP": temporary,
			} {
				t.Setenv(name, value)
			}
			require.NoError(t, os.WriteFile("bicepconfig.json", []byte(tt.config), 0600))
			logs := &output.MockOutput{}
			runner := &Runner{Output: logs, ResourceProviderManifestFilePath: manifest, Target: tt.target, Force: tt.force}
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			if tt.canceled {
				cancel()
			}
			err = runner.Run(ctx)
			if tt.fail || tt.canceled {
				require.ErrorContains(t, err, "Failed to publish Bicep extension")
				require.Empty(t, logs.Writes)
				if tt.canceled {
					require.ErrorIs(t, err, context.Canceled)
				} else {
					var exitError *exec.ExitError
					require.ErrorAs(t, err, &exitError)
					require.Equal(t, 23, exitError.ExitCode())
				}
			} else {
				require.NoError(t, err)
				require.Len(t, logs.Writes, 1)
			}
			entries, err := os.ReadDir(temporary)
			require.NoError(t, err)
			require.Empty(t, entries)
			currentDirectory, err := os.Getwd()
			require.NoError(t, err)
			require.Equal(t, caller, currentDirectory)
			require.Equal(t, ".docker", os.Getenv("DOCKER_CONFIG"))
			config, err := os.ReadFile("bicepconfig.json")
			require.NoError(t, err)
			require.Equal(t, tt.config, string(config))
			if tt.target == "./output.tgz" {
				if tt.canceled {
					require.NoFileExists(t, tt.target)
					return
				}
				data, err := os.ReadFile(tt.target)
				require.NoError(t, err)
				require.Equal(t, "published extension", string(data))
			}
		})
	}
}
