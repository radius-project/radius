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
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
)

// Namespace represents the namespace of UCP.
type Namespace struct {
	ResourceNode

	// availableOperations is the list of available operations for the namespace.
	availableOperations []v1.Operation
}

// NewNamespace creates a new namespace.
func NewNamespace(namespace string) *Namespace {
	return &Namespace{
		ResourceNode: ResourceNode{
			Kind:     NamespaceResourceKind,
			Name:     namespace,
			children: make(map[string]*ResourceNode),
		},
	}
}

// SetAvailableOperations sets the available operations for the namespace.
func (p *Namespace) SetAvailableOperations(operations []v1.Operation) {
	p.availableOperations = operations
}

// GenerateBuilder Builder object by traversing resource nodes from namespace.
func (p *Namespace) GenerateBuilder() Builder {
	return Builder{
		namespaceNode: p,
		registrations: p.resolve(&p.ResourceNode, p.Name, strings.ToLower(p.Name)),
	}
}

func (p *Namespace) resolve(node *ResourceNode, qualifiedType string, qualifiedPattern string) []*OperationRegistration {
	outputs := []*OperationRegistration{}

	newType := qualifiedType
	newPattern := qualifiedPattern

	if node.Kind != NamespaceResourceKind {
		newType = qualifiedType + "/" + node.Name
		newPattern = qualifiedPattern + "/" + strings.ToLower(node.Name)
		newParamName := "{" + node.option.ParamName() + "}"

		// This builds the handler outputs for each resource type.
		ctrls := node.option.BuildHandlerOutputs(BuildOptions{
			ResourceType:        newType,
			ParameterName:       newParamName,
			ResourceNamePattern: newPattern,
		})

		newPattern += "/" + newParamName
		outputs = append(outputs, ctrls...)
	}

	for _, child := range node.children {
		outputs = append(outputs, p.resolve(child, newType, newPattern)...)
	}

	return outputs
}
