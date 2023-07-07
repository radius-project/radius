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

package server

import (
	"time"

	"github.com/go-chi/chi/v5"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/asyncoperation/worker"
	apictrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/frontend/server"
)

type ResourceTypeHandlers struct {
	// RequestConverter is the request converter.
	RequestConverter ConvertToDataModel

	// ResponseConverter is the response converter.
	ResponseConverter ConvertToAPIModel

	List   OperationHandler
	Get    OperationHandler
	Put    OperationHandler
	Patch  OperationHandler
	Delete OperationHandler
	Custom map[string]OperationHandler
}

type OperationHandler interface {
	HandlerOptions() *server.HandlerOptions
}

// ConvertToDataModel is the function to convert to data model.
type ConvertToDataModel func(content []byte, version string) (v1.ResourceDataModel, error)

// ConvertToAPIModel is the function to convert data model to version model.
type ConvertToAPIModel func(model v1.ResourceDataModel, version string) (v1.VersionedModelInterface, error)

type ActionHandle[T any] struct {
	// RequestConverter is the request converter.
	RequestConverter ConvertToDataModel

	// ResponseConverter is the response converter.
	ResponseConverter ConvertToAPIModel

	// DeleteFilters is a slice of filters that execute prior to deleting a resource.
	DeleteFilters []apictrl.DeleteFilter[T]

	// UpdateFilters is a slice of filters that execute prior to updating a resource.
	UpdateFilters []apictrl.UpdateFilter[T]

	APIController server.ControllerFunc

	JobHandler worker.ControllerFactoryFunc

	// AsyncOperationTimeout is the default timeout duration of async put operation.
	AsyncOperationTimeout time.Duration

	// AsyncOperationRetryAfter is the value of the Retry-After header that will be used for async operations.
	// If this is 0 then the default value of v1.DefaultRetryAfter will be used. Consider setting this to a smaller
	// value like 5 seconds if your operations will complete quickly.
	AsyncOperationRetryAfter time.Duration

	DisablePlaneScopeCollection bool
}

func (h *ActionHandle[T]) HandlerOptions() *server.HandlerOptions {
	return nil
}

type ResourceNode struct {
	Type     string
	Name     string
	Children map[string]*ResourceNode

	handlers *ResourceTypeHandlers
}

func (r *ResourceNode) AddChild(name string, handlers *ResourceTypeHandlers) {
	r.Children[name] = &ResourceNode{
		handlers: handlers,
		Name:     name,
		Children: make(map[string]*ResourceNode),
	}
}

type ProviderNamespace struct {
	ResourceNode

	Router chi.Router
}

func (p *ProviderNamespace) Build(apiService *APIService, asyncWorker *AsyncWorker) {
	// build router for frontend controller
	// build async worker controller
}
