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

package show

import (
	"slices"
	"strings"

	"github.com/radius-project/radius/pkg/cli/cmd/resourcetype/common"
)

func display(resourceType *common.ResourceType) string {
	output := &strings.Builder{}
	indentSpaceCount := 0
	output.WriteString("\n\nDESCRIPTION:\n")
	output.WriteString(indent(resourceType.Description, indentSpaceCount+2) + "\n\n") // Add indentation to the description
	for name, properties := range resourceType.APIVersions {
		output.WriteString("APIVERSION : " + name + "\n\n")
		indentSpaceCount += 2
		requiredProperties := map[string]any{}
		readonlonlyProperties := map[string]any{}
		optionalProperties := map[string]any{}
		requiredPropertiesKeys := []string{}
		if properties.Schema["required"] != nil {
			required := properties.Schema["required"].([]any)
			for _, prop := range required {
				requiredPropertiesKeys = append(requiredPropertiesKeys, prop.(string))
			}
		}

		if properties.Schema["properties"] != nil {
			for propName, prop := range properties.Schema["properties"].(map[string]any) {
				prp, ok := prop.(map[string]any)
				if ok {
					if prp["readOnly"] == true {
						readonlonlyProperties[propName] = prop
					} else if slices.Contains(requiredPropertiesKeys, propName) {
						requiredProperties[propName] = prop
					} else {
						optionalProperties[propName] = prop
					}
				}

			}
		}

		writeProperties := func(output *strings.Builder, title string, indentSpaceCount int, props map[string]any) {
			output.WriteString(indent(title+":\n", indentSpaceCount))
			for propName, prop := range props {
				prps := prop.(map[string]any)
				t := prps["type"].(string)
				desc := ""
				if ok := prps["description"]; ok != nil {
					desc = prps["description"].(string)
				}
				output.WriteString(indent("- "+propName+" ("+t+") "+desc, indentSpaceCount+2) + "\n")
			}
			output.WriteString("\n")
		}

		writeProperties(output, "REQUIRED PROPERTIES", indentSpaceCount, requiredProperties)
		writeProperties(output, "OPTIONAL PROPERTIES", indentSpaceCount, optionalProperties)
		writeProperties(output, "READONLY PROPERTIES", indentSpaceCount, readonlonlyProperties)
		indentSpaceCount -= 2
	}

	return output.String()
}

// Helper function to add indentation to each line of a string
func indent(input string, spaceCount int) string {
	indentStr := strings.Repeat(" ", spaceCount)
	lines := strings.Split(input, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = indentStr + line
		}
	}
	return strings.Join(lines, "\n")
}
