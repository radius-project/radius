package converter

import (
	"strings"
	"testing"

	"github.com/Azure/bicep-types/src/bicep-types-go/factory"
	"github.com/Azure/bicep-types/src/bicep-types-go/types"
	"github.com/radius-project/radius/bicep-tools/pkg/manifest"
)

func TestPlatformOptionsAllowsAnyAdditionalProperties(t *testing.T) {
	schema := &manifest.Schema{
		Type: "object",
		Properties: map[string]manifest.Schema{
			"platformOptions": {
				Type: "object",
				AdditionalProperties: &manifest.Schema{
					Type: "any",
				},
			},
		},
	}

	typeFactory := factory.NewTypeFactory()

	typeRef, err := addSchemaType(schema, "test", typeFactory)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	ref, ok := typeRef.(types.TypeReference)
	if !ok {
		t.Fatalf("expected TypeReference, got %T", typeRef)
	}

	allTypes := typeFactory.GetTypes()
	topLevelObj, ok := allTypes[ref.Ref].(*types.ObjectType)
	if !ok {
		t.Fatalf("expected object type, got %T", allTypes[ref.Ref])
	}

	platformProp, found := topLevelObj.Properties["platformOptions"]
	if !found {
		t.Fatalf("expected platformOptions property to exist")
	}

	platformTypeRef, ok := platformProp.Type.(types.TypeReference)
	if !ok {
		t.Fatalf("expected platformOptions property to reference a type, got %T", platformProp.Type)
	}

	platformType, ok := allTypes[platformTypeRef.Ref].(*types.ObjectType)
	if !ok {
		t.Fatalf("expected platformOptions type to be an ObjectType, got %T", allTypes[platformTypeRef.Ref])
	}

	additionalRef, ok := platformType.AdditionalProperties.(types.TypeReference)
	if !ok {
		t.Fatalf("expected additionalProperties to be a TypeReference, got %T", platformType.AdditionalProperties)
	}

	if _, ok := allTypes[additionalRef.Ref].(*types.AnyType); !ok {
		t.Fatalf("expected additionalProperties to resolve to AnyType, got %T", allTypes[additionalRef.Ref])
	}
}

