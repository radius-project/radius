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
package kubernetes

import (
	"context"
	"encoding/json"
	http "net/http"

	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/frontend/server"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
)

var _ armrpc_controller.Controller = (*DiscoveryDoc)(nil)

// DiscoveryDoc is the controller implementation to handle the discovery document.
type DiscoveryDoc struct {
	armrpc_controller.BaseController
}

// # Function Explanation
//
// NewDiscoveryDoc creates a new DiscoveryDoc controller with the given options and returns it, or returns an error if the
// controller cannot be created.
func NewDiscoveryDoc(opts armrpc_controller.Options) (armrpc_controller.Controller, error) {
	return &DiscoveryDoc{armrpc_controller.NewBaseController(opts)}, nil
}

// # Function Explanation
//
// Run responds to a request to /apis/api.ucp.dev/v1alpha3 with a 200 OK response and an empty list of CRDs.
func (e *DiscoveryDoc) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	// Required for the K8s scenario, we are required to respond to a request
	// to /apis/api.ucp.dev/v1alpha3 with a 200 OK response and the following
	// format.
	//
	// This tells the API Server we don't serve any CRDs (empty list).

	// We avoid using the rest package here so we can avoid logging every request.
	// This endpoint is called ..... A ... LOT.
	b, err := json.Marshal(map[string]any{
		"kind":         "APIResourceList",
		"apiVersion":   "v1alpha3",
		"groupVersion": "api.ucp.dev/v1alpha3",
		"resources":    []any{},
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
