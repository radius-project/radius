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
	"container/list"
	"fmt"
	"slices"
	"sort"

	"github.com/radius-project/radius/pkg/cli/cmd/resourcetype/common"
)

// HeadingToSchema holds a nested field path and its schema definition.
type HeadingToSchema struct {
	// Heading is the path to this field (e.g. ".database.server.name").
	Heading string
	// Schema contains the field's metadata, such as type and nested properties.
	Schema FieldSchema
}

// FieldSchema represents the schema of a field in a resource type.
type FieldSchema struct {
	// Name is the name of the field.
	Name string
	// Type is the type of the field (e.g. "string", "object", etc.).
	Type string
	// Description provides additional information about the field.
	Description string
	// IsRequired indicates if the field is required.
	IsRequired bool
	// IsReadOnly indicates if the field is read-only.
	IsReadOnly bool
	// Properties contains nested fields if the type is "object".
	Properties map[string]FieldSchema
}

// display prints the resource type schema details for each APIVersion in a structured format.
func (r *Runner) display(resourceTypeDetails *common.ResourceType) error {
	for apiVersion, apiVersionProperties := range resourceTypeDetails.APIVersions {
		r.Output.LogInfo("API VERSION:%s\n", apiVersion)
		if apiVersionProperties.Schema != nil {
			resourceTypeSchema := GetResourceTypeSchema(apiVersionProperties.Schema)
			queue := list.New()
			queue.PushBack(HeadingToSchema{
				Schema: FieldSchema{
					Properties: resourceTypeSchema,
					Type:       "object",
				},
			})

			for queue.Len() > 0 {
				front := queue.Front()
				queue.Remove(front)
				schema := front.Value.(HeadingToSchema)
				schemaList := []FieldSchema{}
				for _, property := range schema.Schema.Properties {
					if property.Type == "object" {
						queue.PushBack(HeadingToSchema{
							Heading: schema.Heading + "." + property.Name,
							Schema:  property,
						})
					}
					schemaList = append(schemaList, property)
				}
				sort.Slice(schemaList, func(i, j int) bool {
					return schemaList[i].Name < schemaList[j].Name
				})
				r.Output.LogInfo("%s\n", schema.Heading)
				err := r.Output.WriteFormatted(r.Format, schemaList, common.GetResourceTypeShowSchemaTableFormat())
				if err != nil {
					return err
				}
				r.Output.LogInfo("\n")
			}
		}
	}

	return nil
}

// GetResourceTypeSchema extracts the field schema from each fields in the resource type schema.
// It returns a map where the keys are property names and the values are FieldSchema objects.
func GetResourceTypeSchema(schema map[string]any) map[string]FieldSchema {
	fieldSchema := make(map[string]FieldSchema)
	if schema == nil {
		return fieldSchema
	}
	if properties, ok := schema["properties"].(map[string]any); ok {
		requiredList := []string{}
		if required, ok := schema["required"]; ok {
			r, ok := required.([]any)
			if ok {
				for _, req := range r {
					if reqStr, ok := req.(string); ok {
						requiredList = append(requiredList, reqStr)
					} else {
						fmt.Println("Invalid required property:", req)
					}
				}
			}
		}
		for propertyName, property := range properties {
			prop, ok := property.(map[string]any)
			if !ok {
				continue
			}
			schemaType := prop["type"].(string)
			description := ""
			if desc, ok := prop["description"]; ok {
				description = desc.(string)
			}

			isRequired := false
			if slices.Contains(requiredList, propertyName) {
				isRequired = true
			}

			isReadOnly := false
			if readOnly, ok := prop["readOnly"]; ok && readOnly == true {
				isReadOnly = true
			}

			fieldSchema[propertyName] = FieldSchema{
				Name:        propertyName,
				Type:        schemaType,
				Description: description,
				IsRequired:  isRequired,
				IsReadOnly:  isReadOnly,
				Properties:  GetResourceTypeSchema(prop),
			}
		}
	}

	return fieldSchema
}
