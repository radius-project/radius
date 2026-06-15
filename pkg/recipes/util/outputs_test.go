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
)

func Test_ApplyOutputsMapping(t *testing.T) {
	tests := []struct {
		name            string
		values          map[string]any
		secrets         map[string]any
		outputsMap      map[string]string
		expectedValues  map[string]any
		expectedSecrets map[string]any
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
			name:            "empty maps with outputs mapping",
			values:          map[string]any{},
			secrets:         map[string]any{},
			outputsMap:      map[string]string{"host": "hostname"},
			expectedValues:  map[string]any{},
			expectedSecrets: map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values, secrets := ApplyOutputsMapping(tt.values, tt.secrets, tt.outputsMap)
			assert.Equal(t, tt.expectedValues, values)
			assert.Equal(t, tt.expectedSecrets, secrets)
		})
	}
}
