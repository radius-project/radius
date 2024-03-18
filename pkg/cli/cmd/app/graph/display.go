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
	"sort"
	"strings"

	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

// display builds the formatted output for the application graph as text.
func display(applicationResources []*v20231001preview.ApplicationGraphResource, applicationName string) string {
	// Sort by type (containers first), and then by other types, name and then by id.
	containerType := "Applications.Core/containers"
	sort.Slice(applicationResources, func(i, j int) bool {
		if strings.EqualFold(*applicationResources[i].Type, containerType) !=
			strings.EqualFold(*applicationResources[j].Type, containerType) {

			return strings.EqualFold(*applicationResources[i].Type, containerType)
		}

		if *applicationResources[i].Type != *applicationResources[j].Type {
			return *applicationResources[i].Type < *applicationResources[j].Type
		}

		if *applicationResources[i].Name != *applicationResources[j].Name {
			return *applicationResources[i].Name < *applicationResources[j].Name
		}
		return *applicationResources[i].ID < *applicationResources[j].ID

	})

	output := &strings.Builder{}
	output.WriteString(fmt.Sprintf("Displaying application: %s\n\n", applicationName))

	if len(applicationResources) == 0 {
		output.WriteString("(empty)")
		output.WriteString("\n\n")
		return output.String()
	}

	for _, resource := range applicationResources {
		output.WriteString(fmt.Sprintf("Name: %s (%s)\n", *resource.Name, *resource.Type))

		if len(resource.Connections) == 0 {
			output.WriteString("Connections: (none)\n")
		} else {
			output.WriteString("Connections:\n")
			for _, connection := range resource.Connections {
				connectionID, err := resources.Parse(*connection.ID)
				if err != nil {
					output.WriteString(fmt.Sprintf("Error: %s\n", err.Error()))
					continue
				}

				connectionName := connectionID.Name()
				connectionType := connectionID.Type()

				if *connection.Direction == v20231001preview.DirectionOutbound {
					// Outbound
					output.WriteString(fmt.Sprintf("  %s -> %s (%s)\n", *resource.Name, connectionName, connectionType))
				} else {
					// Inbound
					output.WriteString(fmt.Sprintf("  %s (%s) -> %s\n", connectionName, connectionType, *resource.Name))
				}
			}
		}

		if len(resource.OutputResources) == 0 {
			output.WriteString("Resources: (none)\n")
		} else {
			output.WriteString("Resources:\n")
			for _, resource := range resource.OutputResources {
				link := makeHyperlink(resource)
				if link == "" {
					output.WriteString(fmt.Sprintf("  %s (%s)\n \n", *resource.Name, *resource.Type))
				} else {
					output.WriteString(fmt.Sprintf("  %s (%s)\n", link, *resource.Type))
				}
			}
		}

		output.WriteString("\n")

	}
	return output.String()
}

func makeHyperlink(resource *v20231001preview.ApplicationGraphOutputResource) string {
	// Just azure for now.
	provider := providerFromID(*resource.ID)
	if provider != "azure" {
		return ""
	}
	// https://portal.azure.com/#@{tenantId}/resource{resourceId}
	url := fmt.Sprintf("https://portal.azure.com/#@%s/resource%s", "72f988bf-86f1-41af-91ab-2d7cd011db47", *resource.ID)

	// This is the magic incantation for a console hyperlink.
	// \x1b]8;;h { URL } \x07 { link text } \x1b]8;;\x07
	return fmt.Sprintf("\x1b]8;;%s\x07%s\x1b]8;;\x07", url, *resource.Name)
}
