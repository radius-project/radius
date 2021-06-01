// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package components

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_GenericBinding_MarshalJSON(t *testing.T) {
	binding := GenericBinding{
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

func Test_GenericBinding_UnmarshalJSON(t *testing.T) {
	expected := GenericBinding{
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

	actual := GenericBinding{}
	err = json.Unmarshal(b, &actual)
	require.NoError(t, err)

	require.Equal(t, expected, actual)
}

func Test_GenericBinding_UnmarshalJSON_ErrorNoKind(t *testing.T) {
	input := map[string]interface{}{
		"A": 33,
		"B": "some-value",
	}

	b, err := json.Marshal(input)
	require.NoError(t, err)

	actual := GenericBinding{}
	err = json.Unmarshal(b, &actual)
	require.Error(t, err)
	require.Equal(t, "the 'kind' property is required", err.Error())
}

func Test_GenericBinding_UnmarshalJSON_ErrorKindIsNotString(t *testing.T) {
	input := map[string]interface{}{
		"kind": map[string]interface{}{
			"value": true,
		},
		"A": 33,
		"B": "some-value",
	}

	b, err := json.Marshal(input)
	require.NoError(t, err)

	actual := GenericBinding{}
	err = json.Unmarshal(b, &actual)
	require.Error(t, err)
	require.Equal(t, "the 'kind' property must be a string", err.Error())
}

func Test_GenericTrait_MarshalJSON(t *testing.T) {
	trait := GenericTrait{
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

func Test_GenericTrait_UnmarshalJSON(t *testing.T) {
	expected := GenericTrait{
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

	actual := GenericTrait{}
	err = json.Unmarshal(b, &actual)
	require.NoError(t, err)

	require.Equal(t, expected, actual)
}

func Test_GenericTrait_UnmarshalJSON_ErrorNoKind(t *testing.T) {
	input := map[string]interface{}{
		"A": 33,
		"B": "some-value",
	}

	b, err := json.Marshal(input)
	require.NoError(t, err)

	actual := GenericTrait{}
	err = json.Unmarshal(b, &actual)
	require.Error(t, err)
	require.Equal(t, "the 'kind' property is required", err.Error())
}

func Test_GenericTrait_UnmarshalJSON_ErrorKindIsNotString(t *testing.T) {
	input := map[string]interface{}{
		"kind": map[string]interface{}{
			"value": true,
		},
		"A": 33,
		"B": "some-value",
	}

	b, err := json.Marshal(input)
	require.NoError(t, err)

	actual := GenericTrait{}
	err = json.Unmarshal(b, &actual)
	require.Error(t, err)
	require.Equal(t, "the 'kind' property must be a string", err.Error())
}
