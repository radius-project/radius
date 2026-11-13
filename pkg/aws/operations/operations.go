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
	"maps"
	"reflect"
	"strings"

	"github.com/wI2L/jsondiff"
	"golang.org/x/exp/slices"
)

type ResourceTypeSchema struct {
	Properties                      map[string]any `json:"properties,omitempty"`
	ReadOnlyProperties              []string       `json:"readOnlyProperties,omitempty"`
	CreateOnlyProperties            []string       `json:"createOnlyProperties,omitempty"`
	ConditionalCreateOnlyProperties []string       `json:"conditionalCreateOnlyProperties,omitempty"`
	WriteOnlyProperties             []string       `json:"writeOnlyProperties,omitempty"`
}

// FlattenProperties takes a string-keyed map, which may contain nested objects, and recursively flattens it into a single-level map.
// For example:
//
//	"NumShards": 1
//	"ClusterEndpoint": {
//	  "Address": "test-address"
//	  "Port": 3000
//	}
//
// Gets transformed to:
//
//	"NumShards": 1
//	"ClusterEndpoint/Address": "test-address"
//	"ClusterEndpoint/Port": 3000
func FlattenProperties(state map[string]any) map[string]any {
	flattenedState := map[string]any{}

	for k, v := range state {
		// If the value is a map, flatten it
		if v != nil && reflect.TypeOf(v).Kind() == reflect.Map {
			flattenedSubState := FlattenProperties(v.(map[string]any))

			for subK, subV := range flattenedSubState {
				key := k + "/" + subK
				flattenedState[key] = subV
			}
		} else {
			flattenedState[k] = v
		}
	}

	return flattenedState
}

// UnflattenProperties takes in a map of strings to any type and returns an object with the nested
// properties unflattened.
// For example:
//
//	"NumShards": 1
//	"ClusterEndpoint/Address": "test-address"
//	"ClusterEndpoint/Port": 3000
//
// Gets transformed to:
//
//	"NumShards": 1
//	"ClusterEndpoint": {
//	  "Address": "test-address"
//	  "Port": 3000
//	}
func UnflattenProperties(state map[string]any) map[string]any {
	unflattenedState := map[string]any{}

	for k, v := range state {
		splitPath := strings.Split(k, "/")
		rootKey := splitPath[0]

		if len(splitPath) == 1 {
			unflattenedState[rootKey] = v
		} else {
			var currentState any = unflattenedState
			for i := range splitPath {
				subKey := splitPath[i]
				if i == len(splitPath)-1 {
					if currentStateMap, ok := currentState.(map[string]any); ok {
						currentStateMap[subKey] = v
					}
				} else {
					if currentStateMap, ok := currentState.(map[string]any); ok {
						if _, exists := currentStateMap[subKey]; !exists {
							currentStateMap[subKey] = map[string]any{}
						}

						currentState = currentStateMap[subKey]
					}
				}
			}
		}
	}

	return unflattenedState
}

// PatchOptions carries what UCP knows from its record of the last successful deploy of a resource.
type PatchOptions struct {
	// Removed are the property paths the last deploy set that the template no longer sets, relative to the properties
	// object and with "/" between segments, like the keys of FlattenProperties. The patch removes them, except
	// read-only and create-only properties, which AWS cannot clear.
	Removed []string

	// Equivalent, when set, reports whether AWS's value at path, a key of FlattenProperties, means the same as the
	// template's, for a value AWS returns in another form that the schema's rules don't cover. Such a value counts as
	// unchanged. It is used at a path only when Radius knows it owns nothing there that the template drops (see
	// LastApplied and Unresolved).
	Equivalent func(path string, template any, aws any) bool

	// LastApplied is what the last successful deploy set, or nil when UCP has no record of it. Equivalent may count
	// AWS's value as matching when it holds more than the template, such as defaults AWS fills in inside the items of
	// a list, so it can't tell those from a field the last deploy set and the template now leaves out. Removed can't
	// name such a field either when it is inside a list item. So when LastApplied is set, Equivalent is used at a path
	// only when the template's value there is exactly LastApplied's: then the template drops nothing Radius owns
	// there. Otherwise the patch sends the template's value, and AWS clears what it no longer has.
	LastApplied map[string]any

	// Unresolved are paths whose value in LastApplied isn't confirmed in AWS, because an operation that changed them
	// failed partway or UCP can't tell how it ended. AWS may still hold values LastApplied no longer has there, so
	// Equivalent isn't used at a path at, under, or over one of them, and one the template sets nothing at, under, or
	// over is removed at exactly that path.
	Unresolved []string

	// FailUncertain makes GeneratePatchWithOptions return an *UncertainError instead of a patch when it can't tell
	// whether an update is needed at a path: Equivalent counts AWS's value as the template's, but LastApplied doesn't
	// show that AWS added the values beyond the template's (see UncertainError). Set it for a type whose update restarts
	// or interrupts the resource, so an update UCP may not need costs more than a deploy that fails. Otherwise the
	// patch sends the template's value, so a mistake is an update AWS can undo, never a value Radius set left behind.
	FailUncertain bool
}

