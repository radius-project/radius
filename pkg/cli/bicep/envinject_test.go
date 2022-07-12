// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package bicep

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Env_InjectedIfParamAvailable(t *testing.T) {
	input, err := ioutil.ReadFile(filepath.Join("testdata", "test-injectid.json"))
	require.NoError(t, err)
	template := map[string]interface{}{}

	err = json.Unmarshal(input, &template)
	require.NoError(t, err)

	params := map[string]map[string]interface{}{}

	err = InjectEnvironmentParam(template, params, context.TODO(), "/planes/radius/local/resourceGroups/my-rg/providers/Application.Core/environments/my")
	require.NoError(t, err)

	require.Equal(t, "/planes/radius/local/resourceGroups/my-rg/providers/Application.Core/environments/my", params["environment"]["value"])
}

func Test_Env_NotInjectedIfNoParamAvailable(t *testing.T) {
	input, err := ioutil.ReadFile(filepath.Join("testdata", "test-noenv.json"))
	require.NoError(t, err)
	template := map[string]interface{}{}

	err = json.Unmarshal(input, &template)
	require.NoError(t, err)

	params := map[string]map[string]interface{}{}

	err = InjectEnvironmentParam(template, params, context.TODO(), "/planes/radius/local/resourceGroups/my-rg/providers/Application.Core/environments/my")
	require.NoError(t, err)

	require.Nil(t, params["environment"])
}

func Test_Env_NotInjectedIfParamAlreadySet(t *testing.T) {
	input, err := ioutil.ReadFile(filepath.Join("testdata", "test-injectid.json"))
	require.NoError(t, err)
	template := map[string]interface{}{}

	err = json.Unmarshal(input, &template)
	require.NoError(t, err)

	params := map[string]map[string]interface{}{
		"environment": {
			"value": "ANOTHERENV",
		},
	}

	err = InjectEnvironmentParam(template, params, context.TODO(), "/planes/radius/local/resourceGroups/my-rg/providers/Application.Core/environments/my")
	require.NoError(t, err)

	require.Equal(t, "ANOTHERENV", params["environment"]["value"])
}
