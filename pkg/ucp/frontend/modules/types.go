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

	"github.com/project-radius/radius/pkg/sdk"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/hostoptions"
	queueprovider "github.com/project-radius/radius/pkg/ucp/queue/provider"
	secretprovider "github.com/project-radius/radius/pkg/ucp/secret/provider"
	"github.com/project-radius/radius/pkg/validator"
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

// Options defines the options for a module.
type Options struct {
	// Config is the bootstrap configuration loaded from config file.
	Config *hostoptions.UCPConfig

	// Address is the hostname + port of the server hosting UCP.
	Address string

	// PathBase is the base path of the server as it appears in the URL. eg: '/apis/api.ucp.dev/v1alpha3'.
	PathBase string

	// Location is the location of the server hosting UCP.
	Location string

	// DataProvider is the data storage provider.
	DataProvider dataprovider.DataStorageProvider

	// QeueueProvider provides access to the queue for async operations.
	QueueProvider *queueprovider.QueueProvider

	// SecretProvider is the secret store provider used for managing credentials.
	SecretProvider *secretprovider.SecretProvider

	// SpecLoader is the OpenAPI spec loader containing specs for the UCP APIs.
	SpecLoader *validator.Loader

	// UCPConnection is the connection used to communicate with UCP APIs.
	UCPConnection sdk.Connection
}
