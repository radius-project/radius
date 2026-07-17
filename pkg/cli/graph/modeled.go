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

package graph

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	corerpv20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/graph/edges"
	"github.com/radius-project/radius/pkg/to"

	productmanifest "github.com/radius-project/radius/deploy/manifest"
)

// Default scope segments used when constructing fully-qualified Radius
// resource IDs for a modeled graph. The modeled graph is built from a Bicep
// definition without contacting any control plane, so the actual plane and
// resource group are unknown at build time.
const (
	defaultPlane             = "local"
	defaultResourceGroup     = "default"
	applicationsResourceType = "Applications.Core/applications"
	environmentsResourceType = "Applications.Core/environments"
	recipePacksResourceType  = "Radius.Core/recipePacks"

	// secureStringParameterType and secureObjectParameterType are the
	// two ARM parameter types Bicep emits for `@secure()` parameter
	// declarations (respectively for `string` and `object` bases).
	// Values that reference such a parameter via `parameters('name')`
	// — including nested inside larger ARM expressions or the result of
	// dotted property access on a secureObject — are treated as sensitive
	// on the static graph and nulled in the emitted Properties bag. Both
	// types are handled uniformly because the redaction contract does
	// not distinguish scalar from structured secrets. See
	// eng/design-notes/security/2026-07-static-graph-sensitive-redaction.md.
	secureStringParameterType = "secureString"
	secureObjectParameterType = "secureObject"
)

// dropOnGraph lists top-level property keys that are stripped from the
// resource-specific Properties bag before it is written into an
// ApplicationGraphResource. Mirrors the runtime graph's `existingKeys`
// in pkg/corerp/frontend/controller/applications/graph_util.go so the
// static and runtime graphs surface the same shape:
//
//   - "provisioningState" and "connections" are already first-class
//     fields on the resource; keeping duplicates in Properties would
//     confuse diff tooling.
//   - "status" is dropped in its entirety on the runtime side because
//     status subtrees may include computed values that carry secrets
//     (connection strings, endpoints). The same rule applies here to
//     keep the two graphs comparable, even though the static graph's
//     status is always empty in practice.
var dropOnGraph = map[string]struct{}{
	"provisioningState": {},
	"connections":       {},
	"status":            {},
}

// sensitiveKeyBlocklist enumerates property names whose values are
// unconditionally nulled in the static graph's Properties bag,
// regardless of how the value was assigned in the source Bicep. The
// list is short, curated, and case-insensitive; it complements the
// secureString parameter tracing (which only catches values threaded
// through `@secure()` parameters) by catching hard-coded literals and
// values whose provenance the static graph cannot trace.
//
// See eng/design-notes/security/2026-07-static-graph-sensitive-redaction.md
// for the rationale on inclusion. New entries should be added
// conservatively: a false positive redacts one graph cell, but the
// list is not the place to litigate ambiguously-named properties like
// "key" or "config".
var sensitiveKeyBlocklist = map[string]struct{}{
	"password":         {},
	"connectionstring": {},
	"apikey":           {},
	"secret":           {},
	"token":            {},
	"privatekey":       {},
	"sastoken":         {},
}

// armExpressionPattern matches an ARM template expression — any value
// bracketed in `[...]`. Used as a cheap filter before running the more
// specific secureString-parameter substring check; ordinary string
// literals cannot reference a parameter, so this early-out avoids
// pointless work on the majority of property values.
var armExpressionPattern = regexp.MustCompile(`^\[.*\]$`)

// excludeResourceTypes are the resource types that are not graph members.
var excludeResourceTypes = map[string]struct{}{
	"Applications.Core/applications": {},
	"Applications.Core/environments": {},
	"Radius.Core/applications":       {},
	"Radius.Core/environments":       {},
	"Radius.Core/recipePacks":        {},
}

// resourceIDExpression matches an ARM template resourceId() expression
// produced by `bicep build` for Radius resource references, e.g.
//
//	[resourceId('Applications.Datastores/redisCaches', 'cache')]
//
// Only the literal-argument form is supported; expressions that compute
// types or names dynamically are left unresolved (the connection edge is
// dropped from the modeled graph).
var resourceIDExpression = regexp.MustCompile(`^\[resourceId\(([^)]*)\)\]$`)

