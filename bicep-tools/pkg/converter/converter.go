package converter

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/radius-project/radius/bicep-tools/pkg/manifest"
	"github.com/radius-project/radius/bicep-types/src/bicep-types-go/factory"
	"github.com/radius-project/radius/bicep-types/src/bicep-types-go/index"
	"github.com/radius-project/radius/bicep-types/src/bicep-types-go/types"
	"github.com/radius-project/radius/bicep-types/src/bicep-types-go/writers"
)

// ConversionResult represents the output of converting a manifest to Bicep types
type ConversionResult struct {
	TypesContent         string
	IndexContent         string
	DocumentationContent string
}

// Convert transforms a ResourceProvider manifest into Bicep extension files
// Equivalent to TypeScript function convert()
func Convert(provider *manifest.ResourceProvider) (*ConversionResult, error) {
	typeFactory := factory.NewTypeFactory()

	// Process each resource type and API version
	for resourceTypeName, resourceType := range provider.Types {
		for apiVersionName, apiVersion := range resourceType.APIVersions {
			_, err := addResourceTypeForAPIVersion(
				provider,
				resourceTypeName,
				&resourceType,
				apiVersionName,
				&apiVersion,
				typeFactory,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to add resource type %s@%s: %w",
					resourceTypeName, apiVersionName, err)
			}
		}
	}

	// Build the type index
	typesArray := typeFactory.GetTypes()

	// Create type index settings with lowercase name format
	normalizedName := strings.ReplaceAll(strings.ToLower(provider.Namespace), ".", "")
	settings := &index.TypeSettings{
		Name:        fmt.Sprintf("radius%s", normalizedName),
		Version:     "0.0.1",
		IsSingleton: false,
	}

	typeIndex := &index.TypeIndex{
		Resources:         make(map[string]index.ResourceVersionMap),
		ResourceFunctions: make(map[string]index.ResourceFunctionVersionMap),
		Settings:          settings,
	}

	// Populate the index with resources
	for i, t := range typesArray {
		if resourceType, ok := t.(*types.ResourceType); ok {
			resourceName := resourceType.ResourceTypeID
			apiVersion := resourceType.APIVersion

			// Create version map if it doesn't exist
			if typeIndex.Resources[resourceName] == nil {
				typeIndex.Resources[resourceName] = make(index.ResourceVersionMap)
			}

			// Create cross-file type reference for this resource (to types.json)
			typeRef := types.CrossFileTypeReference{Ref: i, RelativePath: "types.json"}
			typeIndex.Resources[resourceName][apiVersion] = typeRef
		}
	}

	// Generate output content
	jsonWriter := writers.NewJSONWriter()
	typesContent, err := jsonWriter.WriteTypesToString(typesArray)
	if err != nil {
		return nil, fmt.Errorf("failed to write types JSON: %w", err)
	}

	indexContent, err := jsonWriter.WriteTypeIndexToString(typeIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to write index JSON: %w", err)
	}

	markdownWriter := writers.NewMarkdownWriter()
	var docBuffer bytes.Buffer
	err = markdownWriter.WriteTypeIndex(&docBuffer, typeIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to write markdown documentation: %w", err)
	}
	documentationContent := docBuffer.String()

	return &ConversionResult{
		TypesContent:         typesContent,
		IndexContent:         indexContent,
		DocumentationContent: documentationContent,
	}, nil
}

// addResourceTypeForAPIVersion creates a resource type for a specific API version
// Equivalent to TypeScript function addResourceTypeForApiVersion()
func addResourceTypeForAPIVersion(
	provider *manifest.ResourceProvider,
	resourceTypeName string,
	resourceType *manifest.ResourceType,
	apiVersionName string,
	apiVersion *manifest.APIVersion,
	typeFactory *factory.TypeFactory,
) (types.ITypeReference, error) {

	qualifiedName := fmt.Sprintf("%s/%s@%s", provider.Namespace, resourceTypeName, apiVersionName)

	// Create the properties type from the schema
	propertyTypeRef, err := addSchemaType(&apiVersion.Schema, resourceTypeName+"Properties", typeFactory)
	if err != nil {
		return nil, fmt.Errorf("failed to create properties type: %w", err)
	}

	// Create the resource body type with standard Azure resource properties
	bodyType := typeFactory.CreateObjectType(qualifiedName)
	bodyType.Properties = map[string]types.ObjectTypeProperty{
		"name": {
			Type:        typeFactory.GetReference(typeFactory.CreateStringType()),
			Flags:       types.TypePropertyFlagsRequired | types.TypePropertyFlagsIdentifier,
			Description: "The resource name.",
		},
		"location": {
			Type:        typeFactory.GetReference(typeFactory.CreateStringType()),
			Flags:       types.TypePropertyFlagsNone,
			Description: "The resource location.",
		},
		"properties": {
			Type:        propertyTypeRef,
			Flags:       types.TypePropertyFlagsRequired,
			Description: "The resource properties.",
		},
		"apiVersion": {
			Type:        typeFactory.GetReference(typeFactory.CreateStringLiteralType(apiVersionName)),
			Flags:       types.TypePropertyFlagsReadOnly | types.TypePropertyFlagsDeployTime | types.TypePropertyFlagsConstant,
			Description: "The API version.",
		},
		"type": {
			Type:        typeFactory.GetReference(typeFactory.CreateStringLiteralType(fmt.Sprintf("%s/%s", provider.Namespace, resourceTypeName))),
			Flags:       types.TypePropertyFlagsReadOnly | types.TypePropertyFlagsDeployTime | types.TypePropertyFlagsConstant,
			Description: "The resource type.",
		},
		"id": {
			Type:        typeFactory.GetReference(typeFactory.CreateStringType()),
			Flags:       types.TypePropertyFlagsReadOnly,
			Description: "The resource id.",
		},
	}

	// Create the resource type
	resourceTypeRef := typeFactory.CreateResourceType(
		qualifiedName,
		fmt.Sprintf("%s/%s", provider.Namespace, resourceTypeName),
		apiVersionName,
		typeFactory.GetReference(bodyType),
	)

	return typeFactory.GetReference(resourceTypeRef), nil
}

