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

package awsproxy

import (
	"context"
	"errors"
	http "net/http"

	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
)

var _ armrpc_controller.Controller = (*CreateOrUpdateAWSResource)(nil)

// CreateOrUpdateAWSResource is the controller that creates or updates the AWS resource under an alias.
type CreateOrUpdateAWSResource struct {
	armrpc_controller.Operation[*datamodel.AWSResource, datamodel.AWSResource]
	engine *engine
}

// NewCreateOrUpdateAWSResource creates a new CreateOrUpdateAWSResource controller. fingerprintKey returns the key for
// fingerprints of write-only values.
func NewCreateOrUpdateAWSResource(opts armrpc_controller.Options, awsClients ucp_aws.Clients, fingerprintKey FingerprintKeyFunc) (armrpc_controller.Controller, error) {
	if fingerprintKey == nil {
		return nil, errors.New("the AWS resource PUT controller needs a source for the fingerprint key")
	}
	return &CreateOrUpdateAWSResource{
		Operation: armrpc_controller.NewOperation(opts, armrpc_controller.ResourceOptions[datamodel.AWSResource]{}),
		engine:    newEngine(opts, awsClients, fingerprintKey),
	}, nil
}

// Run creates or updates the AWS resource under the alias in the URL. See engine.put.
func (p *CreateOrUpdateAWSResource) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	return p.engine.put(ctx, req)
}
