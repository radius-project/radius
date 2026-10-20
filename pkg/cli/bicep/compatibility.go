/*
Copyright 2026 The Radius Authors.

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

package bicep

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/radius-project/radius/pkg/cli/filesystem"
)

// UnknownPinReason is the reason reported for a Radius resource with no determinable extension
// pin: a JSON template (which has no bicepconfig.json concept) or a resource declared inside a
// nested Bicep module (whose own pin configuration Bicep does not expose). Callers distinguish this
// from an actual read/parse error or a detected version mismatch, neither of which the user can
// clear simply by removing the message.
const UnknownPinReason = "the template has no Radius extension pin metadata"

// RadiusExtensionReference describes a configured pin, not the contents of Bicep's cache.
type RadiusExtensionReference struct {
	Reference string
	Reason    string
}

// RadiusExtensionPin describes the Radius extension pin configured for the root template, resolved
// while preparing the template. It is returned alongside the template, rather than written into the
// template's metadata, so that generated manifests and published artifacts built from the template
// are not altered by compatibility bookkeeping.
type RadiusExtensionPin struct {
	// Reference is the configured pin, e.g. "br:example.io/radius:0.60.2". Empty when the template
	// declares no Radius resources, or when the pin could not be read (see Err).
	Reference string

	// Err explains why Reference could not be read. Nil when Reference was read successfully, or
	// when the template declares no Radius resources at all.
	Err error
}

// RadiusExtensionReferences reports pins for Radius resources in template. pin, resolved for the
// root template by PrepareTemplate, applies to resources declared directly in the root template.
// Resources declared inside a nested Bicep module are always reported as unknown, since Bicep does
// not expose per-module pin configuration.
func RadiusExtensionReferences(template map[string]any, pin RadiusExtensionPin) []RadiusExtensionReference {
	seen := map[RadiusExtensionReference]bool{}
	walkTemplateResources(template, func(resource map[string]any, isRoot bool) {
		resourceType, _ := resource["type"].(string)
		if !IsRadiusResourceType(resourceType) {
			return
		}
		var reference, reason string
		if isRoot {
			reference = pin.Reference
			if pin.Err != nil {
				reason = pin.Err.Error()
			}
		}
		if reference == "" && reason == "" {
			reason = UnknownPinReason
		}
		seen[RadiusExtensionReference{Reference: reference, Reason: reason}] = true
	})
	references := make([]RadiusExtensionReference, 0, len(seen))
	for reference := range seen {
		references = append(references, reference)
	}
	sort.Slice(references, func(i, j int) bool {
		if references[i].Reference == references[j].Reference {
			return references[i].Reason < references[j].Reason
		}
		return references[i].Reference < references[j].Reference
	})
	return references
}

// resolveRadiusExtensionPin runs before remote source/configuration cleanup. It does not
// infer the resolved package release from imports.version, which is a provider version.
func resolveRadiusExtensionPin(fs filesystem.FileSystem, sourcePath string, template map[string]any) RadiusExtensionPin {
	hasRadius := false
	walkTemplateResources(template, func(resource map[string]any, _ bool) {
		resourceType, _ := resource["type"].(string)
		if IsRadiusResourceType(resourceType) {
			hasRadius = true
		}
	})
	if !hasRadius {
		return RadiusExtensionPin{}
	}

	reference, err := readRadiusExtensionPin(fs, sourcePath)
	if err != nil {
		return RadiusExtensionPin{Err: err}
	}
	return RadiusExtensionPin{Reference: reference}
}

func readRadiusExtensionPin(fs filesystem.FileSystem, sourcePath string) (string, error) {
	sourcePath, err := filepath.Abs(sourcePath)
	if err != nil {
		return "", fmt.Errorf("could not locate the source configuration: %w", err)
	}
	configPath := findBicepConfig(fs, filepath.Dir(sourcePath))
	if configPath == "" {
		return "", fmt.Errorf("no bicepconfig.json was found for the source")
	}
	data, err := fs.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("could not read bicepconfig.json: %w", err)
	}
	var config struct {
		Extensions map[string]json.RawMessage `json:"extensions"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return "", fmt.Errorf("could not read the Radius pin from bicepconfig.json: %w", err)
	}
	var reference string
	if raw, ok := config.Extensions["radius"]; ok {
		if err := json.Unmarshal(raw, &reference); err != nil {
			return "", fmt.Errorf("the configured Radius extension is not a string reference")
		}
	}
	if reference == "" {
		return "", fmt.Errorf("bicepconfig.json has no radius extension reference")
	}
	return reference, nil
}
