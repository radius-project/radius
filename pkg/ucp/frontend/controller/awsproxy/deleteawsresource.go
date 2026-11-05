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
	http "net/http"

	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
)

var _ armrpc_controller.Controller = (*DeleteAWSResource)(nil)

// DeleteAWSResource is the controller that deletes the AWS resource under an alias.
type DeleteAWSResource struct {
	armrpc_controller.Operation[*datamodel.AWSResource, datamodel.AWSResource]
	engine *engine
}

// NewDeleteAWSResource creates a new DeleteAWSResource controller.
func NewDeleteAWSResource(opts armrpc_controller.Options, awsClients ucp_aws.Clients) (armrpc_controller.Controller, error) {
	return &DeleteAWSResource{
		Operation: armrpc_controller.NewOperation(opts, armrpc_controller.ResourceOptions[datamodel.AWSResource]{}),
		engine:    newEngine(opts, awsClients, nil),
	}, nil
}

// Run deletes the AWS resource under the alias in the URL. See engine.delete.
func (p *DeleteAWSResource) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	return p.engine.delete(ctx, req)
}