func TestNonPlatformOptionsAnyAdditionalPropertiesReturnsError(t *testing.T) {
	schema := &manifest.Schema{
		Type: "object",
		Properties: map[string]manifest.Schema{
			"connections": {
				Type: "object",
				AdditionalProperties: &manifest.Schema{
					Type: "any",
				},
			},
		},
	}

	typeFactory := factory.NewTypeFactory()

	_, err := addSchemaType(schema, "test", typeFactory)
	if err == nil {
		t.Fatalf("expected an error but got none")
	}

	if !strings.Contains(err.Error(), "only allowed for additionalProperties in platformOptions") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestDirectAnyPropertyReturnsError(t *testing.T) {
	schema := &manifest.Schema{
		Type: "object",
		Properties: map[string]manifest.Schema{
			"dynamic": {
				Type: "any",
			},
		},
	}

	typeFactory := factory.NewTypeFactory()

	_, err := addSchemaType(schema, "test", typeFactory)
	if err == nil {
		t.Fatalf("expected an error but got none")
	}

	if !strings.Contains(err.Error(), "only allowed for additionalProperties in platformOptions") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestAddResourceTypeForAPIVersion(t *testing.T) {
	provider := &manifest.ResourceProvider{
		Namespace: "Applications.Test",
		Types: map[string]manifest.ResourceType{
			"testResources": {
				APIVersions: map[string]manifest.APIVersion{
					"2021-01-01": {
						Schema: manifest.Schema{
							Type: "object",
							Properties: map[string]manifest.Schema{
								"a": {Type: "string"},
								"b": {Type: "string"},
							},
						},
					},
				},
			},
		},
	}

	resourceType := provider.Types["testResources"]
	apiVersion := resourceType.APIVersions["2021-01-01"]
	typeFactory := factory.NewTypeFactory()

	result, err := addResourceTypeForAPIVersion(
		provider,
		"testResources",
		&resourceType,
		"2021-01-01",
		&apiVersion,
		typeFactory,
	)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result to not be nil")
	}

	// Verify the resource type was created
	allTypes := typeFactory.GetTypes()
	var addedResourceType *types.ResourceType

	for _, typ := range allTypes {
		if rt, ok := typ.(*types.ResourceType); ok {
			addedResourceType = rt
			break
		}
	}

	if addedResourceType == nil {
		t.Fatal("Expected to find a ResourceType in the factory")
	}

	expectedName := "Applications.Test/testResources@2021-01-01"
	if addedResourceType.Name != expectedName {
		t.Errorf("Expected resource name '%s', got '%s'", expectedName, addedResourceType.Name)
	}

	nameParts := strings.Split(addedResourceType.Name, "@")
	if len(nameParts) != 2 {
		t.Fatalf("Expected resource name to contain '@' separating type and version, got '%s'", addedResourceType.Name)
	}

	expectedResourceTypeID := "Applications.Test/testResources"
	if nameParts[0] != expectedResourceTypeID {
		t.Errorf("Expected resource type ID '%s', got '%s'", expectedResourceTypeID, nameParts[0])
	}

	expectedAPIVersion := "2021-01-01"
	if nameParts[1] != expectedAPIVersion {
		t.Errorf("Expected API version '%s', got '%s'", expectedAPIVersion, nameParts[1])
	}

	// Verify the body type was created correctly
	if addedResourceType.Body == nil {
		t.Fatal("Expected resource body to not be nil")
	}

	// Find the body object type
	bodyTypeRef, ok := addedResourceType.Body.(types.TypeReference)
	if !ok {
		t.Fatal("Expected body to be a TypeReference")
	}
	bodyType, ok := allTypes[bodyTypeRef.Ref].(*types.ObjectType)
	if !ok {
		t.Fatal("Expected body to be an ObjectType")
	}

	// Check that standard properties exist
	expectedProperties := []string{"name", "location", "properties", "apiVersion", "type", "id"}
	for _, prop := range expectedProperties {
		if _, exists := bodyType.Properties[prop]; !exists {
			t.Errorf("Expected property '%s' to exist", prop)
		}
	}

	// Verify the properties object has the schema properties
	propertiesProperty := bodyType.Properties["properties"]
	propertiesTypeRef, ok := propertiesProperty.Type.(types.TypeReference)
	if !ok {
		t.Fatal("Expected properties type to be a TypeReference")
	}
	propertiesType, ok := allTypes[propertiesTypeRef.Ref].(*types.ObjectType)
	if !ok {
		t.Fatal("Expected properties to be an ObjectType")
	}

	if _, exists := propertiesType.Properties["a"]; !exists {
		t.Error("Expected property 'a' to exist in properties object")
	}

	if _, exists := propertiesType.Properties["b"]; !exists {
		t.Error("Expected property 'b' to exist in properties object")
	}
}

func TestAddSchemaType_String(t *testing.T) {
	schema := &manifest.Schema{Type: "string"}
	typeFactory := factory.NewTypeFactory()

	result, err := addSchemaType(schema, "test", typeFactory)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	allTypes := typeFactory.GetTypes()
	typeRef, ok := result.(types.TypeReference)
	if !ok {
		t.Fatal("Expected result to be a TypeReference")
	}
	addedType := allTypes[typeRef.Ref]

	if _, ok := addedType.(*types.StringType); !ok {
		t.Error("Expected result to be a StringType")
	}
}

func TestAddSchemaType_Integer(t *testing.T) {
	schema := &manifest.Schema{Type: "integer"}
	typeFactory := factory.NewTypeFactory()

	result, err := addSchemaType(schema, "test", typeFactory)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	allTypes := typeFactory.GetTypes()
	typeRef, ok := result.(types.TypeReference)
	if !ok {
		t.Fatal("Expected result to be a TypeReference")
	}
	addedType := allTypes[typeRef.Ref]

	if _, ok := addedType.(*types.IntegerType); !ok {
		t.Error("Expected result to be an IntegerType")
	}
}

func TestAddSchemaType_Boolean(t *testing.T) {
	schema := &manifest.Schema{Type: "boolean"}
	typeFactory := factory.NewTypeFactory()

	result, err := addSchemaType(schema, "test", typeFactory)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	allTypes := typeFactory.GetTypes()
	typeRef, ok := result.(types.TypeReference)
	if !ok {
		t.Fatal("Expected result to be a TypeReference")
	}
	addedType := allTypes[typeRef.Ref]

	if _, ok := addedType.(*types.BooleanType); !ok {
		t.Error("Expected result to be a BooleanType")
	}
}

func TestAddSchemaType_Object(t *testing.T) {
	schema := &manifest.Schema{
		Type: "object",
		Properties: map[string]manifest.Schema{
			"a": {Type: "string"},
			"b": {Type: "string"},
		},
	}
	typeFactory := factory.NewTypeFactory()

	result, err := addSchemaType(schema, "test", typeFactory)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	allTypes := typeFactory.GetTypes()
	typeRef, ok := result.(types.TypeReference)
	if !ok {
		t.Fatal("Expected result to be a TypeReference")
	}
	addedType, ok := allTypes[typeRef.Ref].(*types.ObjectType)
	if !ok {
		t.Fatal("Expected result to be an ObjectType")
	}

	if len(addedType.Properties) != 2 {
		t.Errorf("Expected 2 properties, got %d", len(addedType.Properties))
	}

	if _, exists := addedType.Properties["a"]; !exists {
		t.Error("Expected property 'a' to exist")
	}

	if _, exists := addedType.Properties["b"]; !exists {
		t.Error("Expected property 'b' to exist")
	}
}

func TestAddSchemaType_UnsupportedType(t *testing.T) {
	schema := &manifest.Schema{Type: "unsupported"}
	typeFactory := factory.NewTypeFactory()

	_, err := addSchemaType(schema, "test", typeFactory)
	if err == nil {
		t.Error("Expected error for unsupported type, got nil")
	}
}

func TestAddObjectProperties(t *testing.T) {
	schema := &manifest.Schema{
		Type: "object",
		Properties: map[string]manifest.Schema{
			"a": {Type: "string"},
			"b": {Type: "string"},
		},
	}
	typeFactory := factory.NewTypeFactory()

	result, err := addObjectPropertiesInternal(schema, typeFactory, false)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 properties, got %d", len(result))
	}

	if _, exists := result["a"]; !exists {
		t.Error("Expected property 'a' to exist")
	}

	if _, exists := result["b"]; !exists {
		t.Error("Expected property 'b' to exist")
	}
}

