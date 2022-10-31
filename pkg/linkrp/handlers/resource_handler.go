// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"

	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp/outputresource"
)

// PutOptions represents the options for ResourceHandler.Put.
type PutOptions struct {
	// Resource represents the rendered resource.
	Resource *outputresource.OutputResource

	// DependencyProperties is a map of output resource localID to resource properties populated during deployment in the resource handler
	DependencyProperties map[string]map[string]string
}

// DeleteOptions represents the options for ResourceHandler.Delete.
type DeleteOptions struct {
	// Resource represents the rendered resource.
	Resource *outputresource.OutputResource
}

// ResourceHandler interface defines the methods that every output resource will implement
//
//go:generate mockgen -destination=./mock_resource_handler.go -package=handlers -self_package github.com/project-radius/radius/pkg/linkrp/handlers github.com/project-radius/radius/pkg/linkrp/handlers ResourceHandler
type ResourceHandler interface {
	Put(ctx context.Context, options *PutOptions) (resourcemodel.ResourceIdentity, map[string]string, error)
	Delete(ctx context.Context, options *DeleteOptions) error
}
