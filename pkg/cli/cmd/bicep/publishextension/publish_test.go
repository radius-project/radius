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
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/test/radcli"
	"github.com/stretchr/testify/require"
)

const (
	publishHelperEnv     = "RADIUS_PUBLISH_EXTENSION_TEST_HELPER"
	publishHelperModeEnv = "RADIUS_PUBLISH_EXTENSION_TEST_MODE"
)

type publishInvocation struct {
	Args        []string
	Directory   string
	Config      string
	ConfigMode  os.FileMode
	Environment map[string]string
}

func TestMain(m *testing.M) {
	if filename := os.Getenv(publishHelperEnv); filename != "" {
		os.Exit(runPublishHelper(filename)) //nolint:forbidigo // The test binary stands in for Bicep.
	}
	os.Exit(m.Run()) //nolint:forbidigo // Return the test suite's exit status.
}

func runPublishHelper(filename string) int {
	if len(os.Args) < 5 || os.Args[1] != "publish-extension" || os.Args[3] != "--target" {
		fmt.Fprintln(os.Stderr, "unexpected Bicep arguments")
		return 1
	}
	if _, err := os.Stat(os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	directory, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	invocation := publishInvocation{
		Args:        os.Args[1:],
		Directory:   directory,
		Environment: map[string]string{},
	}
	configPath := filepath.Join(directory, "bicepconfig.json")
	if config, err := os.ReadFile(configPath); err == nil {
		invocation.Config = string(config)
		info, err := os.Stat(configPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		invocation.ConfigMode = info.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	for _, name := range []string{"DOCKER_CONFIG", "AZURE_CONFIG_DIR", "AZURE_FEDERATED_TOKEN_FILE", "AZURE_CLIENT_CERTIFICATE_PATH", "SSL_CERT_FILE", "PATH", "SSL_CERT_DIR"} {
		invocation.Environment[name] = os.Getenv(name)
	}
	data, err := json.Marshal(invocation)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := os.WriteFile(filename, data, 0600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Fprintln(os.Stdout, "Bicep stdout")
	fmt.Fprintln(os.Stderr, "Bicep stderr")
	switch os.Getenv(publishHelperModeEnv) {
	case "fail":
		return 23
	case "wait":
		time.Sleep(time.Minute)
		return 1
	}
	target := os.Args[4]
	if !strings.HasPrefix(target, "br:") && !strings.HasPrefix(target, "ts:") {
		if err := os.WriteFile(target, []byte("published extension"), 0600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	return 0
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
	for _, tt := range []struct {
		name       string
		target     string
		absolute   bool
		registry   bool
		preserve   bool
		ociEnabled bool
		ociUnset   bool
		force      bool
		fail       bool
	}{
		{name: "relative local output", target: "./output.tgz"},
		{name: "absolute local output", target: "output.tgz", absolute: true},
		{name: "parent relative local output", target: "../output.tgz"},
		{name: "local force", target: "./output.tgz", force: true},
		{name: "OCI registry", target: "br:ghcr.io/example/extension:v1", registry: true},
		{name: "OCI force", target: "br:ghcr.io/example/extension:v1", registry: true, force: true},
		{name: "ACR registry", target: "br:example.azurecr.cn/extension:v1", registry: true},
		{name: "other registry reference", target: "ts:example/extension:v1", registry: true},
		{name: "TLS loopback default", target: "br:localhost:5000/extension:v1", registry: true, preserve: true, ociUnset: true},
		{name: "TLS IPv4 loopback", target: "br:127.0.0.1:5000/extension:v1", registry: true, preserve: true},
		{name: "TLS IPv6 loopback", target: "br:[::1]:5000/extension:v1", registry: true, preserve: true},
		{name: "explicit OCI loopback", target: "br:localhost:5000/extension:v1", registry: true, preserve: true, ociEnabled: true},
		{name: "noncanonical numeric host", target: "br:0177.1:5000/extension:v1", registry: true, preserve: true},
		{name: "local subprocess failure", target: "./output.tgz", fail: true},
		{name: "registry subprocess failure", target: "br:ghcr.io/example/extension:v1", registry: true, fail: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			caller, temporary, record := setupPublishProcessTest(t)
			callerConfig := []byte(`{"cloud":{"currentProfile":"AzureChinaCloud"},"experimentalFeaturesEnabled":{"localDeploy":true,"ociEnabled":false},"cacheRootDirectory":"~/caller-cache","extensions":{"custom":"./custom.tgz"}}`)
			if tt.ociEnabled {
				callerConfig = bytes.Replace(callerConfig, []byte(`"ociEnabled":false`), []byte(`"ociEnabled":true`), 1)
			}
			if tt.ociUnset {
				callerConfig = bytes.Replace(callerConfig, []byte(`,"ociEnabled":false`), nil, 1)
			}
			configPath := filepath.Join(caller, "bicepconfig.json")
			require.NoError(t, os.WriteFile(configPath, callerConfig, 0600))
			t.Setenv("DOCKER_CONFIG", ".docker")
			t.Setenv("AZURE_CONFIG_DIR", ".azure")
			t.Setenv("AZURE_FEDERATED_TOKEN_FILE", "token")
			t.Setenv("AZURE_CLIENT_CERTIFICATE_PATH", filepath.Join(caller, "certificate.pem"))
			t.Setenv("SSL_CERT_FILE", "ca.pem")
			pathList := strings.Join([]string{filepath.Join(caller, "tools"), "relative-tools", ""}, string(os.PathListSeparator))
			t.Setenv("PATH", pathList)
			t.Setenv("SSL_CERT_DIR", pathList)
			if tt.fail {
				t.Setenv(publishHelperModeEnv, "fail")
			}
			target := tt.target
			if tt.absolute {
				target = filepath.Join(caller, target)
			}
			logs := &output.MockOutput{}
			runner := &Runner{
				Output:                           logs,
				ResourceProviderManifestFilePath: "./provider.yaml",
				Target:                           target,
				Force:                            tt.force,
			}
			stdout, stderr := capturePublishOutput(t)
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			err := runner.Run(ctx)
			if tt.fail {
				require.ErrorContains(t, err, "Failed to publish Bicep extension")
				var exitError *exec.ExitError
				require.ErrorAs(t, err, &exitError)
				require.Equal(t, 23, exitError.ExitCode())
				require.Empty(t, logs.Writes)
			} else {
				require.NoError(t, err)
				require.Equal(t, []any{output.LogOutput{
					Format: "Successfully published Bicep extension %q to %q",
					Params: []any{"./provider.yaml", target},
				}}, logs.Writes)
			}
			invocation := readPublishInvocation(t, record)
			indexDirectory := filepath.Dir(invocation.Args[1])
			require.True(t, filepath.IsAbs(invocation.Args[1]))
			require.Equal(t, temporary, filepath.Dir(indexDirectory))
			expectedTarget := target
			if !tt.registry {
				expectedTarget, err = filepath.Abs(target)
				require.NoError(t, err)
				if !tt.fail {
					published, err := os.ReadFile(expectedTarget)
					require.NoError(t, err)
					require.Equal(t, "published extension", string(published))
				}
			}
			if !tt.registry || tt.preserve {
				require.Equal(t, caller, invocation.Directory)
				require.Equal(t, string(callerConfig), invocation.Config)
			} else {
				require.Equal(t, indexDirectory, invocation.Directory)
				require.JSONEq(t, `{"cloud":{"currentProfile":"AzureChinaCloud"},"experimentalFeaturesEnabled":{"localDeploy":true,"ociEnabled":true},"cacheRootDirectory":"~/caller-cache","extensions":{"custom":"./custom.tgz"}}`, invocation.Config)
			}
			if runtime.GOOS != "windows" {
				require.Equal(t, os.FileMode(0600), invocation.ConfigMode)
			}
			expectedArgs := []string{"publish-extension", filepath.Join(indexDirectory, "index.json"), "--target", expectedTarget}
			if tt.force {
				expectedArgs = append(expectedArgs, "--force")
			}
			require.Equal(t, expectedArgs, invocation.Args)
			for _, name := range []string{"DOCKER_CONFIG", "AZURE_CONFIG_DIR", "AZURE_FEDERATED_TOKEN_FILE", "AZURE_CLIENT_CERTIFICATE_PATH", "SSL_CERT_FILE"} {
				value := os.Getenv(name)
				expected := value
				if tt.registry && !tt.preserve {
					expected, err = filepath.Abs(value)
					require.NoError(t, err)
				}
				require.Equal(t, expected, invocation.Environment[name], name)
			}
			for _, name := range []string{"PATH", "SSL_CERT_DIR"} {
				expected := pathList
				if tt.registry && !tt.preserve {
					expected = strings.Join([]string{filepath.Join(caller, "tools"), filepath.Join(caller, "relative-tools"), caller}, string(os.PathListSeparator))
				}
				require.Equal(t, expected, invocation.Environment[name], name)
				require.Equal(t, pathList, os.Getenv(name))
			}
			require.Equal(t, ".docker", os.Getenv("DOCKER_CONFIG"))
			require.Equal(t, ".azure", os.Getenv("AZURE_CONFIG_DIR"))
			require.Equal(t, "token", os.Getenv("AZURE_FEDERATED_TOKEN_FILE"))
			require.Equal(t, "ca.pem", os.Getenv("SSL_CERT_FILE"))
			unchangedConfig, err := os.ReadFile(configPath)
			require.NoError(t, err)
			require.Equal(t, callerConfig, unchangedConfig)
			require.NoDirExists(t, indexDirectory)
			entries, err := os.ReadDir(temporary)
			require.NoError(t, err)
			require.Empty(t, entries)
			currentDirectory, err := os.Getwd()
			require.NoError(t, err)
			require.Equal(t, caller, currentDirectory)
			stdoutData, err := os.ReadFile(stdout.Name())
			require.NoError(t, err)
			require.Contains(t, string(stdoutData), "Bicep stdout")
			stderrData, err := os.ReadFile(stderr.Name())
			require.NoError(t, err)
			require.Contains(t, string(stderrData), "Bicep stderr")
		})
	}
}

func TestRunner_RunCancellation(t *testing.T) {
	_, temporary, record := setupPublishProcessTest(t)
	t.Setenv(publishHelperModeEnv, "wait")
	stdout, writer, err := os.Pipe()
	require.NoError(t, err)
	originalStdout := os.Stdout
	os.Stdout = writer
	t.Cleanup(func() {
		os.Stdout = originalStdout
		require.NoError(t, writer.Close())
		require.NoError(t, stdout.Close())
	})
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	runner := &Runner{
		Output:                           &output.MockOutput{},
		ResourceProviderManifestFilePath: "./provider.yaml",
		Target:                           "br:ghcr.io/example/extension:v1",
	}
	ready := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if scanner.Text() == "Bicep stdout" {
				close(ready)
				return
			}
		}
	}()
	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()
	select {
	case <-ready:
		cancel()
	case err := <-done:
		t.Fatalf("publishing exited before the subprocess was ready: %v", err)
	case <-ctx.Done():
		t.Fatal("Bicep helper did not start")
	}
	require.ErrorContains(t, <-done, "Failed to publish Bicep extension")
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	invocation := readPublishInvocation(t, record)
	require.NoDirExists(t, invocation.Directory)
	entries, err := os.ReadDir(temporary)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestPublishExtension_RelativeExecutableAndIndex(t *testing.T) {
	_, temporary, record := setupPublishProcessTest(t)
	executable, err := os.Executable()
	require.NoError(t, err)
	caller, err := os.Getwd()
	require.NoError(t, err)
	relativeExecutable, err := filepath.Rel(caller, executable)
	require.NoError(t, err)
	t.Setenv(bicep.BicepEnvVar, relativeExecutable)
	require.NoError(t, os.Mkdir("generated", 0700))
	require.NoError(t, os.WriteFile(filepath.Join("generated", "index.json"), []byte("{}"), 0600))
	require.NoError(t, publishExtension(t.Context(), "generated", "br:ghcr.io/example/extension:v1", false))
	invocation := readPublishInvocation(t, record)
	require.Equal(t, filepath.Join(caller, "generated"), invocation.Directory)
	require.Equal(t, filepath.Join(caller, "generated", "index.json"), invocation.Args[1])
	require.JSONEq(t, `{"experimentalFeaturesEnabled":{"ociEnabled":true}}`, invocation.Config)
	require.NoFileExists(t, filepath.Join(caller, "bicepconfig.json"))
	entries, err := os.ReadDir(temporary)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestRunner_RunInvalidConfig(t *testing.T) {
	caller, temporary, record := setupPublishProcessTest(t)
	configPath := filepath.Join(caller, "bicepconfig.json")
	config := []byte(`{"cloud": /* unfinished`)
	require.NoError(t, os.WriteFile(configPath, config, 0600))
	runner := &Runner{
		Output:                           &output.MockOutput{},
		ResourceProviderManifestFilePath: "./provider.yaml",
		Target:                           "br:ghcr.io/example/extension:v1",
	}
	require.ErrorContains(t, runner.Run(t.Context()), "unterminated JSON comment")
	require.NoFileExists(t, record)
	unchanged, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Equal(t, config, unchanged)
	entries, err := os.ReadDir(temporary)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestPreserveRegistryTransport(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		host     string
		preserve bool
	}{
		{host: "localhost", preserve: true},
		{host: "LOCALHOST:5000", preserve: true},
		{host: "127.0.0.1:5000", preserve: true},
		{host: "127.255.255.254:5000", preserve: true},
		{host: "[::1]:5000", preserve: true},
		{host: "[0:0:0:0:0:0:0:1]:5000", preserve: true},
		{host: "[::1%25lo0]:5000", preserve: true},
		{host: "[::ffff:127.0.0.1]:5000", preserve: true},
		{host: "127.1:5000", preserve: true},
		{host: "127.0.1:5000", preserve: true},
		{host: "2130706433:5000", preserve: true},
		{host: "0177.0.0.1:5000", preserve: true},
		{host: "0x7f000001:5000", preserve: true},
		{host: "0x7f.1:5000", preserve: true},
		{host: "127.16777216:5000", preserve: true},
		{host: "127.09:5000", preserve: true},
		{host: "3232235777:5000", preserve: true},
		{host: "ghcr.io"},
		{host: "radius-registry:5000"},
		{host: "example.azurecr.cn"},
		{host: "128.0.0.1:5000"},
		{host: "192.168.1.1:5000"},
		{host: "[::ffff:127.0.0.2]:5000"},
		{host: "[::]:5000"},
		{host: "localhost.:5000"},
		{host: "localhost.example:5000"},
		{host: "127.example.test"},
		{host: "0b1111111.0.0.1:5000"},
		{host: "127.1.:5000"},
		{host: "[invalid]:not-a-port"},
	} {
		t.Run(tt.host, func(t *testing.T) {
			require.Equal(t, tt.preserve, preserveRegistryTransport("br:"+tt.host+"/extension:v1"))
		})
	}
	require.False(t, preserveRegistryTransport("ts:localhost/extension:v1"))
	require.False(t, preserveRegistryTransport("./output.tgz"))
}

func TestWritePublishConfig(t *testing.T) {
	t.Parallel()
	for _, cache := range []string{"~/caller-cache", filepath.Join(t.TempDir(), "cache")} {
		t.Run(cache, func(t *testing.T) {
			root := t.TempDir()
			caller := filepath.Join(root, "nested", "caller")
			require.NoError(t, os.MkdirAll(caller, 0700))
			cacheJSON, err := json.Marshal(cache)
			require.NoError(t, err)
			preserved := map[string]json.RawMessage{
				"cloud":              json.RawMessage(`{"currentProfile":"custom","profiles":{"custom":{"resourceManagerEndpoint":"https://example.test/","activeDirectoryAuthority":"https://login.example.test/"}},"credentialPrecedence":["ManagedIdentity"],"credentialOptions":{"managedIdentity":{"type":"UserAssigned","clientId":"00000000-0000-0000-0000-000000000001"}}}`),
				"cacheRootDirectory": cacheJSON,
				"extensions":         json.RawMessage(`{"custom":"./custom.tgz","builtIn":"builtin:"}`),
				"moduleAliases":      json.RawMessage(`{"br":{"custom":{"registry":"example.azurecr.cn","modulePath":"modules"}}}`),
				"unknown":            json.RawMessage(`{"integer":9007199254740993123456789,"decimal":0.123456789012345678901,"text":"https://example.test/*keep*/ and \"//keep\" and \\\\"}`),
			}
			original, err := json.Marshal(preserved)
			require.NoError(t, err)
			original = append([]byte("\xef\xbb\xbf// caller configuration\r\n"), original...)
			original = bytes.Replace(original, []byte(`"cloud":`), []byte(`"cloud": /* preserve auth */`), 1)
			configPath := filepath.Join(root, "bicepconfig.json")
			require.NoError(t, os.WriteFile(configPath, original, 0600))
			destination := t.TempDir()
			require.NoError(t, writePublishConfig(caller, destination))
			published, err := os.ReadFile(filepath.Join(destination, "bicepconfig.json"))
			require.NoError(t, err)
			var actual map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(published, &actual))
			require.Len(t, actual, len(preserved)+1)
			for key, expected := range preserved {
				require.Equal(t, expected, actual[key], key)
			}
			require.JSONEq(t, `{"ociEnabled":true}`, string(actual["experimentalFeaturesEnabled"]))
			unchanged, err := os.ReadFile(configPath)
			require.NoError(t, err)
			require.Equal(t, original, unchanged)
		})
	}
}

func TestWritePublishConfig_NearestConfig(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	caller := filepath.Join(root, "nested")
	require.NoError(t, os.Mkdir(caller, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "bicepconfig.json"), []byte(`{"cloud":{"currentProfile":"AzureChinaCloud"}}`), 0600))
	nearest := []byte(`{"experimentalFeaturesEnabled":{"ociEnabled":false,"localDeploy":true}}`)
	require.NoError(t, os.WriteFile(filepath.Join(caller, "bicepconfig.json"), nearest, 0600))
	destination := t.TempDir()
	require.NoError(t, writePublishConfig(caller, destination))
	actual, err := os.ReadFile(filepath.Join(destination, "bicepconfig.json"))
	require.NoError(t, err)
	require.JSONEq(t, `{"experimentalFeaturesEnabled":{"ociEnabled":true,"localDeploy":true}}`, string(actual))
	unchanged, err := os.ReadFile(filepath.Join(caller, "bicepconfig.json"))
	require.NoError(t, err)
	require.Equal(t, nearest, unchanged)
}

func TestWritePublishConfig_Errors(t *testing.T) {
	t.Parallel()
	for _, config := range []string{
		``,
		`{"cloud": /* unterminated`,
		`{"cloud": { "currentProfile": "unfinished }}`,
		`{"cloud": {},}`,
		`{"cloud": {"credentialPrecedence":["AzureCLI",]}}`,
		`null`,
		`[]`,
		`{"experimentalFeaturesEnabled":null}`,
		`{"experimentalFeaturesEnabled":[]}`,
		`{"experimentalFeaturesEnabled":true}`,
	} {
		t.Run(config, func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(root, "bicepconfig.json"), []byte(config), 0600))
			destination := t.TempDir()
			require.Error(t, writePublishConfig(root, destination))
			require.NoFileExists(t, filepath.Join(destination, "bicepconfig.json"))
		})
	}
	t.Run("read failure", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(root, "bicepconfig.json"), 0700))
		require.ErrorContains(t, writePublishConfig(root, t.TempDir()), "failed to read Bicep configuration")
	})
	t.Run("write failure", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "bicepconfig.json"), []byte("{}"), 0600))
		require.ErrorContains(t, writePublishConfig(root, filepath.Join(root, "missing")), "failed to write the publishing configuration")
	})
}

