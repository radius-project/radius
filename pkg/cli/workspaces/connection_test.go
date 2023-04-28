// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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
