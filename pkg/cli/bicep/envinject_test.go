// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package bicep

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Env_InjectedIfParamAvailable(t *testing.T) {
	input, err := os.ReadFile(filepath.Join("testdata", "test-injectid.json"))
	require.NoError(t, err)
	template := map[string]interface{}{}

	err = json.Unmarshal(input, &template)
	require.NoError(t, err)

	params := map[string]map[string]interface{}{}

	err = InjectEnvironmentParam(template, params, context.TODO(), "/planes/radius/local/resourceGroups/my-rg/providers/Application.Core/environments/my")
	require.NoError(t, err)

	require.Equal(t, "/planes/radius/local/resourceGroups/my-rg/providers/Application.Core/environments/my", params["environmentId"]["value"])
}

func Test_Env_NotInjectedIfNoParamAvailable(t *testing.T) {
	input, err := os.ReadFile(filepath.Join("testdata", "test-noenv.json"))
	require.NoError(t, err)
	template := map[string]interface{}{}

	err = json.Unmarshal(input, &template)
	require.NoError(t, err)

	params := map[string]map[string]interface{}{}

	err = InjectEnvironmentParam(template, params, context.TODO(), "/planes/radius/local/resourceGroups/my-rg/providers/Application.Core/environments/my")
	require.NoError(t, err)

	require.Nil(t, params["environmentId"])
}

func Test_Env_NotInjectedIfParamAlreadySet(t *testing.T) {
	input, err := os.ReadFile(filepath.Join("testdata", "test-injectid.json"))
	require.NoError(t, err)
	template := map[string]interface{}{}

	err = json.Unmarshal(input, &template)
	require.NoError(t, err)

	params := map[string]map[string]interface{}{
		"environmentId": {
			"value": "ANOTHERENV",
		},
	}

	err = InjectEnvironmentParam(template, params, context.TODO(), "/planes/radius/local/resourceGroups/my-rg/providers/Application.Core/environments/my")
	require.NoError(t, err)

	require.Equal(t, "ANOTHERENV", params["environmentId"]["value"])
}