func TestAddObjectProperty(t *testing.T) {
	parent := &manifest.Schema{
		Type:       "object",
		Properties: map[string]manifest.Schema{},
	}

	description := "cool description"
	property := &manifest.Schema{
		Type:        "string",
		Description: &description,
	}

	typeFactory := factory.NewTypeFactory()

	result, err := addObjectProperty(parent, "a", property, typeFactory, false)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result.Description != "cool description" {
		t.Errorf("Expected description 'cool description', got '%s'", result.Description)
	}

	if result.Flags != types.TypePropertyFlagsNone {
		t.Errorf("Expected flags to be None, got %v", result.Flags)
	}

	allTypes := typeFactory.GetTypes()
	typeRef, ok := result.Type.(types.TypeReference)
	if !ok {
		t.Fatal("Expected result type to be a TypeReference")
	}
	addedType := allTypes[typeRef.Ref]
	if _, ok := addedType.(*types.StringType); !ok {
		t.Error("Expected property type to be StringType")
	}
}

func TestAddObjectProperty_ReadOnly(t *testing.T) {
	parent := &manifest.Schema{
		Type:       "object",
		Properties: map[string]manifest.Schema{},
	}

	readOnly := true
	description := "cool description"
	property := &manifest.Schema{
		Type:        "string",
		Description: &description,
		ReadOnly:    &readOnly,
	}

	typeFactory := factory.NewTypeFactory()

	result, err := addObjectProperty(parent, "a", property, typeFactory, false)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result.Description != "cool description" {
		t.Errorf("Expected description 'cool description', got '%s'", result.Description)
	}

	expectedFlags := types.TypePropertyFlagsReadOnly
	if result.Flags != expectedFlags {
		t.Errorf("Expected flags to be ReadOnly, got %v", result.Flags)
	}
}

