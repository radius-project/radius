// Package catalog provides infrastructure library catalog management.
// The catalog maps library names to dependency types for detection.
package catalog

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/radius-project/radius/pkg/discovery/dtypes"
	"gopkg.in/yaml.v3"
)

//go:embed libraries.yaml
var embeddedLibraries []byte

// LibraryCatalog maps library names to infrastructure dependency types.
type LibraryCatalog struct {
	mu      sync.RWMutex
	entries map[string]LibraryEntry
}

// LibraryEntry describes a library and its associated dependency.
type LibraryEntry struct {
	// Library name as it appears in package manifests.
	Library string `yaml:"library"`

	// Language this entry applies to.
	Language dtypes.Language `yaml:"language"`

	// DependencyType this library indicates.
	DependencyType dtypes.DependencyType `yaml:"dependencyType"`

	// Confidence score for this mapping (0.0-1.0).
	Confidence float64 `yaml:"confidence"`

	// DefaultPort for this dependency type.
	DefaultPort int `yaml:"defaultPort,omitempty"`

	// ConnectionEnvVar is the typical environment variable for connection.
	ConnectionEnvVar string `yaml:"connectionEnvVar,omitempty"`

	// Aliases are alternative names for the same library.
	Aliases []string `yaml:"aliases,omitempty"`
}

// CatalogFile represents the YAML structure of the library catalog.
type CatalogFile struct {
	Version   string         `yaml:"version"`
	Libraries []LibraryEntry `yaml:"libraries"`
}

// NewLibraryCatalog creates an empty library catalog.
func NewLibraryCatalog() *LibraryCatalog {
	return &LibraryCatalog{
		entries: make(map[string]LibraryEntry),
	}
}

// LoadFromFile loads a library catalog from a YAML file.
func (c *LibraryCatalog) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading catalog file: %w", err)
	}

	return c.LoadFromBytes(data)
}

// LoadFromBytes parses YAML data into the catalog.
func (c *LibraryCatalog) LoadFromBytes(data []byte) error {
	var catalogFile CatalogFile
	if err := yaml.Unmarshal(data, &catalogFile); err != nil {
		return fmt.Errorf("parsing catalog YAML: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, entry := range catalogFile.Libraries {
		key := c.makeKey(entry.Library, entry.Language)
		c.entries[key] = entry

		// Also register aliases
		for _, alias := range entry.Aliases {
			aliasKey := c.makeKey(alias, entry.Language)
			c.entries[aliasKey] = entry
		}
	}

	return nil
}

// Lookup finds a library entry by name and language.
func (c *LibraryCatalog) Lookup(library string, lang dtypes.Language) (LibraryEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := c.makeKey(library, lang)
	entry, ok := c.entries[key]
	return entry, ok
}

// LookupAnyLanguage finds a library entry by name across all languages.
func (c *LibraryCatalog) LookupAnyLanguage(library string) ([]LibraryEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var results []LibraryEntry
	for key, entry := range c.entries {
		// Key format: "library:language"
		parts := strings.SplitN(key, ":", 2)
		if len(parts) >= 1 && parts[0] == library {
			results = append(results, entry)
		}
	}

	return results, len(results) > 0
}

// AllForLanguage returns all entries for a specific language.
func (c *LibraryCatalog) AllForLanguage(lang dtypes.Language) []LibraryEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var results []LibraryEntry
	seen := make(map[string]bool)

	for _, entry := range c.entries {
		if entry.Language == lang {
			// Avoid duplicates from aliases
			if !seen[entry.Library] {
				results = append(results, entry)
				seen[entry.Library] = true
			}
		}
	}

	return results
}

// Add inserts a library entry into the catalog.
func (c *LibraryCatalog) Add(entry LibraryEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := c.makeKey(entry.Library, entry.Language)
	c.entries[key] = entry
}

// Size returns the number of entries in the catalog.
func (c *LibraryCatalog) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

func (c *LibraryCatalog) makeKey(library string, lang dtypes.Language) string {
	return fmt.Sprintf("%s:%s", strings.ToLower(library), lang)
}

// DefaultCatalog is the global library catalog.
var DefaultCatalog *LibraryCatalog

func init() {
	// Initialize the default catalog with embedded data
	DefaultCatalog = NewLibraryCatalog()
	if err := DefaultCatalog.LoadFromBytes(embeddedLibraries); err != nil {
		// Log error but don't panic - discovery will work with empty catalog
		fmt.Fprintf(os.Stderr, "Warning: failed to load embedded library catalog: %v\n", err)
	}
}

// LoadDefaultCatalog loads the built-in library catalog.
func LoadDefaultCatalog(catalogDir string) error {
	DefaultCatalog = NewLibraryCatalog()

	catalogPath := filepath.Join(catalogDir, "libraries.yaml")
	return DefaultCatalog.LoadFromFile(catalogPath)
}