// UncertainError means GeneratePatchWithOptions can't tell whether an update is needed at Paths, and opts.FailUncertain
// is set. AWS's value there counts as the template's by Equivalent, but holds values LastApplied doesn't show AWS
// added.
type UncertainError struct {
	Paths []string
}

func (e *UncertainError) Error() string {
	return fmt.Sprintf("UCP can't tell whether an update is needed at %s", strings.Join(e.Paths, ", "))
}

// GeneratePatch takes in the current state, desired state and type schema of a resource and returns a JSON patch to update the
// current state to the desired state, taking into account read-only, create-only and conditional-create-only properties.
// It is GeneratePatchWithOptions without a record of the last deploy, so it never removes a property.
func GeneratePatch(currentState []byte, desiredState []byte, schema []byte) (jsondiff.Patch, error) {
	return GeneratePatchWithOptions(currentState, desiredState, schema, PatchOptions{})
}

// GeneratePatchWithOptions returns a JSON patch that updates the current state to the desired state.
//
// A value AWS reports in a different form than the template set it counts as unchanged, using the schema's rules: an
// array the schema marks "insertionOrder": false is compared ignoring order, and a property with a supported
// "propertyTransform" is compared through it. An empty object AWS reports is kept unless the template sets values
// inside it.
//
// A property in the current state that the desired state does not set is kept, because AWS or someone else may have
// set it, unless it is in opts.Removed: Radius set it in the last deploy, so the patch clears it. Inside list items,
// with a record of the last deploy (opts.LastApplied), the same holds for a field AWS has that the template doesn't
// set, such as a default AWS fills in: it is kept unless the last deploy may have set it. A list whose items don't all
// match AWS's on the values the template sets is sent as the template sets it.
func GeneratePatchWithOptions(currentState []byte, desiredState []byte, schema []byte, opts PatchOptions) (jsondiff.Patch, error) {
	// See: https://github.com/radius-project/radius/blob/main/docs/adr/ucp/001-aws-resource-updating.md

	// Get the resource type schema - this will tell us the properties of the
	// resource as well as which properties are read-only, create-only, etc.
	var resourceTypeSchema ResourceTypeSchema
	err := json.Unmarshal(schema, &resourceTypeSchema)
	if err != nil {
		return nil, err
	}

	// Get the current state of the resource
	var currentStateObject map[string]any
	err = json.Unmarshal(currentState, &currentStateObject)
	if err != nil {
		return nil, err
	}
	flattenedCurrentStateObject := FlattenProperties(currentStateObject)
	// FlattenProperties drops an empty object, so a property AWS reports as {}, such as DataProtectionPolicy on a log
	// group, would look removed and every redeploy would send a remove. Keep it as AWS reports it.
	emptyObjectPaths := emptyObjects(currentStateObject, "")
	for path := range emptyObjectPaths {
		flattenedCurrentStateObject[path] = map[string]any{}
	}

	// Get the desired state of the resource
	var desiredStateObject map[string]any
	err = json.Unmarshal(desiredState, &desiredStateObject)
	if err != nil {
		return nil, err
	}
	flattenedDesiredStateObject := FlattenProperties(desiredStateObject)

	rules := parseSchemaRules(schema)
	applied, err := decodedLastApplied(opts.LastApplied)
	if err != nil {
		return nil, err
	}
	var owned map[string]any
	if applied != nil {
		owned = FlattenProperties(applied)
	}
	writeOnly := PropertyPaths(resourceTypeSchema.WriteOnlyProperties)

	// A value AWS reports in a different form, such as reordered subnets or a fuller engine version, is unchanged:
	// use AWS's value so the patch leaves it alone. Equivalent is used only where Radius knows what it owns (see
	// PatchOptions.LastApplied). Elsewhere the template's value is sent, unless the type fails instead.
	uncertain := []string{}
	for k, v := range flattenedDesiredStateObject {
		current, ok := flattenedCurrentStateObject[k]
		if !ok {
			continue
		}
		if rules.equivalentAt(k, v, current, desiredStateObject) {
			flattenedDesiredStateObject[k] = current
			continue
		}
		resolved := !overlapsAny(k, opts.Unresolved)
		known := resolved && ownsSame(owned, k, v)
		if opts.Equivalent != nil {
			if known && opts.Equivalent(k, v, current) {
				flattenedDesiredStateObject[k] = current
				continue
			} else if !known && opts.FailUncertain && !ownsAll(owned, k, current, opts.Unresolved) && opts.Equivalent(k, v, current) {
				uncertain = append(uncertain, k)
				continue
			}
		}
		// AWS may fill in fields inside list items, such as a default security group. With a record of the last
		// deploy, keep each one Radius didn't set: the template's list is the last deploy's, or the last deploy set
		// nothing at that field in any item. Without one, such as on a take-over, UCP can't tell AWS's defaults from
		// values set outside the template, so the template's list is sent.
		list, isList := v.([]any)
		if !isList || !resolved || applied == nil {
			continue
		}
		fields := listFields{
			writeOnly: writeOnly,
			awsOwned:  func(path string) bool { return known || !setsAt(applied, strings.Split(path, "/")) },
		}
		if filled, ok := rules.fillAt(k, list, current, desiredStateObject, fields); ok {
			flattenedDesiredStateObject[k] = filled
		}
	}
	if len(uncertain) > 0 {
		slices.Sort(uncertain)
		return nil, &UncertainError{Paths: uncertain}
	}

	// An unresolved path the template no longer sets may hold a value Radius set, such as a list an interrupted update
	// was changing into an object, so AWS's value at exactly that path is removed. Values AWS has under it aren't:
	// Radius owned the path's value, not fields AWS has since reported inside it.
	omitted := map[string]bool{}
	for _, p := range opts.Unresolved {
		if !setsAny(flattenedDesiredStateObject, p) {
			omitted[p] = true
		}
	}

	// Add read-only and create-only properties from current state to the desired state
	for k, v := range flattenedCurrentStateObject {
		if hasPathUnder(flattenedDesiredStateObject, k) {
			// The template sets values inside k, where AWS has a value of another shape, such as a list the template
			// changed into an object, or an object AWS reports as empty. Copying AWS's value would overwrite them.
			continue
		}

		property := fmt.Sprintf("/properties/%s", k)

		isCreateOnlyProperty := slices.Contains(resourceTypeSchema.CreateOnlyProperties, property)
		isWriteOnlyProperty := slices.Contains(resourceTypeSchema.WriteOnlyProperties, property)

		if _, exists := flattenedDesiredStateObject[k]; !exists && (underAny(k, opts.Removed) || omitted[k]) &&
			!overlapsAny(k, PropertyPaths(resourceTypeSchema.ReadOnlyProperties)) && !overlapsAny(k, PropertyPaths(resourceTypeSchema.CreateOnlyProperties)) {
			// Radius set this property in the last deploy and the template no longer sets it: leave it out, so the
			// patch clears it.
			continue
		}

		// If the property is create-only and write-only, then upsert it to the desired state.
		// this will cause a no-op in the patch since it will exactly match the current state
		if isWriteOnlyProperty && isCreateOnlyProperty {
			flattenedDesiredStateObject[k] = v
		} else if _, exists := flattenedDesiredStateObject[k]; !exists {
			// Preserve all properties from the current state that the user didn't
			// explicitly specify. This prevents generating "remove" operations for
			// properties that AWS may have added since the template was written,
			// and ensures the patch only modifies properties the user explicitly
			// specified. This matches the standard behavior of infrastructure-as-code
			// tools like Terraform and CloudFormation, where unmentioned properties
			// are left unchanged.
			flattenedDesiredStateObject[k] = v
		}
	}

	// Convert desired patch state back into unflattened object
	unflattenedDesiredStateObject := UnflattenProperties(flattenedDesiredStateObject)

	// Marshal desired state into bytes
	updatedDesiredState, err := json.Marshal(unflattenedDesiredStateObject)
	if err != nil {
		return nil, err
	}

	// Calculate the patch based on the current state and the updated desired state
	return jsondiff.CompareJSON(currentState, updatedDesiredState)
}

