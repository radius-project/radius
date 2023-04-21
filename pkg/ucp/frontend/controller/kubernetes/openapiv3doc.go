// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	http "net/http"

	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/frontend/server"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
)

var _ armrpc_controller.Controller = (*OpenAPIv3Doc)(nil)

// OpenAPIv3Doc is the controller implementation to handle the OpenAPIv3Doc endpoint.
type OpenAPIv3Doc struct {
	armrpc_controller.BaseController
}

// NewOpenAPIv3Doc creates a new OpenAPIv3Doc.
func NewOpenAPIv3Doc(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &OpenAPIv3Doc{armrpc_controller.NewBaseController(opts.Options)}, nil
}

func (e *OpenAPIv3Doc) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	fmt.Println("@@@@@@ openapiv3 handler")
	// Required for the K8s scenario, we are required to respond to a request
	// to /apis/api.ucp.dev/v1alpha3/openapi/v3 with a 200 OK response and a swagger (openapi v3)
	// doc.
	//
	// We don't need this for any functionality, but it will make the API server happy.

	// We avoid using the rest package here so we can avoid logging every request.
	// This endpoint is called ..... A ... LOT.
	b, err := json.Marshal(map[string]any{
		"swagger": "3.0",
		"info": map[string]any{
			"title":   "Radius APIService",
			"version": "v1alpha3",
		},
		"paths": map[string]any{},
	})
	if err != nil {
		server.HandleError(ctx, w, req, err)
		return nil, nil
	}

	w.WriteHeader(200)
	w.Header().Add("Content-Type", "application/json")

	_, err = w.Write(b)
	if err != nil {
		server.HandleError(ctx, w, req, err)
		return nil, nil
	}

	return nil, nil
}