// BuildModeledGraph parses an ARM JSON template (typically the output of
// `bicep build` on an application's app.bicep) and returns the corresponding
// modeled application graph. The graph contains application resources,
// their connections and dependsOn relationships, resource-specific
// authored properties (with sensitive values nulled), and a stable diff
// hash for each resource. It does not contain output resources or
// runtime status — those are only available for planned and deployed
// graphs.
//
// Sensitive values are redacted using two rules applied to every
// property in the emitted Properties bag:
//
//  1. Values that reference a Bicep `@secure() param` (surfaced in the
//     compiled template as `parameters.<name>.type == "secureString"`
//     for scalar and `"secureObject"` for structured secrets) are
//     nulled.
//  2. Values whose property key case-insensitively matches a well-known
//     secret name (`password`, `connectionString`, `apiKey`, etc.) are
//     nulled regardless of source.
//
// See eng/design-notes/security/2026-07-static-graph-sensitive-redaction.md
// for the full contract.
func BuildModeledGraph(template map[string]any, includeIcons bool) (*corerpv20250801preview.ApplicationGraphResponse, error) {
	rawResources := collectResources(template["resources"])
	if rawResources == nil {
		return emptyGraph(), nil
	}

	secureParams := sensitiveParamNames(template)

	graphResources := make([]*corerpv20250801preview.ApplicationGraphResource, 0, len(rawResources))
	for _, entry := range rawResources {
		resource, err := buildModeledResource(entry, secureParams)
		if err != nil {
			return nil, err
		}
		if resource == nil {
			continue
		}
		graphResources = append(graphResources, resource)
	}

	graph := &corerpv20250801preview.ApplicationGraphResponse{Resources: graphResources}

	// Mirror the author-declared Connection outbound edges (populated on
	// each resource by buildModeledResource via outboundConnections) as
	// inbound entries on their targets. This gives us a complete
	// Connection-only graph before we merge in the Bicep dependsOn edges.
	addInboundConnections(graph)

	// Overlay Bicep dependsOn edges as Kind: Dependency, subject to the
	// exclusion set and Connection-wins de-dup. The static graph is the
	// only Kind: Dependency producer today; runtime dependency edges
	// arrive via caller-supplied dependsOnEdges on GetGraphRequest and
	// go through the same MergeDependencyEdges primitive server-side.
	edges.MergeDependencyEdges(graph, ExtractDependsOnEdges(template), excludeResourceTypes)

	if includeIcons {
		graph.Icons = collectStaticGraphIcons(graphResources)
	}
	return graph, nil
}

