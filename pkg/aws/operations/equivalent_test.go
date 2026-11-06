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
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_GeneratePatchWithOptions_Equivalent(t *testing.T) {
	t.Parallel()

	current := map[string]any{"EngineVersion": "5.0.0", "Config": map[string]any{"Version": "2.1.0", "Mode": "fast"}}
	sameVersion := func(path string, template any, aws any) bool {
		return (path == "EngineVersion" || path == "Config/Version") && aws == template.(string)+".0"
	}
	tests := []struct {
		name       string
		desired    map[string]any
		equivalent func(path string, template any, aws any) bool
		want       int
	}{
		{
			name:       "values the rule matches are unchanged",
			desired:    map[string]any{"EngineVersion": "5.0", "Config": map[string]any{"Version": "2.1", "Mode": "fast"}},
			equivalent: sameVersion,
			want:       0,
		},
		{
			name:       "a value the rule doesn't match is a change",
			desired:    map[string]any{"EngineVersion": "6.0", "Config": map[string]any{"Version": "2.1", "Mode": "fast"}},
			equivalent: sameVersion,
			want:       1,
		},
		{
			name:    "without a rule, another form is a change",
			desired: map[string]any{"EngineVersion": "5.0", "Config": map[string]any{"Version": "2.1", "Mode": "fast"}},
			want:    2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			currentState, err := json.Marshal(current)
			require.NoError(t, err)
			desiredState, err := json.Marshal(tt.desired)
			require.NoError(t, err)
			patch, err := GeneratePatchWithOptions(currentState, desiredState, []byte(`{}`), PatchOptions{Equivalent: tt.equivalent})
			require.NoError(t, err)
			require.Len(t, patch, tt.want)
		})
	}
}

// within reports whether every value template sets is in aws, as for a service that fills in defaults inside the items
// of a list.
func within(template any, aws any) bool {
	switch t := template.(type) {
	case map[string]any:
		a, ok := aws.(map[string]any)
		if !ok {
			return false
		}
		for k, v := range t {
			if !within(v, a[k]) {
				return false
			}
		}
		return true
	case []any:
		a, ok := aws.([]any)
		if !ok || len(a) != len(t) {
			return false
		}
		for i := range t {
			if !within(t[i], a[i]) {
				return false
			}
		}
		return true
	}
	return template == aws
}

