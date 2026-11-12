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

package reconciler

import (
	"encoding/json"
	"strings"

	"github.com/radius-project/radius/pkg/ucp/resources"
)

// defaultDeleteAPIVersion is used when the template does not name a version for the resource type.
const defaultDeleteAPIVersion = "2023-10-01-preview"

// deleteAPIVersion returns the API version to delete resourceID with: the version the template
// deployed that resource type with, for example "2025-08-01-preview" for Radius.Compute/containers.
// Newer resource types reject the older default version.
func deleteAPIVersion(template string, resourceID string) string {
	id, err := resources.ParseResource(resourceID)
	if err != nil || template == "" {
		return defaultDeleteAPIVersion
	}

	var parsed struct {
		Resources json.RawMessage `json:"resources"`
	}
	if err := json.Unmarshal([]byte(template), &parsed); err != nil {
		return defaultDeleteAPIVersion
	}

	type templateResource struct {
		Type string `json:"type"`
	}
	var list []templateResource
	if err := json.Unmarshal(parsed.Resources, &list); err != nil {
		// Templates with symbolic names store resources as an object.
		byName := map[string]templateResource{}
		if err := json.Unmarshal(parsed.Resources, &byName); err != nil {
			return defaultDeleteAPIVersion
		}
		for _, r := range byName {
			list = append(list, r)
		}
	}

	for _, r := range list {
		resourceType, version, ok := strings.Cut(r.Type, "@")
		if ok && version != "" && strings.EqualFold(resourceType, id.Type()) {
			return version
		}
	}
	return defaultDeleteAPIVersion
}