func TestAddObjectProperty_Required(t *testing.T) {
	parent := &manifest.Schema{
		Type:       "object",
		Properties: map[string]manifest.Schema{},
		Required:   []string{"a"},
	}

	description := "cool description"
	property := &manifest.Schema{
		Type:        "string",
		Description: &description,
	}

	typeFactory := factory.NewTypeFactory()

	result, err := addObjectProperty(parent, "a", property, typeFactory, false)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result.Description != "cool description" {
		t.Errorf("Expected description 'cool description', got '%s'", result.Description)
	}

	expectedFlags := types.TypePropertyFlagsRequired
	if result.Flags != expectedFlags {
		t.Errorf("Expected flags to be Required, got %v", result.Flags)
	}
}

func TestConvert(t *testing.T) {
	provider := &manifest.ResourceProvider{
		Namespace: "Applications.Test",
		Types: map[string]manifest.ResourceType{
			"testResources": {
				APIVersions: map[string]manifest.APIVersion{
					"2021-01-01": {
						Schema: manifest.Schema{
							Type: "object",
							Properties: map[string]manifest.Schema{
								"a": {Type: "string"},
								"b": {Type: "integer"},
							},
						},
						Capabilities: []string{"Recipes"},
					},
				},
			},
		},
	}

	result, err := Convert(provider)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result to not be nil")
	}

	if result.TypesContent == "" {
		t.Error("Expected types content to not be empty")
	}

	if result.IndexContent == "" {
		t.Error("Expected index content to not be empty")
	}

	if result.DocumentationContent == "" {
		t.Error("Expected documentation content to not be empty")
	}

	// Basic validation that the types content is valid JSON
	if result.TypesContent[0] != '[' {
		t.Error("Expected types content to start with '['")
	}

	// Basic validation that the index content is valid JSON
	if result.IndexContent[0] != '{' {
		t.Error("Expected index content to start with '{'")
	}
}

func TestAddSchemaType_Array_StringItems(t *testing.T) {
	schema := &manifest.Schema{
		Type: "array",
		Items: &manifest.Schema{
			Type: "string",
		},
	}
	typeFactory := factory.NewTypeFactory()

	result, err := addSchemaType(schema, "testArray", typeFactory)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	allTypes := typeFactory.GetTypes()
	typeRef, ok := result.(types.TypeReference)
	if !ok {
		t.Fatal("Expected result to be a TypeReference")
	}
	added := allTypes[typeRef.Ref]
	if _, ok := added.(*types.ArrayType); !ok {
		t.Error("Expected added type to be an ArrayType")
	}
}

func TestAddSchemaType_Array_ObjectItems(t *testing.T) {
	schema := &manifest.Schema{
		Type: "array",
		Items: &manifest.Schema{
			Type: "object",
			Properties: map[string]manifest.Schema{
				"name":  {Type: "string"},
				"value": {Type: "integer"},
			},
		},
	}
	typeFactory := factory.NewTypeFactory()

	result, err := addSchemaType(schema, "testObjectArray", typeFactory)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	allTypes := typeFactory.GetTypes()
	typeRef, ok := result.(types.TypeReference)
	if !ok {
		t.Fatal("Expected result to be a TypeReference")
	}
	added := allTypes[typeRef.Ref]
	if _, ok := added.(*types.ArrayType); !ok {
		t.Error("Expected added type to be an ArrayType")
	}
}

