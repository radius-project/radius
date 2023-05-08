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

package handlers

import (
	"context"

	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

// ResourceHandler interface defines the methods that every output resource will implement
//
//go:generate mockgen -destination=./mock_resource_handler.go -package=handlers -self_package github.com/project-radius/radius/pkg/linkrp/handlers github.com/project-radius/radius/pkg/linkrp/handlers ResourceHandler
type ResourceHandler interface {
	Put(ctx context.Context, resource *rpv1.OutputResource) (resourcemodel.ResourceIdentity, map[string]string, error)
	Delete(ctx context.Context, resource *rpv1.OutputResource) error
}
