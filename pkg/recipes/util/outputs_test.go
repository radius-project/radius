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

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ValidateOutputsMapping(t *testing.T) {
	tests := []struct {
		name              string
		declaredOutputs   []string
		outputsMap        map[string]string
		secretOutputsMap  map[string]string
		expectedError     string
		expectedMissing   []string
		expectedAvailable []string
	}{
		{
			name:             "accepts declared mappings",
			declaredOutputs:  []string{"endpoint", "primaryConnectionString"},
			outputsMap:       map[string]string{"host": "endpoint"},
			secretOutputsMap: map[string]string{"connectionString": "primaryConnectionString"},
		},
		{
			name:              "reports undeclared mappings and available outputs",
			declaredOutputs:   []string{"zeta", "alpha"},
			outputsMap:        map[string]string{"host": "missingHost"},
			secretOutputsMap:  map[string]string{"connectionString": "missingSecret"},
			expectedError:     `recipe "test-recipe" for resource type "Test.Resources/widgets": invalid outputs mapping: no declared module output matches outputs["host"] -> "missingHost", secrets["connectionString"] -> "missingSecret"; available module outputs: "alpha", "zeta"`,
			expectedMissing:   []string{`outputs["host"] -> "missingHost"`, `secrets["connectionString"] -> "missingSecret"`},
			expectedAvailable: []string{"alpha", "zeta"},
		},
		{
			name:              "reports when the module declares no outputs",
			secretOutputsMap:  map[string]string{"connectionString": "primaryConnectionString"},
			expectedError:     `recipe "test-recipe" for resource type "Test.Resources/widgets": invalid outputs mapping: no declared module output matches secrets["connectionString"] -> "primaryConnectionString"; available module outputs: none`,
			expectedMissing:   []string{`secrets["connectionString"] -> "primaryConnectionString"`},
			expectedAvailable: []string{},
		},
		{
			name:              "reports empty module output name",
			declaredOutputs:   []string{"endpoint"},
			outputsMap:        map[string]string{"host": ""},
			expectedError:     `recipe "test-recipe" for resource type "Test.Resources/widgets": invalid outputs mapping: no declared module output matches outputs["host"] -> ""; available module outputs: "endpoint"`,
			expectedMissing:   []string{`outputs["host"] -> ""`},
			expectedAvailable: []string{"endpoint"},
		},
		{
			name:              "distinguishes mapping categories when property names overlap",
			outputsMap:        map[string]string{"secrets.connectionString": "missingValue"},
			secretOutputsMap:  map[string]string{"connectionString": "missingSecret"},
			expectedError:     `recipe "test-recipe" for resource type "Test.Resources/widgets": invalid outputs mapping: no declared module output matches outputs["secrets.connectionString"] -> "missingValue", secrets["connectionString"] -> "missingSecret"; available module outputs: none`,
			expectedMissing:   []string{`outputs["secrets.connectionString"] -> "missingValue"`, `secrets["connectionString"] -> "missingSecret"`},
			expectedAvailable: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOutputsMapping(
				"test-recipe",
				"Test.Resources/widgets",
				tt.declaredOutputs,
				tt.outputsMap,
				tt.secretOutputsMap)
			if tt.expectedError == "" {
				require.NoError(t, err)
				return
			}

			var mappingErr *OutputMappingError
			require.ErrorAs(t, err, &mappingErr)
			require.EqualError(t, err, tt.expectedError)
			require.Equal(t, tt.expectedMissing, mappingErr.MissingMappings)
			require.Equal(t, tt.expectedAvailable, mappingErr.AvailableOutputs)
		})
	}
}

