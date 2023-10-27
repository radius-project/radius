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

package processors

import (
	"context"
	"fmt"

	"github.com/radius-project/radius/pkg/recipes"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
)

// ResourceProcessor is responsible for processing the results of recipe execution or any
// other change to the lifecycle of a portable resource. Each resource processor supports a single
// Radius resource type (eg: RedisCache).
type ResourceProcessor[P interface {
	*T
	rpv1.RadiusResourceModel
}, T any] interface {
	// Process is called to process the results of recipe execution or any other changes to the resource
	// data model. Process should modify the datamodel in place to perform updates.
	Process(ctx context.Context, resource P, options Options) error

	// Delete is called to delete all the resources created by the resource processor.
	Delete(ctx context.Context, resource P, options Options) error
}

// Options defines the options passed to the resource processor.
type Options struct {
	// RuntimeConfiguration represents the configuration of the target runtime.
	RuntimeConfiguration recipes.RuntimeConfiguration

	// RecipeOutput represents the output of executing a recipe (may be nil).
	RecipeOutput *recipes.RecipeOutput
}

// ValidationError represents a user-facing validation message reported by the processor.
type ValidationError struct {
	Message string
}

// Error returns a string containing the error message for ValidationError.
func (e *ValidationError) Error() string {
	return e.Message
}

//go:generate mockgen -destination=./mock_resourceclient.go -package=processors -self_package github.com/radius-project/radius/pkg/portableresources/processors github.com/radius-project/radius/pkg/portableresources/processors ResourceClient

// ResourceClient is a client used by resource processors for interacting with UCP resources.
type ResourceClient interface {
	// Delete deletes a resource by id.
	//
	// If the API version is omitted, then an attempt will be made to look up the API version.
	Delete(ctx context.Context, id string) error
}

// ResourceError represents an error that occurred while processing a resource.
type ResourceError struct {
	ID    string
	Inner error
}

// Error returns a string describing the error that occurred when attempting to delete a resource.
func (e *ResourceError) Error() string {
	return fmt.Sprintf("failed to delete resource %q: %v", e.ID, e.Inner)
}

// Unwrap returns the underlying error of ResourceError.
func (e *ResourceError) Unwrap() error {
	return e.Inner
}
