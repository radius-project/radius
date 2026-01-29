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

package encryption

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	// testResourceID is a sample resource ID used for testing associated data
	testResourceID = "/planes/radius/local/resourceGroups/test/providers/Test.Resource/testResources/myResource"
)

func TestParseFieldPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected []pathSegment
	}{
		{
			name: "simple-field",
			path: "password",
			expected: []pathSegment{
				{segmentType: segmentTypeField, value: "password"},
			},
		},
		{
			name: "nested-field",
			path: "credentials.password",
			expected: []pathSegment{
				{segmentType: segmentTypeField, value: "credentials"},
				{segmentType: segmentTypeField, value: "password"},
			},
		},
		{
			name: "deeply-nested-field",
			path: "config.database.connection.password",
			expected: []pathSegment{
				{segmentType: segmentTypeField, value: "config"},
				{segmentType: segmentTypeField, value: "database"},
				{segmentType: segmentTypeField, value: "connection"},
				{segmentType: segmentTypeField, value: "password"},
			},
		},
		{
			name: "array-wildcard",
			path: "secrets[*].value",
			expected: []pathSegment{
				{segmentType: segmentTypeField, value: "secrets"},
				{segmentType: segmentTypeWildcard},
				{segmentType: segmentTypeField, value: "value"},
			},
		},
		{
			name: "map-wildcard",
			path: "config[*]",
			expected: []pathSegment{
				{segmentType: segmentTypeField, value: "config"},
				{segmentType: segmentTypeWildcard},
			},
		},
		{
			name: "specific-index",
			path: "items[0].name",
			expected: []pathSegment{
				{segmentType: segmentTypeField, value: "items"},
				{segmentType: segmentTypeIndex, value: "0"},
				{segmentType: segmentTypeField, value: "name"},
			},
		},
		{
			name: "multiple-wildcards",
			path: "data[*].secrets[*].value",
			expected: []pathSegment{
				{segmentType: segmentTypeField, value: "data"},
				{segmentType: segmentTypeWildcard},
				{segmentType: segmentTypeField, value: "secrets"},
				{segmentType: segmentTypeWildcard},
				{segmentType: segmentTypeField, value: "value"},
			},
		},
		{
			name:     "unterminated-bracket",
			path:     "secrets[*",
			expected: nil,
		},
		{
			name:     "unterminated-bracket-with-index",
			path:     "items[0",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseFieldPath(tt.path)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestSensitiveDataHandler_EncryptDecrypt_SimpleField(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	handler, err := NewSensitiveDataHandlerFromKey(key)
	require.NoError(t, err)

	data := map[string]any{
		"username": "admin",
		"password": "super-secret-password",
	}

	// Encrypt
	err = handler.EncryptSensitiveFields(data, []string{"password"}, testResourceID)
	require.NoError(t, err)

	// Verify password is encrypted
	password := data["password"]
	encMap, ok := password.(map[string]any)
	require.True(t, ok, "password should be encrypted map")
	require.NotEmpty(t, encMap["encrypted"])
	require.NotEmpty(t, encMap["nonce"])

	// Username should be unchanged
	require.Equal(t, "admin", data["username"])

	// Decrypt
	err = handler.DecryptSensitiveFields(context.Background(), data, []string{"password"}, testResourceID)
	require.NoError(t, err)

	// Verify password is decrypted
	require.Equal(t, "super-secret-password", data["password"])
	require.Equal(t, "admin", data["username"])
}

func TestSensitiveDataHandler_EncryptDecrypt_NestedField(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	handler, err := NewSensitiveDataHandlerFromKey(key)
	require.NoError(t, err)

	data := map[string]any{
		"name": "test-resource",
		"credentials": map[string]any{
			"username": "admin",
			"password": "nested-secret",
			"apiKey":   "key-12345",
		},
	}

	// Encrypt password and apiKey
	err = handler.EncryptSensitiveFields(data, []string{"credentials.password", "credentials.apiKey"}, testResourceID)
	require.NoError(t, err)

	// Verify encrypted fields
	creds := data["credentials"].(map[string]any)
	require.Equal(t, "admin", creds["username"])

	_, passwordIsEncrypted := creds["password"].(map[string]any)
	require.True(t, passwordIsEncrypted)

	_, apiKeyIsEncrypted := creds["apiKey"].(map[string]any)
	require.True(t, apiKeyIsEncrypted)

	// Decrypt
	err = handler.DecryptSensitiveFields(context.Background(), data, []string{"credentials.password", "credentials.apiKey"}, testResourceID)
	require.NoError(t, err)

	creds = data["credentials"].(map[string]any)
	require.Equal(t, "nested-secret", creds["password"])
	require.Equal(t, "key-12345", creds["apiKey"])
}

func TestSensitiveDataHandler_EncryptDecrypt_ArrayWildcard(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	handler, err := NewSensitiveDataHandlerFromKey(key)
	require.NoError(t, err)

	data := map[string]any{
		"name": "test-resource",
		"secrets": []any{
			map[string]any{"name": "secret1", "value": "value1"},
			map[string]any{"name": "secret2", "value": "value2"},
			map[string]any{"name": "secret3", "value": "value3"},
		},
	}

	// Encrypt all secret values
	err = handler.EncryptSensitiveFields(data, []string{"secrets[*].value"}, testResourceID)
	require.NoError(t, err)

	// Verify all values are encrypted
	secrets := data["secrets"].([]any)
	for i, s := range secrets {
		secret := s.(map[string]any)
		require.Equal(t, "secret"+string(rune('1'+i)), secret["name"])

		_, valueIsEncrypted := secret["value"].(map[string]any)
		require.True(t, valueIsEncrypted, "secret[%d].value should be encrypted", i)
	}

	// Decrypt
	err = handler.DecryptSensitiveFields(context.Background(), data, []string{"secrets[*].value"}, testResourceID)
	require.NoError(t, err)

	secrets = data["secrets"].([]any)
	for i, s := range secrets {
		secret := s.(map[string]any)
		require.Equal(t, "value"+string(rune('1'+i)), secret["value"])
	}
}

func TestSensitiveDataHandler_EncryptDecrypt_MapWildcard(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	handler, err := NewSensitiveDataHandlerFromKey(key)
	require.NoError(t, err)

	data := map[string]any{
		"name": "test-resource",
		"config": map[string]any{
			"database_password": "db-secret",
			"api_key":           "api-secret",
			"token":             "token-secret",
		},
	}

	// Encrypt all config values
	err = handler.EncryptSensitiveFields(data, []string{"config[*]"}, testResourceID)
	require.NoError(t, err)

	// Verify all config values are encrypted
	config := data["config"].(map[string]any)
	for key, value := range config {
		_, isEncrypted := value.(map[string]any)
		require.True(t, isEncrypted, "config[%s] should be encrypted", key)
	}

	// Decrypt
	err = handler.DecryptSensitiveFields(context.Background(), data, []string{"config[*]"}, testResourceID)
	require.NoError(t, err)

	config = data["config"].(map[string]any)
	require.Equal(t, "db-secret", config["database_password"])
	require.Equal(t, "api-secret", config["api_key"])
	require.Equal(t, "token-secret", config["token"])
}

func TestSensitiveDataHandler_EncryptDecrypt_ObjectValue(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	handler, err := NewSensitiveDataHandlerFromKey(key)
	require.NoError(t, err)

	data := map[string]any{
		"name": "test-resource",
		"sensitiveConfig": map[string]any{
			"password":   "secret-pass",
			"privateKey": "-----BEGIN PRIVATE KEY-----\nMIIE...",
			"nested": map[string]any{
				"deep": "value",
			},
		},
	}

	// Encrypt entire object
	err = handler.EncryptSensitiveFields(data, []string{"sensitiveConfig"}, testResourceID)
	require.NoError(t, err)

	// Verify the entire object is encrypted
	_, isEncrypted := data["sensitiveConfig"].(map[string]any)
	require.True(t, isEncrypted)

	encData := data["sensitiveConfig"].(map[string]any)
	require.NotEmpty(t, encData["encrypted"])
	require.NotEmpty(t, encData["nonce"])

	// Decrypt
	err = handler.DecryptSensitiveFields(context.Background(), data, []string{"sensitiveConfig"}, testResourceID)
	require.NoError(t, err)

	// Verify decrypted object
	config := data["sensitiveConfig"].(map[string]any)
	require.Equal(t, "secret-pass", config["password"])
	require.Equal(t, "-----BEGIN PRIVATE KEY-----\nMIIE...", config["privateKey"])

	nested := config["nested"].(map[string]any)
	require.Equal(t, "value", nested["deep"])
}

func TestSensitiveDataHandler_FieldNotFound(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	handler, err := NewSensitiveDataHandlerFromKey(key)
	require.NoError(t, err)

	data := map[string]any{
		"username": "admin",
	}

	// Encrypting non-existent field should be skipped (no error) - supports optional sensitive fields
	err = handler.EncryptSensitiveFields(data, []string{"password"}, testResourceID)
	require.NoError(t, err)

	// Decrypting non-existent field should be skipped (no error)
	err = handler.DecryptSensitiveFields(context.Background(), data, []string{"password"}, testResourceID)
	require.NoError(t, err)
}

func TestSensitiveDataHandler_OptionalSensitiveFields(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	handler, err := NewSensitiveDataHandlerFromKey(key)
	require.NoError(t, err)

	// Simulate a resource with optional sensitive fields - only some are present
	data := map[string]any{
		"name":     "my-resource",
		"password": "secret-password", // present
		// "apiKey" is absent (optional)
		// "credentials.token" is absent (optional nested)
	}

	sensitivePaths := []string{"password", "apiKey", "credentials.token"}

	// Encrypt should succeed even though some fields are missing
	err = handler.EncryptSensitiveFields(data, sensitivePaths, testResourceID)
	require.NoError(t, err)

	// The present field should be encrypted
	_, isEncrypted := data["password"].(map[string]any)
	require.True(t, isEncrypted, "password should be encrypted")

	// Name should be unchanged
	require.Equal(t, "my-resource", data["name"])

	// Missing fields should not be added
	_, hasAPIKey := data["apiKey"]
	require.False(t, hasAPIKey, "apiKey should not be added")

	_, hasCredentials := data["credentials"]
	require.False(t, hasCredentials, "credentials should not be added")

	// Decrypt should also succeed
	err = handler.DecryptSensitiveFields(context.Background(), data, sensitivePaths, testResourceID)
	require.NoError(t, err)

	// Verify decryption worked for the present field
	require.Equal(t, "secret-password", data["password"])
}

func TestSensitiveDataHandler_EmptyValue(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	handler, err := NewSensitiveDataHandlerFromKey(key)
	require.NoError(t, err)

	data := map[string]any{
		"password": "",
	}

	// Empty string should remain empty
	err = handler.EncryptSensitiveFields(data, []string{"password"}, testResourceID)
	require.NoError(t, err)
	require.Equal(t, "", data["password"])
}

func TestSensitiveDataHandler_NilValue(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	handler, err := NewSensitiveDataHandlerFromKey(key)
	require.NoError(t, err)

	data := map[string]any{
		"password": nil,
	}

	// Nil should remain nil
	err = handler.EncryptSensitiveFields(data, []string{"password"}, testResourceID)
	require.NoError(t, err)
	require.Nil(t, data["password"])
}

func TestSensitiveDataHandler_InvalidFieldPath(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	handler, err := NewSensitiveDataHandlerFromKey(key)
	require.NoError(t, err)

	data := map[string]any{
		"password": "secret",
	}

	// Empty path should return error
	err = handler.EncryptSensitiveFields(data, []string{""}, testResourceID)
	require.Error(t, err)
}

func TestSensitiveDataHandler_RoundTrip_ComplexStructure(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	handler, err := NewSensitiveDataHandlerFromKey(key)
	require.NoError(t, err)

	original := map[string]any{
		"name": "my-application",
		"database": map[string]any{
			"host":     "localhost",
			"port":     5432,
			"password": "db-password-123",
		},
		"secrets": []any{
			map[string]any{"key": "API_KEY", "value": "api-secret-value"},
			map[string]any{"key": "AUTH_TOKEN", "value": "auth-token-value"},
		},
		"config": map[string]any{
			"public_setting": "visible",
			"private_key":    "secret-key-data",
		},
	}

	sensitivePaths := []string{
		"database.password",
		"secrets[*].value",
		"config.private_key",
	}

	// Make a copy to encrypt
	data := deepCopyMap(original)

	// Encrypt
	err = handler.EncryptSensitiveFields(data, sensitivePaths, testResourceID)
	require.NoError(t, err)

	// Verify sensitive fields are encrypted
	dbPassword := data["database"].(map[string]any)["password"]
	_, isEncrypted := dbPassword.(map[string]any)
	require.True(t, isEncrypted, "database.password should be encrypted")

	secrets := data["secrets"].([]any)
	for i, s := range secrets {
		secret := s.(map[string]any)
		_, valueEncrypted := secret["value"].(map[string]any)
		require.True(t, valueEncrypted, "secrets[%d].value should be encrypted", i)
	}

	configPrivateKey := data["config"].(map[string]any)["private_key"]
	_, isEncrypted = configPrivateKey.(map[string]any)
	require.True(t, isEncrypted, "config.private_key should be encrypted")

	// Verify non-sensitive fields are unchanged
	require.Equal(t, "my-application", data["name"])
	require.Equal(t, "localhost", data["database"].(map[string]any)["host"])
	require.Equal(t, 5432, data["database"].(map[string]any)["port"])
	require.Equal(t, "visible", data["config"].(map[string]any)["public_setting"])

	// Decrypt
	err = handler.DecryptSensitiveFields(context.Background(), data, sensitivePaths, testResourceID)
	require.NoError(t, err)

	// Verify values are restored
	require.Equal(t, "db-password-123", data["database"].(map[string]any)["password"])
	require.Equal(t, "api-secret-value", data["secrets"].([]any)[0].(map[string]any)["value"])
	require.Equal(t, "auth-token-value", data["secrets"].([]any)[1].(map[string]any)["value"])
	require.Equal(t, "secret-key-data", data["config"].(map[string]any)["private_key"])
}

func TestSensitiveDataHandler_FromProvider(t *testing.T) {
	ctx := context.Background()

	key, err := GenerateKey()
	require.NoError(t, err)

	provider, err := NewInMemoryKeyProvider(key)
	require.NoError(t, err)

	handler, err := NewSensitiveDataHandlerFromProvider(ctx, provider)
	require.NoError(t, err)
	require.NotNil(t, handler)

	// Test basic functionality
	data := map[string]any{
		"secret": "my-secret",
	}

	err = handler.EncryptSensitiveFields(data, []string{"secret"}, testResourceID)
	require.NoError(t, err)

	_, isEncrypted := data["secret"].(map[string]any)
	require.True(t, isEncrypted)

	err = handler.DecryptSensitiveFields(ctx, data, []string{"secret"}, testResourceID)
	require.NoError(t, err)
	require.Equal(t, "my-secret", data["secret"])
}

func TestSensitiveDataHandler_SpecificIndex(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	handler, err := NewSensitiveDataHandlerFromKey(key)
	require.NoError(t, err)

	data := map[string]any{
		"items": []any{
			map[string]any{"value": "public"},
			map[string]any{"value": "secret"},
			map[string]any{"value": "public2"},
		},
	}

	// Only encrypt second item
	err = handler.EncryptSensitiveFields(data, []string{"items[1].value"}, testResourceID)
	require.NoError(t, err)

	items := data["items"].([]any)

	// First and third should remain strings
	require.Equal(t, "public", items[0].(map[string]any)["value"])
	require.Equal(t, "public2", items[2].(map[string]any)["value"])

	// Second should be encrypted
	_, isEncrypted := items[1].(map[string]any)["value"].(map[string]any)
	require.True(t, isEncrypted)

	// Decrypt
	err = handler.DecryptSensitiveFields(context.Background(), data, []string{"items[1].value"}, testResourceID)
	require.NoError(t, err)

	require.Equal(t, "secret", items[1].(map[string]any)["value"])
}

func TestSensitiveDataHandler_DecryptWithSchema_IntegerRestoration(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	handler, err := NewSensitiveDataHandlerFromKey(key)
	require.NoError(t, err)

	// Data with integer fields inside a sensitive object
	data := map[string]any{
		"name": "test-resource",
		"sensitiveConfig": map[string]any{
			"port":     5432,
			"timeout":  30,
			"password": "secret",
			"enabled":  true,
		},
	}

	// Schema that describes the sensitive field
	schema := map[string]any{
		"properties": map[string]any{
			"sensitiveConfig": map[string]any{
				"type":               "object",
				"x-radius-sensitive": true,
				"properties": map[string]any{
					"port": map[string]any{
						"type": "integer",
					},
					"timeout": map[string]any{
						"type": "integer",
					},
					"password": map[string]any{
						"type": "string",
					},
					"enabled": map[string]any{
						"type": "boolean",
					},
				},
			},
		},
	}

	sensitivePaths := []string{"sensitiveConfig"}

	// Encrypt
	err = handler.EncryptSensitiveFields(data, sensitivePaths, testResourceID)
	require.NoError(t, err)

	// Verify it's encrypted
	_, isEncrypted := data["sensitiveConfig"].(map[string]any)["encrypted"]
	require.True(t, isEncrypted)

	// Decrypt WITH schema
	err = handler.DecryptSensitiveFieldsWithSchema(context.Background(), data, sensitivePaths, testResourceID, schema)
	require.NoError(t, err)

	// Verify types are correctly restored
	config := data["sensitiveConfig"].(map[string]any)

	// Integers should be int64, not float64
	port, ok := config["port"].(int64)
	require.True(t, ok, "port should be int64, got %T", config["port"])
	require.Equal(t, int64(5432), port)

	timeout, ok := config["timeout"].(int64)
	require.True(t, ok, "timeout should be int64, got %T", config["timeout"])
	require.Equal(t, int64(30), timeout)

	// String should remain string
	password, ok := config["password"].(string)
	require.True(t, ok, "password should be string")
	require.Equal(t, "secret", password)

	// Boolean should remain boolean
	enabled, ok := config["enabled"].(bool)
	require.True(t, ok, "enabled should be bool")
	require.True(t, enabled)
}

func TestSensitiveDataHandler_DecryptWithSchema_NestedObjects(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	handler, err := NewSensitiveDataHandlerFromKey(key)
	require.NoError(t, err)

	// Data with nested objects containing integers
	data := map[string]any{
		"credentials": map[string]any{
			"database": map[string]any{
				"host":     "localhost",
				"port":     5432,
				"maxConns": 100,
			},
			"apiKey": "secret-key",
		},
	}

	schema := map[string]any{
		"properties": map[string]any{
			"credentials": map[string]any{
				"type":               "object",
				"x-radius-sensitive": true,
				"properties": map[string]any{
					"database": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"host": map[string]any{
								"type": "string",
							},
							"port": map[string]any{
								"type": "integer",
							},
							"maxConns": map[string]any{
								"type": "integer",
							},
						},
					},
					"apiKey": map[string]any{
						"type": "string",
					},
				},
			},
		},
	}

	sensitivePaths := []string{"credentials"}

	// Encrypt and decrypt with schema
	err = handler.EncryptSensitiveFields(data, sensitivePaths, testResourceID)
	require.NoError(t, err)

	err = handler.DecryptSensitiveFieldsWithSchema(context.Background(), data, sensitivePaths, testResourceID, schema)
	require.NoError(t, err)

	// Verify nested integers are restored
	creds := data["credentials"].(map[string]any)
	db := creds["database"].(map[string]any)

	require.Equal(t, "localhost", db["host"])
	require.Equal(t, int64(5432), db["port"])
	require.Equal(t, int64(100), db["maxConns"])
	require.Equal(t, "secret-key", creds["apiKey"])
}

