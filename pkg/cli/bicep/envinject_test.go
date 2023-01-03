// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package bicep

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_InjectEnvironmentParam_InjectedIfParamAvailable(t *testing.T) {
	input, err := os.ReadFile(filepath.Join("testdata", "test-injectenvid.json"))
	require.NoError(t, err)
	template := map[string]any{}

	err = json.Unmarshal(input, &template)
	require.NoError(t, err)

	params := map[string]map[string]any{}

	err = InjectEnvironmentParam(template, params, "/planes/radius/local/resourceGroups/my-rg/providers/Application.Core/environments/my")
	require.NoError(t, err)

	require.Equal(t, "/planes/radius/local/resourceGroups/my-rg/providers/Application.Core/environments/my", params["environment"]["value"])
}

func Test_InjectApplicationParam_InjectedIfParamAvailable(t *testing.T) {
	input, err := os.ReadFile(filepath.Join("testdata", "test-injectappid.json"))
	require.NoError(t, err)
	template := map[string]any{}

	err = json.Unmarshal(input, &template)
	require.NoError(t, err)

	params := map[string]map[string]any{}

	err = InjectApplicationParam(template, params, "/planes/radius/local/resourceGroups/my-rg/providers/Application.Core/applications/my")
	require.NoError(t, err)

	require.Equal(t, "/planes/radius/local/resourceGroups/my-rg/providers/Application.Core/applications/my", params["application"]["value"])
}

func Test_injectParam_InjectedIfParamAvailable(t *testing.T) {
	input, err := os.ReadFile(filepath.Join("testdata", "test-injectenvid.json"))
	require.NoError(t, err)
	template := map[string]any{}

	err = json.Unmarshal(input, &template)
	require.NoError(t, err)

	params := map[string]map[string]any{}

	err = injectParam(template, params, "environment", "/planes/radius/local/resourceGroups/my-rg/providers/Application.Core/environments/my")
	require.NoError(t, err)

	require.Equal(t, "/planes/radius/local/resourceGroups/my-rg/providers/Application.Core/environments/my", params["environment"]["value"])
}

func Test_injectParam_NotInjectedIfNoParamAvailable(t *testing.T) {
	input, err := os.ReadFile(filepath.Join("testdata", "test-noenv.json"))
	require.NoError(t, err)
	template := map[string]any{}

	err = json.Unmarshal(input, &template)
	require.NoError(t, err)

	params := map[string]map[string]any{}

	err = injectParam(template, params, "environment", "/planes/radius/local/resourceGroups/my-rg/providers/Application.Core/environments/my")
	require.NoError(t, err)

	require.Nil(t, params["environment"])
}

func Test_injectParam_NotInjectedIfParamAlreadySet(t *testing.T) {
	input, err := os.ReadFile(filepath.Join("testdata", "test-injectenvid.json"))
	require.NoError(t, err)
	template := map[string]any{}

	err = json.Unmarshal(input, &template)
	require.NoError(t, err)

	params := map[string]map[string]any{
		"environment": {
			"value": "ANOTHERENV",
		},
	}

	err = injectParam(template, params, "environment", "/planes/radius/local/resourceGroups/my-rg/providers/Application.Core/environments/my")
	require.NoError(t, err)

	require.Equal(t, "ANOTHERENV", params["environment"]["value"])
}
