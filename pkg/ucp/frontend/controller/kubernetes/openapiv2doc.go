// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package kubernetes

import (
	"context"
	"encoding/json"
	http "net/http"

	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/ucp/frontend/controller"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
)

var _ armrpc_controller.Controller = (*OpenAPIv2Doc)(nil)

// OpenAPIv2Doc is the controller implementation to handle the OpenAPIv2Doc endpoint.
type OpenAPIv2Doc struct {
	ctrl.BaseController
}

// NewOpenAPIv2Doc creates a new OpenAPIv2Doc.
func NewOpenAPIv2Doc(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &OpenAPIv2Doc{ctrl.NewBaseController(opts)}, nil
}

func (e *OpenAPIv2Doc) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	// Required for the K8s scenario, we are required to respond to a request
	// to /apis/api.ucp.dev/v1alpha3/openapi/v2 with a 200 OK response and a swagger (openapi v2)
	// doc.
	//
	// We don't need this for any functionality, but it will make the API server happy.

	// We avoid using the rest package here so we can avoid logging every request.
	// This endpoint is called ..... A ... LOT.
	b, err := json.Marshal(map[string]any{
		"swagger": "2.0",
		"info": map[string]any{
			"title":   "Radius APIService",
			"version": "v1alpha3",
		},
		"paths": map[string]any{},
	})
	if err != nil {
		controller.HandleError(ctx, w, req, err)
		return nil, nil
	}

	w.WriteHeader(200)
	w.Header().Add("Content-Type", "application/json")

	_, err = w.Write(b)
	if err != nil {
		controller.HandleError(ctx, w, req, err)
		return nil, nil
	}

	return nil, nil
}
