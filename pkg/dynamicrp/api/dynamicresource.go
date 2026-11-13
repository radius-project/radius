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

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
)

// DynamicResource is used as the versioned resource model for dynamic resources.
//
// A dynamic resource uses a user-provided OpenAPI specification to define the resource schema. Therefore,
// the properties of the resource are not known at compile time.
type DynamicResource struct {
	// ID is the resource ID.
	ID *string `json:"id"`
	// Name is the resource name.
	Name *string `json:"name"`
	// Type is the resource type.
	Type *string `json:"type"`
	// Location is the resource location.
	Location *string `json:"location"`
	// Tags are the resource tags.
	Tags map[string]*string `json:"tags,omitempty"`
	// Properties stores the properties of the resource.
	Properties map[string]any `json:"properties,omitempty"`
	// SystemData stores the system data of the resource.
	SystemData map[string]any `json:"systemData,omitempty"`

	// DelegatingResources is a request-only API envelope, not a resource schema property.
	DelegatingResources []datamodel.DelegatingResource `json:"delegatingResources,omitempty"`

	// apiVersion stores the API version for this resource (not serialized).
	// This is set by the converter and used during ConvertTo for proper schema lookups.
	apiVersion string
}

// UnmarshalJSON preserves an authoritative empty delegate list and rejects ambiguous null.
func (d *DynamicResource) UnmarshalJSON(data []byte) error {
	type plain DynamicResource
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		return err
	}
	for key, raw := range envelope {
		if strings.EqualFold(key, "delegatingResources") && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("delegatingResources must be an array, not null")
		}
	}
	var propertyEnvelope map[string]json.RawMessage
	if raw, present := envelope["properties"]; present {
		if err := json.Unmarshal(raw, &propertyEnvelope); err != nil {
			return err
		}
	}
	if raw, present := propertyEnvelope["__radiusDelegatingResources"]; present {
		for key := range envelope {
			if strings.EqualFold(key, "delegatingResources") {
				return fmt.Errorf("delegatingResources cannot be supplied through two envelopes")
			}
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("delegatingResources must be an array, not null")
		}
		if err := json.Unmarshal(raw, &decoded.DelegatingResources); err != nil {
			return err
		}
		// The unchanged engine forwards native properties, not arbitrary API-envelope fields.
		// Extract this planner-only transport property before user-schema validation and persistence.
		delete(decoded.Properties, "__radiusDelegatingResources")
	}
	*d = DynamicResource(decoded)
	return nil
}

// SetAPIVersion sets the API version for this resource.
// This is used by the converter to pass the actual API version from the request.
func (d *DynamicResource) SetAPIVersion(version string) {
	d.apiVersion = version
}
