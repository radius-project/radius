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
// thing. desiredRoot is the template's whole properties object, which property transforms read.
func (r *schemaRules) equivalentAt(path string, desired any, current any, desiredRoot map[string]any) bool {
	if jsonEqual(desired, current) {
		return true
	}
	if r == nil || (len(r.unordered) == 0 && len(r.transforms) == 0) {
		return false
	}
	d, c, root := normalize(desired), normalize(current), normalize(desiredRoot)
	rootMap, _ := root.(map[string]any)
	segments := strings.Split(path, "/")
	parent, _ := valueAt(rootMap, segments[:len(segments)-1])
	parentMap, _ := parent.(map[string]any)
	return r.equivalent(segments, d, c, parentMap, rootMap)
}

func (r *schemaRules) equivalent(segments []string, desired any, current any, parent map[string]any, root map[string]any) bool {
	if jsonEqual(desired, current) {
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
		used := make([]bool, len(c))
		for _, di := range d {
			found := false
			for j, cj := range c {
				if !used[j] && r.equivalent(items, di, cj, nil, root) {
					used[j], found = true, true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
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

func normalize(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return v
	}
	return out
}

// jsonEqual reports whether a and b are the same JSON value.
func jsonEqual(a, b any) bool {
	ab, err1 := json.Marshal(a)
	bb, err2 := json.Marshal(b)
	if err1 != nil || err2 != nil {
		return reflect.DeepEqual(a, b)
	}
	var an, bn any
	_ = json.Unmarshal(ab, &an)
	_ = json.Unmarshal(bb, &bn)
	return reflect.DeepEqual(an, bn)
}

// valueAt returns the value at the path given by segments inside obj.
func valueAt(obj any, segments []string) (any, bool) {
	current := obj
	for _, segment := range segments {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		if current, ok = m[segment]; !ok {
			return nil, false
		}
	}
	return current, true
}
