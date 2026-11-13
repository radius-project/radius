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

package native

import (
	"maps"
	"strings"
)

// Members names the members of a parent that the resource of a type is, for a handler that implements ScopedReader
// and gives each alias its own identifier, the parent and a name joined with "|", such as users added to an IAM group. Two aliases with their own identifiers
// could otherwise list the same member, and deleting either would remove it while the other still lists it. So UCP
// rejects with 409 a PUT that lists a member another alias's record lists for the same parent, and leaves members
// another alias's record lists out of the property sets it gives ReadScoped, Update, and DeleteScoped.
type Members struct {
	// Parent is the top-level property that names the parent, such as GroupName.
	Parent string

	// List is the top-level property that lists the members, such as Users.
	List string

	// Key, for a list of objects, is the field of each item that names the member, such as CertificateArn. It is ""
	// for a list of names.
	Key string

	// IgnoreCase is true when names that differ only by case are the same parent or member in AWS.
	IgnoreCase bool
}

// Of returns the parent and the members of a resource with identifier and properties, each once, in the form UCP
// compares them in. The parent is the one identifier names, as the handler takes it, or Parent of properties for a
// resource with no identifier yet.
func (m *Members) Of(identifier string, properties map[string]any) (string, []string) {
	parent, _, ok := strings.Cut(identifier, "|")
	if !ok || parent == "" {
		parent, _ = properties[m.Parent].(string)
	}
	out := []string{}
	seen := map[string]bool{}
	list, _ := properties[m.List].([]any)
	for _, item := range list {
		if member := m.memberOf(item); member != "" && !seen[member] {
			seen[member] = true
			out = append(out, member)
		}
	}
	return m.fold(parent), out
}

// Without returns properties without the members in drop, or properties itself when it lists none of them.
func (m *Members) Without(properties map[string]any, drop map[string]bool) map[string]any {
	list, ok := properties[m.List].([]any)
	if !ok || len(drop) == 0 {
		return properties
	}
	kept := []any{}
	for _, item := range list {
		if !drop[m.memberOf(item)] {
			kept = append(kept, item)
		}
	}
	if len(kept) == len(list) {
		return properties
	}
	out := maps.Clone(properties)
	out[m.List] = kept
	return out
}

func (m *Members) memberOf(item any) string {
	if m.Key != "" {
		object, _ := item.(map[string]any)
		item = object[m.Key]
	}
	name, _ := item.(string)
	return m.fold(name)
}

func (m *Members) fold(name string) string {
	if m.IgnoreCase {
		return strings.ToLower(name)
	}
	return name
}
