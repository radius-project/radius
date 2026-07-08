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

package resourceproviders

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

var _ controller.Controller = (*GetIcon)(nil)

// GetIcon is the controller implementation to get a resource type's icon by hash.
type GetIcon struct {
	controller.Operation[*datamodel.ResourceType, datamodel.ResourceType]
}

// NewGetIcon creates a new GetIcon controller.
func NewGetIcon(opts controller.Options) (controller.Controller, error) {
	return &GetIcon{
		Operation: controller.NewOperation(opts, controller.ResourceOptions[datamodel.ResourceType]{}),
	}, nil
}

// Run executes the GetIcon operation.
func (r *GetIcon) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	// Extract path parameters from chi. The route is:
	// /planes/radius/{planeName}/providers/System.Resources/resourceproviders/{resourceProviderName}/resourcetypes/{resourceTypeName}/icons/{hash}
	planeName := chi.URLParam(req, "planeName")
	rpName := chi.URLParam(req, "resourceProviderName")
	rtName := chi.URLParam(req, "resourceTypeName")
	hash := chi.URLParam(req, "hash")

	if planeName == "" || rpName == "" || rtName == "" || hash == "" {
		return armrpc_rest.NewBadRequestResponse("invalid icon path"), nil
	}

	rtPath := fmt.Sprintf("/planes/radius/%s/providers/System.Resources/resourceProviders/%s/resourceTypes/%s", planeName, rpName, rtName)
	id, err := resources.Parse(rtPath)
	if err != nil {
		return nil, err
	}

	result, err := r.DatabaseClient().Get(ctx, id.String())
	if err != nil {
		var notFound *database.ErrNotFound
		if errors.As(err, &notFound) {
			return armrpc_rest.NewNotFoundResponse(id), nil
		}
		return nil, err
	}

	rt := &datamodel.ResourceType{}
	if err := result.As(rt); err != nil {
		return nil, err
	}

	// Verify the hash matches what is stored
	if rt.Properties.IconHash == nil || rt.Properties.Icon == nil {
		return armrpc_rest.NewNotFoundResponseWithCause(id, "resource type has no icon"), nil
	}
	if *rt.Properties.IconHash != hash {
		return armrpc_rest.NewNotFoundResponseWithCause(id, fmt.Sprintf("icon with hash %q was not found", hash)), nil
	}

	// Return the verbatim SVG bytes with the correct content type (FR-018)
	return &iconResponse{content: *rt.Properties.Icon}, nil
}

type iconResponse struct {
	content string
}

func (r *iconResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	// Defense in depth: ValidateIcon already rejects <script>, on* handlers,
	// <foreignObject>, and external href references before storage. These
	// headers harden the response against MIME-sniffing and neutralize any
	// residual active content if a client (e.g. a browser) navigates to the
	// icon URL top-level rather than embedding it via <img>.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(r.content))
	return err
}
