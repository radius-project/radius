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

// PropertiesOutputFormat holds a nested field path as Heading and its schema definition.
type PropertiesOutputFormat struct {
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

// PropertyTitleStatus defines the status of properties title in the output display.
type PropertyTitleStatus string

const (
	// PropertyTitleNone indicates that no property title is displayed.
	PropertyTitleNone PropertyTitleStatus = "None"
	// PropertyTitleTopLevel indicates that the top-level properties are displayed.
	PropertyTitleTopLevel PropertyTitleStatus = "TopLevelProperties"
	// PropertyTitleObjectLevel indicates that object-level properties are displayed.
	PropertyTitleObjectLevel PropertyTitleStatus = "ObjectLevelProperties"
)

// display prints the resource type schema details for each APIVersion in a structured format.
// Example of an output format:
// TYPE                          NAMESPACE
// Test.Resources/userTypeAlpha  Test.Resources
//
// DESCRIPTION:
// This is a user type that supports recipes.
// It is designed to handle various resource configurations
// and provides flexibility for managing application and environment
// settings. The user type includes properties such as application,
// environment, database, host, port, username, and password.
// These properties are essential for defining the resource schema
// and ensuring proper integration with the system.
//
// API VERSION : 2023-10-01-preview
//
// TOP-LEVEL PROPERTIES:
//
// NAME         TYPE      REQUIRED  READ-ONLY  DESCRIPTION
// application  string    true      false      The resource ID of the application.
// database     string    false     false      The name of the database.
// environment  string    true      false      The resource ID of the environment.
// host         string    false     true       The host name of the database.
// password     string    false     false      The password for the database.
// port         string    false     false      The port number of the database.
// test         object    false     false      A test object for demonstration purposes.
// username     string    false     false      The username for the database.
//
// OBJECT PROPERTIES:
//
// test
//
// NAME        TYPE      REQUIRED  READ-ONLY  DESCRIPTION
// name        string    false     false      The name of the test object.
// nestedType  object    false     false      A nested object within the test object.
//
// test.nestedType
//
// NAME            TYPE      REQUIRED  READ-ONLY  DESCRIPTION
// nestedProperty  string    false     false      A property within the nested object.

func (r *Runner) display(resourceTypeDetails *common.ResourceType) error {
	r.Output.LogInfo("\nDESCRIPTION:")
	r.Output.LogInfo("%s", resourceTypeDetails.Description)
	for apiVersion, apiVersionProperties := range resourceTypeDetails.APIVersions {
		r.Output.LogInfo("API VERSION: %s\n", apiVersion)
		propertyTitleStatus := PropertyTitleNone
		if apiVersionProperties.Schema != nil {
			resourceTypeSchema := GetResourceTypeSchema(apiVersionProperties.Schema)
			queue := list.New()
			queue.PushBack(PropertiesOutputFormat{
				Schema: FieldSchema{
					Properties: resourceTypeSchema,
					Type:       "object",
				},
			})

			for queue.Len() > 0 {
				front := queue.Front()
				queue.Remove(front)
				schema := front.Value.(PropertiesOutputFormat)
				schemaList := []FieldSchema{}
				for _, property := range schema.Schema.Properties {
					if property.Type == "object" {
						heading := property.Name
						if propertyTitleStatus != PropertyTitleNone {
							heading = schema.Heading + "." + property.Name
						}
						queue.PushBack(PropertiesOutputFormat{
							Heading: heading,
							Schema:  property,
						})
					}
					schemaList = append(schemaList, property)
				}
				sort.Slice(schemaList, func(i, j int) bool {
					return schemaList[i].Name < schemaList[j].Name
				})
				if propertyTitleStatus == PropertyTitleNone {
					propertyTitleStatus = PropertyTitleTopLevel
					r.Output.LogInfo("TOP-LEVEL PROPERTIES:\n")
				} else if propertyTitleStatus == PropertyTitleTopLevel {
					propertyTitleStatus = PropertyTitleObjectLevel
					r.Output.LogInfo("OBJECT PROPERTIES:\n")
				}

				if schema.Heading != "" {
					r.Output.LogInfo("%s\n", schema.Heading)
				}

				err := r.Output.WriteFormatted(r.Format, schemaList, common.GetResourceTypeShowSchemaTableFormat())
				if err != nil {
					return err
				}
				r.Output.LogInfo("")
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