// decodedLastApplied returns lastApplied decoded from JSON as the template's values are, so they compare equal to the
// same template values. It returns nil when lastApplied is nil.
func decodedLastApplied(lastApplied map[string]any) (map[string]any, error) {
	if lastApplied == nil {
		return nil, nil
	}
	b, err := json.Marshal(lastApplied)
	if err != nil {
		return nil, err
	}
	// b is a marshaled map, so it always decodes.
	decoded := map[string]any{}
	_ = json.Unmarshal(b, &decoded)
	return decoded, nil
}

// setsAt reports whether obj has a value at the path given by segments, where "*" stands for every item of a list.
func setsAt(obj any, segments []string) bool {
	if len(segments) == 0 {
		return obj != nil
	}
	if segments[0] == "*" {
		list, _ := obj.([]any)
		for _, item := range list {
			if setsAt(item, segments[1:]) {
				return true
			}
		}
		return false
	}
	m, _ := obj.(map[string]any)
	v, ok := m[segments[0]]
	return ok && setsAt(v, segments[1:])
}

// setsAny reports whether flattened has a key at, under, or over path.
func setsAny(flattened map[string]any, path string) bool {
	for k := range flattened {
		if overlapsAny(k, []string{path}) {
			return true
		}
	}
	return false
}

// ownsAll reports whether AWS's value at k is exactly what the last deploy set there, confirmed: Radius set every value
// AWS has at k, so any value the template leaves out is one Radius set, and an update is needed.
func ownsAll(owned map[string]any, k string, current any, unresolved []string) bool {
	applied, ok := owned[k]
	return ok && !overlapsAny(k, unresolved) && reflect.DeepEqual(applied, current)
}

