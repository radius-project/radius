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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	corerpv20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/to"
	ucpv20231001preview "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/ucplog"

	productmanifest "github.com/radius-project/radius/deploy/manifest"
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
func fetchIcons(ctx context.Context, connection sdk.Connection, graph *corerpv20250801preview.ApplicationGraphResponse, includeBytes bool) (map[string]resourceTypeIcon, error) {
	if graph == nil {
		return nil, nil
	}

	// Collect the set of distinct namespaces referenced by the graph so we
	// issue at most one GetProviderSummary per provider.
	namespaces := map[string]struct{}{}
	for _, resource := range graph.Resources {
		namespace, _, ok := productmanifest.SplitResourceType(to.String(resource.Type))
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
	logger := ucplog.FromContextOrDiscard(ctx)
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
			fullType := namespace + "/" + typeName
			entry := resourceTypeIcon{hash: to.String(rt.IconHash)}
			if includeBytes {
				bytes := to.String(rt.Icon)
				// Integrity check: the registry returned both an
				// `iconHash` (canonical, computed at write time by the
				// server's encryption/ingest pipeline) and `Icon` bytes
				// for the same resource type. If the bytes do not hash
				// back to the advertised hash, storage or transport has
				// diverged them and the bytes are not authoritative. In
				// that case treat the type as if it had no icon
				// registered — leaving `entry.hash` empty triggers the
				// existing default-fallback path in `attachIconHashes`
				// and `buildIconsMap`, so the affected nodes get the
				// product default icon rather than blank tiles, and
				// consumers never receive bytes that do not match their
				// advertised hash. Only performed when bytes were
				// requested (includeBytes=true); the hash-only path
				// forwards the advertised hash unchanged.
				if !bytesMatchHash(bytes, entry.hash) {
					logger.Info("icon integrity check failed; falling back to product default",
						"resourceType", fullType,
						"advertisedHash", entry.hash)
					continue
				}
				entry.bytes = bytes
			}
			icons[fullType] = entry
		}
	}
	return icons, nil
}

// bytesMatchHash reports whether SHA-256(bytes) hex-encoded equals
// advertisedHash. Comparison is case-insensitive on the hex encoding to
// tolerate producers that emit uppercase hex. An empty advertisedHash
// never matches — callers should treat that as "no icon" rather than
// "authoritative empty icon."
func bytesMatchHash(bytes, advertisedHash string) bool {
	if advertisedHash == "" {
		return false
	}
	sum := sha256.Sum256([]byte(bytes))
	return hex.EncodeToString(sum[:]) == advertisedHash ||
		hex.EncodeToString(sum[:]) == toLower(advertisedHash)
}

// toLower is a byte-wise ASCII lowercase for hex strings. Avoids pulling
// in the strings package's Unicode-aware ToLower for a compare that only
// needs to normalize [A-F] -> [a-f].
func toLower(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'F' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return string(out)
}

// attachIconHashes stamps iconHash on every resource in payload, using the
// per-type entries in icons and falling back to the product default (or nil
// if that's unavailable too).
func attachIconHashes(payload *corerpv20250801preview.ApplicationGraphResponse, icons map[string]resourceTypeIcon) {
	if payload == nil {
		return
	}
	for _, r := range payload.Resources {
		if r == nil {
			continue
		}
		if icon, ok := icons[to.String(r.Type)]; ok && icon.hash != "" {
			r.IconHash = to.Ptr(icon.hash)
			continue
		}
		// Missing lookup (external 404, unregistered type, or no icon at
		// registration time) — fall back to the product default.
		// DefaultHash returns nil when the embedded default is unavailable,
		// leaving IconHash unset for the node.
		r.IconHash = productmanifest.DefaultHash()
	}
}
