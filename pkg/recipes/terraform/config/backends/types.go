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
	"github.com/project-radius/radius/pkg/recipes"
)

//go:generate mockgen -destination=./mock_backend.go -package=backends -self_package github.com/project-radius/radius/pkg/recipes/terraform/config/backends github.com/project-radius/radius/pkg/recipes/terraform/config/backends Backend
type Backend interface {
	BuildBackend(resourceRecipe *recipes.ResourceMetadata) (map[string]any, error)
}
