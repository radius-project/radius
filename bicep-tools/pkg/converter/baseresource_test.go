package converter

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/radius-project/radius/bicep-tools/pkg/manifest"
	"go.yaml.in/yaml/v3"
)

// canonicalBaseYAMLPath is the repo-relative path from this package to the
// canonical base resource manifest owned by the schema package.
var canonicalBaseYAMLPath = filepath.Join("..", "..", "..", "pkg", "schema", "baseresource", "base.yaml")

// TestApplyBaseResource_PropertiesMatchCanonicalYAML guards against drift between
// the typed Go literal in baseresource.go and the canonical base.yaml. bicep-tools
// keeps its own copy because it models schemas with manifest.Schema rather than
// the map[string]any the schema package merges, so this test fails CI whenever
// the two definitions diverge.
func TestApplyBaseResource_PropertiesMatchCanonicalYAML(t *testing.T) {
	data, err := os.ReadFile(canonicalBaseYAMLPath)
	if err != nil {
		t.Fatalf("failed to read canonical base manifest %s: %v", canonicalBaseYAMLPath, err)
	}

	var doc struct {
		Properties map[string]manifest.Schema `yaml:"properties"`
		Required   []string                   `yaml:"required"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("failed to parse canonical base manifest: %v", err)
	}

	if !reflect.DeepEqual(doc.Properties, baseResourceProperties) {
		t.Errorf("baseResourceProperties drifted from base.yaml:\n  yaml:    %#v\n  literal: %#v", doc.Properties, baseResourceProperties)
	}

	if !reflect.DeepEqual(doc.Required, baseResourceRequired) {
		t.Errorf("baseResourceRequired drifted from base.yaml:\n  yaml:    %#v\n  literal: %#v", doc.Required, baseResourceRequired)
	}
}

// TestApplyBaseResource_MergesIntoBareSchema verifies the merger injects every
// base property and the required entry into a schema that declares none.
func TestApplyBaseResource_MergesIntoBareSchema(t *testing.T) {
	schema := &manifest.Schema{
		Type: "object",
		Properties: map[string]manifest.Schema{
			"size": {Type: "string"},
		},
	}

	applyBaseResource(schema)

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
	custom := manifest.Schema{Type: "string", Description: ptr("custom")}
	schema := &manifest.Schema{
		Type: "object",
		Properties: map[string]manifest.Schema{
			"environment": custom,
		},
	}

	applyBaseResource(schema)

	got := schema.Properties["environment"]
	if got.Description == nil || *got.Description != "custom" {
		t.Errorf("expected author environment definition to be preserved, got %#v", got)
	}
}

// TestApplyBaseResource_Idempotent verifies applying the merge twice does not
// duplicate the required entry or change the property set.
func TestApplyBaseResource_Idempotent(t *testing.T) {
	schema := &manifest.Schema{Type: "object"}

	applyBaseResource(schema)
	first := len(schema.Properties)
	applyBaseResource(schema)

	if len(schema.Properties) != first {
		t.Errorf("expected stable property count, got %d then %d", first, len(schema.Properties))
	}
	if got := countOccurrences(schema.Required, "environment"); got != 1 {
		t.Errorf("expected environment to appear once in required, got %d", got)
	}
}

// TestApplyBaseResource_NilSchema verifies a nil schema is a no-op.
func TestApplyBaseResource_NilSchema(t *testing.T) {
	applyBaseResource(nil)
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
