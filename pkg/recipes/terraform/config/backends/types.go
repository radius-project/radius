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

package backends

import (
	"context"

	"github.com/radius-project/radius/pkg/recipes"
)

//go:generate mockgen -destination=./mock_backend.go -package=backends -self_package github.com/radius-project/radius/pkg/recipes/terraform/config/backends github.com/radius-project/radius/pkg/recipes/terraform/config/backends Backend

// Backend is an interface for generating Terraform backend configurations.
type Backend interface {
	// BuildBackend generates the Terraform backend configuration for the backend.
	// Returns a map of Terraform backend name to values representing the backend configuration.
	// Returns an error if the backend configuration cannot be generated.
	BuildBackend(resourceRecipe *recipes.ResourceMetadata) (map[string]any, error)

	// ValidateBackendExists checks if the Terraform state file backend source exists.
	// For example, for Kubernetes backend, it checks if the Kubernetes secret for Terraform state file exists.
	// returns true if backend is found, false otherwise.
	ValidateBackendExists(ctx context.Context, name string) (bool, error)
}
