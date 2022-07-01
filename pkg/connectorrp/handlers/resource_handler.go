// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"

	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcemodel"
)

// ResourceHandler interface defines the methods that every output resource will implement
//go:generate mockgen -destination=./mock_resource_handler.go -package=handlers -self_package github.com/project-radius/radius/pkg/connectorrp/handlers github.com/project-radius/radius/pkg/connectorrp/handlers ResourceHandler
type ResourceHandler interface {
	Put(ctx context.Context, resource *outputresource.OutputResource) (resourcemodel.ResourceIdentity, map[string]string, error)
	Delete(ctx context.Context, resource *outputresource.OutputResource) error
}
