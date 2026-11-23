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

package parameters

import (
	"encoding/json"
	"strings"
	"testing"

	radapp "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/stretchr/testify/require"
)

const compiledInputs = `{
  "parameters": {
    "application": {"value":"orders"},
    "password": {"expression":"[externalInputs('opaque-42')]"}
  },
  "externalInputDefinitions": {
    "opaque-42": {"kind":"radius.secretKeyRef","config":{"name":"credentials","key":"password"}}
  }
}`

func TestTranslate(t *testing.T) {
	tests := []struct {
		name string
		edit func(string) string
	}{
		{"missing definition", func(s string) string { return strings.Replace(s, `"opaque-42": {`, `"different": {`, 1) }},
		{"unsupported kind", func(s string) string { return strings.ReplaceAll(s, "radius.secretKeyRef", "other") }},
		{"namespace", func(s string) string {
			return strings.ReplaceAll(s, `"name":"credentials"`, `"name":"credentials","namespace":"other"`)
		}},
		{"null namespace", func(s string) string {
			return strings.ReplaceAll(s, `"name":"credentials"`, `"name":"credentials","namespace":null`)
		}},
		{"unknown field", func(s string) string {
			return strings.ReplaceAll(s, `"key":"password"`, `"key":"password","optional":true`)
		}},
		{"empty key", func(s string) string { return strings.ReplaceAll(s, `"key":"password"`, `"key":""`) }},
		{"invalid name", func(s string) string { return strings.ReplaceAll(s, `"name":"credentials"`, `"name":"../credentials"`) }},
		{"malformed definition", func(s string) string { return strings.ReplaceAll(s, `"config":`, `"unknown":`) }},
		{"composed expression", func(s string) string {
			return strings.ReplaceAll(s, "[externalInputs('opaque-42')]", "[concat(externalInputs('opaque-42'), 'suffix')]")
		}},
		{"value and expression", func(s string) string {
			return strings.ReplaceAll(s, `"password": {"expression":`, `"password": {"value":"literal","expression":`)
		}},
		{"unused definition", func(s string) string { return strings.ReplaceAll(s, `"opaque-42": {`, `"unused":null,"opaque-42": {`) }},
		{"non-string literal", func(s string) string { return strings.ReplaceAll(s, `"value":"orders"`, `"value":42`) }},
		{"null literal", func(s string) string { return strings.ReplaceAll(s, `"value":"orders"`, `"value":null`) }},
		{"unknown binding", func(s string) string { return strings.ReplaceAll(s, `"value":"orders"`, `"reference":{}`) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document, err := Parse([]byte(test.edit(compiledInputs)))
			require.NoError(t, err)
			_, _, err = Translate(document)
			require.Error(t, err)
		})
	}
	for _, stdout := range []bool{false, true} {
		data := []byte(compiledInputs)
		if stdout {
			var err error
			data, err = json.Marshal(map[string]string{"parametersJson": compiledInputs, "templateJson": "{}"})
			require.NoError(t, err)
		}
		document, err := Parse(data)
		require.NoError(t, err)
		values, references, err := Translate(document)
		require.NoError(t, err)
		require.Equal(t, map[string]string{"application": "orders"}, values)
		require.Equal(t, radapp.SecretKeyReference{Name: "credentials", Key: "password"}, references["password"].SecretKeyRef)
	}
}

func TestValidateReferenceTargets(t *testing.T) {
	references := map[string]radapp.ParameterReference{
		"password": {SecretKeyRef: radapp.SecretKeyReference{Name: "credentials", Key: "password"}},
	}
	tests := []struct {
		name        string
		declaration string
		literals    map[string]string
		wantError   bool
	}{
		{"secure string", `{"type":"secureString"}`, nil, false},
		{"ordinary string", `{"type":"string"}`, nil, true},
		{"secure object", `{"type":"secureObject"}`, nil, true},
		{"alternate secure metadata", `{"type":"secureString","metadata":{"secure":true}}`, nil, true},
		{"default", `{"type":"secureString","defaultValue":"unsafe"}`, nil, true},
		{"null default", `{"type":"secureString","defaultValue":null}`, nil, true},
		{"literal conflict", `{"type":"secureString"}`, map[string]string{"password": "unsafe"}, true},
		{"required unbound", `{"type":"secureString"},"other":{"type":"string"}`, nil, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			template := `{"parameters":{"password":` + test.declaration + `}}`
			err := Validate(template, test.literals, references)
			if test.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
	require.Error(t, Validate(`{"parameters":{}}`, nil, references))
}

func TestValidateResolvedValue(t *testing.T) {
	for _, test := range []struct {
		name        string
		constraints string
		value       string
		wantError   bool
	}{
		{"present empty", "", "", false},
		{"minimum", `,"minLength":1`, "", true},
		{"maximum", `,"maxLength":2`, "abc", true},
		{"UTF16 length", `,"maxLength":1`, "\U0001F600", true},
		{"invalid UTF8", "", string([]byte{0xff}), true},
		{"whitespace", `,"minLength":3,"maxLength":3`, " \n ", false},
		{"allowed", `,"allowedValues":["yes"]`, "yes", false},
		{"not allowed", `,"allowedValues":["yes"]`, "no", true},
		{"size bound", "", strings.Repeat("x", 64*1024+1), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			template := `{"parameters":{"password":{"type":"secureString"` + test.constraints + `}}}`
			err := ValidateValue(template, "password", test.value)
			if test.wantError {
				require.Error(t, err)
				require.NotContains(t, err.Error(), "resolved-value-marker")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
