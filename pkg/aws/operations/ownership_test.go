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
	"errors"
	"fmt"
	"math/rand/v2"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"
)

// Test_GeneratePatchWithOptions_OwnershipAdversarial tries to make an Equivalent that lets AWS's list items hold more
// than the template hide a value Radius set. It generates last-applied, template, and AWS lists from a small alphabet,
// so items repeat, look alike, differ only by a scalar, or reorder, and nests some of them inside another list. AWS
// holds the template's values, the last deploy's values that may remain, and defaults.
//
// For every case, after the patch: when the template's list differs from last-applied, or the path is unresolved,
// AWS has exactly the template's list, so nothing Radius set and the template dropped remains. Otherwise, when AWS
// holds every template value, the patch is empty.
func Test_GeneratePatchWithOptions_OwnershipAdversarial(t *testing.T) {
	t.Parallel()

	equivalents := map[string]func(string, any, any) bool{
		"in order":    func(_ string, template any, aws any) bool { return within(template, aws) },
		"any order":   func(_ string, template any, aws any) bool { return covers(template, aws, false) },
		"extra items": func(_ string, template any, aws any) bool { return covers(template, aws, true) },
	}
	r := rand.New(rand.NewPCG(13664, 2))
	for i := range 3000 {
		applied, template := randomList(r), randomList(r)
		if r.IntN(4) == 0 {
			template = clone(applied)
		}
		if r.IntN(3) == 0 {
			template = shuffled(r, applied)
		}
		aws := awsList(r, applied, template)
		unresolved := []string(nil)
		if r.IntN(5) == 0 {
			unresolved = []string{"V"}
		}
		nested := r.IntN(3) == 0
		wrap := func(list []any) any {
			if nested {
				return []any{map[string]any{"Inner": list}}
			}
			return list
		}
		lastApplied := map[string]any{"V": wrap(applied)}
		desired := map[string]any{"V": wrap(template)}
		current := map[string]any{"V": wrap(aws)}

		for name, equivalent := range equivalents {
			currentState, err := json.Marshal(current)
			require.NoError(t, err)
			desiredState, err := json.Marshal(desired)
			require.NoError(t, err)
			patch, err := GeneratePatchWithOptions(currentState, desiredState, []byte(`{}`), PatchOptions{
				Equivalent:  equivalent,
				LastApplied: lastApplied,
				Unresolved:  unresolved,
				Removed:     RemovedPaths(lastApplied, desired),
			})
			require.NoError(t, err)
			result := applyPatch(t, current, patch)
			describe := fmt.Sprintf("case %d, %s: last-applied %v, template %v, AWS %v, unresolved %v, patch %v", i, name, lastApplied["V"], desired["V"], current["V"], unresolved, patch)

			changed := !reflect.DeepEqual(normalize(t, lastApplied["V"]), normalize(t, desired["V"]))
			switch {
			case changed || unresolved != nil:
				require.Equal(t, normalize(t, desired["V"]), result["V"], describe)
			case equivalent("V", normalize(t, desired["V"]), normalize(t, current["V"])):
				require.Empty(t, patch, describe)
			}
		}
	}
}

// randomList returns a list of one to three items, each with a Name, and maybe a Sharpness and a Tint, from small sets,
// so items often repeat or look alike.
func randomList(r *rand.Rand) []any {
	out := []any{}
	for range 1 + r.IntN(3) {
		item := map[string]any{"Name": []string{"a", "b"}[r.IntN(2)]}
		if r.IntN(2) == 0 {
			item["Sharpness"] = float64(1 + r.IntN(2))
		}
		if r.IntN(3) == 0 {
			item["Tint"] = "warm"
		}
		out = append(out, item)
	}
	return out
}

// awsList returns what AWS may hold: the template's items, each with the last deploy's values at the same position
// that may remain, and a default AWS fills in. Sometimes it is the last deploy's list, as when the update never ran.
func awsList(r *rand.Rand, applied []any, template []any) []any {
	if r.IntN(5) == 0 {
		return clone(applied)
	}
	out := []any{}
	for i, item := range template {
		merged := map[string]any{}
		if i < len(applied) && r.IntN(2) == 0 {
			for k, v := range applied[i].(map[string]any) {
				merged[k] = v
			}
		}
		for k, v := range item.(map[string]any) {
			merged[k] = v
		}
		if r.IntN(2) == 0 {
			merged["Codec"] = "h264"
		}
		out = append(out, merged)
	}
	return out
}

func shuffled(r *rand.Rand, list []any) []any {
	out := clone(list)
	r.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

func clone(list []any) []any {
	b, _ := json.Marshal(list)
	out := []any{}
	_ = json.Unmarshal(b, &out)
	return out
}

func normalize(t *testing.T, v any) any {
	b, err := json.Marshal(v)
	require.NoError(t, err)
	var out any
	require.NoError(t, json.Unmarshal(b, &out))
	return out
}

// applyPatch applies the add, remove, and replace operations of patch to a copy of doc.
func applyPatch(t *testing.T, doc map[string]any, patch jsondiff.Patch) map[string]any {
	t.Helper()
	root := normalize(t, doc)
	for _, op := range patch {
		segments := strings.Split(strings.TrimPrefix(op.Path, "/"), "/")
		for i, s := range segments {
			segments[i] = strings.ReplaceAll(strings.ReplaceAll(s, "~1", "/"), "~0", "~")
		}
		var err error
		root, err = applyAt(root, segments, string(op.Type), normalize(t, op.Value))
		require.NoError(t, err, "applying %v", op)
	}
	return root.(map[string]any)
}

func applyAt(node any, segments []string, op string, value any) (any, error) {
	key := segments[0]
	last := len(segments) == 1
	switch n := node.(type) {
	case map[string]any:
		if last {
			if op == "remove" {
				delete(n, key)
			} else {
				n[key] = value
			}
			return n, nil
		}
		child, err := applyAt(n[key], segments[1:], op, value)
		n[key] = child
		return n, err
	case []any:
		index := len(n)
		if key != "-" {
			var err error
			if index, err = strconv.Atoi(key); err != nil {
				return nil, err
			}
		}
		if last {
			switch op {
			case "remove":
				return append(n[:index:index], n[index+1:]...), nil
			case "add":
				return append(n[:index:index], append([]any{value}, n[index:]...)...), nil
			default:
				n[index] = value
				return n, nil
			}
		}
		child, err := applyAt(n[index], segments[1:], op, value)
		n[index] = child
		return n, err
	}
	return nil, errors.New("the path doesn't exist")
}