func TestAddSchemaType_NestedArray(t *testing.T) {
	schema := &manifest.Schema{
		Type: "array",
		Items: &manifest.Schema{
			Type: "array",
			Items: &manifest.Schema{
				Type: "string",
			},
		},
	}
	typeFactory := factory.NewTypeFactory()

	result, err := addSchemaType(schema, "nestedArray", typeFactory)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	allTypes := typeFactory.GetTypes()
	typeRef, ok := result.(types.TypeReference)
	if !ok {
		t.Fatal("Expected result to be a TypeReference")
	}
	added := allTypes[typeRef.Ref]
	if _, ok := added.(*types.ArrayType); !ok {
		t.Error("Expected added type to be an ArrayType")
	}
}

func TestAddSchemaType_Array_NoItems_Error(t *testing.T) {
	schema := &manifest.Schema{
		Type: "array",
	}
	typeFactory := factory.NewTypeFactory()

	_, err := addSchemaType(schema, "testArray", typeFactory)
	if err == nil {
		t.Fatal("Expected error for array without items, got nil")
	}
	expected := "must have an 'items' property"
	if !strings.Contains(err.Error(), expected) {
		t.Fatalf("Expected error to contain %q, got %v", expected, err)
	}
}

// ...existing code...

func TestAddSchemaType_Enum(t *testing.T) {
	schema := &manifest.Schema{
		Type: "enum",
		Enum: []string{"value1", "value2", "value3"},
	}
	typeFactory := factory.NewTypeFactory()

	result, err := addSchemaType(schema, "testEnum", typeFactory)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	allTypes := typeFactory.GetTypes()
	typeRef, ok := result.(types.TypeReference)
	if !ok {
		t.Fatal("Expected result to be a TypeReference")
	}
	addedType, ok := allTypes[typeRef.Ref].(*types.UnionType)
	if !ok {
		t.Fatal("Expected result to be a UnionType")
	}

	if len(addedType.Elements) != 3 {
		t.Errorf("Expected 3 enum elements, got %d", len(addedType.Elements))
	}

	// Verify each enum value is a string literal
	expectedValues := []string{"value1", "value2", "value3"}
	for i, element := range addedType.Elements {
		elementRef, ok := element.(types.TypeReference)
		if !ok {
			t.Fatalf("Expected element %d to be a TypeReference", i)
		}
		stringLiteral, ok := allTypes[elementRef.Ref].(*types.StringLiteralType)
		if !ok {
			t.Fatalf("Expected element %d to be a StringLiteralType", i)
		}
		if stringLiteral.Value != expectedValues[i] {
			t.Errorf("Expected element %d value '%s', got '%s'", i, expectedValues[i], stringLiteral.Value)
		}
	}
}

func TestAddSchemaType_StringWithEnum(t *testing.T) {
	schema := &manifest.Schema{
		Type: "string",
		Enum: []string{"apple", "banana", "cherry"},
	}
	typeFactory := factory.NewTypeFactory()

	result, err := addSchemaType(schema, "fruit", typeFactory)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	allTypes := typeFactory.GetTypes()
	typeRef, ok := result.(types.TypeReference)
	if !ok {
		t.Fatal("Expected result to be a TypeReference")
	}
	addedType, ok := allTypes[typeRef.Ref].(*types.UnionType)
	if !ok {
		t.Fatal("Expected result to be a UnionType")
	}

	if len(addedType.Elements) != 3 {
		t.Errorf("Expected 3 enum elements, got %d", len(addedType.Elements))
	}

	// Verify each element is a StringLiteralType with the correct value
	expectedValues := []string{"apple", "banana", "cherry"}
	for i, element := range addedType.Elements {
		elementRef, ok := element.(types.TypeReference)
		if !ok {
			t.Fatalf("Expected element %d to be a TypeReference", i)
		}
		stringLiteral, ok := allTypes[elementRef.Ref].(*types.StringLiteralType)
		if !ok {
			t.Fatalf("Expected element %d to be a StringLiteralType", i)
		}
		if stringLiteral.Value != expectedValues[i] {
			t.Errorf("Expected element %d value '%s', got '%s'", i, expectedValues[i], stringLiteral.Value)
		}
	}
}

