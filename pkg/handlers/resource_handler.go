// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"

	"github.com/Azure/radius/pkg/radrp/db"
	"github.com/Azure/radius/pkg/radrp/outputresource"
)

type PutOptions struct {
	Application string
	Component   string
	Resource    *outputresource.OutputResource

	Dependencies []Dependency

	// ExistingOutputResource is the current state of the output resource persisted in database
	ExistingOutputResource *db.OutputResource

	// DependencyProperties is a map of output resource localID to resource properties populated during deployment in the resource handler
	DependencyProperties map[string]map[string]string
}

type DeleteOptions struct {
	Application string
	Component   string

	// ExistingOutputResource is the current state of the output resource persisted in database
	ExistingOutputResource *db.OutputResource
}

// ResourceHandler interface defines the methods that every output resource will implement
//go:generate mockgen -destination=./mock_resource_handler.go -package=handlers -self_package github.com/Azure/radius/pkg/handlers github.com/Azure/radius/pkg/handlers ResourceHandler
type ResourceHandler interface {
	Put(ctx context.Context, options *PutOptions) (map[string]string, error)
	Delete(ctx context.Context, options DeleteOptions) error
}

type Dependency struct {
	LocalID    string
	Properties map[string]string
}
