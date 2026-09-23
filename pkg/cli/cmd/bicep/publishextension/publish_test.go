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
	"encoding/json"
	"errors"
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

type publishInvocation struct {
	Args        []string
	Directory   string
	Config      string
	Environment map[string]string
}

func TestMain(m *testing.M) {
	if filename := os.Getenv(publishHelperEnv); filename != "" {
		if err := runPublishHelper(filename); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(23) //nolint:forbidigo // The test binary stands in for a failing Bicep process.
		}
		os.Exit(0) //nolint:forbidigo // Return only the fake Bicep output.
	}
	os.Exit(m.Run()) //nolint:forbidigo // Return the test suite's exit status.
}

func runPublishHelper(filename string) error {
	if len(os.Args) < 5 || os.Args[1] != "publish-extension" || os.Args[3] != "--target" {
		return errors.New("unexpected Bicep arguments")
	}
	if _, err := os.Stat(os.Args[2]); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(os.Args[2]), "bicepconfig.json")); !errors.Is(err, os.ErrNotExist) {
		return errors.New("unexpected configuration in the generated index directory")
	}
	directory, err := os.Getwd()
	if err != nil {
		return err
	}
	config, err := os.ReadFile(filepath.Join(directory, "bicepconfig.json"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	invocation := publishInvocation{
		Args:        os.Args[1:],
		Directory:   directory,
		Config:      string(config),
		Environment: map[string]string{},
	}
	for _, name := range []string{"DOCKER_CONFIG", "AZURE_CONFIG_DIR", "SSL_CERT_FILE", "PATH", "BICEP_TRUSTED_REGISTRIES"} {
		invocation.Environment[name] = os.Getenv(name)
	}
	data, err := json.Marshal(invocation)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filename, data, 0600); err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, "Bicep stdout")
	fmt.Fprintln(os.Stderr, "Bicep stderr")
	if os.Getenv("RADIUS_PUBLISH_EXTENSION_TEST_FAIL") == "true" {
		return errors.New("Bicep publish failed")
	}
	target := os.Args[4]
	if !strings.HasPrefix(target, "br:") && !strings.HasPrefix(target, "ts:") {
		return os.WriteFile(target, []byte("published extension"), 0600)
	}
	return nil
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
	manifest, err := os.ReadFile("testdata/valid.yaml")
	require.NoError(t, err)
	executable, err := os.Executable()
	require.NoError(t, err)

	for _, tt := range []struct {
		name     string
		target   string
		config   string
		local    bool
		absolute bool
		force    bool
		fail     bool
		canceled bool
	}{
		{name: "relative local output", target: "./output.tgz", local: true},
		{name: "absolute local output", target: "output.tgz", local: true, absolute: true},
		{name: "local force", target: "./output.tgz", local: true, force: true},
		{name: "explicit OCI and force", target: "br:ghcr.io/example/extension:v1", force: true, config: "// caller JSONC\n" + `{"experimentalFeaturesEnabled":{"ociEnabled":true},"cloud":{"currentProfile":"AzureChinaCloud"},"cacheRootDirectory":"~/cache","extensions":{"custom":"./custom.tgz"},"integer":9007199254740993123456789}`},
		{name: "OCI false", target: "br:ghcr.io/example/extension:v1", config: `{"experimentalFeaturesEnabled":{"ociEnabled":false}}`},
		{name: "OCI unset", target: "br:ghcr.io/example/extension:v1", config: `{}`},
		{name: "no caller config", target: "br:ghcr.io/example/extension:v1"},
		{name: "ACR reference", target: "br:example.azurecr.cn/extension:v1"},
		{name: "loopback reference", target: "br:localhost:5000/extension:v1", config: `{"experimentalFeaturesEnabled":{"ociEnabled":false}}`},
		{name: "other registry reference", target: "ts:example/extension:v1"},
		{name: "subprocess failure", target: "br:ghcr.io/example/extension:v1", fail: true},
		{name: "cancellation", target: "./output.tgz", canceled: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			caller, temporary := filepath.Join(root, "caller"), filepath.Join(root, "temporary")
			require.NoError(t, os.Mkdir(caller, 0700))
			require.NoError(t, os.Mkdir(temporary, 0700))
			require.NoError(t, os.WriteFile(filepath.Join(caller, "provider.yaml"), manifest, 0600))
			t.Chdir(caller)
			for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
				t.Setenv(name, temporary)
			}
			t.Setenv(bicep.BicepEnvVar, executable)
			record := filepath.Join(caller, "invocation.json")
			t.Setenv(publishHelperEnv, record)
			t.Setenv("RADIUS_PUBLISH_EXTENSION_TEST_FAIL", fmt.Sprint(tt.fail))
			environment := map[string]string{
				"DOCKER_CONFIG":            ".docker",
				"AZURE_CONFIG_DIR":         ".azure",
				"SSL_CERT_FILE":            "certs/ca.pem",
				"PATH":                     "relative-tools",
				"BICEP_TRUSTED_REGISTRIES": "localhost,registry.example",
			}
			for name, value := range environment {
				t.Setenv(name, value)
			}
			configPath := filepath.Join(caller, "bicepconfig.json")
			if tt.config != "" {
				require.NoError(t, os.WriteFile(configPath, []byte(tt.config), 0600))
			}
			target := tt.target
			if tt.absolute {
				target = filepath.Join(caller, target)
			}
			logs := &output.MockOutput{}
			runner := &Runner{Output: logs, ResourceProviderManifestFilePath: "./provider.yaml", Target: target, Force: tt.force}
			stdout, stderr := capturePublishOutput(t)
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
					require.NoFileExists(t, record)
				} else {
					var exitError *exec.ExitError
					require.ErrorAs(t, err, &exitError)
					require.Equal(t, 23, exitError.ExitCode())
				}
			} else {
				require.NoError(t, err)
				require.Equal(t, []any{output.LogOutput{
					Format: "Successfully published Bicep extension %q to %q",
					Params: []any{"./provider.yaml", target},
				}}, logs.Writes)
			}
			entries, err := os.ReadDir(temporary)
			require.NoError(t, err)
			require.Empty(t, entries)
			currentDirectory, err := os.Getwd()
			require.NoError(t, err)
			require.Equal(t, caller, currentDirectory)
			for name, value := range environment {
				require.Equal(t, value, os.Getenv(name), name)
			}
			if tt.config == "" {
				require.NoFileExists(t, configPath)
			} else {
				unchanged, err := os.ReadFile(configPath)
				require.NoError(t, err)
				require.Equal(t, tt.config, string(unchanged))
			}
			if tt.canceled {
				return
			}
			data, err := os.ReadFile(record)
			require.NoError(t, err)
			var invocation publishInvocation
			require.NoError(t, json.Unmarshal(data, &invocation))
			require.Equal(t, caller, invocation.Directory)
			require.Equal(t, tt.config, invocation.Config)
			require.Equal(t, environment, invocation.Environment)
			index := invocation.Args[1]
			require.True(t, filepath.IsAbs(index))
			require.Equal(t, temporary, filepath.Dir(filepath.Dir(index)))
			require.NoDirExists(t, filepath.Dir(index))
			expectedArgs := []string{"publish-extension", index, "--target", target}
			if tt.force {
				expectedArgs = append(expectedArgs, "--force")
			}
			require.Equal(t, expectedArgs, invocation.Args)
			if tt.local {
				published, err := os.ReadFile(target)
				require.NoError(t, err)
				require.Equal(t, "published extension", string(published))
			}
			for file, message := range map[*os.File]string{stdout: "Bicep stdout", stderr: "Bicep stderr"} {
				data, err := os.ReadFile(file.Name())
				require.NoError(t, err)
				require.Contains(t, string(data), message)
			}
		})
	}
}

func capturePublishOutput(t *testing.T) (stdout, stderr *os.File) {
	t.Helper()
	directory := t.TempDir()
	stdout, err := os.Create(filepath.Join(directory, "stdout"))
	require.NoError(t, err)
	stderr, err = os.Create(filepath.Join(directory, "stderr"))
	require.NoError(t, err)
	originalStdout, originalStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = stdout, stderr
	t.Cleanup(func() {
		os.Stdout, os.Stderr = originalStdout, originalStderr
		require.NoError(t, stdout.Close())
		require.NoError(t, stderr.Close())
	})
	return stdout, stderr
}
