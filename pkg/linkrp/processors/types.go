// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package processors

import (
	"context"

	"github.com/project-radius/radius/pkg/recipes"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

// ResourceProcessor is responsible for processing the results of recipe execution or any
// other change to the lifecycle of a link resource. Each resource processor supports a single
// Radius resource type (eg: RedisCache).
type ResourceProcessor[P interface {
	*T
	rpv1.RadiusResourceModel
}, T any] interface {
	// Process is called to process the results of recipe execution or any other changes to the resource
	// data model. Process should modify the datamodel in place to perform updates.
	Process(ctx context.Context, resource P, output *recipes.RecipeOutput) error
}

//go:generate mockgen -destination=./mock_resourceclient.go -package=processors -self_package github.com/project-radius/radius/pkg/linkrp/processors github.com/project-radius/radius/pkg/linkrp/processors ResourceClient

// ResourceClient is a client used by resource processors for interacting with UCP resources.
type ResourceClient interface {
	// Get retrieves a resource by id. Populate the 'obj' parameter with a reference to the desired type.
	Get(ctx context.Context, id string, obj any) error

	// Delete deletes a resource by id.
	Delete(ctx context.Context, id string) error
}