func TestSensitiveDataHandler_DecryptWithSchema_ArrayWithIntegers(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	handler, err := NewSensitiveDataHandlerFromKey(key)
	require.NoError(t, err)

	// Sensitive object containing an array with integers
	data := map[string]any{
		"config": map[string]any{
			"ports": []any{80, 443, 8080},
			"name":  "my-config",
		},
	}

	schema := map[string]any{
		"properties": map[string]any{
			"config": map[string]any{
				"type":               "object",
				"x-radius-sensitive": true,
				"properties": map[string]any{
					"ports": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "integer",
						},
					},
					"name": map[string]any{
						"type": "string",
					},
				},
			},
		},
	}

	sensitivePaths := []string{"config"}

	err = handler.EncryptSensitiveFields(data, sensitivePaths, testResourceID)
	require.NoError(t, err)

	err = handler.DecryptSensitiveFieldsWithSchema(context.Background(), data, sensitivePaths, testResourceID, schema)
	require.NoError(t, err)

	config := data["config"].(map[string]any)
	ports := config["ports"].([]any)

	require.Len(t, ports, 3)
	require.Equal(t, int64(80), ports[0])
	require.Equal(t, int64(443), ports[1])
	require.Equal(t, int64(8080), ports[2])
	require.Equal(t, "my-config", config["name"])
}

