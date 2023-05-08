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

package cli

import (
	"bytes"
	"testing"

	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func Test_ReadWorkspaceSection_NoContent(t *testing.T) {
	var yaml = ``

	v, err := makeConfig(yaml)
	require.NoError(t, err)

	es, err := ReadWorkspaceSection(v)
	require.NoError(t, err)
	require.Empty(t, es.Default)
	require.Empty(t, es.Items)
}

func Test_ReadWorkspaceSection_SomeItems(t *testing.T) {
	var yaml = `
workspaces:
  default: test
  items:
    test:
      connection:
        kind: kubernetes
      scope: /a/b/c
    test2:
      connection:
        kind: kubernetes
      scope: /a/b/c
`

	v, err := makeConfig(yaml)
	require.NoError(t, err)

	es, err := ReadWorkspaceSection(v)
	require.NoError(t, err)
	require.Equal(t, "test", es.Default)
	require.Len(t, es.Items, 2)
}

func Test_ReadWorkspaceSection_Invalid_NoConnection(t *testing.T) {
	var yaml = `
workspaces:
  items:
    test:
      scope: /a/b/c
`

	v, err := makeConfig(yaml)
	require.NoError(t, err)

	_, err = ReadWorkspaceSection(v)
	require.Error(t, err)
}

func Test_GetWorkspace_Nil_NoDefault(t *testing.T) {
	var yaml = `
workspaces:
  items:
    test:
      connection:
        kind: kubernetes
      scope: /a/b/c
`

	v, err := makeConfig(yaml)
	require.NoError(t, err)

	section, err := ReadWorkspaceSection(v)
	require.NoError(t, err)

	ws, err := section.GetWorkspace("")
	require.Nil(t, ws)
	require.NoError(t, err)
}

func Test_GetWorkspace_Invalid_NotFound(t *testing.T) {
	var yaml = `
workspaces:
  default: test2
  items:
    test:
      connection:
        kind: kubernetes
      scope: /a/b/c
`

	v, err := makeConfig(yaml)
	require.NoError(t, err)

	es, err := ReadWorkspaceSection(v)
	require.NoError(t, err)

	_, err = es.GetWorkspace("test2")
	require.Error(t, err)
}

func Test_GetWorkspace_Default_Valid(t *testing.T) {
	var yaml = `
workspaces:
  default: test
  items:
    test:
      connection:
        kind: kubernetes
        context: cool-beans
      scope: /a/b/c
      environment: /a/b/c/providers/Applications.Core/environments/ice-cold
`

	v, err := makeConfig(yaml)
	require.NoError(t, err)

	es, err := ReadWorkspaceSection(v)
	require.NoError(t, err)

	ws, err := es.GetWorkspace("")
	require.NoError(t, err)

	require.Equal(t, "test", ws.Name)
	require.Equal(t, workspaces.Source(workspaces.SourceUserConfig), ws.Source)
	require.Equal(t, "/a/b/c", ws.Scope)
	require.Equal(t, "/a/b/c/providers/Applications.Core/environments/ice-cold", ws.Environment)
	require.Equal(t, map[string]any{"kind": "kubernetes", "context": "cool-beans"}, ws.Connection)
}

func Test_GetWorkspace_Named_Valid(t *testing.T) {
	var yaml = `
workspaces:
  items:
    test:
      connection:
        kind: kubernetes
        context: cool-beans
      scope: /a/b/c
      environment: /a/b/c/providers/Applications.Core/environments/ice-cold
`

	v, err := makeConfig(yaml)
	require.NoError(t, err)

	es, err := ReadWorkspaceSection(v)
	require.NoError(t, err)

	ws, err := es.GetWorkspace("test")
	require.NoError(t, err)

	require.Equal(t, "test", ws.Name)
	require.Equal(t, workspaces.Source(workspaces.SourceUserConfig), ws.Source)
	require.Equal(t, "/a/b/c", ws.Scope)
	require.Equal(t, "/a/b/c/providers/Applications.Core/environments/ice-cold", ws.Environment)
	require.Equal(t, map[string]any{"kind": "kubernetes", "context": "cool-beans"}, ws.Connection)
}

func makeConfig(yaml string) (*viper.Viper, error) {
	v := viper.New()
	v.SetConfigType("YAML")
	err := v.ReadConfig(bytes.NewBuffer([]byte(yaml)))
	if err != nil {
		return nil, err
	}

	return v, nil
}
