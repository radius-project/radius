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
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/worker"
	"github.com/radius-project/radius/pkg/armrpc/frontend/server"
)

// ResourceKind represents the kind of resource.
type ResourceKind string

const (
	// NamespaceResourceKind represents the namespace resource kind.
	NamespaceResourceKind ResourceKind = "Namespace"

	// TrackedResourceKind represents the tracked resource kind.
	TrackedResourceKind ResourceKind = "TrackedResource"

	// ProxyResourceKind represents the proxy resource kind.
	ProxyResourceKind ResourceKind = "ProxyResource"
)

// ResourceOptionBuilder is the interface for resource option.
type ResourceOptionBuilder interface {
	// LinkResource links the resource node to the resource option.
	LinkResource(*ResourceNode)

	// ParamName gets the resource name for resource type.
	ParamName() string

	// BuildHandlerOutputs builds the resource outputs which constructs the API routing path and handlers.
	BuildHandlerOutputs(BuildOptions) []*OperationRegistration
}

// BuildOptions is the options for building resource outputs.
type BuildOptions struct {
	// ResourceType represents the resource type.
	ResourceType string

	// ParameterName represents the resource name for resource type.
	ParameterName string

	// ResourceNamePattern represents the resource name pattern used for HTTP routing path.
	ResourceNamePattern string
}

// OperationRegistration is the output for building resource outputs.
type OperationRegistration struct {
	// ResourceType represents the resource type.
	ResourceType string

	// ResourceNamePattern represents the resource name pattern used for HTTP routing path.
	ResourceNamePattern string

	// Path represents additional custom action path after resource name.
	Path string

	// Method represents the operation method.
	Method v1.OperationMethod

	// APIController represents the API controller handler.
	APIController server.ControllerFactoryFunc

	// AsyncController represents the async controller handler.
	AsyncController worker.ControllerFactoryFunc
}