func TestSensitiveDataHandler_DecryptWithoutSchema_NoTypeCoercion(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	handler, err := NewSensitiveDataHandlerFromKey(key)
	require.NoError(t, err)

	// Data with integer fields
	data := map[string]any{
		"sensitiveConfig": map[string]any{
			"port": 5432,
		},
	}

	sensitivePaths := []string{"sensitiveConfig"}

	err = handler.EncryptSensitiveFields(data, sensitivePaths, testResourceID)
	require.NoError(t, err)

	// Decrypt WITHOUT schema
	err = handler.DecryptSensitiveFields(context.Background(), data, sensitivePaths, testResourceID)
	require.NoError(t, err)

	config := data["sensitiveConfig"].(map[string]any)

	// Without schema, integer should be float64 (standard JSON behavior)
	_, isFloat := config["port"].(float64)
	require.True(t, isFloat, "without schema, port should be float64, got %T", config["port"])
}

func TestGetSchemaForPath(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"credentials": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"password": map[string]any{
						"type": "string",
					},
				},
			},
			"secrets": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"value": map[string]any{
							"type": "string",
						},
					},
				},
			},
			"config": map[string]any{
				"type": "object",
				"additionalProperties": map[string]any{
					"type": "string",
				},
			},
		},
	}

	tests := []struct {
		name         string
		path         string
		expectedType string
	}{
		{
			name:         "simple-nested-field",
			path:         "credentials.password",
			expectedType: "string",
		},
		{
			name:         "array-wildcard-nested",
			path:         "secrets[*].value",
			expectedType: "string",
		},
		{
			name:         "map-wildcard",
			path:         "config[*]",
			expectedType: "string",
		},
		{
			name:         "object-field",
			path:         "credentials",
			expectedType: "object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getSchemaForPath(schema, tt.path)
			require.NotNil(t, result)
			require.Equal(t, tt.expectedType, result["type"])
		})
	}
}