func TestAddSchemaType_EnumWithoutValues(t *testing.T) {
	schema := &manifest.Schema{
		Type: "enum",
		Enum: []string{},
	}
	typeFactory := factory.NewTypeFactory()

	_, err := addSchemaType(schema, "testEnum", typeFactory)
	if err == nil {
		t.Fatal("Expected error for enum without values, got nil")
	}

	expectedError := "enum type 'testEnum' must have at least one value in 'enum' property"
	if err.Error() != expectedError {
		t.Errorf("Expected error message '%s', got '%s'", expectedError, err.Error())
	}
}

func TestAddObjectProperties_WithAdditionalProperties(t *testing.T) {
	schema := &manifest.Schema{
		Type: "object",
		Properties: map[string]manifest.Schema{
			"name": {
				Type: "string",
			},
			"connections": {
				Type: "object",
				AdditionalProperties: &manifest.Schema{
					Type: "object",
					Properties: map[string]manifest.Schema{
						"url": {
							Type: "string",
						},
					},
				},
			},
		},
	}
	typeFactory := factory.NewTypeFactory()

	result, err := addObjectPropertiesInternal(schema, typeFactory, false)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 properties, got %d", len(result))
	}

	if _, exists := result["name"]; !exists {
		t.Error("Expected property 'name' to exist")
	}

	if _, exists := result["connections"]; !exists {
		t.Error("Expected property 'connections' to exist")
	}

	// Verify that connections property was created correctly
	connectionsProperty := result["connections"]
	allTypes := typeFactory.GetTypes()
	connectionsTypeRef, ok := connectionsProperty.Type.(types.TypeReference)
	if !ok {
		t.Fatal("Expected connections type to be a TypeReference")
	}
	connectionsType, ok := allTypes[connectionsTypeRef.Ref].(*types.ObjectType)
	if !ok {
		t.Fatal("Expected connections to be an ObjectType")
	}

	if connectionsType.AdditionalProperties == nil {
		t.Error("Expected connections to have additionalProperties defined")
	}
}

func TestAddSchemaType_AdditionalPropertiesAny_Error(t *testing.T) {
	schema := &manifest.Schema{
		Type: "object",
		Properties: map[string]manifest.Schema{
			"name": {
				Type: "string",
			},
		},
		AdditionalProperties: &manifest.Schema{
			Type:        "any", // "any" type is not supported
			Description: stringPtr("A map of key-value pairs"),
		},
	}
	typeFactory := factory.NewTypeFactory()

	_, err := addSchemaType(schema, "testWithAnyAdditionalProps", typeFactory)
	if err == nil {
		t.Fatal("Expected error for 'any' type in additionalProperties, got nil")
	}

	expectedError := "'any' type is only allowed for additionalProperties in platformOptions"
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("Expected error to contain '%s', got '%s'", expectedError, err.Error())
	}
}

// Helper function for creating string pointers
func stringPtr(s string) *string {
	return &s
}

func TestAddSchemaType_ObjectWithOnlyProperties(t *testing.T) {
	schema := &manifest.Schema{
		Type: "object",
		Properties: map[string]manifest.Schema{
			"fixedProp": {
				Type: "string",
			},
		},
	}
	typeFactory := factory.NewTypeFactory()

	result, err := addSchemaType(schema, "testWithOnlyProperties", typeFactory)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	allTypes := typeFactory.GetTypes()
	typeRef, ok := result.(types.TypeReference)
	if !ok {
		t.Fatal("Expected result to be a TypeReference")
	}
	addedType, ok := allTypes[typeRef.Ref].(*types.ObjectType)
	if !ok {
		t.Fatal("Expected result to be an ObjectType")
	}

	if addedType.Properties == nil {
		t.Fatal("Expected properties to be defined")
	}

	if _, exists := addedType.Properties["fixedProp"]; !exists {
		t.Error("Expected property 'fixedProp' to exist")
	}

	if addedType.AdditionalProperties != nil {
		t.Error("Expected additionalProperties to be undefined/nil")
	}
}

