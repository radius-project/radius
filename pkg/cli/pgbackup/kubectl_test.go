/*
Copyright 2026 The Radius Authors.

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

package pgbackup

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/process"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

const (
	helperLogEnv    = "RADIUS_PGBACKUP_TEST_LOG"
	helperModeEnv   = "RADIUS_PGBACKUP_TEST_MODE"
	helperTargetEnv = "RADIUS_PGBACKUP_TEST_TARGET"
	helperPluginEnv = "RADIUS_PGBACKUP_TEST_PLUGIN"
	helperPolicyEnv = "RADIUS_PGBACKUP_TEST_POLICY"
	helperStderr    = "credential or kubectl diagnostic"
	testContext     = "selected"
	testNamespace   = "custom-namespace"
	testPod         = "postgres-selected-0"
)

type kubectlCall struct {
	Args          []string
	Input         []byte
	Kubeconfig    string
	ConsoleWindow bool
}

func TestMain(m *testing.M) {
	switch strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe") {
	case "kubectl":
		os.Exit(runKubectlHelper()) //nolint:forbidigo // Behave as the fake kubectl executable, without test-runner output.
	case "credential-plugin":
		if err := os.WriteFile(os.Getenv(helperPluginEnv), []byte("plugin ran"), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(9) //nolint:forbidigo // A sentinel plugin must never run during preflight.
	}
	os.Exit(m.Run()) //nolint:forbidigo // Return the test suite's exit status.
}

func runKubectlHelper() int {
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	call := kubectlCall{Args: os.Args[1:], Input: input, Kubeconfig: os.Getenv("KUBECONFIG"), ConsoleWindow: helperConsoleWindow()}
	log, err := os.OpenFile(os.Getenv(helperLogEnv), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	err = json.NewEncoder(log).Encode(call)
	closeErr := log.Close()
	if err != nil || closeErr != nil {
		fmt.Fprintln(os.Stderr, err, closeErr)
		return 1
	}
	operation := call.Args[4]
	if operation == "exec" {
		if call.Args[5] == "-i" {
			operation = "psql"
		} else {
			operation = "pg_dump"
		}
	}
	if operation == os.Getenv(helperTargetEnv) {
		switch os.Getenv(helperModeEnv) {
		case "fail":
			fmt.Fprint(os.Stderr, helperStderr)
			return 7
		case "wait":
			time.Sleep(time.Minute)
			return 1
		case "empty":
			return 0
		}
	}
	if operation == "get" {
		if os.Getenv(helperModeEnv) == "switch-auth" {
			config, err := clientcmd.LoadFromFile(os.Getenv("KUBECONFIG"))
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
			config.AuthInfos["selected-user"].Exec.InteractiveMode = api.AlwaysExecInteractiveMode
			if err := clientcmd.WriteToFile(*config, os.Getenv("KUBECONFIG")); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
		}
		fmt.Fprint(os.Stdout, " \n"+testPod+"\n")
	} else if operation == "pg_dump" {
		fmt.Fprint(os.Stdout, "-- dump "+call.Args[len(call.Args)-1]+"\n")
	}
	return 0
}

func execAuthConfig(mode api.ExecInteractiveMode, version string) api.Config {
	config := *api.NewConfig()
	config.CurrentContext = testContext
	config.Contexts[testContext] = &api.Context{Cluster: "cluster", AuthInfo: "selected-user"}
	config.Contexts["unused"] = &api.Context{Cluster: "cluster", AuthInfo: "unused-user"}
	config.Clusters["cluster"] = &api.Cluster{Server: "https://unused.invalid"}
	config.AuthInfos["selected-user"] = &api.AuthInfo{Exec: &api.ExecConfig{
		Command: "credential-plugin", APIVersion: version, InteractiveMode: mode,
	}}
	config.AuthInfos["unused-user"] = &api.AuthInfo{Exec: &api.ExecConfig{
		Command: "credential-plugin", APIVersion: "client.authentication.k8s.io/v1", InteractiveMode: api.AlwaysExecInteractiveMode,
	}}
	return config
}

func writeKubeconfig(t *testing.T, config api.Config) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	require.NoError(t, clientcmd.WriteToFile(config, path))
	return path
}

func installKubectlHelpers(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	executable, err := os.Executable()
	require.NoError(t, err)
	for _, name := range []string{"kubectl", "credential-plugin"} {
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		src, err := os.Open(executable)
		require.NoError(t, err)
		dst, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
		require.NoError(t, err)
		_, copyErr := io.Copy(dst, src)
		require.NoError(t, src.Close())
		require.NoError(t, dst.Close())
		require.NoError(t, copyErr)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(helperPluginEnv, filepath.Join(dir, "plugin-ran"))
}

func configureKubectlTest(t *testing.T, mode api.ExecInteractiveMode) string {
	t.Helper()
	configPath := writeKubeconfig(t, execAuthConfig(mode, "client.authentication.k8s.io/v1"))
	t.Setenv("KUBECONFIG", configPath)
	t.Setenv(helperLogEnv, filepath.Join(t.TempDir(), "calls.json"))
	t.Setenv(helperModeEnv, "")
	t.Setenv(helperTargetEnv, "")
	t.Cleanup(func() { require.NoFileExists(t, os.Getenv(helperPluginEnv)) })
	return configPath
}

func readKubectlCalls(t *testing.T) []kubectlCall {
	t.Helper()
	file, err := os.Open(os.Getenv(helperLogEnv))
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	defer file.Close()
	var calls []kubectlCall
	decoder := json.NewDecoder(file)
	for {
		var call kubectlCall
		err := decoder.Decode(&call)
		if err == io.EOF {
			return calls
		}
		require.NoError(t, err)
		calls = append(calls, call)
	}
}

func testSQL(db string) []byte {
	return []byte("-- " + db + "\n" + strings.Repeat("SELECT 'finite input';\r\n", 8192) + "\x00\n")
}

func runKubectlPath(ctx context.Context, operation, kubeContext, stateDir string) error {
	switch operation {
	case "get":
		_, err := getPodName(ctx, kubeContext, testNamespace)
		return err
	case "wait":
		return WaitForReady(ctx, kubeContext, testNamespace)
	case "pg_dump":
		return Backup(ctx, kubeContext, testNamespace, stateDir)
	default:
		return Restore(ctx, kubeContext, testNamespace, stateDir)
	}
}

func prepareDumps(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, db := range Databases {
		require.NoError(t, os.WriteFile(filepath.Join(dir, db+".sql"), testSQL(db), 0o600))
	}
	return dir
}

func testKubectl(t *testing.T) {
	switch os.Getenv(helperPolicyEnv) {
	case "windowless":
		require.True(t, process.IsWindowless(), "must exercise the live no-console policy")
	case "attached":
		require.False(t, process.IsWindowless(), "must exercise a real attached console")
	}
	if runtime.GOOS != "windows" {
		require.False(t, process.IsWindowless())
	}
	installKubectlHelpers(t)

	for _, mode := range []api.ExecInteractiveMode{api.NeverExecInteractiveMode, api.IfAvailableExecInteractiveMode, api.AlwaysExecInteractiveMode} {
		for _, operation := range []string{"get", "wait", "pg_dump", "psql"} {
			t.Run(string(mode)+"/"+operation, func(t *testing.T) {
				configPath := configureKubectlTest(t, mode)
				before, err := os.ReadFile(configPath)
				require.NoError(t, err)
				stdin := os.Stdin
				stateDir := prepareDumps(t)
				ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
				defer cancel()
				err = runKubectlPath(ctx, operation, testContext, stateDir)
				if mode == api.AlwaysExecInteractiveMode && process.IsWindowless() {
					require.ErrorContains(t, err, "interactiveMode: Always")
					require.ErrorContains(t, err, "non-interactive Kubernetes credentials")
					require.Empty(t, readKubectlCalls(t), "preflight must reject before any kubectl launch")
				} else {
					require.NoError(t, err)
					assertKubectlCalls(t, operation, testContext)
					if operation == "pg_dump" {
						for _, db := range Databases {
							dump, err := os.ReadFile(filepath.Join(stateDir, db+".sql"))
							require.NoError(t, err)
							require.Equal(t, "-- dump "+db+"\n", string(dump))
						}
					}
				}
				after, err := os.ReadFile(configPath)
				require.NoError(t, err)
				require.Equal(t, before, after, "preflight must not change kubeconfig")
				require.Same(t, stdin, os.Stdin)
				require.Equal(t, configPath, os.Getenv("KUBECONFIG"))
			})
		}
	}

	t.Run("exec paths recheck authentication after lookup", func(t *testing.T) {
		for _, operation := range []string{"pg_dump", "psql"} {
			t.Run(operation, func(t *testing.T) {
				configureKubectlTest(t, api.NeverExecInteractiveMode)
				t.Setenv(helperModeEnv, "switch-auth")
				ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
				defer cancel()
				err := runKubectlPath(ctx, operation, testContext, prepareDumps(t))
				if process.IsWindowless() {
					require.ErrorContains(t, err, "interactiveMode: Always")
					calls := readKubectlCalls(t)
					require.Len(t, calls, 1)
					require.Equal(t, "get", calls[0].Args[4])
				} else {
					require.NoError(t, err)
					assertKubectlCalls(t, operation, testContext)
				}
			})
		}
	})

	t.Run("merged config reaches every command", func(t *testing.T) {
		for _, operation := range []string{"get", "wait", "pg_dump", "psql"} {
			t.Run(operation, func(t *testing.T) {
				configureKubectlTest(t, api.NeverExecInteractiveMode)
				t.Setenv("KUBECONFIG", mergedKubeconfig(t, api.NeverExecInteractiveMode))
				ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
				defer cancel()
				require.NoError(t, runKubectlPath(ctx, operation, "", prepareDumps(t)))
				assertKubectlCalls(t, operation, "")
			})
		}
	})

	t.Run("broken config is inspected only without a console", func(t *testing.T) {
		path := configureKubectlTest(t, api.NeverExecInteractiveMode)
		require.NoError(t, os.WriteFile(path, []byte("not: [valid yaml"), 0o600))
		ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		defer cancel()
		err := WaitForReady(ctx, testContext, testNamespace)
		if process.IsWindowless() {
			require.ErrorContains(t, err, "failed to load kubectl configuration")
			require.NotContains(t, err.Error(), "timed out")
			require.Empty(t, readKubectlCalls(t))
		} else {
			require.NoError(t, err)
			assertKubectlCalls(t, "wait", testContext)
		}
	})

	t.Run("no backup remains a no-op", func(t *testing.T) {
		configureKubectlTest(t, api.AlwaysExecInteractiveMode)
		require.NoError(t, Restore(t.Context(), testContext, testNamespace, t.TempDir()))
		require.Empty(t, readKubectlCalls(t))
	})

	t.Run("empty pod selection", func(t *testing.T) {
		configureKubectlTest(t, api.NeverExecInteractiveMode)
		t.Setenv(helperModeEnv, "empty")
		t.Setenv(helperTargetEnv, "get")
		ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		defer cancel()
		_, err := getPodName(ctx, testContext, testNamespace)
		require.ErrorContains(t, err, "no PostgreSQL pod found")
	})

	for _, operation := range []string{"get", "wait", "pg_dump", "psql"} {
		t.Run(operation+"/ordinary failure", func(t *testing.T) {
			configureKubectlTest(t, api.NeverExecInteractiveMode)
			t.Setenv(helperModeEnv, "fail")
			t.Setenv(helperTargetEnv, operation)
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			err := runKubectlPath(ctx, operation, testContext, prepareDumps(t))
			require.ErrorContains(t, err, helperStderr)
			require.NotContains(t, err.Error(), "timed out")
			var exitErr *exec.ExitError
			require.ErrorAs(t, err, &exitErr)
			require.Equal(t, 7, exitErr.ExitCode())
			require.NotErrorIs(t, err, context.Canceled)
		})
		t.Run(operation+"/canceled before launch", func(t *testing.T) {
			for _, config := range []string{"Never", "Always", "malformed"} {
				t.Run(config, func(t *testing.T) {
					path := configureKubectlTest(t, api.ExecInteractiveMode(config))
					if config == "malformed" {
						require.NoError(t, os.WriteFile(path, []byte("not: [valid yaml"), 0o600))
					}
					ctx, cancel := context.WithCancel(t.Context())
					cancel()
					err := runKubectlPath(ctx, operation, testContext, prepareDumps(t))
					require.ErrorIs(t, err, context.Canceled)
					require.Empty(t, readKubectlCalls(t))
				})
			}
		})
		t.Run(operation+"/cancel running child", func(t *testing.T) {
			configureKubectlTest(t, api.NeverExecInteractiveMode)
			t.Setenv(helperModeEnv, "wait")
			t.Setenv(helperTargetEnv, operation)
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			stateDir := prepareDumps(t)
			done := make(chan error, 1)
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				done <- runKubectlPath(ctx, operation, testContext, stateDir)
			}()
			t.Cleanup(func() {
				cancel()
				<-finished
			})
			// Wait for the child to finish reading stdin before exercising cancellation.
			require.Eventually(t, func() bool {
				data, err := os.ReadFile(os.Getenv(helperLogEnv))
				if err != nil {
					return false
				}
				return strings.Contains(string(data), `"`+operation+`"`) && len(data) > 0 && data[len(data)-1] == '\n'
			}, 10*time.Second, 10*time.Millisecond)
			cancel()
			err := <-done
			require.Error(t, err)
			require.ErrorIs(t, ctx.Err(), context.Canceled)
			require.NotContains(t, err.Error(), "timed out")
			var exitErr *exec.ExitError
			require.ErrorAs(t, err, &exitErr)
		})
	}

	t.Run("canceled during preflight", func(t *testing.T) {
		if !process.IsWindowless() {
			t.Skip("preflight only runs without a Windows console")
		}
		for _, config := range []string{"Always", "malformed"} {
			t.Run(config, func(t *testing.T) {
				path := configureKubectlTest(t, api.ExecInteractiveMode(config))
				if config == "malformed" {
					require.NoError(t, os.WriteFile(path, []byte("not: [valid yaml"), 0o600))
				}
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				err := WaitForReady(cancelAfterCheckContext{Context: ctx, cancel: cancel}, testContext, testNamespace)
				require.ErrorIs(t, err, context.Canceled)
				require.Empty(t, readKubectlCalls(t))
			})
		}
	})
}

type cancelAfterCheckContext struct {
	context.Context
	cancel context.CancelFunc
}

func (ctx cancelAfterCheckContext) Err() error {
	err := ctx.Context.Err()
	// Cancel immediately after sampling the initial state, so validation's error
	// competes with real caller cancellation without timing-dependent sleeps.
	ctx.cancel()
	return err
}

func assertKubectlCalls(t *testing.T, operation, kubeContext string) {
	t.Helper()
	prefix := []string{"--context", kubeContext, "-n", testNamespace}
	lookup := append(append([]string{}, prefix...), "get", "pods", "-l", PodLabelSelector, "-o", "jsonpath={.items[0].metadata.name}")
	var want [][]string
	switch operation {
	case "get":
		want = append(want, lookup)
	case "wait":
		want = append(want, append(append([]string{}, prefix...), "wait", "--for=condition=ready", "pod", "-l", PodLabelSelector, "--timeout=120s"))
	default:
		want = append(want, lookup)
		for _, db := range Databases {
			args := append([]string{}, prefix...)
			if operation == "pg_dump" {
				args = append(args, "exec", testPod, "--", "pg_dump", "-U", PostgresUser, "--format=plain", "--clean", "--if-exists", db)
			} else {
				args = append(args, "exec", "-i", testPod, "--", "psql", "-U", PostgresUser, "-d", db)
			}
			want = append(want, args)
		}
	}
	calls := readKubectlCalls(t)
	require.Len(t, calls, len(want))
	for i, call := range calls {
		require.Equal(t, want[i], call.Args)
		require.Equal(t, os.Getenv("KUBECONFIG"), call.Kubeconfig)
		if process.IsWindowless() {
			require.False(t, call.ConsoleWindow, "the child must not create a console window")
		}
		if operation == "psql" && i > 0 {
			require.Equal(t, testSQL(Databases[i-1]), call.Input, "all SQL bytes must arrive before EOF")
		} else {
			require.Empty(t, call.Input, "commands without input must reach EOF")
		}
	}
}