// addSchemaType converts a manifest schema to a Bicep type
// Equivalent to TypeScript function addSchemaType()
func addSchemaType(schema *manifest.Schema, name string, typeFactory *factory.TypeFactory) (types.ITypeReference, error) {
	// Handle empty schema type (default to object, matching TypeScript behavior)
	schemaType := schema.Type
	if schemaType == "" {
		schemaType = "object"
	}

	switch schemaType {
	case "string":
		stringType := typeFactory.CreateStringType()
		return typeFactory.GetReference(stringType), nil

	case "integer":
		intType := typeFactory.CreateIntegerType()
		return typeFactory.GetReference(intType), nil

	case "boolean":
		boolType := typeFactory.CreateBooleanType()
		return typeFactory.GetReference(boolType), nil

	case "any":
		anyType := typeFactory.CreateAnyType()
		return typeFactory.GetReference(anyType), nil

	case "object":
		objectProperties, err := addObjectProperties(schema, typeFactory)
		if err != nil {
			return nil, fmt.Errorf("failed to add object properties: %w", err)
		}

		objectType := typeFactory.CreateObjectType(name)
		objectType.Properties = objectProperties

		// Handle additionalProperties if specified
		if schema.AdditionalProperties != nil {
			additionalPropsRef, err := addSchemaType(schema.AdditionalProperties, name+"_AdditionalProperties", typeFactory)
			if err != nil {
				return nil, fmt.Errorf("failed to add additional properties: %w", err)
			}
			objectType.AdditionalProperties = additionalPropsRef
		}

		return typeFactory.GetReference(objectType), nil

	default:
		return nil, fmt.Errorf("unsupported schema type: %s", schemaType)
	}
}

// addObjectProperties converts manifest schema properties to Bicep object properties
// Equivalent to TypeScript function addObjectProperties()
func addObjectProperties(schema *manifest.Schema, typeFactory *factory.TypeFactory) (map[string]types.ObjectTypeProperty, error) {
	result := make(map[string]types.ObjectTypeProperty)

	if schema.Properties == nil {
		return result, nil
	}

	for key, propSchema := range schema.Properties {
		property, err := addObjectProperty(schema, key, &propSchema, typeFactory)
		if err != nil {
			return nil, fmt.Errorf("failed to add property %s: %w", key, err)
		}
		result[key] = property
	}

	return result, nil
}

// addObjectProperty converts a single manifest property to a Bicep object property
// Equivalent to TypeScript function addObjectProperty()
func addObjectProperty(
	parent *manifest.Schema,
	key string,
	property *manifest.Schema,
	typeFactory *factory.TypeFactory,
) (types.ObjectTypeProperty, error) {

	propertyTypeRef, err := addSchemaType(property, key, typeFactory)
	if err != nil {
		return types.ObjectTypeProperty{}, fmt.Errorf("failed to create property type: %w", err)
	}

	// Calculate property flags
	var flags types.TypePropertyFlags = types.TypePropertyFlagsNone

	// Check if this property is required
	if parent.Required != nil {
		for _, requiredProp := range parent.Required {
			if requiredProp == key {
				flags |= types.TypePropertyFlagsRequired
				break
			}
		}
	}

	// Check if this property is read-only
	if property.ReadOnly != nil && *property.ReadOnly {
		flags |= types.TypePropertyFlagsReadOnly
	}

	description := ""
	if property.Description != nil {
		description = *property.Description
	}

	return types.ObjectTypeProperty{
		Type:        propertyTypeRef,
		Flags:       flags,
		Description: description,
	}, nil
}
