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
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	corerpv20231001preview "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/to"
)

// StaticGraphArtifact is the JSON envelope committed to .radius/static/app.json.
type StaticGraphArtifact struct {
	Version     string                                          `json:"version"`
	GeneratedAt string                                          `json:"generatedAt"`
	SourceFile  string                                          `json:"sourceFile"`
	Application corerpv20231001preview.ApplicationGraphResponse `json:"application"`
}

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

// resourceDeclRegexp matches Bicep resource declarations like "resource foo 'Type@version' = {"
var resourceDeclRegexp = regexp.MustCompile(`^\s*resource\s+(\w+)\s+'[^']+'\s*=`)

const (
	defaultResourceGroup       = "default"
	defaultPlane               = "/planes/radius/local"
	provisioningStateSucceeded = "Succeeded"
	artifactVersion            = "1.0.0"
)

// BuildStaticGraph parses a compiled ARM JSON file and the original Bicep source
// to produce a StaticGraphArtifact.
func BuildStaticGraph(armJSONPath, bicepPath string) (*StaticGraphArtifact, error) {
	armData, err := os.ReadFile(armJSONPath)
	if err != nil {
		return nil, fmt.Errorf("reading ARM JSON file %s: %w", armJSONPath, err)
	}

	var tmpl armTemplate
	if err := json.Unmarshal(armData, &tmpl); err != nil {
		return nil, fmt.Errorf("parsing ARM JSON: %w", err)
	}

	// Parse source line mappings from the Bicep file.
	lineMap, err := parseSourceLineMap(bicepPath)
	if err != nil {
		// Non-fatal: we can still build the graph without line numbers.
		lineMap = map[string]int{}
	}

	// Build resource lookup for symbolic name → constructed resource ID.
	resourceIDs := make(map[string]string, len(tmpl.Resources))
	for symbolicName, res := range tmpl.Resources {
		resourceIDs[symbolicName] = constructResourceID(res)
	}

	// Build resources and connections.
	resources := make([]*corerpv20231001preview.ApplicationGraphResource, 0, len(tmpl.Resources))
	for symbolicName, res := range tmpl.Resources {
		resourceID := resourceIDs[symbolicName]
		resourceType := extractResourceType(res.Type)
		resourceName := extractResourceName(res)

		graphResource := &corerpv20231001preview.ApplicationGraphResource{
			ID:                to.Ptr(resourceID),
			Type:              to.Ptr(resourceType),
			Name:              to.Ptr(resourceName),
			ProvisioningState: to.Ptr(provisioningStateSucceeded),
			Connections:       []*corerpv20231001preview.ApplicationGraphConnection{},
			OutputResources:   []*corerpv20231001preview.ApplicationGraphOutputResource{},
		}

		// Copy authorable codeReference from properties.
		if codeRef, ok := res.Properties["codeReference"]; ok {
			if s, ok := codeRef.(string); ok {
				graphResource.CodeReference = to.Ptr(s)
			}
		}

		// Map source line number.
		if line, ok := lineMap[symbolicName]; ok {
			lineInt32 := int32(line)
			graphResource.AppDefinitionLine = &lineInt32
		}

		// Extract connections from properties.connections.
		connections := extractConnections(res, resourceIDs)
		graphResource.Connections = append(graphResource.Connections, connections...)

		// Extract dependency edges from dependsOn.
		for _, dep := range res.DependsOn {
			if depID, ok := resourceIDs[dep]; ok {
				graphResource.Connections = append(graphResource.Connections, &corerpv20231001preview.ApplicationGraphConnection{
					ID:        to.Ptr(depID),
					Direction: to.Ptr(corerpv20231001preview.DirectionOutbound),
				})
			}
		}

		// Compute diff hash.
		hash := ComputeDiffHash(res.Properties)
		graphResource.DiffHash = to.Ptr(hash)

		resources = append(resources, graphResource)
	}

	// Sort resources by ID for deterministic output.
	sort.Slice(resources, func(i, j int) bool {
		return to.String(resources[i].ID) < to.String(resources[j].ID)
	})

	// Add inbound connections (reverse edges).
	addInboundConnections(resources)

	artifact := &StaticGraphArtifact{
		Version:     artifactVersion,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		SourceFile:  bicepPath,
		Application: corerpv20231001preview.ApplicationGraphResponse{
			Resources: resources,
		},
	}

	return artifact, nil
}

// parseSourceLineMap reads a Bicep file and maps symbolic resource names to their
// declaration line numbers (1-based).
func parseSourceLineMap(bicepPath string) (map[string]int, error) {
	f, err := os.Open(bicepPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	lineMap := make(map[string]int)
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		matches := resourceDeclRegexp.FindStringSubmatch(line)
		if len(matches) >= 2 {
			lineMap[matches[1]] = lineNum
		}
	}
	return lineMap, scanner.Err()
}

// constructResourceID builds a Radius-style resource ID from an ARM resource.
func constructResourceID(res armResource) string {
	resourceType := extractResourceType(res.Type)
	resourceName := extractResourceName(res)

	// Build the standard Radius resource ID path.
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
func extractConnections(res armResource, resourceIDs map[string]string) []*corerpv20231001preview.ApplicationGraphConnection {
	connMap, ok := res.Properties["connections"]
	if !ok {
		return nil
	}

	connectionsObj, ok := connMap.(map[string]interface{})
	if !ok {
		return nil
	}

	var connections []*corerpv20231001preview.ApplicationGraphConnection
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
			connections = append(connections, &corerpv20231001preview.ApplicationGraphConnection{
				ID:        to.Ptr(targetID),
				Direction: to.Ptr(corerpv20231001preview.DirectionOutbound),
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
func addInboundConnections(resources []*corerpv20231001preview.ApplicationGraphResource) {
	resourceByID := make(map[string]*corerpv20231001preview.ApplicationGraphResource, len(resources))
	for _, r := range resources {
		resourceByID[to.String(r.ID)] = r
	}

	for _, r := range resources {
		for _, conn := range r.Connections {
			if conn.Direction == nil || *conn.Direction != corerpv20231001preview.DirectionOutbound {
				continue
			}

			target, ok := resourceByID[to.String(conn.ID)]
			if !ok {
				continue
			}

			// Check if inbound connection already exists.
			sourceID := to.String(r.ID)
			alreadyExists := false
			for _, existing := range target.Connections {
				if to.String(existing.ID) == sourceID && existing.Direction != nil && *existing.Direction == corerpv20231001preview.DirectionInbound {
					alreadyExists = true
					break
				}
			}

			if !alreadyExists {
				target.Connections = append(target.Connections, &corerpv20231001preview.ApplicationGraphConnection{
					ID:        to.Ptr(sourceID),
					Direction: to.Ptr(corerpv20231001preview.DirectionInbound),
				})
			}
		}
	}
}
