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
	"reflect"
	"slices"
	"strings"
)

// schemaRules are the rules of a CloudFormation resource type schema for values AWS reports in a different form than
// the template set them:
//
//   - Arrays the schema marks "insertionOrder": false are compared ignoring order, as multisets.
//   - A property with a "propertyTransform" matches when AWS's value matches the transformed template value. See
//     transform for the supported expressions.
//
// Paths are relative to the properties object, use "/" between segments, and "*" for the items of an array.
type schemaRules struct {
	unordered  map[string]bool
	transforms map[string]*transform
}

type schemaNode struct {
	Ref            string                `json:"$ref"`
	InsertionOrder *bool                 `json:"insertionOrder"`
	Items          *schemaNode           `json:"items"`
	Properties     map[string]schemaNode `json:"properties"`
	OneOf          []schemaNode          `json:"oneOf"`
	AnyOf          []schemaNode          `json:"anyOf"`
	AllOf          []schemaNode          `json:"allOf"`
}

type rulesSchema struct {
	Properties  map[string]schemaNode `json:"properties"`
	Definitions map[string]schemaNode `json:"definitions"`
}

type transformsSchema struct {
	PropertyTransform map[string]any `json:"propertyTransform"`
}

// maxSchemaDepth bounds the walk through nested and recursive definitions.
const maxSchemaDepth = 12

// parseSchemaRules reads the unordered arrays and the property transforms of a CloudFormation resource type schema.
// A part of the schema in a shape it does not expect is skipped, so those properties are compared as they are.
func parseSchemaRules(schema []byte) *schemaRules {
	r := &schemaRules{unordered: map[string]bool{}, transforms: map[string]*transform{}}

	parsed := rulesSchema{}
	if err := json.Unmarshal(schema, &parsed); err == nil {
		for name, node := range parsed.Properties {
			r.walk(parsed.Definitions, node, name, 0)
		}
	}

	transforms := transformsSchema{}
	if err := json.Unmarshal(schema, &transforms); err == nil {
		for pointer, value := range transforms.PropertyTransform {
			path, ok := strings.CutPrefix(pointer, "/properties/")
			expression, isString := value.(string)
			if !ok || !isString {
				continue
			}
			// A transform with an unsupported function or syntax is left out, so the property is compared as it is.
			if t, err := parseTransform(expression); err == nil {
				r.transforms[path] = t
			}
		}
	}
	return r
}

func (r *schemaRules) walk(definitions map[string]schemaNode, node schemaNode, path string, depth int) {
	if depth > maxSchemaDepth {
		return
	}
	if ref, ok := strings.CutPrefix(node.Ref, "#/definitions/"); ok {
		if def, found := definitions[ref]; found {
			// Fields set next to $ref, such as insertionOrder, apply too.
			if node.InsertionOrder != nil && def.InsertionOrder == nil {
				def.InsertionOrder = node.InsertionOrder
			}
			r.walk(definitions, def, path, depth+1)
		}
		return
	}
	if node.InsertionOrder != nil && !*node.InsertionOrder {
		r.unordered[path] = true
	}
	if node.Items != nil {
		r.walk(definitions, *node.Items, path+"/*", depth+1)
	}
	for name, child := range node.Properties {
		r.walk(definitions, child, path+"/"+name, depth+1)
	}
	for _, branches := range [][]schemaNode{node.OneOf, node.AnyOf, node.AllOf} {
		for _, branch := range branches {
			r.walk(definitions, branch, path, depth+1)
		}
	}
}

// equivalentAt reports whether the template's value of the property at path and the value AWS reports mean the same
// thing. Values are JSON decoded into any, as GeneratePatch reads them. desiredRoot is the template's whole properties
// object, which property transforms read.
func (r *schemaRules) equivalentAt(path string, desired any, current any, desiredRoot map[string]any) bool {
	if reflect.DeepEqual(desired, current) {
		return true
	}
	if r == nil || (len(r.unordered) == 0 && len(r.transforms) == 0) {
		return false
	}
	segments := strings.Split(path, "/")
	parent, _ := valueAt(desiredRoot, segments[:len(segments)-1])
	parentMap, _ := parent.(map[string]any)
	return r.equivalent(segments, desired, current, parentMap, desiredRoot)
}

