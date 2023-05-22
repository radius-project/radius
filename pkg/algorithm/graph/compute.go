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

import (
	"fmt"
	"sort"
	"strings"
)

// ComputeDependencyGraph can compute the dependency graph of any type of named item.
func ComputeDependencyGraph(items []DependencyItem) (DependencyGraph, error) {
	setsByKey := map[string]set{}

	keys := []string{}
	for _, item := range items {
		key := item.Key()

		keys = append(keys, key)

		dependencies, err := item.GetDependencies()
		if err != nil {
			return DependencyGraph{}, err
		}

		setsByKey[key] = set{
			setsByKey:    setsByKey,
			item:         item,
			dependencies: dependencies,
		}
	}

	// Now validate by walking all dependencies and ensure they exist in the graph
	missing := map[string]bool{}
	for _, item := range items {
		dependencies, err := item.GetDependencies()
		if err != nil {
			return DependencyGraph{}, err
		}

		for _, d := range dependencies {
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

	// Sort keys so that our operations that need to iterate have a determinisitic order.
	sort.Strings(keys)

	return DependencyGraph{keys: keys, setsByKey: setsByKey}, nil
}

func (dg DependencyGraph) Order() ([]DependencyItem, error) {
	// Used to indicate members in the ordered set, true means 'in order' false means 'computing order' and
	// can be used to break cycles.
	members := map[string]bool{}

	// Used to store ordered items
	ordered := []DependencyItem{}

	// Starting point doesn't really matter, use original list of items for stable ordering behavior.
	for _, key := range dg.keys {
		set := dg.setsByKey[key]
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