func Test_GeneratePatchWithOptions_EquivalentAfterRemoval(t *testing.T) {
	t.Parallel()

	current := map[string]any{
		"Videos":        []any{map[string]any{"Name": "hd", "Sharpness": 75.0, "Codec": "h264"}},
		"EngineVersion": "5.0.0",
		"Config":        map[string]any{"Version": "2.1.0", "Mode": "fast"},
		"Settings":      []any{"a"},
	}
	equivalent := func(path string, template any, aws any) bool {
		switch path {
		case "Videos":
			return within(template, aws)
		case "EngineVersion", "Config/Version":
			return aws == template.(string)+".0"
		case "Settings":
			return true
		}
		return false
	}
	videos := func(item map[string]any) map[string]any {
		return map[string]any{"Videos": []any{item}, "EngineVersion": "5.0", "Config": map[string]any{"Version": "2.1", "Mode": "fast"}, "Settings": []any{"a"}}
	}
	tests := []struct {
		name        string
		lastApplied map[string]any
		desired     map[string]any
		want        []string
	}{
		{
			name:        "a field removed inside an array item is cleared, and the item becomes the template's",
			lastApplied: videos(map[string]any{"Name": "hd", "Sharpness": 75.0}),
			desired:     videos(map[string]any{"Name": "hd"}),
			want:        []string{"remove /Videos/0/Sharpness", "remove /Videos/0/Codec"},
		},
		{
			name:        "after an array item is removed, the list becomes the template's",
			lastApplied: map[string]any{"Videos": []any{map[string]any{"Name": "hd"}, map[string]any{"Name": "sd"}}},
			desired:     videos(map[string]any{"Name": "hd", "Sharpness": 75.0}),
			want:        []string{"remove /Videos/0/Codec"},
		},
		{
			name:        "an unchanged redeploy after the removal changes nothing",
			lastApplied: videos(map[string]any{"Name": "hd"}),
			desired:     videos(map[string]any{"Name": "hd"}),
			want:        nil,
		},
		{
			name:    "without a record of the last deploy, Equivalent applies as before",
			desired: videos(map[string]any{"Name": "hd"}),
			want:    nil,
		},
		{
			name:        "a changed value is not a removal",
			lastApplied: map[string]any{"EngineVersion": "4.0", "Videos": []any{map[string]any{"Name": "hd", "Sharpness": 70.0}}},
			desired:     videos(map[string]any{"Name": "hd", "Sharpness": 75.0}),
			want:        nil,
		},
		{
			name:        "a removed path outside any Equivalent path doesn't change it",
			lastApplied: map[string]any{"Videos": []any{map[string]any{"Name": "hd"}}, "EngineVersion": "5.0", "Config": map[string]any{"Version": "2.1", "Mode": "fast"}, "Tier": "standard"},
			desired:     videos(map[string]any{"Name": "hd"}),
			want:        nil,
		},
		{
			name:        "a removed sibling under the same parent doesn't change Equivalent for the other",
			lastApplied: map[string]any{"Config": map[string]any{"Version": "2.1", "Mode": "fast"}},
			desired:     map[string]any{"Config": map[string]any{"Version": "2.1"}},
			want:        []string{"remove /Config/Mode"},
		},
		{
			name:        "a removed path under a value Equivalent would match",
			lastApplied: map[string]any{"Settings": map[string]any{"Mode": "fast"}},
			desired:     map[string]any{"Settings": []any{"b"}},
			want:        []string{"replace /Settings/0"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			currentState, err := json.Marshal(current)
			require.NoError(t, err)
			desiredState, err := json.Marshal(tt.desired)
			require.NoError(t, err)
			opts := PatchOptions{Equivalent: equivalent, LastApplied: tt.lastApplied}
			if tt.lastApplied != nil {
				opts.Removed = RemovedPaths(tt.lastApplied, tt.desired)
			}
			patch, err := GeneratePatchWithOptions(currentState, desiredState, []byte(`{}`), opts)
			require.NoError(t, err)
			got := []string{}
			for _, op := range patch {
				got = append(got, string(op.Type)+" "+op.Path)
			}
			if tt.want == nil {
				tt.want = []string{}
			}
			require.ElementsMatch(t, tt.want, got)
		})
	}
}

func Test_drops(t *testing.T) {
	t.Parallel()

	item := func(fields ...string) map[string]any {
		out := map[string]any{}
		for _, f := range fields {
			out[f] = "v"
		}
		return out
	}
	tests := []struct {
		name     string
		applied  any
		template any
		want     bool
	}{
		{name: "nothing applied", applied: nil, template: "x", want: false},
		{name: "a value changed", applied: "5.0", template: "6.0", want: false},
		{name: "a value removed", applied: "5.0", template: nil, want: true},
		{name: "an object kept", applied: item("A"), template: item("A", "B"), want: false},
		{name: "a field removed from an object", applied: item("A", "B"), template: item("A"), want: true},
		{name: "an object replaced by another kind of value", applied: item("A"), template: "x", want: true},
		{name: "an empty object replaced", applied: map[string]any{}, template: "x", want: false},
		{name: "list items reordered", applied: []any{item("A"), item("B")}, template: []any{item("B"), item("A")}, want: false},
		{name: "a field removed from a list item", applied: []any{item("A", "B")}, template: []any{item("A")}, want: true},
		{name: "a list item removed", applied: []any{"a", "b"}, template: []any{"a"}, want: true},
		{name: "a list replaced by another kind of value", applied: []any{"a"}, template: "a", want: true},
		{name: "an empty list replaced", applied: []any{}, template: "a", want: false},
		{name: "each list item needs its own template item", applied: []any{item("A", "B"), item("A", "C")}, template: []any{item("A", "B", "C"), item("A")}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, drops(tt.applied, tt.template))
		})
	}
}
