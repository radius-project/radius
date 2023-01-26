// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ACRPathParser(t *testing.T) {
	repository, tag, err := parseTemplatePath("radiusdev.azurecr.io/recipes/functionaltest/parameters/mongodatabases/azure:1.0")
	require.NoError(t, err)
	require.Equal(t, "radiusdev.azurecr.io/recipes/functionaltest/parameters/mongodatabases/azure", repository)
	require.Equal(t, "1.0", tag)
}

func Test_ACRPathParserErr(t *testing.T) {
	repository, tag, err := parseTemplatePath("http://user:passwd@example.com/test/bar:v1")
	require.Error(t, err)
	require.Equal(t, "", repository)
	require.Equal(t, "", tag)
}

func Test_ContextParameter(t *testing.T) {
	devParams := map[string]any{
		"throughput": 400,
		"port":       2030,
		"name":       "test-parameters",
	}
	operatorParams := map[string]any{
		"port":     2040,
		"name":     "test-parameters-conflict",
		"location": "us-east1",
	}
	expectedParams := map[string]any{
		"throughput": map[string]any{
			"value": 400,
		},
		"port": map[string]any{
			"value": 2030,
		},
		"name": map[string]any{
			"value": "test-parameters",
		},
		"location": map[string]any{
			"value": "us-east1",
		},
	}
	actualParams := handleParameterConflict(devParams, operatorParams)
	require.Equal(t, expectedParams, actualParams)
}