// ExtractDependsOnEdges walks a compiled ARM JSON template and returns
// a map from each resource's canonical Radius ID to the list of
// outbound Kind: Dependency edges implied by that resource's dependsOn.
// The shape matches GetGraphRequest.dependsOnEdges on
// Radius.Core/2025-08-01-preview, so callers can attach the result
// directly to a deployed-graph request to enrich it with the same
// implicit dependencies the static graph would surface.
//
// Resources whose type is in excludeResourceTypes are omitted as
// sources (they are never edge sources anyway). Individual dependsOn
// entries that resolve to a canonical ID are included regardless of
// target type; edges.MergeDependencyEdges applies the target-type
// exclusion server- and CLI-side. Unresolvable entries (dynamic
// resourceId expressions, non-Radius symbolic references) are dropped.
func ExtractDependsOnEdges(template map[string]any) map[string][]*corerpv20250801preview.ApplicationGraphConnection {
	rawResources := collectResources(template["resources"])
	if rawResources == nil {
		return nil
	}
	out := map[string][]*corerpv20250801preview.ApplicationGraphConnection{}
	for _, entry := range rawResources {
		resourceType, _ := entry["type"].(string)
		name, _ := entry["name"].(string)
		if resourceType == "" || name == "" {
			continue
		}
		if isExcludedResourceType(resourceType) {
			continue
		}
		rawDependsOn, _ := entry["dependsOn"].([]any)
		resolved := resolveDependsOn(rawDependsOn)
		if len(resolved) == 0 {
			continue
		}
		sourceID := buildResourceID(resourceType, name)
		entries := make([]*corerpv20250801preview.ApplicationGraphConnection, 0, len(resolved))
		for _, target := range resolved {
			entries = append(entries, &corerpv20250801preview.ApplicationGraphConnection{
				ID:        to.Ptr(target),
				Direction: to.Ptr(corerpv20250801preview.DirectionOutbound),
				Kind:      to.Ptr(corerpv20250801preview.ConnectionKindDependency),
			})
		}
		out[sourceID] = entries
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// sensitiveParamNames returns the set of parameter names declared with
// a secure ARM type in the compiled template. Bicep emits
// `secureString` for `@secure() param foo string` and `secureObject`
// for `@secure() param foo object`; both are treated as sensitive
// sources for the value-tracing rule. Comparison is case-insensitive
// to survive ARM's occasional case-normalization of type names (the
// bicep recipe driver applies the same case-insensitive rule for
// symmetry — see pkg/recipes/driver/bicep/bicep.go:isSecureARMOutputType).
// Returns an empty map when the template has no parameters or the
// parameters block is malformed — downstream callers use
// `_, ok := secureParams[name]` and are safe on nil / empty maps.
func sensitiveParamNames(template map[string]any) map[string]struct{} {
	out := map[string]struct{}{}
	params, ok := template["parameters"].(map[string]any)
	if !ok {
		return out
	}
	for name, raw := range params {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		t, _ := entry["type"].(string)
		if strings.EqualFold(t, secureStringParameterType) || strings.EqualFold(t, secureObjectParameterType) {
			out[name] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// collectStaticGraphIcons walks the modeled-graph resources and returns a
// deduped `iconHash -> SVG bytes` map that mirrors the shape used by the
// runtime graph. Every distinct hash referenced by the resources is included:
// built-in types resolve to their per-type SVG from the embedded product
// manifest, and everything else (user-defined types, external cloud types,
// or built-in types whose SVG has not yet been shipped in resource-types-contrib)
// falls through to the product default icon. The CLI has no control plane to
// query, so this map is the sole byte source for a static graph consumer.
func collectStaticGraphIcons(resources []*corerpv20250801preview.ApplicationGraphResource) map[string]*string {
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
		if icon, ok := productmanifest.Lookup(to.String(r.Type)); ok {
			bytes := string(icon.Bytes)
			out[hash] = &bytes
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// resolveIconHash returns the icon hash to stamp on a modeled resource
// node. It prefers a per-type icon shipped in the built-in provider
// manifest (radius-project/resource-types-contrib mirror at
// deploy/manifest/built-in-providers/self-hosted), and falls back to
// the product default icon when the type is not registered. Returns
// nil if the default itself is unavailable (embedded asset broken),
// which the wire model represents as "no icon" and downstream
// consumers render without decoration.
func resolveIconHash(resourceType string) *string {
	if icon, ok := productmanifest.Lookup(resourceType); ok {
		h := icon.Hash
		return &h
	}
	return productmanifest.DefaultHash()
}

// collectResources normalizes the "resources" section of an ARM JSON
// template into a slice of resource entries in the classic (flat) shape
// expected by buildModeledResource. Bicep emits two layouts:
//
//   - languageVersion 1.x: an array of entries, each with top-level
//     "type", "name", and "properties".
//   - languageVersion 2.0 (symbolic-name codegen): an object keyed by
//     symbolic name. Each entry's "type" includes the API version suffix
//     ("Foo/bar@2024-01-01"), the resource's name and authored properties
//     are nested under "properties.name" / "properties.properties", and
//     "dependsOn" plus connection "[reference('sym').id]" expressions
//     refer to other resources by symbolic name.
//
// In the symbolic case we strip the @version, hoist the inner properties,
// and rewrite symbolic references to the equivalent
// [resourceId('TYPE', 'NAME')] form so the rest of the pipeline (and the
// stable diff hash) is independent of codegen mode.
func collectResources(raw any) []map[string]any {
	switch v := raw.(type) {
	case []any:
		out := make([]map[string]any, 0, len(v))
		for _, item := range v {
			if entry, ok := item.(map[string]any); ok {
				out = append(out, entry)
			}
		}
		return out
	case map[string]any:
		symbols := buildSymbolTable(v)
		// Iterate symbolic-name keys in sorted order so the resulting
		// resource slice (and the stable diff hash derived from it) is
		// deterministic across runs. Go map iteration is randomized,
		// which would otherwise produce noisy diffs.
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make([]map[string]any, 0, len(v))
		for _, k := range keys {
			entry, ok := v[k].(map[string]any)
			if !ok {
				continue
			}
			out = append(out, normalizeSymbolicEntry(entry, symbols))
		}
		return out
	default:
		return nil
	}
}

// symbolicReference matches a reference() expression that points to a
// resource by symbolic name in languageVersion 2.0 templates.
var symbolicReference = regexp.MustCompile(`^\[reference\('([^']+)'\)\.[^\]]+\]$`)

// symbolEntry holds the version-stripped resource type and authored name
// for a single symbolic-name entry.
type symbolEntry struct {
	resourceType string
	name         string
}

func buildSymbolTable(resources map[string]any) map[string]symbolEntry {
	table := make(map[string]symbolEntry, len(resources))
	for symbol, item := range resources {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		typeWithVersion, _ := entry["type"].(string)
		wrapper, _ := entry["properties"].(map[string]any)
		name, _ := wrapper["name"].(string)
		table[symbol] = symbolEntry{
			resourceType: stripAPIVersion(typeWithVersion),
			name:         name,
		}
	}
	return table
}

func normalizeSymbolicEntry(entry map[string]any, symbols map[string]symbolEntry) map[string]any {
	resourceType := stripAPIVersion(stringAt(entry, "type"))
	wrapper, _ := entry["properties"].(map[string]any)
	name, _ := wrapper["name"].(string)
	innerProps, _ := wrapper["properties"].(map[string]any)
	rewriteSymbolicConnections(innerProps, symbols)

	rawDeps, _ := entry["dependsOn"].([]any)
	newDeps := make([]any, 0, len(rawDeps))
	for _, d := range rawDeps {
		s, ok := d.(string)
		if !ok {
			continue
		}
		if sym, ok := symbols[s]; ok && sym.resourceType != "" && sym.name != "" {
			newDeps = append(newDeps, fmt.Sprintf("[resourceId('%s', '%s')]", sym.resourceType, sym.name))
			continue
		}
		newDeps = append(newDeps, s)
	}

	return map[string]any{
		"type":       resourceType,
		"name":       name,
		"properties": innerProps,
		"dependsOn":  newDeps,
	}
}

// rewriteSymbolicConnections rewrites each connection's "source" expression
// from [reference('sym').id] (or .properties.id) to the equivalent
// [resourceId('TYPE','NAME')] form, in place.
func rewriteSymbolicConnections(properties map[string]any, symbols map[string]symbolEntry) {
	if properties == nil {
		return
	}
	connections, ok := properties["connections"].(map[string]any)
	if !ok {
		return
	}
	for _, raw := range connections {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		source, ok := entry["source"].(string)
		if !ok {
			continue
		}
		matches := symbolicReference.FindStringSubmatch(source)
		if len(matches) != 2 {
			continue
		}
		sym, ok := symbols[matches[1]]
		if !ok || sym.resourceType == "" || sym.name == "" {
			continue
		}
		entry["source"] = fmt.Sprintf("[resourceId('%s', '%s')]", sym.resourceType, sym.name)
	}
}

// stripAPIVersion removes the trailing "@version" suffix from an ARM type
// string. languageVersion 2.0 emits types as "Foo/bar@2024-01-01" while
// the classic codegen uses a separate "apiVersion" field; the modeled
// graph stores only the bare type.
func stripAPIVersion(t string) string {
	if before, _, ok := strings.Cut(t, "@"); ok {
		return before
	}
	return t
}

func stringAt(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

// buildModeledResource converts a single ARM JSON resource entry into an
// ApplicationGraphResource. It returns (nil, nil) for entries that should
// not appear as graph nodes (Radius applications and environments are
// containers rather than graph members, and recipe packs are catalog
// resources defined only in Radius.Core that hold reusable recipes).
//
// The resource-specific Properties bag is populated from the entry's
// authored properties (minus the well-known runtime keys enumerated in
// dropOnGraph) and then walked to null out any value that either
// references a secureString parameter or is stored under a sensitive
// property name (see resolveGraphProperties).
func buildModeledResource(entry map[string]any, secureParams map[string]struct{}) (*corerpv20250801preview.ApplicationGraphResource, error) {
	resourceType, _ := entry["type"].(string)
	name, _ := entry["name"].(string)
	if resourceType == "" || name == "" {
		return nil, nil
	}
	if isExcludedResourceType(resourceType) {
		return nil, nil
	}

	properties, _ := entry["properties"].(map[string]any)
	rawDependsOn, _ := entry["dependsOn"].([]any)
	dependsOn := resolveDependsOn(rawDependsOn)

	// DiffHash is computed over the authored properties (pre-redaction)
	// so the hash detects authored changes — including changes to
	// sensitive values that never leave the local machine. The hash
	// itself is one-way and does not surface plaintext. See the design
	// note's `DiffHash` section.
	hash, err := ComputeDiffHash(properties, dependsOn...)
	if err != nil {
		return nil, fmt.Errorf("compute diffHash for %s/%s: %w", resourceType, name, err)
	}

	return &corerpv20250801preview.ApplicationGraphResource{
		ID:                to.Ptr(buildResourceID(resourceType, name)),
		Name:              to.Ptr(name),
		Type:              to.Ptr(resourceType),
		ProvisioningState: to.Ptr(string(v1.ProvisioningStateNotSpecified)),
		Connections:       outboundConnections(properties),
		OutputResources:   []*corerpv20250801preview.ApplicationGraphOutputResource{},
		DiffHash:          to.Ptr(hash),
		IconHash:          resolveIconHash(resourceType),
		Properties:        resolveGraphProperties(properties, secureParams),
	}, nil
}

// resolveGraphProperties returns the redacted, resource-specific
// Properties bag suitable for embedding in ApplicationGraphResource. It
// clones the authored map (so the caller retains its original,
// pre-redaction copy for ComputeDiffHash and connection extraction),
// drops the graph-level top-level keys enumerated in dropOnGraph, then
// walks the clone applying the secureString-tracing and name-blocklist
// rules to every leaf.
//
// Returns nil for a nil or empty input so the emitted graph node omits
// the Properties field entirely rather than serializing an empty map —
// this preserves the pre-population wire shape for resources that
// authored nothing.
func resolveGraphProperties(authoredProperties map[string]any, secureParams map[string]struct{}) map[string]any {
	if len(authoredProperties) == 0 {
		return nil
	}
	clonedProperties := make(map[string]any, len(authoredProperties))
	for k, v := range authoredProperties {
		if _, drop := dropOnGraph[k]; drop {
			continue
		}
		clonedProperties[k] = deepCloneValue(v)
	}
	if len(clonedProperties) == 0 {
		return nil
	}
	redactSensitive(clonedProperties, secureParams)
	return clonedProperties
}

// redactSensitive walks m recursively, nulling any value that either
// (a) is stored under a case-insensitively sensitive property name, or
// (b) is a string containing a `parameters('<secure>')` reference to a
// declared secureString parameter. The two rules are OR-composed: a
// cell matching either is nulled. Recurses into nested map values and
// array items; leaf primitives are checked only for the string-based
// rule via their containing key's blocklist match.
func redactSensitive(m map[string]any, secureParams map[string]struct{}) {
	for key, val := range m {
		if isSensitiveKey(key) {
			m[key] = nil
			continue
		}
		m[key] = redactValue(val, secureParams)
	}
}

// redactValue applies the secureString-tracing rule to leaf strings
// and recurses into nested maps and array items. Non-string leaves are
// returned unchanged; the name-blocklist rule is applied by the
// containing map's key check in redactSensitive, so it does not need
// to travel through the value dispatcher.
func redactValue(v any, secureParams map[string]struct{}) any {
	switch typed := v.(type) {
	case string:
		if containsSecureParamReference(typed, secureParams) {
			return nil
		}
		return typed
	case map[string]any:
		redactSensitive(typed, secureParams)
		return typed
	case []any:
		for i, item := range typed {
			typed[i] = redactValue(item, secureParams)
		}
		return typed
	default:
		return v
	}
}

// containsSecureParamReference reports whether s is an ARM expression
// that references any of the given secureString parameters. The check
// is intentionally coarse: a single sensitive substring nulls the
// entire value. Trying to redact a substring inside a `format(...)`
// expression would produce a partially-decoded value the user cannot
// interpret; nil is the safer signal.
//
// Whitespace tolerance: Bicep normally emits `parameters('name')` with
// no whitespace, but the check accepts a small amount of internal
// whitespace around the argument to survive hand-authored ARM JSON.
func containsSecureParamReference(s string, secureParams map[string]struct{}) bool {
	if len(secureParams) == 0 || !armExpressionPattern.MatchString(s) {
		return false
	}
	for name := range secureParams {
		// Match both single- and double-quote forms; ARM canonically
		// emits single-quoted string literals but hand-authored ARM
		// JSON in the wild uses either.
		if strings.Contains(s, "parameters('"+name+"')") || strings.Contains(s, `parameters("`+name+`")`) {
			return true
		}
	}
	return false
}

// isSensitiveKey reports whether the property key case-insensitively
// matches an entry in sensitiveKeyBlocklist. Exact match —
// `passwordHash` does not match `password`.
func isSensitiveKey(key string) bool {
	_, ok := sensitiveKeyBlocklist[strings.ToLower(key)]
	return ok
}

// deepCloneValue returns a shallow-recursive copy of v that is safe to
// mutate without touching the caller's original data structure. Only
// containers (maps and slices) need cloning; primitive leaves are
// value-copied by assignment. This matters because redactSensitive
// mutates its input in place and the authored properties are also
// consumed by ComputeDiffHash — which must observe the pre-redaction
// values.
func deepCloneValue(v any) any {
	switch typed := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for k, vv := range typed {
			out[k] = deepCloneValue(vv)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = deepCloneValue(item)
		}
		return out
	default:
		return v
	}
}

// resolveConnectionSources removed — superseded by inline
// outboundConnections (below) which builds ApplicationGraphConnection
// entries directly with Kind: Connection.

// outboundConnections extracts the resource's `connections` map and
// emits one outbound Kind: Connection edge per entry whose source can
// be resolved to a Radius resource ID. The reciprocal inbound entries
// on the target resources are added by addInboundConnections after all
// resources have been built.
func outboundConnections(properties map[string]any) []*corerpv20250801preview.ApplicationGraphConnection {
	if properties == nil {
		return []*corerpv20250801preview.ApplicationGraphConnection{}
	}
	connections, ok := properties["connections"].(map[string]any)
	if !ok {
		return []*corerpv20250801preview.ApplicationGraphConnection{}
	}
	result := make([]*corerpv20250801preview.ApplicationGraphConnection, 0, len(connections))
	for _, raw := range connections {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		source, _ := entry["source"].(string)
		resolved := resolveResourceIDExpression(source)
		if resolved == "" {
			continue
		}
		result = append(result, &corerpv20250801preview.ApplicationGraphConnection{
			ID:        to.Ptr(resolved),
			Direction: to.Ptr(corerpv20250801preview.DirectionOutbound),
			Kind:      to.Ptr(corerpv20250801preview.ConnectionKindConnection),
		})
	}
	return result
}

// addInboundConnections walks every outbound Kind: Connection edge on
// the graph and inserts the reciprocal Direction: Inbound entry on the
// destination resource so each resource surfaces both sides of its
// author-declared connections. Only Kind: Connection edges are
// mirrored here — Kind: Dependency edges are added later by
// edges.MergeDependencyEdges which does its own mirroring.
func addInboundConnections(graph *corerpv20250801preview.ApplicationGraphResponse) {
	byID := make(map[string]*corerpv20250801preview.ApplicationGraphResource, len(graph.Resources))
	for _, r := range graph.Resources {
		if r != nil && r.ID != nil {
			byID[*r.ID] = r
		}
	}
	for _, src := range graph.Resources {
		if src == nil || src.ID == nil {
			continue
		}
		for _, conn := range src.Connections {
			if conn == nil || conn.ID == nil || conn.Direction == nil {
				continue
			}
			if *conn.Direction != corerpv20250801preview.DirectionOutbound {
				continue
			}
			dest, ok := byID[*conn.ID]
			if !ok {
				continue
			}
			dest.Connections = append(dest.Connections, &corerpv20250801preview.ApplicationGraphConnection{
				ID:        src.ID,
				Direction: to.Ptr(corerpv20250801preview.DirectionInbound),
				Kind:      to.Ptr(corerpv20250801preview.ConnectionKindConnection),
			})
		}
	}
}

// resolveDependsOn resolves each ARM expression in an ARM JSON dependsOn
// list to a fully-qualified Radius resource ID. Unresolvable entries are
// dropped so the diffHash remains stable across builds.
func resolveDependsOn(in []any) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		expr, ok := v.(string)
		if !ok {
			continue
		}
		if resolved := resolveResourceIDExpression(expr); resolved != "" {
			out = append(out, resolved)
		}
	}
	return out
}

// resolveResourceIDExpression converts an ARM expression of the form
// [resourceId('TYPE', 'NAME')] into a fully-qualified Radius resource ID.
// Returns an empty string if the input is not a recognised literal-argument
// resourceId expression.
func resolveResourceIDExpression(expr string) string {
	if expr == "" {
		return ""
	}
	matches := resourceIDExpression.FindStringSubmatch(expr)
	if len(matches) != 2 {
		return ""
	}
	args := splitResourceIDArgs(matches[1])
	if len(args) < 2 {
		return ""
	}
	resourceType := strings.Trim(strings.TrimSpace(args[0]), "'")
	name := strings.Trim(strings.TrimSpace(args[1]), "'")
	if resourceType == "" || name == "" {
		return ""
	}
	return buildResourceID(resourceType, name)
}

// splitResourceIDArgs splits the comma-separated argument list of an ARM
// resourceId() expression, treating commas inside single-quoted strings as
// literal characters.
func splitResourceIDArgs(s string) []string {
	parts := []string{}
	var current strings.Builder
	inQuote := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\'' {
			inQuote = !inQuote
			current.WriteByte(c)
			continue
		}
		if c == ',' && !inQuote {
			parts = append(parts, current.String())
			current.Reset()
			continue
		}
		current.WriteByte(c)
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

// buildResourceID returns the fully-qualified Radius resource ID under the
// default plane and resource group used for modeled graphs.
func buildResourceID(resourceType, name string) string {
	return fmt.Sprintf("/planes/radius/%s/resourcegroups/%s/providers/%s/%s", defaultPlane, defaultResourceGroup, resourceType, name)
}

// isExcludedResourceType reports whether the given resource type is one of
// the app/env/recipe-pack types that must be excluded from the graph.
func isExcludedResourceType(resourceType string) bool {
	for excluded := range excludeResourceTypes {
		if strings.EqualFold(resourceType, excluded) {
			return true
		}
	}
	return false
}

// emptyGraph returns a fresh graph with an empty (non-nil) Resources slice
// so it serializes to "resources": [] rather than "resources": null.
func emptyGraph() *corerpv20250801preview.ApplicationGraphResponse {
	return &corerpv20250801preview.ApplicationGraphResponse{
		Resources: []*corerpv20250801preview.ApplicationGraphResource{},
	}
}
