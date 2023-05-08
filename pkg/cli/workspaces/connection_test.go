/*
------------------------------------------------------------
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
------------------------------------------------------------
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
