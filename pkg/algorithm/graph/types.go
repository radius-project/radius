// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package graph

type DependencyGraph struct {
	// We store the keys in sorted order so that our operations are deterministic.
	keys      []string
	setsByKey map[string]set
}

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

func (s set) Key() string {
	return s.item.Key()
}

func (s set) Item() DependencyItem {
	return s.item
}

func (s set) GetDirectDependencies() []DependencySet {
	results := []DependencySet{}
	for _, d := range s.dependencies {
		other := s.setsByKey[d]
		results = append(results, other)
	}

	return results
}

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
