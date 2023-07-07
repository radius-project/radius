package server

import (
	"time"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/asyncoperation/worker"
	apictrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
)

type ResourceTypeOption[T any] struct {
	// RequestConverter is the request converter.
	RequestConverter v1.ConvertToDataModel[T]

	// ResponseConverter is the response converter.
	ResponseConverter v1.ConvertToAPIModel[T]

	ListHandler   *ActionHandler[T]
	GetHandler    *ActionHandler[T]
	PutHandler    *ActionHandler[T]
	PatchHandler  *ActionHandler[T]
	DeleteHandler *ActionHandler[T]
}

type ActionHandler[T any] struct {
	APIHandler func(apictrl.Options) (apictrl.Controller, error)

	// DeleteFilters is a slice of filters that execute prior to deleting a resource.
	DeleteFilters []apictrl.DeleteFilter[T]

	// UpdateFilters is a slice of filters that execute prior to updating a resource.
	UpdateFilters []apictrl.UpdateFilter[T]

	JobHandler worker.ControllerFactoryFunc

	// AsyncOperationTimeout is the default timeout duration of async put operation.
	AsyncOperationTimeout time.Duration

	// AsyncOperationRetryAfter is the value of the Retry-After header that will be used for async operations.
	// If this is 0 then the default value of v1.DefaultRetryAfter will be used. Consider setting this to a smaller
	// value like 5 seconds if your operations will complete quickly.
	AsyncOperationRetryAfter time.Duration

	// RequestConverter is the request converter.
	RequestConverter v1.ConvertToDataModel[T]

	// ResponseConverter is the response converter.
	ResponseConverter v1.ConvertToAPIModel[T]
}

type ProviderNamespace struct {
	ProviderName string

	ResourceTypes map[string]any
}

func (p *ProviderNamespace) AddResourceType(name string, options any) {
	p.ResourceTypes[name] = options
}
