// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rest

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ComponentBinding_MarshalJSON(t *testing.T) {
	binding := ComponentBinding{
		Kind: "testkind",
		AdditionalProperties: map[string]interface{}{
			"A": 33,
			"B": "some-value",
		},
	}

	expected := map[string]interface{}{
		"kind": "testkind",
		"A":    float64(33),
		"B":    "some-value",
	}

	b, err := json.Marshal(binding)
	require.NoError(t, err)

	actual := map[string]interface{}{}
	err = json.Unmarshal(b, &actual)
	require.NoError(t, err)

	require.Equal(t, expected, actual)
}

func Test_ComponentBinding_UnmarshalJSON(t *testing.T) {
	expected := ComponentBinding{
		Kind: "testkind",
		AdditionalProperties: map[string]interface{}{
			"A": float64(33),
			"B": "some-value",
		},
	}

	input := map[string]interface{}{
		"kind": "testkind",
		"A":    33,
		"B":    "some-value",
	}

	b, err := json.Marshal(input)
	require.NoError(t, err)

	actual := ComponentBinding{}
	err = json.Unmarshal(b, &actual)
	require.NoError(t, err)

	require.Equal(t, expected, actual)
}

func Test_ComponentBinding_UnmarshalJSON_ErrorNoKind(t *testing.T) {
	input := map[string]interface{}{
		"A": 33,
		"B": "some-value",
	}

	b, err := json.Marshal(input)
	require.NoError(t, err)

	actual := ComponentBinding{}
	err = json.Unmarshal(b, &actual)
	require.Error(t, err)
	require.Equal(t, "the 'kind' property is required", err.Error())
}

func Test_ComponentBinding_UnmarshalJSON_ErrorKindIsNotString(t *testing.T) {
	input := map[string]interface{}{
		"kind": map[string]interface{}{
			"value": true,
		},
		"A": 33,
		"B": "some-value",
	}

	b, err := json.Marshal(input)
	require.NoError(t, err)

	actual := ComponentBinding{}
	err = json.Unmarshal(b, &actual)
	require.Error(t, err)
	require.Equal(t, "the 'kind' property must be a string", err.Error())
}

func Test_ComponentTrait_MarshalJSON(t *testing.T) {
	trait := ComponentTrait{
		Kind: "testkind",
		AdditionalProperties: map[string]interface{}{
			"A": 33,
			"B": "some-value",
		},
	}

	expected := map[string]interface{}{
		"kind": "testkind",
		"A":    float64(33),
		"B":    "some-value",
	}

	b, err := json.Marshal(trait)
	require.NoError(t, err)

	actual := map[string]interface{}{}
	err = json.Unmarshal(b, &actual)
	require.NoError(t, err)

	require.Equal(t, expected, actual)
}

func Test_ComponentTrait_UnmarshalJSON(t *testing.T) {
	expected := ComponentTrait{
		Kind: "testkind",
		AdditionalProperties: map[string]interface{}{
			"A": float64(33),
			"B": "some-value",
		},
	}

	input := map[string]interface{}{
		"kind": "testkind",
		"A":    33,
		"B":    "some-value",
	}

	b, err := json.Marshal(input)
	require.NoError(t, err)

	actual := ComponentTrait{}
	err = json.Unmarshal(b, &actual)
	require.NoError(t, err)

	require.Equal(t, expected, actual)
}

func Test_ComponentTrait_UnmarshalJSON_ErrorNoKind(t *testing.T) {
	input := map[string]interface{}{
		"A": 33,
		"B": "some-value",
	}

	b, err := json.Marshal(input)
	require.NoError(t, err)

	actual := ComponentTrait{}
	err = json.Unmarshal(b, &actual)
	require.Error(t, err)
	require.Equal(t, "the 'kind' property is required", err.Error())
}

func Test_ComponentTrait_UnmarshalJSON_ErrorKindIsNotString(t *testing.T) {
	input := map[string]interface{}{
		"kind": map[string]interface{}{
			"value": true,
		},
		"A": 33,
		"B": "some-value",
	}

	b, err := json.Marshal(input)
	require.NoError(t, err)

	actual := ComponentTrait{}
	err = json.Unmarshal(b, &actual)
	require.Error(t, err)
	require.Equal(t, "the 'kind' property must be a string", err.Error())
}
