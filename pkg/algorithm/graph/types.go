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

package graph

type DependencyGraph struct {
	// We store the keys in sorted order so that our operations are deterministic.
	keys      []string
	setsByKey map[string]set
}

// # Function Explanation
//
// Lookup returns the DependencySet associated with the given key if it exists, and a boolean indicating
// whether the key was found.
func (dg DependencyGraph) Lookup(key string) (DependencySet, bool) {
	item, ok := dg.setsByKey[key]
	return item, ok
}

type DependencyItem interface {
	Key() string
	GetDependencies() ([]string, error)
}

type DependencySet interface {
	Key() string
	Item() DependencyItem
	GetDirectDependencies() []DependencySet
	GetTransitiveDependencies() []DependencySet
}

type set struct {
	setsByKey    map[string]set
	item         DependencyItem
	dependencies []string
}

// # Function Explanation
//
// Key returns the key of the item in the set.
func (s set) Key() string {
	return s.item.Key()
}

// # Function Explanation
//
// Item returns the DependencyItem stored in the set.
func (s set) Item() DependencyItem {
	return s.item
}

// # Function Explanation
//
// GetDirectDependencies returns a slice of DependencySet objects that are direct dependencies of the set.
func (s set) GetDirectDependencies() []DependencySet {
	results := []DependencySet{}
	for _, d := range s.dependencies {
		other := s.setsByKey[d]
		results = append(results, other)
	}

	return results
}

// # Function Explanation
//
// GetTransitiveDependencies returns a slice of DependencySet objects that are transitively dependent on the set.
func (s set) GetTransitiveDependencies() []DependencySet {
	transitive := map[string]bool{
		// Start with 'self' as part of the set to break cycles, we'll remove it later.
		s.Key(): true,
	}

	for _, d := range s.dependencies {
		other := s.setsByKey[d]
		other.walk(transitive)
	}

	// Remove 'self' as part of the set
	delete(transitive, s.Key())

	results := []DependencySet{}
	for d := range transitive {
		other := s.setsByKey[d]
		results = append(results, other)
	}

	return results
}

// Depth-first, preorder traversal
func (s set) walk(names map[string]bool) {
	_, ok := names[s.Key()]
	if ok {
		// Already part of the set
		return
	}

	names[s.Key()] = true

	for _, d := range s.dependencies {
		other := s.setsByKey[d]
		other.walk(names)
	}
}
