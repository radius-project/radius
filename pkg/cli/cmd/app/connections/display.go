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

package connections

import (
	"fmt"
	"sort"
	"strings"
)

// display builds the formatted output for the application graph as text.
func display(graph *applicationGraph) string {
	applicationResources := []resourceEntry{}
	for _, resource := range graph.Resources {
		applicationResources = append(applicationResources, resource)
	}

	// Sort by type (containers first), and then by name, then by id.
	containerType := "Applications.Core/containers"
	sort.Slice(applicationResources, func(i, j int) bool {
		if strings.EqualFold(applicationResources[i].Type, containerType) !=
			strings.EqualFold(applicationResources[j].Type, containerType) {

			return strings.EqualFold(applicationResources[i].Type, containerType)
		}

		if applicationResources[i].Type != applicationResources[j].Type {
			return applicationResources[i].Type < applicationResources[j].Type
		}

		if applicationResources[i].Name != applicationResources[j].Name {
			return applicationResources[i].Name < applicationResources[j].Name
		}

		if applicationResources[i].ID != applicationResources[j].ID {
			return applicationResources[i].ID < applicationResources[j].ID
		}

		return applicationResources[i].Error < applicationResources[j].Error
	})

	output := &strings.Builder{}

	output.WriteString(fmt.Sprintf("Displaying application: %s\n\n", graph.ApplicationName))

	if len(applicationResources) == 0 {
		output.WriteString("(empty)")
		output.WriteString("\n\n")
		return output.String()
	}

	for _, resource := range applicationResources {
		if resource.Error != "" {
			output.WriteString(fmt.Sprintf("Error: %s\n", resource.Error))
			continue
		}

		output.WriteString(fmt.Sprintf("Name: %s (%s)\n", resource.Name, resource.Type))

		if len(resource.Connections) == 0 {
			output.WriteString("Connections: (none)\n")
		} else {
			output.WriteString("Connections:\n")
			for _, connection := range resource.Connections {
				if connection.To.Error != "" {
					output.WriteString(fmt.Sprintf("  %s -> %s (%s)\n", connection.From.Name, "error", connection.To.Error))
					continue
				}

				// We format the connection differently depending on whether it's inbound or outbound.
				if connection.From.ID == resource.ID {
					// Outbound
					output.WriteString(fmt.Sprintf("  %s -> %s (%s)\n", connection.From.Name, connection.To.Name, connection.To.Type))
				} else {
					// Inbound
					output.WriteString(fmt.Sprintf("  %s (%s) -> %s\n", connection.From.Name, connection.From.Type, connection.To.Name))
				}
			}
		}

		if len(resource.Resources) == 0 {
			output.WriteString("Resources: (none)\n")
		} else {
			output.WriteString("Resources:\n")
			for _, resource := range resource.Resources {
				if resource.Error != "" {
					output.WriteString(fmt.Sprintf("Error: %s\n", resource.Error))
					continue
				}

				link := makeHyperlink(resource)
				if link == "" {
					output.WriteString(fmt.Sprintf("  %s (%s: %s)\n", resource.Name, resource.Provider, resource.Type))
				} else {
					output.WriteString(fmt.Sprintf("  %s (%s: %s) %s\n", resource.Name, resource.Provider, resource.Type, link))
				}
			}
		}

		output.WriteString("\n")
	}

	return output.String()
}

func makeHyperlink(resource outputResourceEntry) string {
	// Just azure for now.
	if resource.Provider != "azure" {
		return ""
	}

	// format of an Azure portal URL:
	//
	// https://portal.azure.com/#@{tenantId}/resource{resourceId}
	url := fmt.Sprintf("https://portal.azure.com/#@%s/resource%s", "72f988bf-86f1-41af-91ab-2d7cd011db47", resource.ID)

	// This is the magic incantation for a console hyperlink.
	// \x1b]8;;h { URL } \x07 { link text } \x1b]8;;\x07
	return fmt.Sprintf("\x1b]8;;%s\x07%s\x1b]8;;\x07\n", url, "open portal")
}
