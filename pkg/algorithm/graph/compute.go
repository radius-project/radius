// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package graph

import (
	"fmt"
	"sort"
	"strings"
)

// ComputeDependencyGraph can compute the dependency graph of any type of named item.
func ComputeDependencyGraph(items []DependencyItem) (DependencyGraph, error) {
	setsByKey := map[string]set{}

	for _, item := range items {
		setsByKey[item.Key()] = set{
			setsByKey:    setsByKey,
			item:         item,
			dependencies: item.GetDependencies(),
		}
	}

	// Now validate by walking all dependencies and ensure they exist in the graph
	missing := map[string]bool{}
	for _, item := range items {
		for _, d := range item.GetDependencies() {
			_, ok := setsByKey[d]
			if !ok {
				missing[d] = true
			}
		}
	}

	if len(missing) > 0 {
		// Build a nicely formatted error message
		names := []string{}
		for k := range missing {
			names = append(names, fmt.Sprintf("'%s'", k))
		}

		sort.Strings(names)
		return DependencyGraph{}, fmt.Errorf("the dependency graph has references to the following missing items %s", strings.Join(names, ", "))
	}

	return DependencyGraph{items: items, setsByKey: setsByKey}, nil
}

func (dg DependencyGraph) Order() ([]DependencyItem, error) {
	// Used to indicate members in the ordered set, true means 'in order' false means 'computing order' and
	// can be used to break cycles.
	members := map[string]bool{}

	// Used to store ordered items
	ordered := []DependencyItem{}

	// Starting point doesn't really matter, use original list of items for stable ordering behavior.
	for _, item := range dg.items {
		set := dg.setsByKey[item.Key()]
		err := ensureInDependencyOrder(set, members, &ordered)
		if err != nil {
			return nil, err
		}
	}

	return ordered, nil
}

func ensureInDependencyOrder(set set, members map[string]bool, ordered *[]DependencyItem) error {
	key := set.Key()
	complete, ok := members[key]
	if ok && !complete {
		return fmt.Errorf("a dependency cycle was detected")
	} else if ok {
		// Already in the set
		return nil
	}

	members[key] = false
	for _, d := range set.dependencies {
		other := set.setsByKey[d]
		err := ensureInDependencyOrder(other, members, ordered)
		if err != nil {
			return err
		}
	}

	members[key] = true
	*ordered = append(*ordered, set.Item())
	return nil
}
