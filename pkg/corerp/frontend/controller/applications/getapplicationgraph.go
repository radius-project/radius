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

package applications

import (
	"context"
	"net/http"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/datamodel/converter"

	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
)

var _ ctrl.Controller = (*GetApplicationGraph)(nil)

// GetApplicationGraph is the controller implementation to get application graph.
type GetApplicationGraph struct {
	ctrl.Operation[*datamodel.Application, datamodel.Application]
}

// NewGetRecipeMetadata creates a new controller for retrieving recipe metadata from an environment.
func NewGetApplicationGraph(opts ctrl.Options) (ctrl.Controller, error) {
	return &GetApplicationGraph{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.Application]{
				RequestConverter:  converter.ApplicationDataModelFromVersioned,
				ResponseConverter: converter.ApplicationDataModelToVersioned,
			},
		),
	}, nil
}

func (r *GetApplicationGraph) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	return rest.NewBadRequestResponse("waiting to be implemented"), nil
}
