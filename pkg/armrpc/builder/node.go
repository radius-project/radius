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

package builder

import (
	"errors"
	"strings"
)

var (
	// ErrResourceAlreadyExists represents an error when a resource already exists.
	ErrResourceAlreadyExists = errors.New("resource already exists")
)

// ResourceNode is a node in the resource tree.
type ResourceNode struct {
	// Kind is the resource kind.
	Kind ResourceKind

	// Name is the resource name.
	Name string

	// option includes the resource handlers.
	option ResourceOptionBuilder

	// children includes the child resources and custom actions of this resource.
	children map[string]*ResourceNode
}

// AddResource adds a new child resource type and API handlers and returns new resource node.
func (r *ResourceNode) AddResource(name string, option ResourceOptionBuilder) *ResourceNode {
	normalized := strings.ToLower(name)

	if _, ok := r.children[normalized]; ok {
		panic(ErrResourceAlreadyExists)
	}

	child := &ResourceNode{
		Name:     name,
		children: make(map[string]*ResourceNode),
		option:   option,
	}

	switch r.Kind {
	case NamespaceResourceKind:
		child.Kind = TrackedResourceKind

	case TrackedResourceKind:
		child.Kind = ProxyResourceKind

	default:
		child.Kind = ProxyResourceKind
	}

	option.LinkResource(child)
	r.children[normalized] = child

	return child
}
