// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radyaml

import (
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/stretchr/testify/require"
)

func Test_Parse_Success(t *testing.T) {
	reader := strings.NewReader(`
name: todo
build:
- name: todo_build
  npm:
    directory: '.'
    script: 'dev:start'
    container:
      image: 'radius.azurecr.io/magpie'
stages:
- name: infra
  deploy:
    bicep: infra.bicep
- name: app
  deploy:
    bicep: app.bicep
    params:
    - name: todo_build
`)

	parsed, err := Parse(reader)
	require.NoError(t, err)

	expected := Manifest{
		Name: "todo",
		Build: []BuildTarget{
			{
				Name: "todo_build",
				NPM: &NPMBuild{
					Directory: ".",
					Script:    "dev:start",
					Container: &NPMBuildContainer{
						Image: "radius.azurecr.io/magpie",
					},
				},
			},
		},
		Stages: []Stage{
			{
				Name: "infra",
				Deploy: &DeployStage{
					Bicep: to.StringPtr("infra.bicep"),
				},
			},
			{
				Name: "app",
				Deploy: &DeployStage{
					Bicep: to.StringPtr("app.bicep"),
					Params: []DeployStageParameter{
						{
							Name: "todo_build",
						},
					},
				},
			},
		},
	}
	require.Equal(t, expected, parsed)
}