func Test_ApplyOutputsMapping(t *testing.T) {
	tests := []struct {
		name              string
		values            map[string]any
		secrets           map[string]any
		outputsMap        map[string]string
		secretOutputsMap  map[string]string
		expectedValues    map[string]any
		expectedSecrets   map[string]any
		expectedError     string
		expectedMissing   []string
		expectedAvailable []string
	}{
		{
			name:            "nil outputs map passes through values",
			values:          map[string]any{"hostname": "myhost", "port": 5432},
			secrets:         map[string]any{"password": "secret"},
			outputsMap:      nil,
			expectedValues:  map[string]any{"hostname": "myhost", "port": 5432},
			expectedSecrets: map[string]any{"password": "secret"},
		},
		{
			name:            "empty outputs map passes through values",
			values:          map[string]any{"hostname": "myhost"},
			secrets:         map[string]any{"password": "secret"},
			outputsMap:      map[string]string{},
			expectedValues:  map[string]any{"hostname": "myhost"},
			expectedSecrets: map[string]any{"password": "secret"},
		},
		{
			name:            "maps output names to property names",
			values:          map[string]any{"hostname": "myhost", "port_number": 5432},
			secrets:         map[string]any{},
			outputsMap:      map[string]string{"host": "hostname", "port": "port_number"},
			expectedValues:  map[string]any{"host": "myhost", "port": 5432},
			expectedSecrets: map[string]any{},
		},
		{
			name:            "missing output key in values is skipped silently",
			values:          map[string]any{"hostname": "myhost"},
			secrets:         map[string]any{},
			outputsMap:      map[string]string{"host": "hostname", "port": "nonexistent"},
			expectedValues:  map[string]any{"host": "myhost"},
			expectedSecrets: map[string]any{},
		},
		{
			name:            "sensitive output mapping",
			values:          map[string]any{},
			secrets:         map[string]any{"db_password": "secret123"},
			outputsMap:      map[string]string{"password": "db_password"},
			expectedValues:  map[string]any{},
			expectedSecrets: map[string]any{"password": "secret123"},
		},
		{
			name:            "nil values and secrets with nil outputs map",
			values:          nil,
			secrets:         nil,
			outputsMap:      nil,
			expectedValues:  map[string]any{},
			expectedSecrets: map[string]any{},
		},
		{
			name:            "nil values and secrets with outputs mapping returns empty maps",
			values:          nil,
			secrets:         nil,
			outputsMap:      map[string]string{"host": "hostname"},
			expectedValues:  map[string]any{},
			expectedSecrets: map[string]any{},
		},
		{
			name:            "empty maps with outputs mapping",
			values:          map[string]any{},
			secrets:         map[string]any{},
			outputsMap:      map[string]string{"host": "hostname"},
			expectedValues:  map[string]any{},
			expectedSecrets: map[string]any{},
		},
		{
			name:             "secretOutputs forces a plain value output to a secret (AVM case)",
			values:           map[string]any{"name": "myhub", "primaryConnectionString": "Endpoint=sb://..."},
			secrets:          map[string]any{},
			outputsMap:       map[string]string{"host": "name"},
			secretOutputsMap: map[string]string{"connectionString": "primaryConnectionString"},
			expectedValues:   map[string]any{"host": "myhub"},
			expectedSecrets:  map[string]any{"connectionString": "Endpoint=sb://..."},
		},
		{
			name:             "secretOutputs maps a module-classified secret output",
			values:           map[string]any{},
			secrets:          map[string]any{"primaryKey": "abc123"},
			secretOutputsMap: map[string]string{"accessKey": "primaryKey"},
			expectedValues:   map[string]any{},
			expectedSecrets:  map[string]any{"accessKey": "abc123"},
		},
		{
			name:              "secretOutputs with missing module output fails",
			values:            map[string]any{"name": "myhub"},
			secrets:           map[string]any{},
			secretOutputsMap:  map[string]string{"connectionString": "nonexistent"},
			expectedError:     `invalid outputs mapping: missing deployment output values for secrets["connectionString"] -> "nonexistent"; available deployment outputs: "name"`,
			expectedMissing:   []string{`secrets["connectionString"] -> "nonexistent"`},
			expectedAvailable: []string{"name"},
		},
		{
			name:              "secretOutputs with null module output fails",
			values:            map[string]any{"name": "myhub", "primaryConnectionString": nil},
			secrets:           map[string]any{},
			secretOutputsMap:  map[string]string{"connectionString": "primaryConnectionString"},
			expectedError:     `invalid outputs mapping: missing deployment output values for secrets["connectionString"] -> "primaryConnectionString"; available deployment outputs: "name"`,
			expectedMissing:   []string{`secrets["connectionString"] -> "primaryConnectionString"`},
			expectedAvailable: []string{"name"},
		},
		{
			name:            "null ordinary output is skipped",
			values:          map[string]any{"hostname": nil},
			outputsMap:      map[string]string{"host": "hostname"},
			expectedValues:  map[string]any{},
			expectedSecrets: map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values, secrets, err := ApplyOutputsMapping(tt.values, tt.secrets, tt.outputsMap, tt.secretOutputsMap)
			if tt.expectedError != "" {
				require.EqualError(t, err, tt.expectedError)
				require.Nil(t, values)
				require.Nil(t, secrets)

				var mappingErr *MissingOutputValuesError
				require.ErrorAs(t, err, &mappingErr)
				require.Equal(t, tt.expectedMissing, mappingErr.MissingMappings)
				require.Equal(t, tt.expectedAvailable, mappingErr.AvailableOutputs)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedValues, values)
			assert.Equal(t, tt.expectedSecrets, secrets)
		})
	}
}
