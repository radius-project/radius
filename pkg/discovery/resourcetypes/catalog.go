// Package resourcetypes provides Resource Type catalog management.
// The catalog maps dependency types to Radius Resource Type definitions.
package resourcetypes

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/radius-project/radius/pkg/discovery/dtypes"
	"gopkg.in/yaml.v3"
)

//go:embed types.yaml
var embeddedTypes []byte

func init() {
	// Initialize the default catalog with embedded data
	DefaultCatalog = NewCatalog()
	if err := DefaultCatalog.LoadFromBytes(embeddedTypes); err != nil {
		// Log error but don't panic - discovery will work with empty catalog
		fmt.Fprintf(os.Stderr, "Warning: failed to load embedded Resource Type catalog: %v\n", err)
	}
}

// Catalog manages Resource Type definitions.
type Catalog struct {
	mu      sync.RWMutex
	entries map[dtypes.DependencyType]ResourceTypeEntry
}

// ResourceTypeEntry describes a Resource Type for a dependency.
type ResourceTypeEntry struct {
	// DependencyType this entry matches.
	DependencyType dtypes.DependencyType `yaml:"dependencyType"`

	// ResourceTypeName is the full Radius Resource Type name.
	ResourceTypeName string `yaml:"resourceTypeName"`

	// APIVersion for the Resource Type.
	APIVersion string `yaml:"apiVersion"`

	// Description of the Resource Type.
	Description string `yaml:"description,omitempty"`

	// DefaultProperties to apply when generating Bicep.
	DefaultProperties map[string]interface{} `yaml:"defaultProperties,omitempty"`

	// SchemaURL for the Resource Type JSON Schema.
	SchemaURL string `yaml:"schemaUrl,omitempty"`

	// Source indicates where this entry came from ("catalog" or "contrib").
	Source string `yaml:"source,omitempty"`
}

// CatalogFile represents the YAML structure of the Resource Type catalog.
type CatalogFile struct {
	Version       string              `yaml:"version"`
	ResourceTypes []ResourceTypeEntry `yaml:"resourceTypes"`
}

// NewCatalog creates an empty Resource Type catalog.
func NewCatalog() *Catalog {
	return &Catalog{
		entries: make(map[dtypes.DependencyType]ResourceTypeEntry),
	}
}

// LoadFromFile loads a Resource Type catalog from a YAML file.
func (c *Catalog) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading catalog file: %w", err)
	}

	return c.LoadFromBytes(data)
}

// LoadFromBytes parses YAML data into the catalog.
func (c *Catalog) LoadFromBytes(data []byte) error {
	var catalogFile CatalogFile
	if err := yaml.Unmarshal(data, &catalogFile); err != nil {
		return fmt.Errorf("parsing catalog YAML: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, entry := range catalogFile.ResourceTypes {
		c.entries[entry.DependencyType] = entry
	}

	return nil
}

// Lookup finds a Resource Type entry by dependency type.
func (c *Catalog) Lookup(depType dtypes.DependencyType) (ResourceTypeEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[depType]
	return entry, ok
}

// All returns all Resource Type entries.
func (c *Catalog) All() []ResourceTypeEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]ResourceTypeEntry, 0, len(c.entries))
	for _, entry := range c.entries {
		result = append(result, entry)
	}
	return result
}

// Add inserts a Resource Type entry into the catalog.
func (c *Catalog) Add(entry ResourceTypeEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[entry.DependencyType] = entry
}

// Size returns the number of entries in the catalog.
func (c *Catalog) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// ToResourceType converts a catalog entry to a dtypes.ResourceType.
func (e *ResourceTypeEntry) ToResourceType() dtypes.ResourceType {
	return dtypes.ResourceType{
		Name:       e.ResourceTypeName,
		APIVersion: e.APIVersion,
		Properties: e.DefaultProperties,
		Schema:     e.SchemaURL,
	}
}

// DefaultCatalog is the global Resource Type catalog.
var DefaultCatalog *Catalog

// LoadDefaultCatalog loads the built-in Resource Type catalog.
func LoadDefaultCatalog(catalogDir string) error {
	DefaultCatalog = NewCatalog()

	catalogPath := filepath.Join(catalogDir, "types.yaml")
	return DefaultCatalog.LoadFromFile(catalogPath)
}

// Match creates a ResourceTypeMapping from a dependency and catalog entry.
func Match(dep dtypes.DetectedDependency, entry ResourceTypeEntry) dtypes.ResourceTypeMapping {
	// Determine the match source based on the entry source
	matchSource := dtypes.MatchCatalog
	if entry.Source == "contrib" {
		matchSource = dtypes.MatchContrib
	}

	return dtypes.ResourceTypeMapping{
		DependencyID: dep.ID,
		ResourceType: entry.ToResourceType(),
		MatchSource:  matchSource,
		Confidence:   dep.Confidence * 0.95, // Slightly reduce confidence for catalog match
	}
}