// Test versioned key rotation support
func TestSensitiveDataHandler_VersionedKeyRotation(t *testing.T) {
	ctx := context.Background()

	// Generate two different keys
	key1, err := GenerateKey()
	require.NoError(t, err)
	key2, err := GenerateKey()
	require.NoError(t, err)

	// Create a provider with both keys, version 1 is current
	provider, err := NewInMemoryKeyProviderWithVersions(map[int][]byte{1: key1, 2: key2}, 1)
	require.NoError(t, err)

	// Create handler with version 1 as current
	handler1, err := NewSensitiveDataHandlerFromProvider(ctx, provider)
	require.NoError(t, err)

	// Encrypt data with version 1
	data := map[string]any{
		"password": "secret-v1",
	}
	err = handler1.EncryptSensitiveFields(data, []string{"password"}, testResourceID)
	require.NoError(t, err)

	// Verify it's encrypted
	encData := data["password"].(map[string]any)
	require.NotEmpty(t, encData["encrypted"])
	require.Equal(t, float64(1), encData["version"]) // JSON unmarshals to float64

	// Simulate key rotation: set version 2 as current
	err = provider.SetCurrentVersion(2)
	require.NoError(t, err)

	// Create new handler (would happen on pod restart after rotation)
	handler2, err := NewSensitiveDataHandlerFromProvider(ctx, provider)
	require.NoError(t, err)

	// Handler2 should be able to decrypt data encrypted with version 1
	// because it has access to all keys via the provider
	err = handler2.DecryptSensitiveFields(ctx, data, []string{"password"}, testResourceID)
	require.NoError(t, err)
	require.Equal(t, "secret-v1", data["password"])

	// Now encrypt new data with handler2 (should use version 2)
	data2 := map[string]any{
		"password": "secret-v2",
	}
	err = handler2.EncryptSensitiveFields(data2, []string{"password"}, testResourceID)
	require.NoError(t, err)

	encData2 := data2["password"].(map[string]any)
	require.Equal(t, float64(2), encData2["version"]) // Should be version 2

	// Decrypt the version 2 data
	err = handler2.DecryptSensitiveFields(ctx, data2, []string{"password"}, testResourceID)
	require.NoError(t, err)
	require.Equal(t, "secret-v2", data2["password"])
}