// ownsSame reports whether the template's value v at path k drops nothing the last deploy set there: UCP has no
// record of the last deploy (owned is nil), or its value at k is exactly v.
func ownsSame(owned map[string]any, k string, v any) bool {
	if owned == nil {
		return true
	}
	applied, ok := owned[k]
	return ok && reflect.DeepEqual(applied, v)
}

// emptyObjects returns the flattened paths of the empty objects in state, which FlattenProperties leaves out.
func emptyObjects(state map[string]any, prefix string) map[string]bool {
	out := map[string]bool{}
	for k, v := range state {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		if len(m) == 0 {
			out[prefix+k] = true
			continue
		}
		maps.Copy(out, emptyObjects(m, prefix+k+"/"))
	}
	return out
}

// hasPathUnder reports whether flattened has a key inside the object at path.
func hasPathUnder(flattened map[string]any, path string) bool {
	for k := range flattened {
		if strings.HasPrefix(k, path+"/") {
			return true
		}
	}
	return false
}

// RemovedPaths returns the paths of lastApplied, the properties of the last deploy, that desired no longer sets. Paths
// are relative to the properties object, like the keys of FlattenProperties, and the result is sorted. Arrays are one
// value, so a removed array item is not a removed path: the array itself changed.
func RemovedPaths(lastApplied map[string]any, desired map[string]any) []string {
	flatDesired := FlattenProperties(desired)
	out := []string{}
	for path := range FlattenProperties(lastApplied) {
		if _, ok := flatDesired[path]; !ok {
			out = append(out, path)
		}
	}
	slices.Sort(out)
	return out
}

// RemovedCreateOnly returns the create-only properties among removed that the current state still has. AWS cannot
// clear them, so the template has to keep setting them. Removed paths are relative to the properties object, like the
// keys of FlattenProperties, and the result is sorted.
func RemovedCreateOnly(currentState map[string]any, schema []byte, removed []string) ([]string, error) {
	var resourceTypeSchema ResourceTypeSchema
	if err := json.Unmarshal(schema, &resourceTypeSchema); err != nil {
		return nil, err
	}
	createOnly := PropertyPaths(resourceTypeSchema.CreateOnlyProperties)
	readOnly := PropertyPaths(resourceTypeSchema.ReadOnlyProperties)

	current := FlattenProperties(currentState)
	out := []string{}
	for _, r := range removed {
		if !overlapsAny(r, createOnly) || underAny(r, readOnly) || slices.Contains(out, r) {
			continue
		}
		if _, ok := current[r]; ok || hasPathUnder(current, r) {
			out = append(out, r)
		}
	}
	slices.Sort(out)
	return out, nil
}

// PropertyPaths returns the paths of a schema property list, such as createOnlyProperties, relative to the properties
// object: "/properties/A/B" becomes "A/B".
func PropertyPaths(schemaPaths []string) []string {
	out := []string{}
	for _, p := range schemaPaths {
		if name, ok := strings.CutPrefix(p, "/properties/"); ok {
			out = append(out, name)
		}
	}
	return out
}

// overlapsAny reports whether removing the flattened path k removes one of paths or a value inside one: k is one of
// paths, inside one, or holds one. Arrays are one value, so removing an array such as "Members" also removes a
// property inside its items, such as "Members/*/Name". A "*" segment matches any segment.
func overlapsAny(k string, paths []string) bool {
	segments := strings.Split(k, "/")
	for _, p := range paths {
		pattern := strings.Split(p, "/")
		n := min(len(segments), len(pattern))
		match := true
		for i := range n {
			if pattern[i] != "*" && pattern[i] != segments[i] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// underAny reports whether the flattened path k is one of paths or inside one of them.
func underAny(k string, paths []string) bool {
	for _, p := range paths {
		if k == p || strings.HasPrefix(k, p+"/") {
			return true
		}
	}
	return false
}

// ParsePropertyName transforms a propertyIdentifer of the form /properties/<propertyName> to <propertyName>.
// It returns an error if the property identifier is not in the expected format.
func ParsePropertyName(propertyIdentifier string) (string, error) {
	prefix := "/properties/"
	if after, ok := strings.CutPrefix(propertyIdentifier, prefix); ok {
		return after, nil
	}
	return "", fmt.Errorf("property identifier %s is not in the format /properties/<propertyName>", propertyIdentifier)
}
