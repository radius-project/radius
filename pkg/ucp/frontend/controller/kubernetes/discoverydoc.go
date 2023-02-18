// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package kubernetes

import (
	"context"
	"encoding/json"
	http "net/http"

	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/frontend/controller"
)

var _ ctrl.Controller = (*DiscoveryDoc)(nil)

// DiscoveryDoc is the controller implementation to handle the discovery document.
type DiscoveryDoc struct {
	ctrl.Operation[*datamodel.KubernetesResource, datamodel.KubernetesResource]
}

// NewDiscoveryDoc creates a new DiscoveryDoc.
func NewDiscoveryDoc(opts ctrl.Options) (ctrl.Controller, error) {
	return &DiscoveryDoc{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.KubernetesResource]{},
		),
	}, nil
}

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