func (r *schemaRules) equivalent(segments []string, desired any, current any, parent map[string]any, root map[string]any) bool {
	if reflect.DeepEqual(desired, current) {
		return true
	}
	path := strings.Join(segments, "/")
	switch d := desired.(type) {
	case []any:
		c, ok := current.([]any)
		if !ok || len(d) != len(c) {
			return false
		}
		items := append(append([]string{}, segments...), "*")
		if !r.unordered[path] {
			for i := range d {
				if !r.equivalent(items, d[i], c[i], nil, root) {
					return false
				}
			}
			return true
		}
		matches := make([][]bool, len(d))
		for i := range d {
			matches[i] = make([]bool, len(c))
			for j := range c {
				matches[i][j] = r.equivalent(items, d[i], c[j], nil, root)
			}
		}
		return perfectMatching(matches)
	case map[string]any:
		c, ok := current.(map[string]any)
		if !ok {
			return false
		}
		for k := range c {
			if _, set := d[k]; !set && c[k] != nil {
				return false
			}
		}
		for k, v := range d {
			if !r.equivalent(append(append([]string{}, segments...), k), v, c[k], d, root) {
				return false
			}
		}
		return true
	default:
		t := r.transforms[path]
		return t != nil && t.matches(parent, root, current)
	}
}

// listFields says which fields inside list items fillAt can take from AWS.
type listFields struct {
	// writeOnly are the schema's write-only paths, relative to the properties object, such as Users/*/Password. AWS
	// never returns them, so the template's value is kept, and the fingerprints decide whether to send it.
	writeOnly []string

	// awsOwned reports whether a field AWS has inside a list item and the template doesn't set, at a path with "*" for
	// the items of a list, such as VpcConfigs/*/SecurityGroups, is AWS's, so it stays. Otherwise the patch removes it.
	awsOwned func(path string) bool
}

// fillAt returns the template's value of the list at path with the fields AWS filled in inside its items, such as a
// default security group, and whether every value the template sets inside the list matches AWS's, by the schema's
// rules. It doesn't match when AWS lacks an item or a field the template sets: then the template's value is sent.
// desiredRoot is the template's whole properties object, which property transforms read.
func (r *schemaRules) fillAt(path string, desired []any, current any, desiredRoot map[string]any, fields listFields) (any, bool) {
	segments := strings.Split(path, "/")
	parent, _ := valueAt(desiredRoot, segments[:len(segments)-1])
	parentMap, _ := parent.(map[string]any)
	return r.fill(segments, desired, current, parentMap, desiredRoot, fields)
}

func (r *schemaRules) fill(segments []string, desired any, current any, parent map[string]any, root map[string]any, fields listFields) (any, bool) {
	if r.equivalent(segments, desired, current, parent, root) {
		return current, true
	}
	switch d := desired.(type) {
	case []any:
		c, ok := current.([]any)
		if !ok || len(d) != len(c) {
			return nil, false
		}
		items := append(slices.Clone(segments), "*")
		out := make([]any, len(c))
		if !r.unordered[strings.Join(segments, "/")] {
			for i := range d {
				if out[i], ok = r.fill(items, d[i], c[i], nil, root, fields); !ok {
					return nil, false
				}
			}
			return out, true
		}
		filled := make([][]any, len(d))
		matches := make([][]bool, len(d))
		for i := range d {
			filled[i] = make([]any, len(c))
			matches[i] = make([]bool, len(c))
			for j := range c {
				filled[i][j], matches[i][j] = r.fill(items, d[i], c[j], nil, root, fields)
			}
		}
		owner, ok := matchItems(matches)
		if !ok {
			return nil, false
		}
		// Keep AWS's order, so items that match produce no patch.
		for j, i := range owner {
			out[j] = filled[i][j]
		}
		return out, true
	case map[string]any:
		c, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		out := map[string]any{}
		for k, v := range d {
			field := append(slices.Clone(segments), k)
			cv, has := c[k]
			switch {
			case has:
				if out[k], ok = r.fill(field, v, cv, d, root, fields); !ok {
					return nil, false
				}
			case v == nil:
			case underAny(strings.Join(field, "/"), fields.writeOnly):
				out[k] = v
			default:
				return nil, false
			}
		}
		for k, cv := range c {
			if _, set := d[k]; !set && fields.awsOwned(strings.Join(append(slices.Clone(segments), k), "/")) {
				out[k] = cv
			}
		}
		return out, true
	}
	return nil, false
}

// perfectMatching reports whether every template item can be paired with its own AWS item, where matches[i][j]
// reports whether template item i matches AWS item j. An item can match more than one counterpart when transforms
// overlap, such as "8.4" and "8.4.11" both matching "8.4.11", so an earlier pairing moves when that frees a match for
// a later item (augmenting paths).
func perfectMatching(matches [][]bool) bool {
	_, ok := matchItems(matches)
	return ok
}

// matchItems pairs every template item with its own AWS item, as perfectMatching does. It returns the template item
// paired with each AWS item, and whether every template item has one.
func matchItems(matches [][]bool) (map[int]int, bool) {
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
	return owner, true
}

// valueAt returns the value at the path given by segments inside obj.
func valueAt(obj any, segments []string) (any, bool) {
	current := obj
	for _, segment := range segments {
		// A value that is not an object has no fields: the lookup on a nil map finds nothing.
		m, _ := current.(map[string]any)
		var ok bool
		if current, ok = m[segment]; !ok {
			return nil, false
		}
	}
	return current, true
}
