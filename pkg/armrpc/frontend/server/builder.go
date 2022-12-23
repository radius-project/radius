// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package server

import (
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/frontend/controller"
)

type OperationHandler struct {
	ResourceType string
	Method       v1.OperationMethod
	Controller   controller.Controller
}

type ResourceBuilder[P interface {
	*TResource
	conv.ResourceDataModel
}, TResource any, TOptions any] struct {
	resourceType  string
	options       TOptions
	deleteFilters []controller.DeleteFilter[TResource]
	updateFilters []controller.UpdateFilter[TResource]
}

func NewResourceBuilder[P interface {
	*TResource
	conv.ResourceDataModel
}, TResource any, TOptions any](resourceType string, controllerOptions controller.Options, options TOptions) *ResourceBuilder[P, TResource, TOptions] {
	return &ResourceBuilder[P, TResource, TOptions]{resourceType: resourceType, options: options}
}

func (b *ResourceBuilder[P, TResource, TOptions]) AddDeleteFilter(filter func(options TOptions) controller.DeleteFilter[TResource]) *ResourceBuilder[P, TResource, TOptions] {
	builder := *b
	builder.deleteFilters = append(b.deleteFilters, filter(b.options))
	return &builder
}

func (b *ResourceBuilder[P, TResource, TOptions]) AddUpdateFilter(filter func(options TOptions) controller.UpdateFilter[TResource]) *ResourceBuilder[P, TResource, TOptions] {
	builder := *b
	builder.updateFilters = append(b.updateFilters, filter(b.options))
	return &builder
}

func (b *ResourceBuilder[P, TResource, TOptions]) Handlers() []OperationHandler {
	handlers := []OperationHandler{
		// Code to build the handlers/operations runs here.
	}

	return handlers
}
