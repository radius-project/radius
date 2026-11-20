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

package kubernetes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/kubeutil"
	"github.com/radius-project/radius/test/k8sutil"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
)

func Test_NewCLIClientConfig_KubeconfigLoadingRules(t *testing.T) {
	const (
		homeServer       = "https://home.example.com"
		homeSharedServer = "https://home-shared.example.com"
		envServer        = "https://env.example.com"
		envSharedServer  = "https://env-shared.example.com"
		extraServer      = "https://extra.example.com"
		extraEnvServer   = "https://extra-env.example.com"
		partialServer    = "https://partial.example.com"
	)

	tests := []struct {
		name string
		// home selects the file at the default kubeconfig path: "" for a valid file, or "missing", "malformed",
		// "empty", or "no-current-context".
		home string
		// kubeconfig lists the files in KUBECONFIG by key. A nil value leaves KUBECONFIG unset.
		kubeconfig  []string
		contextName string

		wantCurrentContext string
		wantHost           string
		// wantLoadErr is the expected loadCLIConfig error. wantErr is the expected NewCLIClientConfig error.
		wantLoadErr string
		wantErr     string
		// wantErrPath is the key of a file whose path must appear in each expected error.
		wantErrPath string
	}{
		{
			name:               "default kubeconfig when KUBECONFIG is unset",
			wantCurrentContext: "home-context",
			wantHost:           homeServer,
		},
		{
			name:               "KUBECONFIG replaces the default kubeconfig",
			kubeconfig:         []string{"env"},
			wantCurrentContext: "env-context",
			wantHost:           envServer,
		},
		{
			name:               "context with the same name in both files resolves from KUBECONFIG",
			kubeconfig:         []string{"env"},
			contextName:        "shared",
			wantCurrentContext: "env-context",
			wantHost:           envSharedServer,
		},
		{
			name:               "context only in the default kubeconfig is not found when KUBECONFIG is set",
			kubeconfig:         []string{"env"},
			contextName:        "home-context",
			wantCurrentContext: "env-context",
			wantErr:            `context "home-context" does not exist`,
		},
		{
			name:               "merged files use the first current context and the first context definition",
			kubeconfig:         []string{"env", "extra"},
			wantCurrentContext: "env-context",
			wantHost:           envServer,
		},
		{
			name:               "explicit context selects a context from a later merged file",
			kubeconfig:         []string{"env", "extra"},
			contextName:        "extra-context",
			wantCurrentContext: "env-context",
			wantHost:           extraServer,
		},
		{
			name:               "merged partial files resolve a context from one file and its cluster from another",
			kubeconfig:         []string{"partial-context", "partial-cluster"},
			wantCurrentContext: "partial",
			wantHost:           partialServer,
		},
		{
			name:               "missing files in KUBECONFIG are skipped",
			kubeconfig:         []string{"missing", "extra"},
			wantCurrentContext: "extra-context",
			wantHost:           extraServer,
		},
		{
			name:        "malformed file in KUBECONFIG is an error that names the file",
			kubeconfig:  []string{"env", "malformed"},
			wantLoadErr: "error loading config file",
			wantErr:     "error loading config file",
			wantErrPath: "malformed",
		},
		{
			name:        "malformed default kubeconfig is an error that names the file",
			home:        "malformed",
			wantLoadErr: "error loading config file",
			wantErr:     "error loading config file",
			wantErrPath: "home",
		},
		{
			name:        "KUBECONFIG with only missing files does not fall back to the default kubeconfig",
			kubeconfig:  []string{"missing"},
			wantLoadErr: "no usable kubeconfig found",
			wantErr:     "no usable kubeconfig found",
			wantErrPath: "missing",
		},
		{
			name:        "no kubeconfig files",
			home:        "missing",
			wantLoadErr: "no usable kubeconfig found",
			wantErr:     "no usable kubeconfig found",
			wantErrPath: "home",
		},
		{
			name:        "empty default kubeconfig",
			home:        "empty",
			wantLoadErr: "no usable kubeconfig found",
			wantErr:     "no usable kubeconfig found",
			wantErrPath: "home",
		},
		{
			name:               "named context that does not exist is an error",
			contextName:        "not-present",
			wantCurrentContext: "home-context",
			wantErr:            `context "not-present" does not exist`,
		},
		{
			name:               "kubeconfig without a current context keeps the client-go error",
			home:               "no-current-context",
			wantCurrentContext: "",
			wantErr:            "invalid configuration: no configuration has been provided",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			paths := map[string]string{
				"home":            k8sutil.IsolateKubeconfig(t),
				"env":             filepath.Join(dir, "env.yaml"),
				"extra":           filepath.Join(dir, "extra.yaml"),
				"partial-context": filepath.Join(dir, "partial-context.yaml"),
				"partial-cluster": filepath.Join(dir, "partial-cluster.yaml"),
				"malformed":       filepath.Join(dir, "malformed.yaml"),
				"missing":         filepath.Join(dir, "missing.yaml"),
			}

			switch tc.home {
			case "":
				k8sutil.WriteKubeconfig(t, paths["home"], "home-context", map[string]string{
					"home-context": homeServer,
					"shared":       homeSharedServer,
				})
			case "malformed":
				writeKubeconfigTestFile(t, paths["home"], "this is: [not valid yaml")
			case "empty":
				writeKubeconfigTestFile(t, paths["home"], "")
			case "no-current-context":
				k8sutil.WriteKubeconfig(t, paths["home"], "", map[string]string{"home-context": homeServer})
			case "missing":
			default:
				t.Fatalf("unknown home kubeconfig %q", tc.home)
			}

			k8sutil.WriteKubeconfig(t, paths["env"], "env-context", map[string]string{
				"env-context": envServer,
				"shared":      envSharedServer,
			})
			k8sutil.WriteKubeconfig(t, paths["extra"], "extra-context", map[string]string{
				"extra-context": extraServer,
				"env-context":   extraEnvServer,
			})
			writeKubeconfigTestFile(t, paths["partial-context"], `apiVersion: v1
kind: Config
current-context: partial
contexts:
- name: partial
  context: {cluster: partial-cluster, user: partial-user}
`)
			writeKubeconfigTestFile(t, paths["partial-cluster"], `apiVersion: v1
kind: Config
clusters:
- name: partial-cluster
  cluster: {server: `+partialServer+`}
users:
- name: partial-user
  user: {}
`)
			writeKubeconfigTestFile(t, paths["malformed"], "this is: [not valid yaml")

			if tc.kubeconfig != nil {
				files := make([]string, 0, len(tc.kubeconfig))
				for _, key := range tc.kubeconfig {
					files = append(files, paths[key])
				}
				t.Setenv(clientcmd.RecommendedConfigPathEnvVar, strings.Join(files, string(filepath.ListSeparator)))
			}

			var wantErrPath string
			if tc.wantErrPath != "" {
				var ok bool
				wantErrPath, ok = paths[tc.wantErrPath]
				require.Truef(t, ok, "unknown wantErrPath %q", tc.wantErrPath)
			}

			raw, err := loadCLIConfig()
			if tc.wantLoadErr != "" {
				require.ErrorContains(t, err, tc.wantLoadErr)
				require.NotEmpty(t, wantErrPath, "a load error must name the file it came from")
				require.ErrorContains(t, err, wantErrPath)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.wantCurrentContext, raw.CurrentContext)
			}

			config, err := NewCLIClientConfig(tc.contextName)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				if wantErrPath != "" {
					require.ErrorContains(t, err, wantErrPath)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantHost, config.Host)
		})
	}
}

func Test_NewCLIClientConfig_QPSAndBurst(t *testing.T) {
	k8sutil.WriteKubeconfig(t, k8sutil.IsolateKubeconfig(t), "home-context", map[string]string{
		"home-context": "https://home.example.com",
	})

	t.Run("CLI defaults", func(t *testing.T) {
		t.Setenv("RADIUS_QPS_AND_BURST", "")
		config, err := NewCLIClientConfig("")
		require.NoError(t, err)
		require.Equal(t, kubeutil.DefaultCLIQPS, config.QPS)
		require.Equal(t, kubeutil.DefaultCLIBurst, config.Burst)
	})

	t.Run("environment variable override", func(t *testing.T) {
		t.Setenv("RADIUS_QPS_AND_BURST", "800")
		config, err := NewCLIClientConfig("")
		require.NoError(t, err)
		require.Equal(t, float32(800), config.QPS)
		require.Equal(t, 800, config.Burst)
	})
}

func writeKubeconfigTestFile(t *testing.T, path string, contents string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, os.WriteFile(path, []byte(contents), 0600))
}
