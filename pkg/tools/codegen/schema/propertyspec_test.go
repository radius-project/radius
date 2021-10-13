// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package schema

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestPropertySpecUnmarshalJSON(t *testing.T) {
	for _, tc := range []struct {
		name      string
		input     string
		expected  PropertySpec
		expectErr bool
	}{{
		name: "valid",
		input: `{
                  "description": "Trait kind",
                  "type": "string",
                  "enum": ["dapr.io/App@v1alpha1"]
                }`,
		expected: PropertySpec{
			Enum:        []interface{}{"dapr.io/App@v1alpha1"},
			Type:        "string",
			Description: "Trait kind",
		},
	}, {
		name: "no enum",
		input: `{
                  "description": "Trait kind",
                  "type": "string"
                }`,
		expected: PropertySpec{
			Type:        "string",
			Description: "Trait kind",
		},
	}, {
		name:     "empty",
		input:    `{}`,
		expected: PropertySpec{},
	}, {
		name:      "wrong enum type",
		input:     `{ "enum": 42 }`,
		expectErr: true,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			out := PropertySpec{}
			err := json.Unmarshal([]byte(tc.input), &out)
			if tc.expectErr && err == nil {
				t.Fatal("Expected an error, saw none")
			}
			if !tc.expectErr && err != nil {
				t.Fatalf("Unexpected error %v", err)
			}
			if diff := cmp.Diff(tc.expected, out, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("Unexpected diff (-want, +got): %s", diff)
			}
		})
	}
}
