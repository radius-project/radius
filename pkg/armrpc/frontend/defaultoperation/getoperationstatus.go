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

package defaultoperation

import (
	"context"
	"errors"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	manager "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*GetOperationStatus)(nil)

// GetOperationStatus is the controller implementation to get an async operation status.
type GetOperationStatus struct {
	ctrl.BaseController
}

// NewGetOperationStatus creates a new GetOperationStatus.
func NewGetOperationStatus(opts ctrl.Options) (ctrl.Controller, error) {
	return &GetOperationStatus{ctrl.NewBaseController(opts)}, nil
}

// Run returns the async operation status.
// Spec: https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/async-api-reference.md#azure-asyncoperation-resource-format
func (e *GetOperationStatus) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	// TODO: Add additional validation

	os := &manager.Status{}
	_, err := e.GetResource(ctx, serviceCtx.ResourceID.String(), os)
	if err != nil && errors.Is(&store.ErrNotFound{}, err) {
		return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	}

	return rest.NewOKResponse(os.AsyncOperationStatus), nil
}
