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

package native

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
)

const (
	// generatedPrefix starts every name UCP generates.
	generatedPrefix = "radius-"

	// generatedHexLength is how many hex characters of the hash follow the prefix, before the name is cut to fit
	// MaxLength, and minHexLength is how many must stay after the cut, so names stay unique.
	generatedHexLength = 12
	minHexLength       = 8
)

// NameRules are the rules a type's schema sets for its name property (Options.NameProperty), which a name UCP
// generates must follow. They are read from the property's own definition in the schema.
type NameRules struct {
	// Property is the name property.
	Property string

	// MinLength and MaxLength are the property's minLength and maxLength, or 0 when the schema sets none.
	MinLength int
	MaxLength int

	// Pattern is the property's pattern, or nil when the schema sets none.
	Pattern *regexp.Regexp
}

// nameRules reads the rules for property from schema. The property must be a string defined in the schema's
// properties, and the names UCP generates must fit it.
func nameRules(schema string, property string) (*NameRules, error) {
	var parsed struct {
		Properties map[string]struct {
			Type      string `json:"type"`
			Pattern   string `json:"pattern"`
			MinLength int    `json:"minLength"`
			MaxLength int    `json:"maxLength"`
		} `json:"properties"`
	}
	if err := json.Unmarshal([]byte(schema), &parsed); err != nil {
		return nil, fmt.Errorf("failed to read the schema: %w", err)
	}
	definition, ok := parsed.Properties[property]
	if !ok || definition.Type != "string" {
		return nil, fmt.Errorf("the name property %s is not a string property in the schema", property)
	}
	rules := &NameRules{Property: property, MinLength: definition.MinLength, MaxLength: definition.MaxLength}
	if definition.Pattern != "" {
		pattern, err := regexp.Compile(definition.Pattern)
		if err != nil {
			return nil, fmt.Errorf("the pattern of the name property %s can't be read: %w", property, err)
		}
		rules.Pattern = pattern
	}
	// Names differ only in their hex characters, so names with every hex character show whether the rules allow them.
	for _, sample := range []string{"0123456789ab", "cdef01234567"} {
		name := rules.fit(sample)
		if len(name) < len(generatedPrefix)+minHexLength || len(name) < rules.MinLength || (rules.Pattern != nil && !rules.Pattern.MatchString(name)) {
			return nil, fmt.Errorf("the rules of the name property %s allow no name UCP generates, such as %s", property, name)
		}
	}
	return rules, nil
}

// Generate returns the name for the operation seed, the record's client token: "radius-" followed by 12 lowercase
// hex characters of the SHA-256 hash of seed, such as radius-3f9a1c0b7d2e, cut to MaxLength. The same seed always
// gives the same name, so every attempt of one create sends the same name. The registry checked that such names fit
// the rules, with at least 8 hex characters.
func (r *NameRules) Generate(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return r.fit(hex.EncodeToString(sum[:])[:generatedHexLength])
}

// fit returns the name with the hex characters, cut to MaxLength.
func (r *NameRules) fit(hexCharacters string) string {
	name := generatedPrefix + hexCharacters
	if r.MaxLength > 0 && len(name) > r.MaxLength {
		name = name[:r.MaxLength]
	}
	return name
}
