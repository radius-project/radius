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

	armrpcv1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/ucp/aws/servicecontext"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
)

var _ armrpc_controller.Controller = (*GetAWSOperationResults)(nil)

// GetAWSOperationResults is the controller implementation to get AWS resource operation results.
type GetAWSOperationResults struct {
	armrpc_controller.Operation[*datamodel.AWSResource, datamodel.AWSResource]
}

// NewGetAWSOperationResults creates a new GetAWSOperationResults controller.
func NewGetAWSOperationResults(opts armrpc_controller.Options) (armrpc_controller.Controller, error) {
	return &GetAWSOperationResults{
		Operation: armrpc_controller.NewOperation(opts, armrpc_controller.ResourceOptions[datamodel.AWSResource]{}),
	}, nil
}

// Run returns 202 while an AWS resource operation is in progress and 204 when it finished. UCP owns every create,
// update, and delete of an AWS resource, so the status is in UCP's data store. An operation that is not there returns
// 404.
func (p *GetAWSOperationResults) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	serviceCtx := servicecontext.AWSRequestContextFromContext(ctx)
	if _, errResponse := readRegionFromRequest(req.URL.Path, p.Options().PathBase); errResponse != nil {
		return errResponse, nil
	}

	status, err := getOperationStatus(ctx, p.Options().StatusManager, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	} else if status == nil {
		return armrpc_rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	}
	if !status.Status.IsTerminal() {
		headers := map[string]string{
			"Location":    req.URL.String(),
			"Retry-After": armrpcv1.DefaultRetryAfter,
		}
		return armrpc_rest.NewAsyncOperationResultResponse(headers), nil
	}
	return armrpc_rest.NewNoContentResponse(), nil
}
