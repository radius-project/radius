// ------------------------------------------------------------
// Copyright 2023 The Radius Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// ------------------------------------------------------------

package workspaces

import (
	"fmt"

	"github.com/radius-project/radius/pkg/sdk"
)

// KindGit is the connection kind for Git workspace mode.
const KindGit string = "git"

// GitWorkspaceName is the reserved name for the built-in Git workspace.
const GitWorkspaceName string = "git"

// GitConnectionConfig represents a Git workspace connection.
// Git workspace mode operates on the current directory's Git repository
// without requiring a connection to a control plane.
type GitConnectionConfig struct {
	Kind string `mapstructure:"kind"`
}

// String returns a display string for the Git workspace connection.
func (c *GitConnectionConfig) String() string {
	return "Git workspace (local repository)"
}

// GetKind returns the connection kind.
func (c *GitConnectionConfig) GetKind() string {
	return KindGit
}

// Connect returns an error because Git workspace mode doesn't use SDK connections.
// Git workspace operations are local and don't connect to a remote control plane.
func (c *GitConnectionConfig) Connect() (sdk.Connection, error) {
	return nil, fmt.Errorf("Git workspace does not use SDK connections; use local Git operations instead")
}

// MakeGitWorkspace creates the built-in Git workspace.
// This workspace is always available and operates on the current Git repository.
func MakeGitWorkspace() *Workspace {
	return &Workspace{
		Source: SourceUserConfig,
		Name:   GitWorkspaceName,
		Connection: map[string]any{
			"kind": KindGit,
		},
	}
}

// IsGitWorkspace returns true if the workspace is a Git workspace.
func (ws Workspace) IsGitWorkspace() bool {
	obj, ok := ws.Connection["kind"]
	if !ok {
		return false
	}

	kind, ok := obj.(string)
	if !ok {
		return false
	}

	return kind == KindGit
}

// IsBuiltIn returns true if the workspace is a built-in workspace.
// Currently, only the Git workspace is built-in.
func (ws Workspace) IsBuiltIn() bool {
	return ws.Name == GitWorkspaceName && ws.IsGitWorkspace()
}
