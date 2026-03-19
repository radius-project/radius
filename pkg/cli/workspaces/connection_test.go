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

// workspaces contains functionality for using the workspace concept of the CLI to connect and interact
// with the remote endpoints that are described by the workspace concept
// (Radius control plane, environment, et al).
package workspaces

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_IsSameKubernetesContext(t *testing.T) {
	ctx := "kind-kind"
	ws := Workspace{
		Name: "my_workspace",
		Connection: map[string]any{
			"kind":    "kubernetes",
			"context": "kind-kind",
		},
	}

	isSame := ws.IsSameKubernetesContext(ctx)
	require.Equal(t, isSame, true)

	ctx = "aks1"
	isSame = ws.IsSameKubernetesContext(ctx)
	require.Equal(t, isSame, false)
}

func Test_ConnectionConfig_GitHub(t *testing.T) {
	ws := Workspace{
		Name: "github-workspace",
		Connection: map[string]any{
			"kind":    KindGitHub,
			"context": "k3d-radius-github",
		},
	}

	cfg, err := ws.ConnectionConfig()
	require.NoError(t, err)

	ghCfg, ok := cfg.(*GitHubConnectionConfig)
	require.True(t, ok, "expected *GitHubConnectionConfig, got %T", cfg)
	require.Equal(t, KindGitHub, ghCfg.Kind)
	require.Equal(t, "k3d-radius-github", ghCfg.Context)
}

func Test_KubernetesContext_GitHub(t *testing.T) {
	ws := Workspace{
		Name: "github-workspace",
		Connection: map[string]any{
			"kind":    KindGitHub,
			"context": "k3d-radius-github",
		},
	}

	got, ok := ws.KubernetesContext()
	require.True(t, ok)
	require.Equal(t, "k3d-radius-github", got)
}

func Test_ConnectionConfigEquals_GitHub(t *testing.T) {
	ws := Workspace{
		Connection: map[string]any{
			"kind":    KindGitHub,
			"context": "k3d-radius-github",
		},
	}

	same := &GitHubConnectionConfig{Kind: KindGitHub, Context: "k3d-radius-github"}
	different := &GitHubConnectionConfig{Kind: KindGitHub, Context: "k3d-other"}
	kubernetes := &KubernetesConnectionConfig{Kind: KindKubernetes, Context: "k3d-radius-github"}

	require.True(t, ws.ConnectionConfigEquals(same))
	require.False(t, ws.ConnectionConfigEquals(different))
	require.False(t, ws.ConnectionConfigEquals(kubernetes))
}
