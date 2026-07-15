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
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	corerpv20231001preview "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	corerpv20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/to"
	ucpv20231001preview "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
)

// resourceTypeIcon captures the icon metadata for a single resource type as
// fetched from the UCP resource-type registry. `hash` is always populated when
// the type has an icon registered; `bytes` is only populated when the caller
// requested inline icons.
type resourceTypeIcon struct {
	hash  string
	bytes string
}

// fetchIcons queries UCP for the icon metadata of every distinct resource type
// present in the given graph. It returns a map keyed by the full resource type
// (e.g. "Radius.Compute/containers") so callers can attach `iconHash` per graph
// node and optionally build a deduped `icons` map from the response.
//
// When includeBytes is true the returned entries carry both the hash and the
// verbatim SVG bytes; when false only the hash is populated. Batching by
// namespace means we make one GetProviderSummary call per distinct provider in
// the graph rather than one per resource type.
func fetchIcons(ctx context.Context, connection sdk.Connection, graph *corerpv20231001preview.ApplicationGraphResponse, includeBytes bool) (map[string]resourceTypeIcon, error) {
	if graph == nil {
		return nil, nil
	}

	// Collect the set of distinct namespaces referenced by the graph so we
	// issue at most one GetProviderSummary per provider.
	namespaces := map[string]struct{}{}
	for _, resource := range graph.Resources {
		namespace, _, ok := splitResourceType(to.String(resource.Type))
		if !ok {
			continue
		}
		namespaces[namespace] = struct{}{}
	}
	if len(namespaces) == 0 {
		return nil, nil
	}

	clientOptions := sdk.NewClientOptions(connection)
	client, err := ucpv20231001preview.NewResourceProvidersClient(&aztoken.AnonymousCredential{}, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create UCP resource-providers client: %w", err)
	}

	var opts *ucpv20231001preview.ResourceProvidersClientGetProviderSummaryOptions
	if includeBytes {
		opts = &ucpv20231001preview.ResourceProvidersClientGetProviderSummaryOptions{
			IncludeIcons: to.Ptr(true),
		}
	}

	icons := map[string]resourceTypeIcon{}
	for namespace := range namespaces {
		summary, err := client.GetProviderSummary(ctx, "local", namespace, opts)
		if err != nil {
			// `computeGraph` deliberately adds connected external nodes such as
			// `Microsoft.Storage/storageAccounts` that live outside the local
			// Radius resource-type registry. Their providers are not registered
			// with UCP, so GetProviderSummary returns 404. Treat that as "no
			// icons for this namespace" rather than failing the whole graph
			// request — the corresponding nodes simply end up with a nil
			// iconHash.
			var respErr *azcore.ResponseError
			if errors.As(err, &respErr) && respErr.StatusCode == http.StatusNotFound {
				continue
			}
			return nil, fmt.Errorf("failed to fetch resource provider summary for %q: %w", namespace, err)
		}
		for typeName, rt := range summary.ResourceTypes {
			if rt == nil || rt.IconHash == nil {
				continue
			}
			entry := resourceTypeIcon{hash: to.String(rt.IconHash)}
			if includeBytes {
				entry.bytes = to.String(rt.Icon)
			}
			icons[namespace+"/"+typeName] = entry
		}
	}
	return icons, nil
}

// splitResourceType splits a fully qualified resource type of the form
// "<namespace>/<typeName>" (e.g. "Radius.Compute/containers"). It returns
// (namespace, typeName, true) on success, or ("", "", false) if the input does
// not match the expected shape.
func splitResourceType(resourceType string) (string, string, bool) {
	slash := strings.Index(resourceType, "/")
	if slash <= 0 || slash == len(resourceType)-1 {
		return "", "", false
	}
	return resourceType[:slash], resourceType[slash+1:], true
}

// convertGraphResponseWithIcons converts a graph payload produced by the shared
// v20231001preview computation into the v20250801preview wire shape and, when
// icons is non-nil, attaches per-node `iconHash` values from the lookup. The
// two struct types share the same JSON contract for existing fields; this
// conversion exists so we can additionally set the v20250801preview-only
// `IconHash` and `Icons` fields without polluting the older API surface.
func convertGraphResponseWithIcons(payload *corerpv20231001preview.ApplicationGraphResponse, icons map[string]resourceTypeIcon) *corerpv20250801preview.ApplicationGraphResponse {
	out := &corerpv20250801preview.ApplicationGraphResponse{
		Resources: make([]*corerpv20250801preview.ApplicationGraphResource, 0, len(payload.Resources)),
	}
	for _, src := range payload.Resources {
		if src == nil {
			continue
		}
		dst := &corerpv20250801preview.ApplicationGraphResource{
			ID:                src.ID,
			Name:              src.Name,
			Type:              src.Type,
			ProvisioningState: src.ProvisioningState,
			DiffHash:          src.DiffHash,
			Properties:        src.Properties,
			Connections:       convertConnections(src.Connections),
			OutputResources:   convertOutputResources(src.OutputResources),
		}
		if icon, ok := icons[to.String(src.Type)]; ok && icon.hash != "" {
			dst.IconHash = to.Ptr(icon.hash)
		}
		out.Resources = append(out.Resources, dst)
	}
	return out
}

func convertConnections(in []*corerpv20231001preview.ApplicationGraphConnection) []*corerpv20250801preview.ApplicationGraphConnection {
	if len(in) == 0 {
		return nil
	}
	out := make([]*corerpv20250801preview.ApplicationGraphConnection, 0, len(in))
	for _, c := range in {
		if c == nil {
			continue
		}
		converted := &corerpv20250801preview.ApplicationGraphConnection{ID: c.ID}
		if c.Direction != nil {
			converted.Direction = to.Ptr(corerpv20250801preview.Direction(*c.Direction))
		}
		out = append(out, converted)
	}
	return out
}

func convertOutputResources(in []*corerpv20231001preview.ApplicationGraphOutputResource) []*corerpv20250801preview.ApplicationGraphOutputResource {
	if len(in) == 0 {
		return nil
	}
	out := make([]*corerpv20250801preview.ApplicationGraphOutputResource, 0, len(in))
	for _, r := range in {
		if r == nil {
			continue
		}
		out = append(out, &corerpv20250801preview.ApplicationGraphOutputResource{
			ID:        r.ID,
			Name:      r.Name,
			Type:      r.Type,
			PortalURL: r.PortalURL,
		})
	}
	return out
}
