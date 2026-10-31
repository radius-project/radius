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

package aws

import (
	"github.com/go-chi/chi/v5"
	"github.com/radius-project/radius/pkg/ucp"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/frontend/modules"
	"github.com/radius-project/radius/pkg/validator"
)

const (
	// OperationTypeAWSResource is the operation status type for AWS resources.
	OperationStatusResourceType = "System.AWS/operationStatuses"

	// OperationTypeAWSResource is the operation result type for AWS resources.
	OperationResultsResourceType = "System.AWS/operationResults"
)

// NewModule creates a new AWS module.
func NewModule(options *ucp.Options) *Module {
	m := Module{options: options,
		router: chi.NewRouter()}
	m.router.NotFound(validator.APINotFoundHandler())
	m.router.MethodNotAllowed(validator.APIMethodNotAllowedHandler())

	return &m
}

var _ modules.Initializer = &Module{}

// Module defines the module for AWS functionality.
type Module struct {
	options *ucp.Options
	router  chi.Router

	fingerprintKey    *ucp_aws.FingerprintKey
	fingerprintKeyErr error
}

// FingerprintKey returns the HMAC key for fingerprints of write-only AWS property values, loaded by Initialize. It
// returns an error naming the Kubernetes Secret when the key could not be loaded.
func (m *Module) FingerprintKey() (*ucp_aws.FingerprintKey, error) {
	return m.fingerprintKey, m.fingerprintKeyErr
}

// PlaneType returns the type of plane this module is for.
func (m *Module) PlaneType() string {
	return "aws"
}
