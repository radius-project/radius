package converter

import (
	"testing"

	"github.com/radius-project/radius/bicep-tools/pkg/manifest"
	"github.com/radius-project/radius/bicep-types/src/bicep-types-go/factory"
	"github.com/radius-project/radius/bicep-types/src/bicep-types-go/types"
)

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

	expectedResourceTypeID := "Applications.Test/testResources"
	if addedResourceType.ResourceTypeID != expectedResourceTypeID {
		t.Errorf("Expected resource type ID '%s', got '%s'", expectedResourceTypeID, addedResourceType.ResourceTypeID)
	}

	if addedResourceType.APIVersion != "2021-01-01" {
		t.Errorf("Expected API version '2021-01-01', got '%s'", addedResourceType.APIVersion)
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

	result, err := addObjectProperties(schema, typeFactory)
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

	result, err := addObjectProperty(parent, "a", property, typeFactory)
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

	result, err := addObjectProperty(parent, "a", property, typeFactory)
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

	result, err := addObjectProperty(parent, "a", property, typeFactory)
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
