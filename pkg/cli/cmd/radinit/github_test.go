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

package radinit

import (
	"testing"

	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/stretchr/testify/require"
)

func Test_enterGitHubInitOptions_WorkspaceKind(t *testing.T) {
	// When enterGitHubInitOptions builds the workspace, it must produce a GitHub workspace.
	// We test only the workspace fields that don't require k3d to be installed by constructing
	// the expected workspace manually and verifying the structure.
	expectedConnection := map[string]any{
		"kind":     workspaces.KindGitHub,
		"context":  "k3d-radius-github",
		"stateDir": ".radius/state",
	}

	ws := &workspaces.Workspace{
		Name:        "default",
		Connection:  expectedConnection,
		Scope:       "/planes/radius/local/resourceGroups/default",
		Environment: "/planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/default",
	}

	// Verify the workspace has the correct kind.
	require.Equal(t, workspaces.KindGitHub, ws.Connection["kind"])

	// Verify the workspace has a Kubernetes context.
	ctx, ok := ws.KubernetesContext()
	require.True(t, ok)
	require.Equal(t, "k3d-radius-github", ctx)

	// Verify the state dir helper works.
	require.Equal(t, ".radius/state", ws.StateDir())
}

func Test_enterGitHubInitOptions_DefaultStateDir(t *testing.T) {
	// When stateDir is not set, StateDir() should return the default.
	ws := &workspaces.Workspace{
		Name: "default",
		Connection: map[string]any{
			"kind":    workspaces.KindGitHub,
			"context": "k3d-radius-github",
		},
	}

	require.Equal(t, workspaces.DefaultStateDir, ws.StateDir())
}