func TestAddSchemaType_ObjectWithOnlyAdditionalProperties(t *testing.T) {
	schema := &manifest.Schema{
		Type: "object",
		AdditionalProperties: &manifest.Schema{
			Type: "string",
		},
	}
	typeFactory := factory.NewTypeFactory()

	result, err := addSchemaType(schema, "testWithOnlyAdditionalProps", typeFactory)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	allTypes := typeFactory.GetTypes()
	typeRef, ok := result.(types.TypeReference)
	if !ok {
		t.Fatal("Expected result to be a TypeReference")
	}
	addedType, ok := allTypes[typeRef.Ref].(*types.ObjectType)
	if !ok {
		t.Fatal("Expected result to be an ObjectType")
	}

	if addedType.Properties == nil {
		addedType.Properties = make(map[string]types.ObjectTypeProperty)
	}
	if len(addedType.Properties) != 0 {
		t.Errorf("Expected properties to be empty, got %d properties", len(addedType.Properties))
	}

	if addedType.AdditionalProperties == nil {
		t.Fatal("Expected additionalProperties to be defined")
	}

	// Verify additionalProperties is StringType
	additionalPropsRef, ok := addedType.AdditionalProperties.(types.TypeReference)
	if !ok {
		t.Fatal("Expected additionalProperties to be a TypeReference")
	}
	_, ok = allTypes[additionalPropsRef.Ref].(*types.StringType)
	if !ok {
		t.Fatal("Expected additionalProperties to be a StringType")
	}
}

func TestAddSchemaType_ObjectWithAdditionalProperties(t *testing.T) {
	schema := &manifest.Schema{
		Type: "object",
		Properties: map[string]manifest.Schema{
			"name": {
				Type: "string",
			},
		},
		AdditionalProperties: &manifest.Schema{
			Type: "object",
			Properties: map[string]manifest.Schema{
				"endpoint": {
					Type: "string",
				},
				"status": {
					Type: "string",
				},
			},
		},
	}
	typeFactory := factory.NewTypeFactory()

	result, err := addSchemaType(schema, "testWithAdditionalProps", typeFactory)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	allTypes := typeFactory.GetTypes()
	typeRef, ok := result.(types.TypeReference)
	if !ok {
		t.Fatal("Expected result to be a TypeReference")
	}
	addedType, ok := allTypes[typeRef.Ref].(*types.ObjectType)
	if !ok {
		t.Fatal("Expected result to be an ObjectType")
	}

	if addedType.Properties == nil {
		t.Fatal("Expected properties to be defined")
	}

	if _, exists := addedType.Properties["name"]; !exists {
		t.Error("Expected property 'name' to exist")
	}

	if addedType.AdditionalProperties == nil {
		t.Fatal("Expected additionalProperties to be defined")
	}

	// Verify additionalProperties type
	additionalPropsRef, ok := addedType.AdditionalProperties.(types.TypeReference)
	if !ok {
		t.Fatal("Expected additionalProperties to be a TypeReference")
	}
	additionalPropsType, ok := allTypes[additionalPropsRef.Ref].(*types.ObjectType)
	if !ok {
		t.Fatal("Expected additionalProperties to be an ObjectType")
	}

	if _, exists := additionalPropsType.Properties["endpoint"]; !exists {
		t.Error("Expected additionalProperties to have property 'endpoint'")
	}

	if _, exists := additionalPropsType.Properties["status"]; !exists {
		t.Error("Expected additionalProperties to have property 'status'")
	}
}

