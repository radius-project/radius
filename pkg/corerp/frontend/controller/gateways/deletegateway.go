/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

package gateways

import (
	"context"
	"net/http"
	"time"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
)

var _ ctrl.Controller = (*DeleteGateway)(nil)

var (
	// AsyncDeleteGatewayOperationTimeout is the default timeout duration of async delete gateway operation.
	AsyncDeleteGatewayOperationTimeout = time.Duration(120) * time.Second
)

// DeleteGateway is the controller implementation to delete gateway resource.
type DeleteGateway struct {
	ctrl.Operation[*datamodel.Gateway, datamodel.Gateway]
}

// NewDeleteGateway creates a new DeleteGateway.
func NewDeleteGateway(opts ctrl.Options) (ctrl.Controller, error) {
	return &DeleteGateway{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.Gateway]{
				RequestConverter:  converter.GatewayDataModelFromVersioned,
				ResponseConverter: converter.GatewayDataModelToVersioned,
			},
		),
	}, nil
}

func (dc *DeleteGateway) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	old, etag, err := dc.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if old == nil {
		return rest.NewNoContentResponse(), nil
	}

	if r, err := dc.PrepareResource(ctx, req, nil, old, etag); r != nil || err != nil {
		return r, err
	}

	if r, err := dc.PrepareAsyncOperation(ctx, old, v1.ProvisioningStateAccepted, AsyncDeleteGatewayOperationTimeout, &etag); r != nil || err != nil {
		return r, err
	}

	return dc.ConstructAsyncResponse(ctx, req.Method, etag, old)
}
