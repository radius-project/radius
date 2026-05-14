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

package manifest

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"strings"

	yaml "github.com/goccy/go-yaml"
)

// DefaultsConfig represents the structure of the defaults.yaml configuration file
// that lists which resource type manifests should be registered by default.
type DefaultsConfig struct {
	// DefaultRegistration is a list of resource type names to register.
	// Each entry uses the format Radius.<Namespace>/<typeName> (e.g., Radius.Compute/containers).
	DefaultRegistration []string `yaml:"defaultRegistration"`
}

// LoadDefaultManifests reads defaults.yaml from the provided fs.FS and returns the
// parsed, validated, and merged resource providers for default registration.
//
// The function performs the following steps:
//  1. Reads defaults.yaml from the embedded filesystem to get the list of resource
//     type names (e.g., Radius.Compute/containers).
//  2. Resolves each name to a manifest file path using the naming convention:
//     strip the "Radius." prefix, then <Namespace>/<typeName>/<typeName>.yaml.
//  3. Reads and parses each manifest using the existing ReadBytes function.
//  4. Validates that the manifest's namespace matches the expected namespace
//     derived from the resource type name.
//  5. Validates that the manifest contains the expected resource type in its
//     types map.
//  6. Validates schemas using the existing validateManifestSchemas function.
//  7. Merges manifests sharing a namespace into a single ResourceProvider
//     (e.g., three Radius.Compute manifests become one provider with all types).
//
// Returns nil, nil if defaults.yaml has no entries. Returns an error if any step
// fails (missing files, parse errors, validation failures, namespace mismatches).
func LoadDefaultManifests(ctx context.Context, fsys fs.FS) ([]ResourceProvider, error) {
	if fsys == nil {
		return nil, fmt.Errorf("embedded filesystem is nil")
	}

	// Read the defaults configuration that lists which resource types to register.
	data, err := fs.ReadFile(fsys, "defaults.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to read defaults.yaml: %w", err)
	}

	// Parse defaults.yaml with strict mode to catch typos in field names.
	config := DefaultsConfig{}
	decoder := yaml.NewDecoder(bytes.NewReader(data), yaml.Strict())
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to parse defaults.yaml: %w", err)
	}

	if len(config.DefaultRegistration) == 0 {
		return nil, nil
	}

	// Parse and validate each manifest, merging providers that share a namespace.
	// For example, Radius.Compute/containers and Radius.Compute/routes both belong
	// to the Radius.Compute namespace and are merged into a single ResourceProvider.
	merged := map[string]*ResourceProvider{}
	for _, name := range config.DefaultRegistration {
		// Resolve the resource type name to a file path.
		path, err := resolveResourceTypePath(name)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve resource type %s: %w", name, err)
		}

		// Read and parse the manifest file.
		manifestData, err := fs.ReadFile(fsys, path)
		if err != nil {
			return nil, fmt.Errorf("failed to read manifest for %s (resolved to %s): %w", name, path, err)
		}

		provider, err := ReadBytes(manifestData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse manifest %s: %w", path, err)
		}

		// Validate the manifest namespace matches the expected namespace from the
		// resource type name (e.g., Radius.Compute/containers expects
		// namespace Radius.Compute).
		parts := strings.SplitN(name, "/", 2)
		expectedNamespace := parts[0]
		expectedTypeName := parts[1]

		if provider.Namespace != expectedNamespace {
			return nil, fmt.Errorf("manifest %s declares namespace %q but expected %q from defaults.yaml entry %s", path, provider.Namespace, expectedNamespace, name)
		}

		// Validate the manifest contains the expected resource type.
		if _, ok := provider.Types[expectedTypeName]; !ok {
			return nil, fmt.Errorf("manifest %s does not define resource type %q listed in defaults.yaml entry %s", path, expectedTypeName, name)
		}

		// Validate the manifest schemas against OpenAPI format.
		if err := validateManifestSchemas(ctx, provider); err != nil {
			return nil, fmt.Errorf("failed to validate manifest %s: %w", path, err)
		}

		// Merge only the expected resource type into the provider for this namespace.
		// defaults.yaml is authoritative: only the types explicitly listed are registered,
		// even if the manifest file contains additional types.
		if existing, ok := merged[provider.Namespace]; ok {
			existing.Types[expectedTypeName] = provider.Types[expectedTypeName]
		} else {
			merged[provider.Namespace] = &ResourceProvider{
				Namespace: provider.Namespace,
				Location:  provider.Location,
				Types: map[string]*ResourceType{
					expectedTypeName: provider.Types[expectedTypeName],
				},
			}
		}
	}

	result := make([]ResourceProvider, 0, len(merged))
	for _, provider := range merged {
		result = append(result, *provider)
	}

	return result, nil
}

// resolveResourceTypePath converts a resource type name to a file path.
// Format: Radius.<Namespace>/<typeName> -> <Namespace>/<typeName>/<typeName>.yaml
// Example: Radius.Compute/containers -> Compute/containers/containers.yaml
func resolveResourceTypePath(name string) (string, error) {
	parts := strings.SplitN(name, "/", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid resource type name %q: expected format Radius.<Namespace>/<typeName>", name)
	}

	namespace := parts[0]
	typeName := parts[1]

	// Validate namespace matches the expected format (e.g., Radius.Compute).
	if !resourceProviderNamespaceRegex.MatchString(namespace) {
		return "", fmt.Errorf("invalid namespace %q: must match format Radius.<Namespace> (e.g., Radius.Compute)", namespace)
	}

	if !strings.HasPrefix(namespace, "Radius.") {
		return "", fmt.Errorf("invalid namespace %q: must start with 'Radius.'", namespace)
	}

	// Validate type name matches the expected format (e.g., containers).
	if !resourceTypeRegex.MatchString(typeName) {
		return "", fmt.Errorf("invalid type name %q: must be camelCase (e.g., containers)", typeName)
	}

	// Strip "Radius." prefix
	namespaceSuffix := strings.TrimPrefix(namespace, "Radius.")

	return fmt.Sprintf("%s/%s/%s.yaml", namespaceSuffix, typeName, typeName), nil
}
