/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package operations

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"
)

func TestFlattenProperties(t *testing.T) {
	properties := map[string]any{
		"A": map[string]any{
			"B": map[string]any{
				"C": "D",
			},
			"E": "F",
		},
		"G": "H",
	}

	flattened := FlattenProperties(properties)
	require.Equal(t, map[string]any{
		"A/B/C": "D",
		"A/E":   "F",
		"G":     "H",
	}, flattened)
}

func TestUnflattenProperties(t *testing.T) {
	properties := map[string]any{
		"A/B/C": "D",
		"A/E":   "F",
		"G":     "H",
	}

	unflattened := UnflattenProperties(properties)
	require.Equal(t, map[string]any{
		"A": map[string]any{
			"B": map[string]any{
				"C": "D",
			},
			"E": "F",
		},
		"G": "H",
	}, unflattened)
}

func TestFlattenUnflattenInverses(t *testing.T) {
	properties := map[string]any{
		"A": map[string]any{
			"B": map[string]any{
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
	properties := map[string]any{
		"ClusterEndpoint:": map[string]any{
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
		currentState  map[string]any
		desiredState  map[string]any
		schema        map[string]any
		expectedPatch jsondiff.Patch
	}{
		{
			"No updates creates empty patch",
			map[string]any{
				"A": "B",
				"C": map[string]any{
					"D": map[string]any{
						"E": "F",
					},
					"G": map[string]any{
						"I": "J",
					},
					"K": "L",
				},
			},
			map[string]any{
				"A": "B",
				"C": map[string]any{
					"G": map[string]any{
						"I": "J",
					},
				},
			},
			map[string]any{
				"properties": map[string]any{
					"A": map[string]any{},
					"C": map[string]any{},
				},
				"readOnlyProperties": []any{
					"/properties/C/D/E",
				},
				"createOnlyProperties": []any{
					"/properties/C/K",
				},
			},
			nil,
		},
		{
			"Update creates patch",
			map[string]any{
				"A": "B",
				"C": map[string]any{
					"D": map[string]any{
						"E": "F",
					},
					"G": map[string]any{
						"I": "J",
					},
					"K": "L",
				},
			},
			map[string]any{
				"A": "Test",
				"C": map[string]any{
					"G": map[string]any{
						"I": "Test2",
					},
				},
			},
			map[string]any{
				"properties": map[string]any{
					"A": map[string]any{},
					"C": map[string]any{},
				},
				"readOnlyProperties": []any{
					"/properties/C/D/E",
				},
				"createOnlyProperties": []any{
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
			map[string]any{
				"A": map[string]any{
					"B": map[string]any{
						"C": "D",
						"E": "F",
					},
				},
			},
			map[string]any{
				"A": map[string]any{
					"B": map[string]any{
						"C": "D",
						"E": "Test",
					},
				},
			},
			map[string]any{
				"properties": map[string]any{
					"A": map[string]any{},
				},
				"createOnlyProperties": []any{
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
			"Unspecified properties are preserved (no remove)",
			map[string]any{
				"A": map[string]any{
					"B": map[string]any{
						"C": "D",
						"E": "F",
					},
				},
				"G": "H",
			},
			map[string]any{
				"G": "H",
			},
			map[string]any{
				"properties": map[string]any{
					"A": map[string]any{},
					"G": map[string]any{},
				},
			},
			nil,
		},
		{
			"Updating create-and-write-only property noops",
			map[string]any{
				"A": "B",
			},
			map[string]any{
				"A": "C",
			},
			map[string]any{
				"properties": map[string]any{
					"A": map[string]any{},
				},
				"createOnlyProperties": []any{
					"/properties/A",
				},
				"writeOnlyProperties": []any{
					"/properties/A",
				},
			},
			nil,
		},
		{
			"conditional-create-only-property noops if not updated",
			map[string]any{
				"A": "B",
			},
			map[string]any{},
			map[string]any{
				"properties": map[string]any{
					"A": map[string]any{},
				},
				"conditionalCreateOnlyProperties": []any{
					"/properties/A",
				},
			},
			nil,
		},
		{
			"can update conditional-create-only-property",
			map[string]any{
				"A": "B",
			},
			map[string]any{
				"A": "C",
			},
			map[string]any{
				"properties": map[string]any{
					"A": map[string]any{},
				},
				"conditionalCreateOnlyProperties": []any{
					"/properties/A",
				},
			},
			jsondiff.Patch{
				{
					Type:     "replace",
					Path:     "/A",
					OldValue: "B",
					Value:    "C",
				},
			},
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

			// During the update of jsondiff from 0.2.0 to 0.5.1, the valueLen property was added to the Operation struct:
			// https://github.com/wI2L/jsondiff/commit/55095c280821a7f78781ac1eafbd187a5f3475be#diff-8b221d1ef8bed3732b54a25e03a71dabd47bc88034b268ad3f92f3c45f4744a3R40
			// We can't set the valueLen property in the expectedPatch because it's not exported.
			// That is why we are comparing the length and some of the fields of the operations in the expected and actual patches.
			require.Equal(t, len(testCase.expectedPatch), len(patch))
			for i, expectedOperation := range testCase.expectedPatch {
				require.Equal(t, expectedOperation.Type, patch[i].Type)
				require.Equal(t, expectedOperation.Path, patch[i].Path)
				require.Equal(t, expectedOperation.OldValue, patch[i].OldValue)
				require.Equal(t, expectedOperation.Value, patch[i].Value)
			}
		})
	}
}

func TestFlattenProperties_EscapesKeys(t *testing.T) {
	t.Parallel()

	properties := map[string]any{
		"M": map[string]any{
			"tenant/id": "slash",
			"tenant":    map[string]any{"id": "nested"},
			"a~1b":      "tilde",
			"a/b":       map[string]any{"~": "both"},
		},
		"x/y": "top",
	}

	flattened := FlattenProperties(properties)
	require.Equal(t, map[string]any{
		"M/tenant~1id": "slash",
		"M/tenant/id":  "nested",
		"M/a~01b":      "tilde",
		"M/a~1b/~0":    "both",
		"x~1y":         "top",
	}, flattened)
	require.Equal(t, properties, UnflattenProperties(flattened))
}

func TestPathSegments(t *testing.T) {
	t.Parallel()

	require.Equal(t, []string{"M", "tenant/id", "a~1b", "*"}, PathSegments("M/tenant~1id/a~01b/*"))
	require.Equal(t, []string{"A"}, PathSegments("A"))
	require.Equal(t, "M/tenant~1id/a~01b/*", JoinPath([]string{"M", "tenant/id", "a~1b", "*"}))
}

// Test_GeneratePatchWithOptions_KeysWithSlashOrTilde covers map keys that hold "/" or "~". Each key stays one segment
// of its path, so an unchanged template sends nothing, and a change or removal names the key AWS has.
func Test_GeneratePatchWithOptions_KeysWithSlashOrTilde(t *testing.T) {
	t.Parallel()

	schema := []byte(`{
		"properties": {"Name": {}, "Context": {}, "Labels": {}},
		"createOnlyProperties": ["/properties/Context"]
	}`)
	properties := func(context map[string]any, labels map[string]any) map[string]any {
		return map[string]any{"Name": "n", "Context": context, "Labels": labels}
	}
	context := map[string]any{"tenant/id": "a", "tenant": map[string]any{"id": "b"}}
	labels := map[string]any{"a~1b": "tilde", "a/b": "slash", "x/y": map[string]any{}}
	aws := properties(context, labels)

	tests := []struct {
		name        string
		current     map[string]any
		desired     map[string]any
		lastApplied map[string]any
		equivalent  func(path string, template any, aws any) bool
		unresolved  []string
		want        []string
	}{
		{
			name:        "an unchanged template sends nothing",
			current:     aws,
			desired:     aws,
			lastApplied: aws,
		},
		{
			name:    "an unchanged template with no record sends nothing",
			current: aws,
			desired: aws,
		},
		{
			name:        "a changed value is replaced at its key",
			current:     aws,
			desired:     properties(context, map[string]any{"a~1b": "tilde", "a/b": "changed", "x/y": map[string]any{}}),
			lastApplied: aws,
			want:        []string{"replace /Labels/a~1b"},
		},
		{
			name:        "a key the last deploy set and the template drops is removed",
			current:     aws,
			desired:     properties(context, map[string]any{"a/b": "slash", "x/y": map[string]any{}}),
			lastApplied: aws,
			want:        []string{"remove /Labels/a~01b"},
		},
		{
			name:    "a key with no record of the last deploy is kept",
			current: aws,
			desired: properties(context, map[string]any{"a/b": "slash"}),
		},
		{
			name:        "Equivalent gets the escaped path",
			current:     properties(context, map[string]any{"a/b": "SLASH"}),
			desired:     properties(context, map[string]any{"a/b": "slash"}),
			lastApplied: properties(context, map[string]any{"a/b": "slash"}),
			equivalent: func(path string, template any, aws any) bool {
				return path == "Labels/a~1b" && strings.EqualFold(template.(string), aws.(string))
			},
		},
		{
			name:        "a field AWS fills in inside a list under such a key is kept",
			current:     map[string]any{"Outputs": map[string]any{"a/b": []any{map[string]any{"Name": "a", "Codec": "h264"}}}},
			desired:     map[string]any{"Outputs": map[string]any{"a/b": []any{map[string]any{"Name": "a"}}}},
			lastApplied: map[string]any{"Outputs": map[string]any{"a/b": []any{map[string]any{"Name": "a"}}}},
		},
		{
			name:        "an unresolved escaped path is sent as the template sets it",
			current:     properties(context, map[string]any{"a/b": "SLASH"}),
			desired:     properties(context, map[string]any{"a/b": "slash"}),
			lastApplied: properties(context, map[string]any{"a/b": "slash"}),
			equivalent: func(path string, template any, aws any) bool {
				return strings.EqualFold(template.(string), aws.(string))
			},
			unresolved: []string{"Labels/a~1b"},
			want:       []string{"replace /Labels/a~1b"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.ElementsMatch(t, orEmpty(tt.want), patchOps(t, tt.current, tt.desired, schema, PatchOptions{
				LastApplied: tt.lastApplied,
				Removed:     RemovedPaths(tt.lastApplied, tt.desired),
				Equivalent:  tt.equivalent,
				Unresolved:  tt.unresolved,
			}))
		})
	}
}

// Test_GeneratePatchWithOptions_LegacyUnresolved covers unresolved paths that UCP saved before it escaped keys, with
// each key as it is. Each is matched against paths written the same way, so it works as it did when UCP saved it.
func Test_GeneratePatchWithOptions_LegacyUnresolved(t *testing.T) {
	t.Parallel()

	schema := []byte(`{"properties": {"Name": {}, "Labels": {}, "Config": {}}}`)
	tests := []struct {
		name        string
		current     map[string]any
		desired     map[string]any
		lastApplied map[string]any
		legacy      []string
		equivalent  func(path string, template any, aws any) bool
		want        []string
	}{
		{
			name:        "a path of the key tenant/id doesn't remove the key id inside tenant",
			current:     map[string]any{"Labels": map[string]any{"tenant/id": "a", "tenant": map[string]any{"id": "outside"}}},
			desired:     map[string]any{"Labels": map[string]any{"tenant/id": "a"}},
			lastApplied: map[string]any{"Labels": map[string]any{"tenant/id": "a"}},
			legacy:      []string{"Labels/tenant/id"},
		},
		{
			name:        "a path of the key a~1b doesn't remove the key a/b",
			current:     map[string]any{"Name": "n", "Labels": map[string]any{"a/b": "outside"}},
			desired:     map[string]any{"Name": "n"},
			lastApplied: map[string]any{"Name": "n"},
			legacy:      []string{"Labels/a~1b"},
		},
		{
			name:        "a path of the key a~1b removes that key",
			current:     map[string]any{"Labels": map[string]any{"a~1b": "set by Radius", "keep": "k"}},
			desired:     map[string]any{"Labels": map[string]any{"keep": "k"}},
			lastApplied: map[string]any{"Labels": map[string]any{"keep": "k"}},
			legacy:      []string{"Labels/a~1b"},
			want:        []string{"remove /Labels/a~01b"},
		},
		{
			name:        "a path of an object emptied by a failed update removes the list AWS still has",
			current:     map[string]any{"Name": "n", "Config": []any{"set by Radius"}},
			desired:     map[string]any{"Name": "n"},
			lastApplied: map[string]any{"Name": "n", "Config": map[string]any{}},
			legacy:      []string{"Config"},
			want:        []string{"remove /Config"},
		},
		{
			name:        "a path no value of the record has still removes AWS's value there",
			current:     map[string]any{"Name": "n", "Config": map[string]any{"Items": []any{"x"}, "Other": "new"}},
			desired:     map[string]any{"Name": "n", "Config": map[string]any{"Other": "new"}},
			lastApplied: map[string]any{"Name": "n", "Config": map[string]any{"Other": "new"}},
			legacy:      []string{"Config", "Config/Items", "Config/Other"},
			want:        []string{"remove /Config/Items"},
		},
		{
			name:        "Equivalent isn't used at a legacy path",
			current:     map[string]any{"Labels": map[string]any{"a/b": "SLASH"}},
			desired:     map[string]any{"Labels": map[string]any{"a/b": "slash"}},
			lastApplied: map[string]any{"Labels": map[string]any{"a/b": "slash"}},
			legacy:      []string{"Labels/a/b"},
			equivalent: func(path string, template any, aws any) bool {
				return strings.EqualFold(template.(string), aws.(string))
			},
			want: []string{"replace /Labels/a~1b"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.ElementsMatch(t, orEmpty(tt.want), patchOps(t, tt.current, tt.desired, schema, PatchOptions{
				LastApplied:      tt.lastApplied,
				Removed:          RemovedPaths(tt.lastApplied, tt.desired),
				Equivalent:       tt.equivalent,
				LegacyUnresolved: tt.legacy,
			}))
		})
	}
}

func Test_RemovedCreateOnly_KeyWithSlash(t *testing.T) {
	t.Parallel()

	current := map[string]any{"Context": map[string]any{"tenant/id": "a"}}
	removed := RemovedPaths(current, map[string]any{})
	require.Equal(t, []string{"Context/tenant~1id"}, removed)

	got, err := RemovedCreateOnly(current, []byte(`{"createOnlyProperties": ["/properties/Context"]}`), removed)
	require.NoError(t, err)
	require.Equal(t, []string{"Context/tenant~1id"}, got)
}

func Test_ParsePropertyName(t *testing.T) {
	testCases := []struct {
		name   string
		input  string
		output string
		err    error
	}{
		{
			"ParsePropertyName successfully parses single property",
			"/properties/propertyName",
			"propertyName",
			nil,
		},
		{
			"ParsePropertyName successfully parses sub-properties",
			"/properties/propertyName/subProperty/subSubProperty",
			"propertyName/subProperty/subSubProperty",
			nil,
		},
		{
			"ParsePropertyName returns an error if input is invalid",
			"propertyName",
			"",
			fmt.Errorf("property identifier propertyName is not in the format /properties/<propertyName>"),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actual, err := ParsePropertyName(testCase.input)
			require.Equal(t, testCase.output, actual)
			require.Equal(t, err, testCase.err)
		})
	}
}
