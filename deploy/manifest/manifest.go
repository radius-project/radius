/*
Copyright 2024 The Radius Authors.

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

// Package manifest exposes the contents of deploy/manifest/defaults.yaml,
// the single source of truth listing the resource types that Radius ships
// as defaults (default registration in UCP and default recipe pack entries
// in the CLI).
//
// The YAML file is embedded into the binary at build time via go:embed so
// the default set is deterministic, offline, and changes are caught at
// build time rather than runtime.
package manifest

import (
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

// defaultsYAML is the raw contents of defaults.yaml, embedded at build time.
//
//go:embed defaults.yaml
var defaultsYAML []byte

// Defaults is the parsed schema of defaults.yaml.
type Defaults struct {
	// DefaultRegistration is the list of resource types that ship as defaults
	// in Radius. Each entry is of the form "Radius.<Namespace>/<typeName>".
	DefaultRegistration []string `yaml:"defaultRegistration"`
}

// DefaultsYAML returns the raw embedded contents of deploy/manifest/defaults.yaml.
func DefaultsYAML() []byte {
	return defaultsYAML
}

// ParseDefaults parses the embedded defaults.yaml and returns the result.
// A non-nil error indicates the embedded YAML is malformed. The list of
// registered types is returned without validation; callers that require
// non-empty or well-formed entries must validate the slice themselves.
func ParseDefaults() (Defaults, error) {
	var d Defaults
	if err := yaml.Unmarshal(defaultsYAML, &d); err != nil {
		return Defaults{}, fmt.Errorf("failed to parse embedded deploy/manifest/defaults.yaml: %w", err)
	}
	return d, nil
}