func TestSensitiveDataHandler_DecryptWithOldKeyVersion(t *testing.T) {
	ctx := context.Background()

	// Generate keys
	key1, err := GenerateKey()
	require.NoError(t, err)
	key2, err := GenerateKey()
	require.NoError(t, err)
	key3, err := GenerateKey()
	require.NoError(t, err)

	// Create provider with all three keys
	provider, err := NewInMemoryKeyProviderWithVersions(map[int][]byte{1: key1, 2: key2, 3: key3}, 3)
	require.NoError(t, err)

	// Create handlers for each version to encrypt data
	err = provider.SetCurrentVersion(1)
	require.NoError(t, err)
	handler1, err := NewSensitiveDataHandlerFromProvider(ctx, provider)
	require.NoError(t, err)

	err = provider.SetCurrentVersion(2)
	require.NoError(t, err)
	handler2, err := NewSensitiveDataHandlerFromProvider(ctx, provider)
	require.NoError(t, err)

	err = provider.SetCurrentVersion(3)
	require.NoError(t, err)
	handler3, err := NewSensitiveDataHandlerFromProvider(ctx, provider)
	require.NoError(t, err)

	// Encrypt data with each version
	dataV1 := map[string]any{"secret": "encrypted-with-v1"}
	dataV2 := map[string]any{"secret": "encrypted-with-v2"}
	dataV3 := map[string]any{"secret": "encrypted-with-v3"}

	err = handler1.EncryptSensitiveFields(dataV1, []string{"secret"}, testResourceID)
	require.NoError(t, err)
	err = handler2.EncryptSensitiveFields(dataV2, []string{"secret"}, testResourceID)
	require.NoError(t, err)
	err = handler3.EncryptSensitiveFields(dataV3, []string{"secret"}, testResourceID)
	require.NoError(t, err)

	// Using handler3 (current), should be able to decrypt all versions
	err = handler3.DecryptSensitiveFields(ctx, dataV1, []string{"secret"}, testResourceID)
	require.NoError(t, err)
	require.Equal(t, "encrypted-with-v1", dataV1["secret"])

	err = handler3.DecryptSensitiveFields(ctx, dataV2, []string{"secret"}, testResourceID)
	require.NoError(t, err)
	require.Equal(t, "encrypted-with-v2", dataV2["secret"])

	err = handler3.DecryptSensitiveFields(ctx, dataV3, []string{"secret"}, testResourceID)
	require.NoError(t, err)
	require.Equal(t, "encrypted-with-v3", dataV3["secret"])
}

