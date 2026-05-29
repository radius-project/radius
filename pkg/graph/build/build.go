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

package build

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	corerpv20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/to"
)

// armTemplate represents the relevant parts of a compiled ARM JSON template.
type armTemplate struct {
	Schema          string                 `json:"$schema"`
	LanguageVersion string                 `json:"languageVersion"`
	Resources       map[string]armResource `json:"resources"`
}

// armResource represents a single resource in the ARM template resources map.
type armResource struct {
	Type       string                 `json:"type"`
	Properties map[string]interface{} `json:"properties"`
	DependsOn  []string               `json:"dependsOn"`
}

// resourceIDRegexp matches resourceId('Type', 'name') expressions in ARM JSON.
var resourceIDRegexp = regexp.MustCompile(`resourceId\('([^']+)',\s*'([^']+)'\)`)

const (
	defaultResourceGroup       = "default"
	defaultPlane               = "/planes/radius/local"
	provisioningStateSucceeded = "Succeeded"
)

// BuildStaticGraph parses a compiled ARM JSON file and returns the application
// graph as a corerpv20250801preview.ApplicationGraphResponse.
func BuildStaticGraph(armJSONPath string) (*corerpv20250801preview.ApplicationGraphResponse, error) {
	armData, err := os.ReadFile(armJSONPath)
	if err != nil {
		return nil, fmt.Errorf("reading ARM JSON file %s: %w", armJSONPath, err)
	}

	var tmpl armTemplate
	if err := json.Unmarshal(armData, &tmpl); err != nil {
		return nil, fmt.Errorf("parsing ARM JSON: %w", err)
	}

	// Build resource lookup for symbolic name → constructed resource ID.
	resourceIDs := make(map[string]string, len(tmpl.Resources))
	for symbolicName, res := range tmpl.Resources {
		resourceIDs[symbolicName] = constructResourceID(res)
	}

	// Build resources and connections.
	resources := make([]*corerpv20250801preview.ApplicationGraphResource, 0, len(tmpl.Resources))
	for _, res := range tmpl.Resources {
		r := &corerpv20250801preview.ApplicationGraphResource{
			ID:                to.Ptr(constructResourceID(res)),
			Type:              to.Ptr(extractResourceType(res.Type)),
			Name:              to.Ptr(extractResourceName(res)),
			ProvisioningState: to.Ptr(provisioningStateSucceeded),
			Connections:       []*corerpv20250801preview.ApplicationGraphConnection{},
			OutputResources:   []*corerpv20250801preview.ApplicationGraphOutputResource{},
		}

		// Extract connections from properties.connections.
		r.Connections = append(r.Connections, extractConnections(res, resourceIDs)...)

		// Extract dependency edges from dependsOn.
		for _, dep := range res.DependsOn {
			if depID, ok := resourceIDs[dep]; ok {
				r.Connections = append(r.Connections, &corerpv20250801preview.ApplicationGraphConnection{
					ID:        to.Ptr(depID),
					Direction: to.Ptr(corerpv20250801preview.DirectionOutbound),
				})
			}
		}

		resources = append(resources, r)
	}

	// Sort resources by ID for deterministic output.
	sort.Slice(resources, func(i, j int) bool {
		return derefString(resources[i].ID) < derefString(resources[j].ID)
	})

	// Add inbound connections (reverse edges).
	addInboundConnections(resources)

	return &corerpv20250801preview.ApplicationGraphResponse{
		Resources: resources,
	}, nil
}

func extractAuthorableProperties(res armResource) map[string]interface{} {
	if nested, ok := res.Properties["properties"].(map[string]interface{}); ok {
		return nested
	}

	return res.Properties
}

// constructResourceID builds a Radius-style resource ID from an ARM resource.
func constructResourceID(res armResource) string {
	resourceType := extractResourceType(res.Type)
	resourceName := extractResourceName(res)

	return fmt.Sprintf("%s/resourcegroups/%s/providers/%s/%s",
		defaultPlane, defaultResourceGroup, resourceType, resourceName)
}

