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

// RadiusExtensionReference describes a configured pin, not the contents of Bicep's cache.
type RadiusExtensionReference struct {
	Reference string
	Reason    string
}

// RadiusExtensionReferences reports pins for templates that contain Radius resources.
// Missing metadata, including in nested modules, is reported as unknown.
func RadiusExtensionReferences(template map[string]any) []RadiusExtensionReference {
	seen := map[RadiusExtensionReference]bool{}
	walkTemplateResources(template, func(owner, resource map[string]any) {
		resourceType, _ := resource["type"].(string)
		if !IsRadiusResourceType(resourceType) {
			return
		}
		metadata, _ := owner["metadata"].(map[string]any)
		rad, _ := metadata["_rad"].(map[string]any)
		reference, _ := rad["radiusExtension"].(string)
		reason, _ := rad["radiusExtensionError"].(string)
		if reference == "" && reason == "" {
			reason = "the template has no Radius extension pin metadata"
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

// recordRadiusExtensionPin runs before remote source/configuration cleanup. It does not
// infer the resolved package release from imports.version, which is a provider version.
func recordRadiusExtensionPin(fs filesystem.FileSystem, sourcePath string, template map[string]any) {
	hasRadius := false
	walkTemplateResources(template, func(_ map[string]any, resource map[string]any) {
		resourceType, _ := resource["type"].(string)
		if IsRadiusResourceType(resourceType) {
			hasRadius = true
		}
	})
	if !hasRadius {
		return
	}

	reference, err := readRadiusExtensionPin(fs, sourcePath)
	metadata, ok := template["metadata"].(map[string]any)
	if !ok {
		metadata = map[string]any{}
		template["metadata"] = metadata
	}
	rad := map[string]any{}
	if err != nil {
		rad["radiusExtensionError"] = err.Error()
	} else {
		rad["radiusExtension"] = reference
		rad["radiusExtensionError"] = "Bicep does not report the resolved Radius artifact release; the configured pin is not compiler provenance"
	}
	metadata["_rad"] = rad
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