func TestSensitiveDataHandler_DecryptWithMissingKeyVersion(t *testing.T) {
	ctx := context.Background()

	// Generate keys
	key1, err := GenerateKey()
	require.NoError(t, err)
	key2, err := GenerateKey()
	require.NoError(t, err)

	// Create provider with only key version 2 (simulates key 1 was removed after grace period)
	provider, err := NewInMemoryKeyProviderWithVersions(map[int][]byte{2: key2}, 2)
	require.NoError(t, err)

	// Create handler with only version 2
	handler, err := NewSensitiveDataHandlerFromProvider(ctx, provider)
	require.NoError(t, err)

	// Create data that was encrypted with version 1 (using a separate handler)
	tempProvider, _ := NewInMemoryKeyProviderWithVersions(map[int][]byte{1: key1}, 1)
	tempHandler, _ := NewSensitiveDataHandlerFromProvider(ctx, tempProvider)

	dataV1 := map[string]any{"secret": "old-secret"}
	err = tempHandler.EncryptSensitiveFields(dataV1, []string{"secret"}, testResourceID)
	require.NoError(t, err)

	// Trying to decrypt with handler that doesn't have version 1 should fail
	err = handler.DecryptSensitiveFields(ctx, dataV1, []string{"secret"}, testResourceID)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrFieldDecryptionFailed)
	require.Contains(t, err.Error(), "key version not found")
}

// Helper function to deep copy a map for testing
func deepCopyMap(original map[string]any) map[string]any {
	result := make(map[string]any)
	for key, value := range original {
		switch v := value.(type) {
		case map[string]any:
			result[key] = deepCopyMap(v)
		case []any:
			result[key] = deepCopySlice(v)
		default:
			result[key] = value
		}
	}
	return result
}

func deepCopySlice(original []any) []any {
	result := make([]any, len(original))
	for i, value := range original {
		switch v := value.(type) {
		case map[string]any:
			result[i] = deepCopyMap(v)
		case []any:
			result[i] = deepCopySlice(v)
		default:
			result[i] = value
		}
	}
	return result
}
