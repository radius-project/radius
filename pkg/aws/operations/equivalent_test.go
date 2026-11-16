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

// covers reports whether aws has every value template sets, pairing list items in any order, as EMR's Equivalent does.
// With extra, aws may hold more list items than template, as EMR does for applications it installs.
func covers(template any, aws any, extra bool) bool {
	switch t := template.(type) {
	case map[string]any:
		a, ok := aws.(map[string]any)
		if !ok {
			return false
		}
		for k, v := range t {
			if !covers(v, a[k], false) {
				return false
			}
		}
		return true
	case []any:
		a, ok := aws.([]any)
		if !ok || (!extra && len(a) != len(t)) {
			return false
		}
		matches := make([][]bool, len(t))
		for i := range t {
			matches[i] = make([]bool, len(a))
			for j := range a {
				matches[i][j] = covers(t[i], a[j], false)
			}
		}
		return perfectMatching(matches)
	}
	return template == aws
}

func Test_GeneratePatchWithOptions_EquivalentOwnership(t *testing.T) {
	t.Parallel()

	item := func(fields ...any) map[string]any {
		out := map[string]any{}
		for i := 0; i < len(fields); i += 2 {
			out[fields[i].(string)] = fields[i+1]
		}
		return out
	}
	list := func(items ...any) []any { return items }
	within := func(path string, template any, aws any) bool { return path != "Version" && within(template, aws) }
	anyOrder := func(path string, template any, aws any) bool { return covers(template, aws, false) }
	extraItems := func(path string, template any, aws any) bool { return covers(template, aws, true) }
	version := func(path string, template any, aws any) bool { return aws == template.(string)+".0" }

	tests := []struct {
		name        string
		equivalent  func(path string, template any, aws any) bool
		lastApplied map[string]any
		unresolved  []string
		current     map[string]any
		desired     map[string]any
		want        []string
	}{
		{
			name:        "a field removed inside a list item is cleared",
			equivalent:  within,
			lastApplied: map[string]any{"Videos": list(item("Name", "hd", "Sharpness", 75.0))},
			current:     map[string]any{"Videos": list(item("Name", "hd", "Sharpness", 75.0, "Codec", "h264"))},
			desired:     map[string]any{"Videos": list(item("Name", "hd"))},
			want:        []string{"remove /Videos/0/Sharpness", "remove /Videos/0/Codec"},
		},
		{
			name:        "an unchanged deploy keeps AWS's defaults",
			equivalent:  within,
			lastApplied: map[string]any{"Videos": list(item("Name", "hd"))},
			current:     map[string]any{"Videos": list(item("Name", "hd", "Codec", "h264"))},
			desired:     map[string]any{"Videos": list(item("Name", "hd"))},
			want:        nil,
		},
		{
			name:       "without a record of the last deploy, Equivalent is used",
			equivalent: within,
			current:    map[string]any{"Videos": list(item("Name", "hd", "Codec", "h264"))},
			desired:    map[string]any{"Videos": list(item("Name", "hd"))},
			want:       nil,
		},
		{
			name:        "items reordered while a field moves between them",
			equivalent:  anyOrder,
			lastApplied: map[string]any{"Videos": list(item("Name", "a", "Sharpness", 1.0), item("Name", "b"))},
			current:     map[string]any{"Videos": list(item("Name", "a", "Sharpness", 1.0), item("Name", "b", "Sharpness", 1.0))},
			desired:     map[string]any{"Videos": list(item("Name", "b", "Sharpness", 1.0), item("Name", "a"))},
			want:        []string{"replace /Videos/0/Name", "replace /Videos/1/Name", "remove /Videos/1/Sharpness"},
		},
		{
			name:        "items that look the same",
			equivalent:  within,
			lastApplied: map[string]any{"Videos": list(item("Name", "hd", "Sharpness", 1.0), item("Name", "hd"))},
			current:     map[string]any{"Videos": list(item("Name", "hd", "Sharpness", 1.0), item("Name", "hd", "Sharpness", 1.0))},
			desired:     map[string]any{"Videos": list(item("Name", "hd"), item("Name", "hd", "Sharpness", 1.0))},
			want:        []string{"remove /Videos/0/Sharpness"},
		},
		{
			name:        "items that differ only by a scalar",
			equivalent:  within,
			lastApplied: map[string]any{"Rules": list(item("Id", 1.0, "Sharpness", 75.0), item("Id", 2.0, "Tint", 5.0))},
			current:     map[string]any{"Rules": list(item("Id", 1.0, "Sharpness", 75.0, "Tint", 5.0), item("Id", 2.0, "Sharpness", 75.0, "Tint", 5.0))},
			desired:     map[string]any{"Rules": list(item("Id", 1.0, "Tint", 5.0), item("Id", 2.0, "Sharpness", 75.0))},
			want:        []string{"remove /Rules/0/Sharpness", "remove /Rules/1/Tint"},
		},
		{
			name:        "a field moved between items of a nested list",
			equivalent:  within,
			lastApplied: map[string]any{"Outer": list(item("Inner", list(item("Name", "a", "Sharpness", 1.0), item("Name", "b"))))},
			current:     map[string]any{"Outer": list(item("Inner", list(item("Name", "a", "Sharpness", 1.0), item("Name", "b", "Sharpness", 1.0))))},
			desired:     map[string]any{"Outer": list(item("Inner", list(item("Name", "a"), item("Name", "b", "Sharpness", 1.0))))},
			want:        []string{"remove /Outer/0/Inner/0/Sharpness"},
		},
		{
			name:        "one item added while another is removed",
			equivalent:  extraItems,
			lastApplied: map[string]any{"Apps": list(item("Name", "a", "Version", "1"), item("Name", "b"))},
			current:     map[string]any{"Apps": list(item("Name", "a", "Version", "1"), item("Name", "b", "Version", "1"), item("Name", "c", "Version", "1"))},
			desired:     map[string]any{"Apps": list(item("Name", "b"), item("Name", "c", "Version", "1"))},
			want:        []string{"replace /Apps/0/Name", "remove /Apps/0/Version", "replace /Apps/1/Name", "remove /Apps/2"},
		},
		{
			name:        "an unresolved path is sent though last-applied matches",
			equivalent:  within,
			lastApplied: map[string]any{"Videos": list(item("Name", "hd"))},
			unresolved:  []string{"Videos"},
			current:     map[string]any{"Videos": list(item("Name", "hd", "Sharpness", 75.0))},
			desired:     map[string]any{"Videos": list(item("Name", "hd"))},
			want:        []string{"remove /Videos/0/Sharpness"},
		},
		{
			name:        "a path under an unresolved one is sent",
			equivalent:  version,
			lastApplied: map[string]any{"Config": map[string]any{"Version": "2.1"}},
			unresolved:  []string{"Config"},
			current:     map[string]any{"Config": map[string]any{"Version": "2.1.0"}},
			desired:     map[string]any{"Config": map[string]any{"Version": "2.1"}},
			want:        []string{"replace /Config/Version"},
		},
		{
			name:        "an unresolved path elsewhere doesn't change Equivalent here",
			equivalent:  version,
			lastApplied: map[string]any{"Config": map[string]any{"Version": "2.1"}, "Mode": "fast"},
			unresolved:  []string{"Mode"},
			current:     map[string]any{"Config": map[string]any{"Version": "2.1.0"}, "Mode": "fast"},
			desired:     map[string]any{"Config": map[string]any{"Version": "2.1"}, "Mode": "fast"},
			want:        nil,
		},
		{
			name:        "a changed value is sent once",
			equivalent:  version,
			lastApplied: map[string]any{"EngineVersion": "5.0"},
			current:     map[string]any{"EngineVersion": "6.0.0"},
			desired:     map[string]any{"EngineVersion": "6.0"},
			want:        []string{"replace /EngineVersion"},
		},
		{
			name:        "an object replaced by a list",
			equivalent:  func(string, any, any) bool { return true },
			lastApplied: map[string]any{"Settings": map[string]any{"Mode": "fast"}},
			current:     map[string]any{"Settings": list("a")},
			desired:     map[string]any{"Settings": list("b")},
			want:        []string{"replace /Settings/0"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			currentState, err := json.Marshal(tt.current)
			require.NoError(t, err)
			desiredState, err := json.Marshal(tt.desired)
			require.NoError(t, err)
			opts := PatchOptions{Equivalent: tt.equivalent, LastApplied: tt.lastApplied, Unresolved: tt.unresolved}
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

// Test_GeneratePatchWithOptions_EquivalentItemCorrespondence is the review's example: the template moves Sharpness
// from one item to the other. Items that look alike can't show that no owned field was removed, so Equivalent isn't
// used and the patch removes Sharpness from the first item.
func Test_GeneratePatchWithOptions_EquivalentItemCorrespondence(t *testing.T) {
	t.Parallel()

	lastApplied := map[string]any{"Videos": []any{map[string]any{"Name": "hd", "Sharpness": 75}, map[string]any{"Name": "sd"}}}
	current := map[string]any{"Videos": []any{map[string]any{"Name": "hd", "Sharpness": 75}, map[string]any{"Name": "sd", "Sharpness": 75}}}
	desired := map[string]any{"Videos": []any{map[string]any{"Name": "hd"}, map[string]any{"Name": "sd", "Sharpness": 75}}}

	currentState, err := json.Marshal(current)
	require.NoError(t, err)
	desiredState, err := json.Marshal(desired)
	require.NoError(t, err)
	patch, err := GeneratePatchWithOptions(currentState, desiredState, []byte(`{}`), PatchOptions{
		Equivalent:  func(path string, template any, aws any) bool { return within(template, aws) },
		LastApplied: lastApplied,
		Removed:     RemovedPaths(lastApplied, desired),
	})
	require.NoError(t, err)
	require.Len(t, patch, 1)
	require.Equal(t, "remove", string(patch[0].Type))
	require.Equal(t, "/Videos/0/Sharpness", patch[0].Path)
}

func Test_GeneratePatchWithOptions_FailUncertain(t *testing.T) {
	t.Parallel()

	item := func(fields ...any) map[string]any {
		out := map[string]any{}
		for i := 0; i < len(fields); i += 2 {
			out[fields[i].(string)] = fields[i+1]
		}
		return out
	}
	tests := []struct {
		name          string
		lastApplied   map[string]any
		unresolved    []string
		current       map[string]any
		desired       map[string]any
		failUncertain bool
		wantErr       []string
		want          []string
	}{
		{
			name:          "AWS has values last-applied doesn't show it added, so the type fails",
			lastApplied:   map[string]any{"Videos": []any{item("Name", "hd", "Sharpness", 75.0)}},
			current:       map[string]any{"Videos": []any{item("Name", "hd", "Sharpness", 75.0, "Codec", "h264")}},
			desired:       map[string]any{"Videos": []any{item("Name", "hd")}},
			failUncertain: true,
			wantErr:       []string{"Videos"},
		},
		{
			name:        "by default, the template's value is sent instead",
			lastApplied: map[string]any{"Videos": []any{item("Name", "hd", "Sharpness", 75.0)}},
			current:     map[string]any{"Videos": []any{item("Name", "hd", "Sharpness", 75.0, "Codec", "h264")}},
			desired:     map[string]any{"Videos": []any{item("Name", "hd")}},
			want:        []string{"remove /Videos/0/Sharpness", "remove /Videos/0/Codec"},
		},
		{
			name:          "an unresolved path the template didn't change fails",
			lastApplied:   map[string]any{"Videos": []any{item("Name", "hd")}},
			unresolved:    []string{"Videos"},
			current:       map[string]any{"Videos": []any{item("Name", "hd", "Sharpness", 75.0)}},
			desired:       map[string]any{"Videos": []any{item("Name", "hd")}},
			failUncertain: true,
			wantErr:       []string{"Videos"},
		},
		{
			name:          "AWS has exactly what the last deploy set, so a removal is certain and sent",
			lastApplied:   map[string]any{"Videos": []any{item("Name", "hd", "Sharpness", 75.0, "Codec", "h264")}},
			current:       map[string]any{"Videos": []any{item("Name", "hd", "Sharpness", 75.0, "Codec", "h264")}},
			desired:       map[string]any{"Videos": []any{item("Name", "hd", "Codec", "h264")}},
			failUncertain: true,
			want:          []string{"remove /Videos/0/Sharpness"},
		},
		{
			name:          "an unresolved path can't be certain, even when AWS has the last-applied value",
			lastApplied:   map[string]any{"Videos": []any{item("Name", "hd", "Sharpness", 75.0)}},
			unresolved:    []string{"Videos"},
			current:       map[string]any{"Videos": []any{item("Name", "hd", "Sharpness", 75.0)}},
			desired:       map[string]any{"Videos": []any{item("Name", "hd")}},
			failUncertain: true,
			wantErr:       []string{"Videos"},
		},
		{
			name:          "a value Equivalent doesn't match is a known change, and sent",
			lastApplied:   map[string]any{"Videos": []any{item("Name", "hd")}},
			current:       map[string]any{"Videos": []any{item("Name", "hd", "Codec", "h264")}},
			desired:       map[string]any{"Videos": []any{item("Name", "sd")}},
			failUncertain: true,
			want:          []string{"replace /Videos/0/Name", "remove /Videos/0/Codec"},
		},
		{
			name:          "an unchanged path uses Equivalent and doesn't fail",
			lastApplied:   map[string]any{"Videos": []any{item("Name", "hd")}},
			current:       map[string]any{"Videos": []any{item("Name", "hd", "Codec", "h264")}},
			desired:       map[string]any{"Videos": []any{item("Name", "hd")}},
			failUncertain: true,
			want:          nil,
		},
		{
			name:          "every uncertain path is named, sorted",
			lastApplied:   map[string]any{"B": []any{item("X", 1.0)}, "A": []any{item("X", 1.0)}},
			current:       map[string]any{"B": []any{item("X", 1.0, "D", 1.0)}, "A": []any{item("X", 1.0, "D", 1.0)}},
			desired:       map[string]any{"B": []any{item()}, "A": []any{item()}},
			failUncertain: true,
			wantErr:       []string{"A", "B"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			currentState, err := json.Marshal(tt.current)
			require.NoError(t, err)
			desiredState, err := json.Marshal(tt.desired)
			require.NoError(t, err)
			patch, err := GeneratePatchWithOptions(currentState, desiredState, []byte(`{}`), PatchOptions{
				Equivalent:    func(path string, template any, aws any) bool { return within(template, aws) },
				LastApplied:   tt.lastApplied,
				Unresolved:    tt.unresolved,
				Removed:       RemovedPaths(tt.lastApplied, tt.desired),
				FailUncertain: tt.failUncertain,
			})
			if tt.wantErr != nil {
				var uncertain *UncertainError
				require.ErrorAs(t, err, &uncertain)
				require.Equal(t, tt.wantErr, uncertain.Paths)
				require.Contains(t, err.Error(), tt.wantErr[0])
				return
			}
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

func Test_GeneratePatchWithOptions_LastAppliedNotJSON(t *testing.T) {
	t.Parallel()

	_, err := GeneratePatchWithOptions([]byte(`{}`), []byte(`{}`), []byte(`{}`), PatchOptions{LastApplied: map[string]any{"A": func() {}}})
	require.ErrorContains(t, err, "unsupported type")
}

// Test_GeneratePatchWithOptions_ShapeConflict checks a template that changes a property's shape, such as a list into
// an object, where AWS still has the old shape. The template's value wins, every time, and AWS's old value doesn't
// survive under the new one.
func Test_GeneratePatchWithOptions_ShapeConflict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current map[string]any
		desired map[string]any
		want    any
	}{
		{
			name:    "a list becomes an object",
			current: map[string]any{"Config": []any{map[string]any{"Name": "hd", "Sharpness": 75.0}}},
			desired: map[string]any{"Config": map[string]any{"Items": []any{map[string]any{"Name": "hd"}}}},
			want:    map[string]any{"Items": []any{map[string]any{"Name": "hd"}}},
		},
		{
			name:    "an object becomes a list",
			current: map[string]any{"Config": map[string]any{"Mode": "fast", "Items": []any{"a"}}},
			desired: map[string]any{"Config": []any{"b"}},
			want:    []any{"b"},
		},
		{
			name:    "an object becomes a scalar",
			current: map[string]any{"Config": map[string]any{"Mode": "fast"}},
			desired: map[string]any{"Config": "off"},
			want:    "off",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			currentState, err := json.Marshal(tt.current)
			require.NoError(t, err)
			desiredState, err := json.Marshal(tt.desired)
			require.NoError(t, err)
			// Map iteration order varies, so a result that depends on it shows up in some runs.
			for range 50 {
				patch, err := GeneratePatchWithOptions(currentState, desiredState, []byte(`{}`), PatchOptions{})
				require.NoError(t, err)
				require.Equal(t, tt.want, applyPatch(t, tt.current, patch)["Config"])
			}
		})
	}
}

// Test_GeneratePatchWithOptions_UnresolvedOmitted checks an unresolved path the template no longer sets, such as a
// list an interrupted update was changing into an object. AWS may still have the old value, which Radius set, so the
// patch clears it.
// Test_GeneratePatchWithOptions_UnresolvedOmittedKeepsAWSFields is the independent review's example: an update of
// unknown outcome changed a list into an object, and AWS added a field inside the object. The template then leaves the
// property out. The field Radius set is removed, and the one AWS added stays.
func Test_GeneratePatchWithOptions_UnresolvedOmittedKeepsAWSFields(t *testing.T) {
	t.Parallel()

	current := map[string]any{"Config": map[string]any{"Owned": "x", "AWSOnly": "keep"}}
	lastApplied := map[string]any{"Config": map[string]any{"Owned": "x"}}
	desired := map[string]any{}
	currentState, err := json.Marshal(current)
	require.NoError(t, err)
	patch, err := GeneratePatchWithOptions(currentState, []byte(`{}`), []byte(`{}`), PatchOptions{
		LastApplied: lastApplied,
		Unresolved:  []string{"Config", "Config/Owned"},
		Removed:     RemovedPaths(lastApplied, desired),
	})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"Config": map[string]any{"AWSOnly": "keep"}}, applyPatch(t, current, patch))
}

func Test_GeneratePatchWithOptions_UnresolvedOmitted(t *testing.T) {
	t.Parallel()

	current := map[string]any{"Config": []any{map[string]any{"Name": "hd", "Sharpness": 75.0}}, "Mode": "fast", "Settings": map[string]any{"A": "1", "B": "aws"}}
	lastApplied := map[string]any{"Config": map[string]any{"Items": []any{map[string]any{"Name": "hd"}}}, "Mode": "fast", "Settings": map[string]any{"A": "1"}}
	tests := []struct {
		name       string
		desired    map[string]any
		unresolved []string
		want       map[string]any
	}{
		{
			name:       "the template leaves the property out",
			desired:    map[string]any{"Mode": "fast", "Settings": map[string]any{"A": "1"}},
			unresolved: []string{"Config", "Config/Items"},
			want:       map[string]any{"Mode": "fast", "Settings": map[string]any{"A": "1", "B": "aws"}},
		},
		{
			name:       "the template sets an empty object",
			desired:    map[string]any{"Mode": "fast", "Config": map[string]any{}, "Settings": map[string]any{"A": "1"}},
			unresolved: []string{"Config"},
			want:       map[string]any{"Mode": "fast", "Settings": map[string]any{"A": "1", "B": "aws"}},
		},
		{
			name:       "without unresolved paths, AWS's value the template leaves out is kept, as before",
			desired:    map[string]any{"Mode": "fast", "Settings": map[string]any{"A": "1"}},
			unresolved: nil,
			want:       current,
		},
		{
			name:       "an unresolved object the template still sets keeps a field AWS set inside it",
			desired:    map[string]any{"Mode": "fast", "Config": []any{map[string]any{"Name": "hd", "Sharpness": 75.0}}, "Settings": map[string]any{"A": "1"}},
			unresolved: []string{"Settings"},
			want:       map[string]any{"Mode": "fast", "Config": []any{map[string]any{"Name": "hd", "Sharpness": 75.0}}, "Settings": map[string]any{"A": "1", "B": "aws"}},
		},
		{
			name:       "an unresolved path the template sets inside isn't removed",
			desired:    map[string]any{"Mode": "fast", "Config": []any{"x"}, "Settings": map[string]any{"A": "1"}},
			unresolved: []string{"Config/Items"},
			want:       map[string]any{"Mode": "fast", "Config": []any{"x"}, "Settings": map[string]any{"A": "1", "B": "aws"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			currentState, err := json.Marshal(current)
			require.NoError(t, err)
			desiredState, err := json.Marshal(tt.desired)
			require.NoError(t, err)
			patch, err := GeneratePatchWithOptions(currentState, desiredState, []byte(`{}`), PatchOptions{
				LastApplied: lastApplied,
				Unresolved:  tt.unresolved,
				Removed:     RemovedPaths(lastApplied, tt.desired),
			})
			require.NoError(t, err)
			require.Equal(t, normalize(t, tt.want), applyPatch(t, current, patch))
		})
	}
}
