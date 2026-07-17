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

package v20250801preview

import (
	"context"
	"encoding/json"
	"net/http"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	corerpv20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/datamodel/converter"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/to"

	productmanifest "github.com/radius-project/radius/deploy/manifest"
)

var _ ctrl.Controller = (*GetGraphv20250801preview)(nil)

// GetGraphv20250801preview is the controller implementation to get the application graph for
// Radius.Core/applications resources.
type GetGraphv20250801preview struct {
	ctrl.Operation[*datamodel.Application_v20250801preview, datamodel.Application_v20250801preview]
	connection sdk.Connection
}

// NewGetGraphv20250801preview creates a new instance of the GetGraphv20250801preview controller.
func NewGetGraphv20250801preview(opts ctrl.Options, connection sdk.Connection) (ctrl.Controller, error) {
	return &GetGraphv20250801preview{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.Application_v20250801preview]{
				RequestConverter:  converter.Application20250801DataModelFromVersioned,
				ResponseConverter: converter.Application20250801DataModelToVersioned,
			},
		),
		connection,
	}, nil
}

// Run handles the getGraph custom action for Radius.Core/applications. It looks up the application,
// resolves its environment, lists application- and environment-scoped resources, and returns the
// computed application graph enriched with each node's `iconHash`. When the
// request body sets `includeIcons: true` the response also carries a deduped `icons` map from
// iconHash to verbatim SVG bytes.
func (ctrl *GetGraphv20250801preview) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	sCtx := v1.ARMRequestContextFromContext(ctx)

	// Request route for getGraph has the operation name as suffix which must be removed to get the resource id.
	// route id format: /planes/radius/local/resourcegroups/default/providers/Radius.Core/applications/<app>/getGraph
	applicationID := sCtx.ResourceID.Truncate()
	applicationResource, _, err := ctrl.GetResource(ctx, applicationID)
	if err != nil {
		return nil, err
	}
	if applicationResource == nil {
		return rest.NewNotFoundResponse(sCtx.ResourceID), nil
	}

	graphReq, err := readGraphRequest(req)
	if err != nil {
		return nil, err
	}
	includeIcons := to.Bool(graphReq.IncludeIcons)

	payload, err := computeGraphPayload(ctx, applicationID, applicationResource.Properties.Environment, ctrl.connection, graphReq.DependsOnEdges)
	if err != nil {
		return nil, err
	}

	// Per-node iconHash comes from the resource-type registry: one
	// GetProviderSummary call per distinct namespace in the graph, then a
	// lookup by "<namespace>/<typeName>". A type without a registered icon
	// simply leaves the corresponding node's IconHash nil (default
	// substitution is a control-plane-side concern, not the graph layer's).
	icons, err := fetchIcons(ctx, ctrl.connection, payload, includeIcons)
	if err != nil {
		return nil, err
	}

	attachIconHashes(payload, icons)

	// When the caller opted in with includeIcons=true, dedupe by hash and
	// emit the icons map alongside the resources.
	if includeIcons {
		payload.Icons = buildIconsMap(payload.Resources, icons)
	}

	return rest.NewOKResponse(payload), nil
}

// readGraphRequest parses the optional GetGraphRequest body once and returns
// the parsed struct. Missing bodies, empty bodies, and bodies posted without a
// JSON content type (typical for existing clients that pre-date the additive
// fields on this shape) all resolve to a zero-value struct rather than an
// error, so both includeIcons and dependsOnEdges stay additive on the wire.
// The returned pointer is never nil, letting callers use zero-value fields
// directly without another nil check.
func readGraphRequest(req *http.Request) (*corerpv20250801preview.GetGraphRequest, error) {
	parsed := &corerpv20250801preview.GetGraphRequest{}
	if req.Body == nil || req.ContentLength == 0 {
		return parsed, nil
	}
	if req.Header.Get("Content-Type") == "" {
		return parsed, nil
	}
	body, err := ctrl.ReadJSONBody(req)
	if err != nil {
		// A non-JSON content type is not a client error for getGraph — the
		// body is optional. Any other read failure is real and should bubble
		// up.
		if err == ctrl.ErrUnsupportedContentType {
			return parsed, nil
		}
		return nil, err
	}
	if len(body) == 0 {
		return parsed, nil
	}
	if err := json.Unmarshal(body, parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

// buildIconsMap returns the deduped icons map keyed by iconHash, containing the
// verbatim SVG bytes for every distinct hash referenced by the response's
// resources. Two byte sources feed into the map:
//
//  1. `icons` — the per-type entries returned by fetchIcons, which carry
//     bytes fetched from the resource-type registry when the caller opted in
//     via includeIcons=true.
//  2. The product default icon embedded in the deploy/manifest package, used
//     for any node whose hash matches the default (types registered without
//     an icon get the default hash at registration time; external nodes such
//     as Microsoft.Storage/storageAccounts get the default hash from
//     attachIconHashes). Registered types do not carry the
//     default bytes on their storage record, so this substitution is what
//     makes the response self-contained.
func buildIconsMap(resources []*corerpv20250801preview.ApplicationGraphResource, icons map[string]resourceTypeIcon) map[string]*string {
	if len(resources) == 0 {
		return nil
	}
	defaultIcon := productmanifest.Default()
	hasDefault := defaultIcon.Hash != "" && len(defaultIcon.Bytes) > 0
	out := map[string]*string{}
	for _, r := range resources {
		if r == nil || r.IconHash == nil {
			continue
		}
		hash := *r.IconHash
		if _, already := out[hash]; already {
			continue
		}
		if hasDefault && hash == defaultIcon.Hash {
			bytes := string(defaultIcon.Bytes)
			out[hash] = &bytes
			continue
		}
		icon, ok := icons[to.String(r.Type)]
		if !ok || icon.bytes == "" {
			continue
		}
		bytes := icon.bytes
		out[hash] = &bytes
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