func TestStripJSONComments(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		input string
		want  string
		valid bool
	}{
		{name: "line comments", input: "// before\r\n{\"value\": 1 // after\n}\n// end", want: `{"value":1}`, valid: true},
		{name: "block comments", input: "/* before */ { /*a\r\nb*/ \"value\": /**/ true } /* end */", want: `{"value":true}`, valid: true},
		{name: "strings and escapes", input: `{"url":"https://example.test/*keep*/","quote":"\"//keep\"","slashes":"\\\\","unicode":"\u002f\u002a"}`, want: `{"url":"https://example.test/*keep*/","quote":"\"//keep\"","slashes":"\\\\","unicode":"\u002f\u002a"}`, valid: true},
		{name: "BOM", input: "\xef\xbb\xbf/*comment*/{}", want: `{}`, valid: true},
		{name: "comments between tokens", input: `{"value":t/*comment*/rue}`},
		{name: "trailing comma", input: `{"value":true, /*comment*/}`},
		{name: "missing comma", input: `{"a":1 /*comment*/ "b":2}`},
		{name: "unclosed string", input: `{"value":"text //not a comment}`},
		{name: "multiple objects", input: `{} /*comment*/ {}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input := []byte(tt.input)
			actual, err := stripJSONComments(input)
			require.NoError(t, err)
			require.Equal(t, tt.input, string(input))
			require.Equal(t, tt.valid, json.Valid(actual), string(actual))
			if tt.valid {
				var compact bytes.Buffer
				require.NoError(t, json.Compact(&compact, actual))
				require.Equal(t, tt.want, compact.String())
			}
		})
	}
	for _, input := range []string{`/*`, `/*a`, `/**`, `/*/`, `{}` + "/* missing end\n"} {
		t.Run(input, func(t *testing.T) {
			_, err := stripJSONComments([]byte(input))
			require.ErrorContains(t, err, "unterminated JSON comment")
		})
	}
}

func setupPublishProcessTest(t *testing.T) (caller, temporary, record string) {
	t.Helper()
	manifest, err := os.ReadFile("testdata/valid.yaml")
	require.NoError(t, err)
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	caller = filepath.Join(root, "caller")
	temporary = filepath.Join(root, "temporary")
	require.NoError(t, os.Mkdir(caller, 0700))
	require.NoError(t, os.Mkdir(temporary, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "bicepconfig.json"), []byte("{}"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(caller, "provider.yaml"), manifest, 0600))
	t.Chdir(caller)
	t.Setenv("TMPDIR", temporary)
	t.Setenv("TMP", temporary)
	t.Setenv("TEMP", temporary)
	executable, err := os.Executable()
	require.NoError(t, err)
	t.Setenv(bicep.BicepEnvVar, executable)
	record = filepath.Join(caller, "invocation.json")
	t.Setenv(publishHelperEnv, record)
	t.Setenv(publishHelperModeEnv, "")
	return caller, temporary, record
}

func readPublishInvocation(t *testing.T, filename string) publishInvocation {
	t.Helper()
	data, err := os.ReadFile(filename)
	require.NoError(t, err)
	var invocation publishInvocation
	require.NoError(t, json.Unmarshal(data, &invocation))
	return invocation
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