func TestAddResourceTypeForAPIVersion_WithAdditionalProperties(t *testing.T) {
	provider := &manifest.ResourceProvider{
		Namespace: "Applications.Test",
		Types: map[string]manifest.ResourceType{
			"testResources": {
				APIVersions: map[string]manifest.APIVersion{
					"2021-01-01": {
						Schema: manifest.Schema{
							Type: "object",
							Properties: map[string]manifest.Schema{
								"name": {Type: "string"},
								"connections": {
									Type: "object",
									AdditionalProperties: &manifest.Schema{
										Type: "object",
										Properties: map[string]manifest.Schema{
											"endpoint": {Type: "string"},
											"status": {
												Type: "enum",
												Enum: []string{"active", "inactive"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	resourceType := provider.Types["testResources"]
	apiVersion := resourceType.APIVersions["2021-01-01"]
	typeFactory := factory.NewTypeFactory()

	result, err := addResourceTypeForAPIVersion(
		provider,
		"testResources",
		&resourceType,
		"2021-01-01",
		&apiVersion,
		typeFactory,
	)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result to not be nil")
	}

	allTypes := typeFactory.GetTypes()
	var addedResourceType *types.ResourceType

	for _, typ := range allTypes {
		if rt, ok := typ.(*types.ResourceType); ok {
			addedResourceType = rt
			break
		}
	}

	if addedResourceType == nil {
		t.Fatal("Expected to find a ResourceType in the factory")
	}

	expectedName := "Applications.Test/testResources@2021-01-01"
	if addedResourceType.Name != expectedName {
		t.Errorf("Expected resource name '%s', got '%s'", expectedName, addedResourceType.Name)
	}

	// Get the body type
	bodyTypeRef, ok := addedResourceType.Body.(types.TypeReference)
	if !ok {
		t.Fatal("Expected body to be a TypeReference")
	}
	addedBodyType, ok := allTypes[bodyTypeRef.Ref].(*types.ObjectType)
	if !ok {
		t.Fatal("Expected body to be an ObjectType")
	}

	// Get the properties property
	addedPropertiesProperty, ok := addedBodyType.Properties["properties"]
	if !ok {
		t.Fatal("Expected properties property to exist")
	}

	propertiesTypeRef, ok := addedPropertiesProperty.Type.(types.TypeReference)
	if !ok {
		t.Fatal("Expected properties type to be a TypeReference")
	}
	addedPropertiesType, ok := allTypes[propertiesTypeRef.Ref].(*types.ObjectType)
	if !ok {
		t.Fatal("Expected properties to be an ObjectType")
	}

	// Verify connections property exists
	if _, exists := addedPropertiesType.Properties["connections"]; !exists {
		t.Fatal("Expected connections property to exist")
	}

	connectionsProperty := addedPropertiesType.Properties["connections"]
	connectionsTypeRef, ok := connectionsProperty.Type.(types.TypeReference)
	if !ok {
		t.Fatal("Expected connections type to be a TypeReference")
	}
	connectionsType, ok := allTypes[connectionsTypeRef.Ref].(*types.ObjectType)
	if !ok {
		t.Fatal("Expected connections to be an ObjectType")
	}

	// Verify connections has additionalProperties
	if connectionsType.AdditionalProperties == nil {
		t.Fatal("Expected connections additionalProperties to be defined")
	}

	// Verify additionalProperties structure
	additionalPropsRef, ok := connectionsType.AdditionalProperties.(types.TypeReference)
	if !ok {
		t.Fatal("Expected additionalProperties to be a TypeReference")
	}
	additionalPropsType, ok := allTypes[additionalPropsRef.Ref].(*types.ObjectType)
	if !ok {
		t.Fatal("Expected additionalProperties to be an ObjectType")
	}

	if _, exists := additionalPropsType.Properties["endpoint"]; !exists {
		t.Error("Expected additionalProperties to have property 'endpoint'")
	}

	if _, exists := additionalPropsType.Properties["status"]; !exists {
		t.Error("Expected additionalProperties to have property 'status'")
	}

	// Verify status is an enum (UnionType)
	statusProperty := additionalPropsType.Properties["status"]
	statusTypeRef, ok := statusProperty.Type.(types.TypeReference)
	if !ok {
		t.Fatal("Expected status type to be a TypeReference")
	}
	statusType, ok := allTypes[statusTypeRef.Ref].(*types.UnionType)
	if !ok {
		t.Fatal("Expected status to be a UnionType")
	}

	if len(statusType.Elements) != 2 {
		t.Errorf("Expected status union to have 2 elements, got %d", len(statusType.Elements))
	}
}
