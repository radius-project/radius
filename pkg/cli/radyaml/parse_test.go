// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radyaml

import (
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/stretchr/testify/require"
)

func Test_Parse_Failure_RejectsUnknownFields(t *testing.T) {
	reader := strings.NewReader(`
name: todo
stages:
- name: infra
  extra: 'definitely'
  bicep:
    template: infra.bicep
- name: app
  bicep:
    template: app.bicep
`)

	parsed, err := Parse(reader)
	require.Error(t, err)
	require.Contains(t, err.Error(), "field extra not found in type radyaml.Stage")
	require.Equal(t, parsed, Manifest{})
}

func Test_Parse_Failure_NoBuilder(t *testing.T) {
	reader := strings.NewReader(`
name: todo
stages:
- name: infra
  build: 
    frontend: {}
  bicep:
    template: infra.bicep
- name: app
  bicep:
    template: app.bicep
`)

	parsed, err := Parse(reader)
	require.Error(t, err)
	require.Contains(t, err.Error(), "a build target should specify a single builder")
	require.Equal(t, parsed, Manifest{})
}

func Test_Parse_Failure_ExtraBuilder(t *testing.T) {
	reader := strings.NewReader(`
name: todo
stages:
- name: infra
  build: 
    frontend:
      docker: {}
      npm: {}
  bicep:
    template: infra.bicep
- name: app
  bicep:
    template: app.bicep
`)

	parsed, err := Parse(reader)
	require.Error(t, err)
	require.Contains(t, err.Error(), "a build target should specify a single builder")
	require.Equal(t, parsed, Manifest{})
}

func Test_Parse_Success(t *testing.T) {
	reader := strings.NewReader(`
name: todo
stages:
- name: infra
  bicep:
    template: infra.bicep
  profiles:
    dev: 
      bicep:
        template: infra-dev.bicep
    staging: 
      bicep:
        template: infra-staging.bicep
- name: app
  build:
    backend:
      docker:
        context: src
        image: 'radius.azurecr.io/backend:latest'
  bicep:
    template: app.bicep
`)

	parsed, err := Parse(reader)
	require.NoError(t, err)

	expected := Manifest{
		Name: "todo",
		Stages: []Stage{
			{
				Name: "infra",
				Bicep: &BicepStage{
					Template: to.Ptr("infra.bicep"),
				},
				Profiles: map[string]Profile{
					"dev": {
						Bicep: &BicepStage{
							Template: to.Ptr("infra-dev.bicep"),
						},
					},
					"staging": {
						Bicep: &BicepStage{
							Template: to.Ptr("infra-staging.bicep"),
						},
					},
				},
			},
			{
				Name: "app",
				Build: map[string]*BuildTarget{
					"backend": {
						Builder: "docker",
						Values: map[string]interface{}{
							"context": "src",
							"image":   "radius.azurecr.io/backend:latest",
						},
					},
				},
				Bicep: &BicepStage{
					Template: to.Ptr("app.bicep"),
				},
			},
		},
	}
	require.Equal(t, expected, parsed)
}
