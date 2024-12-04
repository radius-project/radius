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

package modules

import (
	"context"
	"net/http"
)

// Initializer is an interface that can be implemented by modules that want to provide functionality for a plane.
type Initializer interface {
	// Initialize initializes and returns the http.Handler that will be registered with the router to handle requests for the plane.
	Initialize(ctx context.Context) (http.Handler, error)

	// PlaneType returns the type of plane that the module is providing functionality for. This should match
	// the plane type in the URL path for the plane.
	//
	// Examples:
	//
	// - aws
	// - azure
	// - kubernetes
	// - radius
	PlaneType() string
}
