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

func Test_Parse_Success(t *testing.T) {
	reader := strings.NewReader(`
name: todo
stages:
- name: infra
  bicep:
    template: infra.bicep
- name: app
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
					Template: to.StringPtr("infra.bicep"),
				},
			},
			{
				Name: "app",
				Bicep: &BicepStage{
					Template: to.StringPtr("app.bicep"),
				},
			},
		},
	}
	require.Equal(t, expected, parsed)
}
