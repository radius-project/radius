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

package radius

import (
	"github.com/go-chi/chi/v5"
	"github.com/radius-project/radius/pkg/ucp/frontend/modules"
	"github.com/radius-project/radius/pkg/validator"
)

// NewModule creates a new Radius module.
func NewModule(options modules.Options) *Module {
	router := chi.NewRouter()
	router.NotFound(validator.APINotFoundHandler())
	router.MethodNotAllowed(validator.APIMethodNotAllowedHandler())

	return &Module{options: options, router: router}
}

var _ modules.Initializer = &Module{}

// Module defines the module for Radius functionality.
type Module struct {
	options modules.Options
	router  chi.Router
}

// PlaneType returns the type of plane this module is for.
func (m *Module) PlaneType() string {
	return "radius"
}
