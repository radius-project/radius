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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

func TestValidateExecAuth(t *testing.T) {
	installKubectlHelpers(t)
	for _, tt := range []struct {
		name    string
		mode    api.ExecInteractiveMode
		version string
		wantErr string
	}{
		{name: "never", mode: api.NeverExecInteractiveMode, version: "v1"},
		{name: "if available", mode: api.IfAvailableExecInteractiveMode, version: "v1"},
		{name: "always", mode: api.AlwaysExecInteractiveMode, version: "v1", wantErr: "interactiveMode: Always"},
		{name: "v1 requires mode", version: "v1", wantErr: "interactiveMode must be specified"},
		{name: "v1beta1 defaults mode", version: "v1beta1"},
		{name: "invalid mode", mode: "sometimes", version: "v1beta1", wantErr: "invalid interactiveMode"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := writeKubeconfig(t, execAuthConfig(tt.mode, "client.authentication.k8s.io/"+tt.version))
			t.Setenv("KUBECONFIG", path)
			err := validateExecAuth(testContext)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.NoFileExists(t, os.Getenv(helperPluginEnv))
		})
	}

	for _, tt := range []struct {
		name    string
		modify  func(*api.Config)
		context string
		wantErr string
	}{
		{name: "no exec auth", modify: func(c *api.Config) { c.AuthInfos["selected-user"].Exec = nil }},
		{name: "unused invalid exec", modify: func(c *api.Config) { c.AuthInfos["unused-user"].Exec.InteractiveMode = "invalid" }},
		{name: "explicit context overrides current", context: testContext, modify: func(c *api.Config) { c.CurrentContext = "unused" }},
		{name: "explicit context ignores missing current", context: testContext, modify: func(c *api.Config) { c.CurrentContext = "missing" }},
		{name: "explicit context without exec ignores missing current", context: testContext, modify: func(c *api.Config) {
			c.CurrentContext = "missing"
			c.AuthInfos["selected-user"].Exec = nil
		}},
		{name: "current context is used", modify: func(c *api.Config) { c.CurrentContext = "unused" }, wantErr: "interactiveMode: Always"},
		{name: "missing context", context: "missing", wantErr: "context"},
		{name: "missing current context", modify: func(c *api.Config) { c.CurrentContext = "missing" }, wantErr: "context"},
		{name: "missing command", modify: func(c *api.Config) { c.AuthInfos["selected-user"].Exec.Command = "" }, wantErr: "command must be specified"},
		{name: "missing API version", modify: func(c *api.Config) { c.AuthInfos["selected-user"].Exec.APIVersion = "" }, wantErr: "apiVersion must be specified"},
		{name: "conflicting auth", modify: func(c *api.Config) {
			c.AuthInfos["selected-user"].AuthProvider = &api.AuthProviderConfig{Name: "unused"}
		}, wantErr: "authProvider cannot be provided"},
		{name: "relative missing CA", modify: func(c *api.Config) { c.Clusters["cluster"].CertificateAuthority = "missing-ca" }, wantErr: "unable to read certificate-authority"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			config := execAuthConfig(api.NeverExecInteractiveMode, "client.authentication.k8s.io/v1")
			if tt.modify != nil {
				tt.modify(&config)
			}
			t.Setenv("KUBECONFIG", writeKubeconfig(t, config))
			err := validateExecAuth(tt.context)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.NoFileExists(t, os.Getenv(helperPluginEnv))
		})
	}
}

func mergedKubeconfig(t *testing.T, mode api.ExecInteractiveMode) string {
	t.Helper()
	first := *api.NewConfig()
	first.CurrentContext = testContext
	first.Contexts[testContext] = &api.Context{Cluster: "cluster", AuthInfo: "selected-user"}
	first.AuthInfos["selected-user"] = &api.AuthInfo{Exec: &api.ExecConfig{
		Command: "credential-plugin", APIVersion: "client.authentication.k8s.io/v1", InteractiveMode: mode,
	}}
	second := execAuthConfig(api.AlwaysExecInteractiveMode, "client.authentication.k8s.io/v1")
	second.CurrentContext = "unused"
	second.Contexts[testContext] = &api.Context{Cluster: "cluster", AuthInfo: "unused-user"}
	firstPath := writeKubeconfig(t, first)
	return strings.Join([]string{firstPath, "", filepath.Join(t.TempDir(), "missing"), writeKubeconfig(t, second), firstPath}, string(os.PathListSeparator))
}

func TestValidateExecAuth_Merging(t *testing.T) {
	installKubectlHelpers(t)
	for _, mode := range []api.ExecInteractiveMode{api.NeverExecInteractiveMode, api.AlwaysExecInteractiveMode} {
		t.Run(string(mode), func(t *testing.T) {
			t.Setenv("KUBECONFIG", mergedKubeconfig(t, mode))
			for _, kubeContext := range []string{"", testContext} {
				err := validateExecAuth(kubeContext)
				if mode == api.AlwaysExecInteractiveMode {
					require.ErrorContains(t, err, "interactiveMode: Always")
				} else {
					require.NoError(t, err, "first user and context win; cluster comes from the second file")
				}
			}
			require.NoFileExists(t, os.Getenv(helperPluginEnv))
		})
	}
	t.Run("bad second file is not ignored", func(t *testing.T) {
		good := writeKubeconfig(t, execAuthConfig(api.NeverExecInteractiveMode, "client.authentication.k8s.io/v1"))
		bad := filepath.Join(t.TempDir(), "bad")
		require.NoError(t, os.WriteFile(bad, []byte("not: [valid yaml"), 0o600))
		t.Setenv("KUBECONFIG", good+string(os.PathListSeparator)+bad)
		require.ErrorContains(t, validateExecAuth(testContext), "failed to load kubectl configuration")
	})
}

func TestValidateExecAuth_DefaultHome(t *testing.T) {
	const helperEnv = "RADIUS_PGBACKUP_TEST_HOME"
	if os.Getenv(helperEnv) != "" {
		require.Equal(t, filepath.Join(os.Getenv(helperEnv), ".kube", "config"), clientcmd.RecommendedHomeFile)
		require.ErrorContains(t, validateExecAuth(""), "interactiveMode: Always")
		return
	}
	home := t.TempDir()
	config := execAuthConfig(api.AlwaysExecInteractiveMode, "client.authentication.k8s.io/v1")
	require.NoError(t, clientcmd.WriteToFile(config, filepath.Join(home, ".kube", "config")))
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestValidateExecAuth_DefaultHome$")
	cmd.Env = append(os.Environ(), helperEnv+"="+home, "HOME="+home, "USERPROFILE="+home, "KUBECONFIG=")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}