// extractResourceType extracts the resource type without the API version.
// Input: "Applications.Core/containers@2023-10-01-preview" → "Applications.Core/containers"
func extractResourceType(fullType string) string {
	if idx := strings.Index(fullType, "@"); idx != -1 {
		return fullType[:idx]
	}
	return fullType
}

// extractResourceName gets the resource name from ARM properties or type.
func extractResourceName(res armResource) string {
	if name, ok := res.Properties["name"]; ok {
		if s, ok := name.(string); ok {
			return s
		}
	}
	// Fall back to last segment of type.
	parts := strings.Split(extractResourceType(res.Type), "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return "unknown"
}

// extractConnections parses the properties.connections map and resolves
// resourceId() expressions to full resource IDs.
func extractConnections(res armResource, resourceIDs map[string]string) []*corerpv20250801preview.ApplicationGraphConnection {
	connMap, ok := extractAuthorableProperties(res)["connections"]
	if !ok {
		return nil
	}

	connectionsObj, ok := connMap.(map[string]interface{})
	if !ok {
		return nil
	}

	var connections []*corerpv20250801preview.ApplicationGraphConnection
	// Sort keys for deterministic output.
	keys := make([]string, 0, len(connectionsObj))
	for k := range connectionsObj {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		connVal := connectionsObj[key]
		connObj, ok := connVal.(map[string]interface{})
		if !ok {
			continue
		}

		source, ok := connObj["source"].(string)
		if !ok {
			continue
		}

		// Try to resolve resourceId() expression.
		targetID := resolveResourceIDExpression(source, resourceIDs)
		if targetID != "" {
			connections = append(connections, &corerpv20250801preview.ApplicationGraphConnection{
				ID:        to.Ptr(targetID),
				Direction: to.Ptr(corerpv20250801preview.DirectionOutbound),
			})
		}
	}

	return connections
}

// resolveResourceIDExpression resolves a resourceId('Type', 'name') expression.
func resolveResourceIDExpression(expr string, resourceIDs map[string]string) string {
	// Handle bracket expressions like "[resourceId('Type', 'name')]"
	expr = strings.TrimPrefix(expr, "[")
	expr = strings.TrimSuffix(expr, "]")

	matches := resourceIDRegexp.FindStringSubmatch(expr)
	if len(matches) < 3 {
		return ""
	}

	resourceType := extractResourceType(matches[1])
	resourceName := matches[2]

	return fmt.Sprintf("%s/resourcegroups/%s/providers/%s/%s",
		defaultPlane, defaultResourceGroup, resourceType, resourceName)
}

// addInboundConnections adds reverse (Inbound) connection edges to resources
// that are referenced by outbound connections from other resources.
func addInboundConnections(resources []*corerpv20250801preview.ApplicationGraphResource) {
	resourceByID := make(map[string]*corerpv20250801preview.ApplicationGraphResource, len(resources))
	for _, r := range resources {
		resourceByID[derefString(r.ID)] = r
	}

	for _, r := range resources {
		for _, conn := range r.Connections {
			if conn.Direction == nil || *conn.Direction != corerpv20250801preview.DirectionOutbound {
				continue
			}

			target, ok := resourceByID[derefString(conn.ID)]
			if !ok {
				continue
			}

			// Check if inbound connection already exists.
			alreadyExists := false
			for _, existing := range target.Connections {
				if derefString(existing.ID) == derefString(r.ID) &&
					existing.Direction != nil && *existing.Direction == corerpv20250801preview.DirectionInbound {
					alreadyExists = true
					break
				}
			}

			if !alreadyExists {
				target.Connections = append(target.Connections, &corerpv20250801preview.ApplicationGraphConnection{
					ID:        r.ID,
					Direction: to.Ptr(corerpv20250801preview.DirectionInbound),
				})
			}
		}
	}
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
