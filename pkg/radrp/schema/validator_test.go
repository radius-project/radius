// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package schema

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestComponentValidator(t *testing.T) {
	v := componentValidator

	for _, tc := range []struct {
		name    string
		input   string
		expects []ValidationError
	}{{
		name: "valid",
		input: `{
                  "id":         "an ID",
                  "name":       "a name",
                  "kind":       "a kind",
                  "location":   "a location",
                  "properties": {}
                }`,
	}, {
		name:    "invalid json",
		input:   "{{}",
		expects: invalidJSONError(nil),
	}, {
		name: "missing required top-level fields",
		input: `{
                }`,
		expects: requiredFieldErrs("(root)", "kind", "properties"),
	}, {
		name: "wrong types for top-level fields",
		input: `{
                  "location":   42,
                  "tags":       42,
                  "id":         42,
                  "name":       42,
                  "kind":       42,
                  "properties": 42
                }`,
		expects: append(
			invalidTypeErrs("(root)", "string", "integer",
				"id", "name", "kind", "location"),
			invalidTypeErrs("(root)", "object", "integer",
				"tags", "properties")...),
	}, {
		name: "wrong types for tags values",
		input: `{
                  "id": "id", "name": "name", "kind": "kind", "location": "location", "properties": {},
                  "tags": {
                    "key": 42
                  }
                }`,
		expects: invalidTypeErrs("(root).tags", "string", "integer", "key"),
	}, {
		name: "wrong types for properties.* fields",
		input: `{
                  "id": "id", "name": "name", "kind": "kind", "location": "location",
                  "properties": {
                    "revision":        42,
                    "config":          42,
                    "run":             42,
                    "bindings":        42,
                    "uses":            42,
                    "traits":          42,
                    "status":          42
                  }
                }`,
		expects: append(append(
			invalidTypeErrs("(root).properties", "object", "integer",
				"config", "run", "bindings", "status"),
			invalidTypeErrs("(root).properties", "array", "integer",
				"uses", "traits")...),
			invalidTypeErr("(root).properties.revision", "string", "integer")),
	}, {
		name: "unrecognized trait.* fields",
		input: `{
                  "id": "id", "name": "name", "kind": "kind", "location": "location",
                  "properties": {
                    "traits": [{
                      "huh": "invalid"
                    }]
                  }
                }`,
		expects: additionalFieldErrs("(root).properties.traits.0", "huh"),
	}, {
		name: "valid DaprTrait",
		input: `{
                  "id": "id", "name": "name", "kind": "kind", "location": "location",
                  "properties": {
                    "traits": [{
                      "kind":   "dapr.io/App@v1alpha1",
                      "appId":   "appId",
                      "appPort": 9090
                    }]
                  }
                }`,
	}, {
		name: "valid InboundRouteTrait",
		input: `{
                  "id": "id", "name": "name", "kind": "kind", "location": "location",
                  "properties": {
                    "traits": [{
                      "kind":     "radius.dev/InboundRoute@v1alpha1",
                      "hostName": "localhost",
                      "binding":  "foo"
                    }]
                  }
                }`,
	}, {
		name: "cannot combine InboundRouteTrait and DaprTrait",
		input: `{
                  "id": "id", "name": "name", "kind": "kind", "location": "location",
                  "properties": {
                    "traits": [{
                      "kind":     "radius.dev/InboundRoute@v1alpha1",
                      "hostName": "localhost",
                      "binding":  "foo",
                      "appId":    "wrong, cannot combine traits"
                    }]
                  }
                }`,
		expects: additionalFieldErrs("(root).properties.traits.0", "appId"),
	}, {
		name: "valid ManualScalingTrait",
		input: `{
			"id": "id", "name": "name", "kind": "kind", "location": "location",
			"properties": {
			  "traits": [{
				"kind":     "radius.dev/ManualScaling@v1alpha1",
				"replicas":    2
			  }]
			}
		  }`,
	}, {
		name: "valid binding expressions",
		input: `{
                  "id": "id", "name": "name", "kind": "kind", "location": "location",
                  "properties": {
                    "uses": [
                      {
                        "binding": "[[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Components', 'radius', 'frontend-backend', 'frontend')).bindings.default]",
                        "env": {
                          "SERVICE__BACKEND__HOST": "[[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Components', 'radius', 'frontend-backend', 'frontend')).bindings.default.host]"
                        },
                        "secrets": {
                          "store": "[[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Components', 'radius', 'frontend-backend', 'frontend')).bindings.default]",
                          "keys": {
                            "secret": "[[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Components', 'radius', 'frontend-backend', 'frontend')).bindings.default]"
                          }
                        }
                      }
                    ]
                  }
                }`,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			errs := v.ValidateJSON([]byte(tc.input))
			compareErrors(t, tc.expects, stripInnerJSONError(errs))
		})
	}
}

