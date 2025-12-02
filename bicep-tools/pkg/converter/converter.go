package converter

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/Azure/bicep-types/src/bicep-types-go/factory"
	"github.com/Azure/bicep-types/src/bicep-types-go/index"
	"github.com/Azure/bicep-types/src/bicep-types-go/types"
	"github.com/Azure/bicep-types/src/bicep-types-go/writers"
	"github.com/radius-project/radius/bicep-tools/pkg/manifest"
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
			resourceName := resourceType.Name

			// Extract resource type and API version from the full name
			// Expected format: "Test.Resources/userTypeAlpha@2023-10-01-preview"
			parts := strings.Split(resourceName, "@")
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid resource name format: %s", resourceName)
			}

			resourceTypeWithoutVersion := parts[0] // "Test.Resources/userTypeAlpha"
			apiVersion := parts[1]                 // "2023-10-01-preview"

			// Initialize the resource map if it doesn't exist
			if typeIndex.Resources == nil {
				typeIndex.Resources = make(map[string]index.ResourceVersionMap)
			}

			// Initialize the version map for this resource type if it doesn't exist
			if typeIndex.Resources[resourceTypeWithoutVersion] == nil {
				typeIndex.Resources[resourceTypeWithoutVersion] = make(index.ResourceVersionMap)
			}

			// Create cross-file type reference for this resource (to types.json)
			typeRef := types.CrossFileTypeReference{Ref: i, RelativePath: "types.json"}
			typeIndex.Resources[resourceTypeWithoutVersion][apiVersion] = typeRef
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
	bodyType := typeFactory.CreateObjectType(qualifiedName, nil, nil, nil)
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
			Flags:       types.TypePropertyFlagsReadOnly | types.TypePropertyFlagsDeployTimeConstant,
			Description: "The API version.",
		},
		"type": {
			Type:        typeFactory.GetReference(typeFactory.CreateStringLiteralType(fmt.Sprintf("%s/%s", provider.Namespace, resourceTypeName))),
			Flags:       types.TypePropertyFlagsReadOnly | types.TypePropertyFlagsDeployTimeConstant,
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
		typeFactory.GetReference(bodyType),
		types.ScopeTypeNone,
		types.ScopeTypeNone,
		nil,
	)

	return typeFactory.GetReference(resourceTypeRef), nil
}

// addSchemaType converts a manifest schema to a Bicep type
// Equivalent to TypeScript function addSchemaType()
func addSchemaType(schema *manifest.Schema, name string, typeFactory *factory.TypeFactory) (types.ITypeReference, error) {
	return addSchemaTypeInternal(schema, name, typeFactory, false)
}

// addSchemaTypeInternal converts a manifest schema to a Bicep type with additional context.
// inPlatformOptions indicates whether we are currently traversing within a platformOptions property.
func addSchemaTypeInternal(schema *manifest.Schema, name string, typeFactory *factory.TypeFactory, inPlatformOptions bool) (types.ITypeReference, error) {
	// Handle empty schema type (default to object, matching TypeScript behavior)
	schemaType := schema.Type
	if schemaType == "" {
		schemaType = "object"
	}

	switch schemaType {
	case "string":
		// Handle the edge case: string with enum constraint
		if len(schema.Enum) > 0 {
			var enumTypeRefs []types.ITypeReference
			for _, value := range schema.Enum {
				stringLiteralType := typeFactory.CreateStringLiteralType(value)
				enumTypeRefs = append(enumTypeRefs, typeFactory.GetReference(stringLiteralType))
			}
			unionType := typeFactory.CreateUnionType(enumTypeRefs)
			return typeFactory.GetReference(unionType), nil
		}

		// Regular string without constraints
		stringType := typeFactory.CreateStringType()
		return typeFactory.GetReference(stringType), nil

	case "enum":
		// Handle explicit enum type
		if len(schema.Enum) == 0 {
			return nil, fmt.Errorf("enum type '%s' must have at least one value in 'enum' property", name)
		}
		var enumTypeRefs []types.ITypeReference
		for _, value := range schema.Enum {
			stringLiteralType := typeFactory.CreateStringLiteralType(value)
			enumTypeRefs = append(enumTypeRefs, typeFactory.GetReference(stringLiteralType))
		}
		unionType := typeFactory.CreateUnionType(enumTypeRefs)
		return typeFactory.GetReference(unionType), nil

	case "integer":
		intType := typeFactory.CreateIntegerType()
		return typeFactory.GetReference(intType), nil

	case "boolean":
		boolType := typeFactory.CreateBooleanType()
		return typeFactory.GetReference(boolType), nil

	case "any":
		if !inPlatformOptions {
			return nil, fmt.Errorf("'any' type is only allowed for additionalProperties in platformOptions")
		}
		anyType := typeFactory.CreateAnyType()
		return typeFactory.GetReference(anyType), nil

	case "array":
		if schema.Items == nil {
			return nil, fmt.Errorf("array type '%s' must have an 'items' property", name)
		}
		itemRef, err := addSchemaTypeInternal(schema.Items, name+"Item", typeFactory, inPlatformOptions)
		return typeFactory.GetReference(typeFactory.CreateArrayType(itemRef)), err

	case "object":
		objectProperties, err := addObjectPropertiesInternal(schema, typeFactory, inPlatformOptions)
		if err != nil {
			return nil, fmt.Errorf("failed to add object properties: %w", err)
		}

		objectType := typeFactory.CreateObjectType(name, nil, nil, nil)
		objectType.Properties = objectProperties

		// Handle additionalProperties if specified
		if schema.AdditionalProperties != nil {
			additionalPropsRef, err := addSchemaTypeInternal(
				schema.AdditionalProperties,
				name+"AdditionalProperties",
				typeFactory,
				inPlatformOptions,
			)
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

// addObjectPropertiesInternal converts manifest schema properties to Bicep object properties with context tracking
func addObjectPropertiesInternal(schema *manifest.Schema, typeFactory *factory.TypeFactory, inPlatformOptions bool) (map[string]types.ObjectTypeProperty, error) {
	result := make(map[string]types.ObjectTypeProperty)

	if schema.Properties == nil {
		return result, nil
	}

	for key, propSchema := range schema.Properties {
		property, err := addObjectProperty(schema, key, &propSchema, typeFactory, inPlatformOptions)
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
	inPlatformOptions bool,
) (types.ObjectTypeProperty, error) {

	// Track whether we're entering platformOptions
	childInPlatformOptions := inPlatformOptions
	if key == "platformOptions" {
		childInPlatformOptions = true
	}

	propertyTypeRef, err := addSchemaTypeInternal(property, key, typeFactory, childInPlatformOptions)
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
