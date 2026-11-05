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

package emr

import (
	"reflect"
	"strings"
)

// equivalent treats values EMR returns in another form as the template's:
//
//   - LogUri: EMR returns an s3:// location as s3n://, and may add a trailing "/".
//   - Applications: EMR returns every application it installed, with its version, and may name it in another case.
//     Each application the template lists must be among them.
//   - Instances/TaskInstanceGroups: as other lists, but EMR changes the count of a group with an automatic scaling
//     policy, so its count isn't compared.
//   - Other lists of objects, such as instance groups, configurations, and placement groups: EMR fills in defaults,
//     such as an instance group's Market or EBS volumes, and may return the items in another order. Each item the
//     template lists must match its own item in AWS's list, on the values the template sets.
func equivalent(path string, template any, aws any) bool {
	switch path {
	case "LogUri":
		t, okT := template.(string)
		a, okA := aws.(string)
		return okT && okA && s3Location(t) == s3Location(a)
	case "Applications":
		return covers(lowerNames(template), lowerNames(aws), true)
	case "Tags":
		return false
	case "Instances/TaskInstanceGroups":
		return scaledGroupsCover(template, aws)
	}
	if !objectList(template) || !objectList(aws) {
		return false
	}
	return covers(template, aws, false)
}

// s3Location returns an S3 location without its scheme and trailing "/".
func s3Location(uri string) string {
	for _, scheme := range []string{"s3://", "s3n://", "s3a://"} {
		if rest, ok := strings.CutPrefix(uri, scheme); ok {
			return strings.TrimSuffix(rest, "/")
		}
	}
	return strings.TrimSuffix(uri, "/")
}

func objectList(v any) bool {
	list, ok := v.([]any)
	if !ok || len(list) == 0 {
		return false
	}
	for _, item := range list {
		if _, ok := item.(map[string]any); !ok {
			return false
		}
	}
	return true
}

// lowerNames returns a copy of a list of applications with their names in lowercase.
func lowerNames(v any) any {
	list, ok := v.([]any)
	if !ok {
		return v
	}
	out := make([]any, len(list))
	for i, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			out[i] = item
			continue
		}
		copied := map[string]any{}
		for k, value := range m {
			copied[k] = value
		}
		if name, ok := m["Name"].(string); ok {
			copied["Name"] = strings.ToLower(name)
		}
		out[i] = copied
	}
	return out
}

// covers reports whether aws has every value template sets. An object covers another when it has each of its
// properties with a covering value. A list of objects covers another of the same length when each item has its own
// covering item, in any order, or of any length when extra is true. Other values must be equal.
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
		if !objectList(t) {
			return reflect.DeepEqual(t, a)
		}
		matches := make([][]bool, len(t))
		for i := range t {
			matches[i] = make([]bool, len(a))
			for j := range a {
				matches[i][j] = covers(t[i], a[j], false)
			}
		}
		_, ok = matching(matches)
		return ok
	}
	return reflect.DeepEqual(template, aws)
}

// matching pairs every template item i with its own AWS item, where matches[i][j] reports whether template item i
// matches AWS item j. It returns the AWS item of each template item, and false when there is no such pairing.
func matching(matches [][]bool) ([]int, bool) {
	owner := map[int]int{}
	var assign func(i int, seen map[int]bool) bool
	assign = func(i int, seen map[int]bool) bool {
		for j, ok := range matches[i] {
			if !ok || seen[j] {
				continue
			}
			seen[j] = true
			if k, taken := owner[j]; !taken || assign(k, seen) {
				owner[j] = i
				return true
			}
		}
		return false
	}
	for i := range matches {
		if !assign(i, map[int]bool{}) {
			return nil, false
		}
	}
	pairs := make([]int, len(matches))
	for j, i := range owner {
		pairs[i] = j
	}
	return pairs, true
}

// scaledGroupsCover reports whether aws, the task instance groups AWS returns, covers template, the template's,
// leaving out the count of each AWS group with an automatic scaling policy.
func scaledGroupsCover(template any, aws any) bool {
	t, okT := template.([]any)
	a, okA := aws.([]any)
	if !okT || !okA || len(t) != len(a) {
		return false
	}
	matches := make([][]bool, len(t))
	for i := range t {
		matches[i] = make([]bool, len(a))
		for j := range a {
			ti, aj := t[i], a[j]
			if asMap(aj)["AutoScalingPolicy"] != nil {
				ti, aj = withoutCount(ti), withoutCount(aj)
			}
			matches[i][j] = covers(ti, aj, false)
		}
	}
	_, ok := matching(matches)
	return ok
}
