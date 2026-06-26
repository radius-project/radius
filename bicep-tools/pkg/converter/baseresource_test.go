package converter

import (
	"reflect"
	"testing"

	"github.com/radius-project/radius/bicep-tools/pkg/manifest"
)

// TestBaseResource_DecodedFromCanonicalYAML verifies the base property set is
// decoded from the canonical base manifest (pkg/schema/baseresource/base.yaml)
// into bicep-tools' manifest.Schema model with the expected names, required
// entries, and connections shape (including disableDefaultEnvVars). It guards
// against base.yaml changes that fail to decode into manifest.Schema.
func TestBaseResource_DecodedFromCanonicalYAML(t *testing.T) {
	base, err := loadBaseResource()
	if err != nil {
		t.Fatalf("loadBaseResource: %v", err)
	}

	for _, name := range []string{"application", "environment", "connections", "codeReference"} {
		if _, ok := base.properties[name]; !ok {
			t.Errorf("expected base property %q to be decoded from base.yaml", name)
		}
	}

	if !reflect.DeepEqual(base.required, []string{"environment"}) {
		t.Errorf("expected required [environment], got %v", base.required)
	}

	connections := base.properties["connections"]
	if connections.AdditionalProperties == nil {
		t.Fatal("expected connections to declare additionalProperties")
	}
	for _, name := range []string{"source", "disableDefaultEnvVars"} {
		if _, ok := connections.AdditionalProperties.Properties[name]; !ok {
			t.Errorf("expected connections.additionalProperties to declare %q", name)
		}
	}
	if !contains(connections.AdditionalProperties.Required, "source") {
		t.Errorf("expected connections.additionalProperties.required to contain source, got %v", connections.AdditionalProperties.Required)
	}
}

// TestApplyBaseResource_MergesIntoBareSchema verifies the merger injects every
// base property and the required entry into a schema that declares none.
func TestApplyBaseResource_MergesIntoBareSchema(t *testing.T) {
	base, err := loadBaseResource()
	if err != nil {
		t.Fatalf("loadBaseResource: %v", err)
	}

	schema := &manifest.Schema{
		Type: "object",
		Properties: map[string]manifest.Schema{
			"size": {Type: "string"},
		},
	}

	base.apply(schema)

	for _, name := range []string{"size", "application", "environment", "connections", "codeReference"} {
		if _, ok := schema.Properties[name]; !ok {
			t.Errorf("expected property %q to be present after merge", name)
		}
	}

	if !contains(schema.Required, "environment") {
		t.Errorf("expected environment to be required after merge, got %v", schema.Required)
	}
}

// TestApplyBaseResource_PerTypeWins verifies an author's own declaration of a
// base property is never overwritten by the merge.
func TestApplyBaseResource_PerTypeWins(t *testing.T) {
	base, err := loadBaseResource()
	if err != nil {
		t.Fatalf("loadBaseResource: %v", err)
	}

	custom := manifest.Schema{Type: "string", Description: ptr("custom")}
	schema := &manifest.Schema{
		Type: "object",
		Properties: map[string]manifest.Schema{
			"environment": custom,
		},
	}

	base.apply(schema)

	got := schema.Properties["environment"]
	if got.Description == nil || *got.Description != "custom" {
		t.Errorf("expected author environment definition to be preserved, got %#v", got)
	}
}

// TestApplyBaseResource_Idempotent verifies applying the merge twice does not
// duplicate the required entry or change the property set.
func TestApplyBaseResource_Idempotent(t *testing.T) {
	base, err := loadBaseResource()
	if err != nil {
		t.Fatalf("loadBaseResource: %v", err)
	}

	schema := &manifest.Schema{Type: "object"}

	base.apply(schema)
	first := len(schema.Properties)
	base.apply(schema)

	if len(schema.Properties) != first {
		t.Errorf("expected stable property count, got %d then %d", first, len(schema.Properties))
	}
	if got := countOccurrences(schema.Required, "environment"); got != 1 {
		t.Errorf("expected environment to appear once in required, got %d", got)
	}
}

// TestApplyBaseResource_NilSchema verifies a nil schema is a no-op.
func TestApplyBaseResource_NilSchema(t *testing.T) {
	base, err := loadBaseResource()
	if err != nil {
		t.Fatalf("loadBaseResource: %v", err)
	}

	base.apply(nil)
}

func contains(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

func countOccurrences(values []string, target string) int {
	count := 0
	for _, v := range values {
		if v == target {
			count++
		}
	}
	return count
}

// ptr returns a pointer to v.
func ptr[T any](v T) *T {
	return &v
}