func TestApplicationValidator(t *testing.T) {
	v := applicationValidator

	for _, tc := range []struct {
		name    string
		input   string
		expects []ValidationError
	}{{
		name: "valid",
		input: `{
                  "id":         "an ID",
                  "name":       "a name",
                  "kind":       "a kind",
                  "location":   "a location"
                }`,
	}, {
		name:    "invalid json",
		input:   "{{}",
		expects: invalidJSONError(nil),
	}, {
		name: "wrong types for top level fields",
		input: `{
                  "location":   false,
                  "tags":       false,
                  "properties": false
                }`,
		expects: append(
			invalidTypeErrs("(root)", "object", "boolean",
				"tags", "properties"),
			invalidTypeErr("(root).location", "string", "boolean")),
	}, {
		name: "wrong types for tags values",
		input: `{
                  "id": "id", "name": "name", "kind": "kind", "location": "location",
                  "tags": {
                    "key": 42
                  }
                }`,
		expects: invalidTypeErrs("(root).tags", "string", "integer", "key"),
	}} {
		t.Run(tc.name, func(t *testing.T) {
			errs := v.ValidateJSON([]byte(tc.input))
			compareErrors(t, tc.expects, stripInnerJSONError(errs))
		})
	}
}

func TestDeploymentValidator(t *testing.T) {
	v := deploymentValidator

	for _, tc := range []struct {
		name    string
		input   string
		expects []ValidationError
	}{{
		name: "valid",
		input: `{
                  "id":         "id",
                  "name":       "name",
                  "kind":       "kind",
                  "location":   "location",
                  "properties": {
                    "provisioningState": "READY",
                    "components": [{
                      "componentName": "component-name",
                      "revision":      "revision"
                    }]
                  }
                }`,
	}, {
		name:    "invalid json",
		input:   "{{}",
		expects: invalidJSONError(nil),
	}, {
		name: "missing required fields at root",
		input: `{
                }`,
		expects: requiredFieldErrs("(root)", "properties"),
	}, {
		name: "wrong types for top level fields",
		input: `{
                  "name":       false,
                  "location":   false,
                  "tags":       false,
                  "properties": false
                }`,
		expects: append(
			invalidTypeErrs("(root)", "object", "boolean",
				"tags", "properties"),
			invalidTypeErrs("(root)", "string", "boolean",
				"location", "name")...),
	}, {
		name: "wrong types for tags values",
		input: `{
                  "id": "id", "name": "name", "kind": "kind", "location": "location",
                  "properties": {
                    "components": []
                  },
                  "tags": {
                    "key": 42
                  }
                }`,
		expects: invalidTypeErrs("(root).tags", "string", "integer", "key"),
	}, {
		name: "unexpected properties.foo field",
		input: `{
                  "id": "id", "name": "name", "kind": "kind", "location": "location",
                  "properties": {
                    "foo": "additionalProperty not allowed",
                    "components": []
                  }
                }`,
		expects: additionalFieldErrs("(root).properties", "foo"),
	}, {
		name: "wrong types for components values",
		input: `{
                  "id": "id", "name": "name", "kind": "kind", "location": "location",
                  "properties": {
                    "components": ["wrong", "type"]
                  }
                }`,
		expects: invalidTypeErrs("(root).properties.components", "object", "string", "0", "1"),
	}, {
		name: "wrong types for components[].* fields",
		input: `{
                  "id": "id", "name": "name", "kind": "kind", "location": "location",
                  "properties": {
                    "components": [{
                      "componentName": 42,
                      "revision":      42
                    }]
                  }
                }`,
		expects: invalidTypeErrs("(root).properties.components.0", "string", "integer",
			"componentName", "revision"),
	}, {
		name: "missing required fields for components[]",
		input: `{
                  "id": "id", "name": "name", "kind": "kind", "location": "location",
                  "properties": {
                    "components": [{}]
                  }
                }`,
		expects: requiredFieldErrs("(root).properties.components.0", "componentName"),
	}} {
		t.Run(tc.name, func(t *testing.T) {
			errs := v.ValidateJSON([]byte(tc.input))
			compareErrors(t, tc.expects, stripInnerJSONError(errs))
		})
	}
}

