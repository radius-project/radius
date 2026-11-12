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

// Package parameters translates compiler inputs without resolving credentials.
package parameters

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	radapp "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"k8s.io/apimachinery/pkg/util/validation"
)

var directInput = regexp.MustCompile(`^\[externalInputs\('([^']+)'\)\]$`)

// Document is the compiled ARM parameter document, including external definitions.
type Document struct {
	Parameters               map[string]map[string]json.RawMessage `json:"parameters"`
	ExternalInputDefinitions map[string]json.RawMessage            `json:"externalInputDefinitions,omitempty"`
}

// Parse accepts both --outfile JSON and the --stdout build-params envelope.
func Parse(data []byte) (Document, error) {
	var envelope struct {
		ParametersJSON *string `json:"parametersJson"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return Document{}, fmt.Errorf("invalid compiled parameter document")
	}
	if envelope.ParametersJSON != nil {
		data = []byte(*envelope.ParametersJSON)
	}
	var document Document
	if err := json.Unmarshal(data, &document); err != nil || document.Parameters == nil {
		return Document{}, fmt.Errorf("invalid compiled parameters field")
	}
	return document, nil
}

// Translate preserves literal strings and converts only direct supported bindings.
// It deliberately does not evaluate arbitrary ARM expressions.
func Translate(document Document) (map[string]string, map[string]radapp.ParameterReference, error) {
	literals := map[string]string{}
	references := map[string]radapp.ParameterReference{}
	used := map[string]bool{}
	for name, binding := range document.Parameters {
		if len(binding) != 1 {
			return nil, nil, fmt.Errorf("parameter %q must have exactly one value or expression", name)
		}
		if value, ok := binding["value"]; ok {
			var literal string
			if err := json.Unmarshal(value, &literal); err != nil || bytes.Equal(value, []byte("null")) {
				return nil, nil, fmt.Errorf("parameter %q must be a string", name)
			}
			literals[name] = literal
			continue
		}
		var expression string
		if err := json.Unmarshal(binding["expression"], &expression); err != nil {
			return nil, nil, fmt.Errorf("parameter %q has an unsupported binding", name)
		}
		match := directInput.FindStringSubmatch(expression)
		if match == nil {
			return nil, nil, fmt.Errorf("parameter %q requires a direct externalInputs expression", name)
		}
		raw, ok := document.ExternalInputDefinitions[match[1]]
		if !ok {
			return nil, nil, fmt.Errorf("parameter %q has no external input definition", name)
		}
		var definition struct {
			Kind   string `json:"kind"`
			Config struct {
				Name string `json:"name"`
				Key  string `json:"key"`
			} `json:"config"`
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&definition); err != nil || definition.Kind != "radius.secretKeyRef" {
			return nil, nil, fmt.Errorf("parameter %q has an unsupported external input definition", name)
		}
		reference := radapp.SecretKeyReference{Name: definition.Config.Name, Key: definition.Config.Key}
		if err := ValidateReference(reference); err != nil {
			return nil, nil, fmt.Errorf("parameter %q has an invalid Secret name or key", name)
		}
		references[name] = radapp.ParameterReference{SecretKeyRef: reference}
		used[match[1]] = true
	}
	if len(used) != len(document.ExternalInputDefinitions) {
		return nil, nil, fmt.Errorf("unused external input definitions")
	}
	return literals, references, nil
}

// ValidateReference enforces the same name/key contract for generated and direct objects.
func ValidateReference(reference radapp.SecretKeyReference) error {
	if reference.Namespace != nil || len(validation.IsDNS1123Subdomain(reference.Name)) != 0 ||
		reference.Key == "" || len(validation.IsConfigMapKey(reference.Key)) != 0 {
		return fmt.Errorf("invalid Secret name or key")
	}
	return nil
}

type templateParameter struct {
	Type          string          `json:"type"`
	DefaultValue  json.RawMessage `json:"defaultValue"`
	MinLength     *int            `json:"minLength"`
	MaxLength     *int            `json:"maxLength"`
	AllowedValues []string        `json:"allowedValues"`
	Metadata      json.RawMessage `json:"metadata"`
}

// Validate checks reference targets and missing required bindings without reading Secrets.
func Validate(template string, literals map[string]string, references map[string]radapp.ParameterReference) error {
	if len(references) > 64 {
		return fmt.Errorf("SecretInputInvalid: at most 64 parameter references are supported")
	}
	var document struct {
		Parameters map[string]json.RawMessage `json:"parameters"`
	}
	if err := json.Unmarshal([]byte(template), &document); err != nil {
		return fmt.Errorf("SecretInputInvalid: invalid template")
	}
	for name, reference := range references {
		if _, ok := literals[name]; ok {
			return fmt.Errorf("SecretInputConflict: parameter %q has both a literal and a reference", name)
		}
		if err := ValidateReference(reference.SecretKeyRef); err != nil {
			return fmt.Errorf("SecretInputInvalid: parameter %q has an invalid Secret name or key", name)
		}
		raw, ok := document.Parameters[name]
		if !ok {
			return fmt.Errorf("SecretInputInvalid: parameter %q is not declared", name)
		}
		var parameter templateParameter
		if err := json.Unmarshal(raw, &parameter); err != nil || !strings.EqualFold(parameter.Type, "secureString") {
			return fmt.Errorf("SecretInputInvalid: parameter %q must declare type secureString", name)
		}
		if parameter.DefaultValue != nil {
			return fmt.Errorf("SecretInputConflict: remove the default for parameter %q", name)
		}
		// Reject alternate secure metadata instead of guessing its meaning.
		var metadata map[string]json.RawMessage
		if len(parameter.Metadata) != 0 && string(parameter.Metadata) != "null" {
			if err := json.Unmarshal(parameter.Metadata, &metadata); err != nil {
				return fmt.Errorf("SecretInputInvalid: parameter %q has unsupported metadata", name)
			}
			if _, ok := metadata["secure"]; ok {
				return fmt.Errorf("SecretInputInvalid: parameter %q has unsupported secure metadata", name)
			}
		}
	}
	for name, raw := range document.Parameters {
		var parameter struct {
			DefaultValue json.RawMessage `json:"defaultValue"`
		}
		if err := json.Unmarshal(raw, &parameter); err != nil {
			return fmt.Errorf("SecretInputInvalid: invalid parameter declaration")
		}
		_, literal := literals[name]
		_, reference := references[name]
		if parameter.DefaultValue == nil && !literal && !reference {
			return fmt.Errorf("SecretInputInvalid: required parameter %q is unbound", name)
		}
	}
	return nil
}

// ValidateValue checks secure-string constraints without including the value in errors.
func ValidateValue(template, name, value string) error {
	var document struct {
		Parameters map[string]json.RawMessage `json:"parameters"`
	}
	if err := json.Unmarshal([]byte(template), &document); err != nil {
		return fmt.Errorf("SecretInputInvalid: invalid template")
	}
	var parameter templateParameter
	if err := json.Unmarshal(document.Parameters[name], &parameter); err != nil {
		return fmt.Errorf("SecretInputInvalid: invalid parameter declaration")
	}
	// ARM string lengths are measured in UTF-16 code units.
	length := 0
	for _, character := range value {
		length++
		if character > 0xffff {
			length++
		}
	}
	valid := utf8.ValidString(value) && len(value) <= 64*1024
	valid = valid && (parameter.MinLength == nil || length >= *parameter.MinLength)
	valid = valid && (parameter.MaxLength == nil || length <= *parameter.MaxLength)
	if parameter.AllowedValues != nil {
		allowed := false
		for _, candidate := range parameter.AllowedValues {
			allowed = allowed || candidate == value
		}
		valid = valid && allowed
	}
	if !valid {
		return fmt.Errorf("SecretInputInvalid: parameter %q violates secure-string constraints or the 64 KiB input limit", name)
	}
	return nil
}
