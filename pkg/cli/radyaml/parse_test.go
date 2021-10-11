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
name: my-app
stages:
- name: infra
  deploy:
    bicep: infra.bicep
- name: app
  deploy:
    bicep: app.bicep
    params:
    - name: todo_build
      npm:
        directory: '.'
        script: 'dev:start'
        container:
          image: 'radius.azurecr.io/magpie'
`)

	parsed, err := Parse(reader)
	require.NoError(t, err)

	expected := Manifest{
		Name: "my-app",
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
							NPM: &NPMBuild{
								Directory: ".",
								Script:    "dev:start",
								Container: &NPMBuildContainer{
									Image: "radius.azurecr.io/magpie",
								},
							},
						},
					},
				},
			},
		},
	}
	require.Equal(t, expected, parsed)
}