func TestScopeValidator(t *testing.T) {
	v := scopeValidator

	for _, tc := range []struct {
		name    string
		input   string
		expects []ValidationError
	}{{
		name: "valid",
		input: `{
                  "id":         "id",
                  "name":       "name",
                  "kind":       "kind",
                  "location":   "location"
                }`,
	}, {
		name:    "invalid json",
		input:   "{{}",
		expects: invalidJSONError(nil),
	}, {
		name: "wrong types for top level fields",
		input: `{
                  "name": false,
                  "location":   false,
                  "tags":       false
                }`,
		expects: append(
			invalidTypeErrs("(root)", "object", "boolean", "tags"),
			invalidTypeErrs("(root)", "string", "boolean",
				"location", "name")...),
	}, {
		name: "wrong types for tags values",
		input: `{
                  "id": "id", "name": "name", "kind": "kind", "location": "location",
                  "tags": {
                    "key": 42
                  }
                }`,
		expects: invalidTypeErrs("(root).tags", "string", "integer", "key"),
	}} {
		t.Run(tc.name, func(t *testing.T) {
			errs := v.ValidateJSON([]byte(tc.input))
			compareErrors(t, tc.expects, stripInnerJSONError(errs))
		})
	}
}

func TestValidatorFactory(t *testing.T) {
	type Application struct{}
	type Component struct{}
	type Deployment struct{}
	type Scope struct{}
	type Foo struct{}

	for _, tc := range []struct {
		name  string
		input interface{}
		want  *validator
	}{{
		name:  "Application",
		input: Application{},
		want:  applicationValidator,
	}, {
		name:  "&Application",
		input: &Application{},
		want:  applicationValidator,
	}, {
		name:  "Component",
		input: Component{},
		want:  componentValidator,
	}, {
		name:  "Component",
		input: &Component{},
		want:  componentValidator,
	}, {
		name:  "Deployment",
		input: Deployment{},
		want:  deploymentValidator,
	}, {
		name:  "&Deployment",
		input: &Deployment{},
		want:  deploymentValidator,
	}, {
		name:  "Scope",
		input: Scope{},
		want:  scopeValidator,
	}, {
		name:  "&Scope",
		input: &Scope{},
		want:  scopeValidator,
	}, {
		name:  "Unknown",
		input: Foo{},
		want:  nil,
	}, {
		name:  "nil",
		input: nil,
		want:  nil,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidatorFor(tc.input)
			if tc.want == nil {
				if err == nil {
					t.Error("Expected error, saw nil")
				}
				return
			}
			if diff := cmp.Diff(tc.want, got, cmpopts.IgnoreUnexported(validator{})); diff != "" {
				t.Errorf("ValidatorFor() mismatch (-want +got):\n%s", diff)
			}
			if tc.want != nil && err != nil {
				t.Errorf("Expected no error, saw %v", err)
			}
		})
	}
}

func compareErrors(t *testing.T, wants []ValidationError, gots []ValidationError) {
	wantSet := asErrSet(wants)
	gotSet := asErrSet(gots)

	for want := range wantSet {
		if _, got := gotSet[want]; !got {
			t.Errorf("Missing expected validation error: %#v", want)
		}
	}
	for got := range gotSet {
		if _, want := wantSet[got]; !want {
			t.Errorf("Unexpected validation error: %#v", got)
		}
	}
}

func asErrSet(errs []ValidationError) map[ValidationError]struct{} {
	set := make(map[ValidationError]struct{}, len(errs))
	yes := struct{}{}
	for _, err := range errs {
		set[err] = yes
	}
	return set
}

func stripInnerJSONError(errs []ValidationError) []ValidationError {
	r := make([]ValidationError, len(errs))
	for i, err := range errs {
		r[i] = err
		r[i].JSONError = nil
	}
	return r
}

func requiredFieldErrs(position string, fields ...string) []ValidationError {
	errs := make([]ValidationError, len(fields))
	for i, f := range fields {
		errs[i] = requiredFieldErr(position, f)
	}
	return errs
}

func requiredFieldErr(position string, field string) ValidationError {
	return ValidationError{
		Position: position,
		Message:  fmt.Sprintf("%s is required", field),
	}
}

func additionalFieldErrs(position string, fields ...string) []ValidationError {
	errs := make([]ValidationError, len(fields))
	for i, f := range fields {
		errs[i] = additionalFieldErr(position, f)
	}
	return errs
}

func additionalFieldErr(position string, field string) ValidationError {
	return ValidationError{
		Position: position,
		Message:  fmt.Sprintf("Additional property %s is not allowed", field),
	}
}

func invalidTypeErr(position string, want string, got string) ValidationError {
	return ValidationError{
		Position: position,
		Message:  fmt.Sprintf("Invalid type. Expected: %s, given: %s", want, got),
	}
}

func invalidTypeErrs(position string, want string, got string, fields ...string) []ValidationError {
	errs := make([]ValidationError, len(fields))
	for i, f := range fields {
		errs[i] = invalidTypeErr(fmt.Sprintf("%s.%s", position, f), want, got)
	}
	return errs
}
