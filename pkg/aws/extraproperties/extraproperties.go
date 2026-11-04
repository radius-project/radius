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

// Package extraproperties lists, for each AWS resource type that has them, the read-only properties UCP adds when it
// reads a resource, beyond what Cloud Control returns. UCP reads each value from another AWS call, such as MSK's
// GetBootstrapBrokers, and adds it only to its response: it never sends one to AWS or stores one in a record.
//
// The list is in extra_properties.json, in CloudFormation schema form, so the Bicep type generator in
// radius-project/bicep-types-aws can read the same file and add these properties to each type.
package extraproperties

import (
	_ "embed"
	"encoding/json"
	"maps"
	"slices"
	"strings"
)

// property is the CloudFormation schema of an extra property.
type property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

//go:embed extra_properties.json
var data []byte

// byLowerType maps each lowercase type name to its extra properties.
var byLowerType = mustParse(data)

func mustParse(data []byte) map[string]map[string]property {
	parsed := map[string]map[string]property{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		panic("extra_properties.json is not valid: " + err.Error())
	}
	out := map[string]map[string]property{}
	for typeName, properties := range parsed {
		out[strings.ToLower(typeName)] = properties
	}
	return out
}

// Names returns the sorted names of the extra properties of typeName, or nil when it has none. The type name is
// matched without regard to case.
func Names(typeName string) []string {
	properties := byLowerType[strings.ToLower(typeName)]
	if len(properties) == 0 {
		return nil
	}
	return slices.Sorted(maps.Keys(properties))
}

// Without returns properties without the extra properties of typeName. It returns properties itself when there is
// nothing to remove, and otherwise a copy, so the caller's map never changes.
func Without(typeName string, properties map[string]any) map[string]any {
	names := Names(typeName)
	if !slices.ContainsFunc(names, func(name string) bool { _, ok := properties[name]; return ok }) {
		return properties
	}
	out := maps.Clone(properties)
	for _, name := range names {
		delete(out, name)
	}
	return out
}
