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
	app_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/applications"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/to"
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
// computed application graph enriched with each node's `iconHash` (spec 003 FR-011). When the
// request body sets `includeIcons: true` the response also carries a deduped `icons` map from
// iconHash to verbatim SVG bytes (FR-013, FR-015).
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

	includeIcons, err := readIncludeIcons(req)
	if err != nil {
		return nil, err
	}

	payload, err := app_ctrl.ComputeGraphPayload(ctx, applicationID, applicationResource.Properties.Environment, ctrl.connection)
	if err != nil {
		return nil, err
	}

	// Per-node iconHash comes from the resource-type registry: one
	// GetProviderSummary call per distinct namespace in the graph, then a
	// lookup by "<namespace>/<typeName>". A type without a registered icon
	// simply leaves the corresponding node's IconHash nil (FR-011 default
	// substitution is a control-plane-side concern, not the graph layer's).
	icons, err := fetchIcons(ctx, ctrl.connection, payload, includeIcons)
	if err != nil {
		return nil, err
	}

	response := convertGraphResponseWithIcons(payload, icons)

	// When the caller opted in with includeIcons=true, dedupe by hash and
	// emit the icons map alongside the resources (spec 003 FR-013).
	if includeIcons {
		response.Icons = buildIconsMap(response.Resources, icons)
	}

	return rest.NewOKResponse(response), nil
}

// readIncludeIcons parses the optional GetGraphRequest body and returns the
// value of its includeIcons field. Missing bodies, empty bodies, and bodies
// posted without a JSON content type (typical for existing clients that pre-date
// the flag) all resolve to the default value false so this feature stays
// additive on the wire (spec 003 NFR-004).
func readIncludeIcons(req *http.Request) (bool, error) {
	if req.Body == nil || req.ContentLength == 0 {
		return false, nil
	}
	contentType := req.Header.Get("Content-Type")
	if contentType == "" {
		return false, nil
	}
	body, err := ctrl.ReadJSONBody(req)
	if err != nil {
		// A non-JSON content type is not a client error for getGraph — the
		// body is optional. Any other read failure is real and should bubble
		// up.
		if err == ctrl.ErrUnsupportedContentType {
			return false, nil
		}
		return false, err
	}
	if len(body) == 0 {
		return false, nil
	}
	parsed := corerpv20250801preview.GetGraphRequest{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return false, err
	}
	return to.Bool(parsed.IncludeIcons), nil
}

// buildIconsMap returns the deduped icons map keyed by iconHash, containing the
// verbatim SVG bytes for every distinct hash referenced by the response's
// resources. Nodes whose type has no registered icon are skipped.
func buildIconsMap(resources []*corerpv20250801preview.ApplicationGraphResource, icons map[string]resourceTypeIcon) map[string]*string {
	if len(resources) == 0 || len(icons) == 0 {
		return nil
	}
	out := map[string]*string{}
	for _, r := range resources {
		if r == nil || r.IconHash == nil {
			continue
		}
		hash := *r.IconHash
		if _, already := out[hash]; already {
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
