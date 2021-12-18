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

func TestSchemaUnmarshalJSON(t *testing.T) {
	for _, tc := range []struct {
		name      string
		input     string
		expected  Schema
		expectErr bool
	}{{
		name:     "empty",
		input:    `{}`,
		expected: Schema{},
	}, {
		name: "traits",
		input: `{
		  "definitions": {
		    "ResourceTrait": {
		      "description": "Trait of a resource",
		      "type": "object",
		      "oneOf": [{
		        "$ref": "#/definitions/DaprTrait"
		      }]
		    },
		    "DaprTrait": {
		      "type": "object",
		      "description": "Dapr Sidecar Trait",
		      "properties": {
		        "kind": {
		          "description": "Trait kind",
		          "type": "string",
		          "enum": ["dapr.io/App@v1alpha1"]
		        },
		        "appPort": {
		          "description": "Dapr appPort",
		          "type": "integer"
		        },
		        "appId": {
		          "description": "Dapr appId",
		          "type": "string"
		        }
		      },
		      "additionalProperties": false
		    }
		  }
		}`,
		expected: Schema{
			Definitions: map[string]*TypeSpec{
				"ResourceTrait": {
					OneOf: []*TypeRef{
						NewTypeRef("#/definitions/DaprTrait"),
					},
					AdditionalProperties: map[string]interface{}{
						"type":        "object",
						"description": "Trait of a resource",
					},
				},
				"DaprTrait": {
					Properties: map[string]*PropertySpec{
						"kind": {
							Enum:        []interface{}{"dapr.io/App@v1alpha1"},
							Type:        "string",
							Description: "Trait kind",
						},
						"appPort": {
							Type:        "integer",
							Description: "Dapr appPort",
						},
						"appId": {
							Type:        "string",
							Description: "Dapr appId",
						},
					},
					AdditionalProperties: map[string]interface{}{
						"type":                 "object",
						"description":          "Dapr Sidecar Trait",
						"additionalProperties": false,
					},
				}},
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			out := Schema{}
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
