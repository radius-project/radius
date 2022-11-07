// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package operations

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"
)

func TestFlattenProperties(t *testing.T) {
	properties := map[string]interface{}{
		"A": map[string]interface{}{
			"B": map[string]interface{}{
				"C": "D",
			},
			"E": "F",
		},
		"G": "H",
	}

	flattened := FlattenProperties(properties)
	require.Equal(t, map[string]interface{}{
		"A/B/C": "D",
		"A/E":   "F",
		"G":     "H",
	}, flattened)
}

func TestUnflattenProperties(t *testing.T) {
	properties := map[string]interface{}{
		"A/B/C": "D",
		"A/E":   "F",
		"G":     "H",
	}

	unflattened := UnflattenProperties(properties)
	require.Equal(t, map[string]interface{}{
		"A": map[string]interface{}{
			"B": map[string]interface{}{
				"C": "D",
			},
			"E": "F",
		},
		"G": "H",
	}, unflattened)
}

func TestFlattenUnflattenInverses(t *testing.T) {
	properties := map[string]interface{}{
		"A": map[string]interface{}{
			"B": map[string]interface{}{
				"C": "D",
			},
			"E": "F",
		},
		"G": "H",
	}

	flattened := FlattenProperties(properties)
	unflattened := UnflattenProperties(flattened)
	require.Equal(t, properties, unflattened)
}

func TestFlattenUnflattenRealData(t *testing.T) {
	properties := map[string]interface{}{
		"ClusterEndpoint:": map[string]interface{}{
			"Address": "https://A1B2C3D4E5F6.gr7.us-west-2.eks.amazonaws.com",
			"Port":    443,
		},
		"ClusterName": "my-cluster",
	}

	flattened := FlattenProperties(properties)
	unflattened := UnflattenProperties(flattened)
	require.Equal(t, properties, unflattened)
}

func Test_GeneratePatch(t *testing.T) {
	testCases := []struct {
		name          string
		currentState  map[string]interface{}
		desiredState  map[string]interface{}
		schema        map[string]interface{}
		expectedPatch jsondiff.Patch
	}{
		{
			"No updates creates empty patch",
			map[string]interface{}{
				"A": "B",
				"C": map[string]interface{}{
					"D": map[string]interface{}{
						"E": "F",
					},
					"G": map[string]interface{}{
						"I": "J",
					},
					"K": "L",
				},
			},
			map[string]interface{}{
				"A": "B",
				"C": map[string]interface{}{
					"G": map[string]interface{}{
						"I": "J",
					},
				},
			},
			map[string]interface{}{
				"properties": map[string]interface{}{
					"A": map[string]interface{}{},
					"C": map[string]interface{}{},
				},
				"readOnlyProperties": []interface{}{
					"/properties/C/D/E",
				},
				"createOnlyProperties": []interface{}{
					"/properties/C/K",
				},
			},
			nil,
		},
		{
			"Update creates patch",
			map[string]interface{}{
				"A": "B",
				"C": map[string]interface{}{
					"D": map[string]interface{}{
						"E": "F",
					},
					"G": map[string]interface{}{
						"I": "J",
					},
					"K": "L",
				},
			},
			map[string]interface{}{
				"A": "Test",
				"C": map[string]interface{}{
					"G": map[string]interface{}{
						"I": "Test2",
					},
				},
			},
			map[string]interface{}{
				"properties": map[string]interface{}{
					"A": map[string]interface{}{},
					"C": map[string]interface{}{},
				},
				"readOnlyProperties": []interface{}{
					"/properties/C/D/E",
				},
				"createOnlyProperties": []interface{}{
					"/properties/C/K",
				},
			},
			jsondiff.Patch{
				{
					Type:     "replace",
					Path:     "/A",
					OldValue: "B",
					Value:    "Test",
				},
				{
					Type:     "replace",
					Path:     "/C/G/I",
					OldValue: "J",
					Value:    "Test2",
				},
			},
		},
		{
			"Specify create-only properties",
			map[string]interface{}{
				"A": map[string]interface{}{
					"B": map[string]interface{}{
						"C": "D",
						"E": "F",
					},
				},
			},
			map[string]interface{}{
				"A": map[string]interface{}{
					"B": map[string]interface{}{
						"C": "D",
						"E": "Test",
					},
				},
			},
			map[string]interface{}{
				"properties": map[string]interface{}{
					"A": map[string]interface{}{},
				},
				"createOnlyProperties": []interface{}{
					"/properties/A/B/C",
				},
			},
			jsondiff.Patch{
				{
					Type:     "replace",
					Path:     "/A/B/E",
					OldValue: "F",
					Value:    "Test",
				},
			},
		},
		{
			"Remove object",
			map[string]interface{}{
				"A": map[string]interface{}{
					"B": map[string]interface{}{
						"C": "D",
						"E": "F",
					},
				},
				"G": "H",
			},
			map[string]interface{}{
				"G": "H",
			},
			map[string]interface{}{
				"properties": map[string]interface{}{
					"A": map[string]interface{}{},
					"G": map[string]interface{}{},
				},
			},
			jsondiff.Patch{
				{
					Type: "remove",
					Path: "/A",
					OldValue: map[string]interface{}{
						"B": map[string]interface{}{
							"C": "D",
							"E": "F",
						},
					},
					Value: nil,
				},
			},
		},
		{
			"Updating create-and-write-only property noops",
			map[string]interface{}{
				"A": "B",
			},
			map[string]interface{}{
				"A": "C",
			},
			map[string]interface{}{
				"properties": map[string]interface{}{
					"A": map[string]interface{}{},
				},
				"createOnlyProperties": []interface{}{
					"/properties/A",
				},
				"writeOnlyProperties": []interface{}{
					"/properties/A",
				},
			},
			nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			desiredStateBytes, err := json.Marshal(testCase.desiredState)
			require.NoError(t, err)

			currentStateBytes, err := json.Marshal(testCase.currentState)
			require.NoError(t, err)

			schemaBytes, err := json.Marshal(testCase.schema)
			require.NoError(t, err)

			patch, err := GeneratePatch(currentStateBytes, desiredStateBytes, schemaBytes)
			require.NoError(t, err)

			require.Equal(t, testCase.expectedPatch, patch)
		})

	}
}
