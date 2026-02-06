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
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var (
	// ErrFieldNotFound is returned when a field path cannot be found in the data.
	ErrFieldNotFound = errors.New("field not found")

	// ErrInvalidFieldPath is returned when a field path is invalid.
	ErrInvalidFieldPath = errors.New("invalid field path")

	// ErrFieldEncryptionFailed is returned when encryption of a field fails.
	ErrFieldEncryptionFailed = errors.New("field encryption failed")

	// ErrFieldDecryptionFailed is returned when decryption of a field fails.
	ErrFieldDecryptionFailed = errors.New("field decryption failed")

	// ErrFieldRedactionFailed is returned when redaction of a field fails.
	ErrFieldRedactionFailed = errors.New("field redaction failed")
)

// SensitiveDataHandler provides methods for encrypting and decrypting sensitive fields
// in data structures based on field paths marked with x-radius-sensitive annotation.
type SensitiveDataHandler struct {
	encryptor   *Encryptor
	keyProvider KeyProvider
}

// NewSensitiveDataHandler creates a new SensitiveDataHandler with the provided encryptor.
// Note: This constructor does not support versioned key rotation for decryption.
// Use NewSensitiveDataHandlerFromProvider for full versioned key support.
func NewSensitiveDataHandler(encryptor *Encryptor) *SensitiveDataHandler {
	return &SensitiveDataHandler{encryptor: encryptor}
}

// NewSensitiveDataHandlerFromKey creates a new SensitiveDataHandler from a raw encryption key.
// Note: This constructor does not support versioned key rotation for decryption.
// Use NewSensitiveDataHandlerFromProvider for full versioned key support.
func NewSensitiveDataHandlerFromKey(key []byte) (*SensitiveDataHandler, error) {
	encryptor, err := NewEncryptor(key)
	if err != nil {
		return nil, err
	}
	return &SensitiveDataHandler{encryptor: encryptor}, nil
}

// NewSensitiveDataHandlerFromProvider creates a new SensitiveDataHandler using a versioned key provider.
// This is the recommended constructor as it supports key rotation:
// - Encryption uses the current key version
// - Decryption reads the version from encrypted data and fetches the appropriate key
func NewSensitiveDataHandlerFromProvider(ctx context.Context, provider KeyProvider) (*SensitiveDataHandler, error) {
	key, version, err := provider.GetCurrentKey(ctx)
	if err != nil {
		return nil, err
	}
	encryptor, err := NewEncryptorWithVersion(key, version)
	if err != nil {
		return nil, err
	}
	return &SensitiveDataHandler{
		encryptor:   encryptor,
		keyProvider: provider,
	}, nil
}

// EncryptSensitiveFields encrypts all sensitive fields in the data based on the provided field paths.
// The data is modified in place. Field paths support dot notation and [*] for arrays/maps.
// Examples: "credentials.password", "secrets[*].value", "config[*]"
//
// The resourceID is used as Associated Data (AD) for context binding. This prevents encrypted
// values from being moved between different resources. The resourceID should be the full
// resource ID (e.g., "/planes/radius/local/resourceGroups/test/providers/Foo.Bar/myResources/test").
//
// Returns an error if any field encryption fails. In case of error, partial encryption may have occurred.
// Fields that are not found are skipped - this allows optional sensitive fields to be absent.
func (h *SensitiveDataHandler) EncryptSensitiveFields(data map[string]any, sensitiveFieldPaths []string, resourceID string) error {
	for _, path := range sensitiveFieldPaths {
		// Build associated data from resource ID and field path
		ad := buildAssociatedData(resourceID, path)
		if err := h.encryptFieldAtPath(data, path, ad); err != nil {
			// Skip fields that are not found - they may not exist in this resource instance
			// (e.g., optional sensitive properties)
			if errors.Is(err, ErrFieldNotFound) {
				continue
			}
			return fmt.Errorf("%w: path %q: %v", ErrFieldEncryptionFailed, path, err)
		}
	}
	return nil
}

// DecryptSensitiveFields decrypts all sensitive fields in the data based on the provided field paths.
// The data is modified in place. Field paths support dot notation and [*] for arrays/maps.
//
// The resourceID must match what was provided during encryption for successful decryption.
// The context is used to fetch versioned keys from the key provider when needed.
//
// Note: This method does not use schema information for type restoration. Numbers in decrypted
// objects will be returned as float64 (standard Go JSON behavior). For accurate type restoration,
// use DecryptSensitiveFieldsWithSchema instead.
//
// Returns an error if any field decryption fails. In case of error, partial decryption may have occurred.
func (h *SensitiveDataHandler) DecryptSensitiveFields(ctx context.Context, data map[string]any, sensitiveFieldPaths []string, resourceID string) error {
	for _, path := range sensitiveFieldPaths {
		ad := buildAssociatedData(resourceID, path)
		if err := h.decryptFieldAtPath(ctx, data, path, nil, ad); err != nil {
			// Skip fields that are not found - they may not exist in this resource instance
			if errors.Is(err, ErrFieldNotFound) {
				continue
			}
			return fmt.Errorf("%w: path %q: %v", ErrFieldDecryptionFailed, path, err)
		}
	}
	return nil
}

// DecryptSensitiveFieldsWithSchema decrypts all sensitive fields in the data using schema information
// for accurate type restoration. The schema should be the OpenAPI schema for the resource type.
// The data is modified in place. Field paths support dot notation and [*] for arrays/maps.
//
// The resourceID must match what was provided during encryption for successful decryption.
// The context is used to fetch versioned keys from the key provider when needed.
// The schema is used to restore the correct types for fields within encrypted objects (e.g., integers
// that would otherwise be decoded as float64).
//
// Returns an error if any field decryption fails. In case of error, partial decryption may have occurred.
func (h *SensitiveDataHandler) DecryptSensitiveFieldsWithSchema(ctx context.Context, data map[string]any, sensitiveFieldPaths []string, resourceID string, schema map[string]any) error {
	for _, path := range sensitiveFieldPaths {
		// Get the schema for this specific field path
		fieldSchema := getSchemaForPath(schema, path)
		ad := buildAssociatedData(resourceID, path)
		if err := h.decryptFieldAtPath(ctx, data, path, fieldSchema, ad); err != nil {
			// Skip fields that are not found - they may not exist in this resource instance
			if errors.Is(err, ErrFieldNotFound) {
				continue
			}
			return fmt.Errorf("%w: path %q: %v", ErrFieldDecryptionFailed, path, err)
		}
	}
	return nil
}

// RedactSensitiveFields nullifies all sensitive fields in the data based on the provided field paths.
// The data is modified in place. Field paths support dot notation and [*] for arrays/maps.
//
// Fields that are not found are skipped - this allows optional sensitive fields to be absent.
func (h *SensitiveDataHandler) RedactSensitiveFields(data map[string]any, sensitiveFieldPaths []string) error {
	for _, path := range sensitiveFieldPaths {
		if err := h.redactFieldAtPath(data, path); err != nil {
			// Skip fields that are not found - they may not exist in this resource instance
			if errors.Is(err, ErrFieldNotFound) {
				continue
			}
			return fmt.Errorf("%w: path %q: %v", ErrFieldRedactionFailed, path, err)
		}
	}
	return nil
}

// getEncryptorForDecryption returns the appropriate encryptor for decrypting data.
// If a keyProvider is available and the data contains a version, it fetches the versioned key.
// Otherwise, it falls back to the default encryptor.
func (h *SensitiveDataHandler) getEncryptorForDecryption(ctx context.Context, encryptedJSON []byte) (*Encryptor, error) {
	// If no key provider, use the default encryptor
	if h.keyProvider == nil {
		return h.encryptor, nil
	}

	// Extract the version from the encrypted data
	version, err := GetEncryptedDataVersion(encryptedJSON)
	if err != nil {
		return nil, err
	}

	// If version is 0 (unversioned/legacy data), use the default encryptor
	if version == 0 {
		return h.encryptor, nil
	}

	// Fetch the key for this specific version
	key, err := h.keyProvider.GetKeyByVersion(ctx, version)
	if err != nil {
		return nil, fmt.Errorf("failed to get key for version %d: %w", version, err)
	}

	return NewEncryptorWithVersion(key, version)
}

// encryptFieldAtPath encrypts the value at the given field path in the data.
func (h *SensitiveDataHandler) encryptFieldAtPath(data map[string]any, path string, associatedData []byte) error {
	processor := func(value any) (any, error) {
		return h.encryptValue(value, associatedData)
	}
	return h.processFieldAtPath(data, path, processor)
}

// decryptFieldAtPath decrypts the value at the given field path in the data.
// If fieldSchema is provided, it will be used for type restoration.
func (h *SensitiveDataHandler) decryptFieldAtPath(ctx context.Context, data map[string]any, path string, fieldSchema map[string]any, associatedData []byte) error {
	processor := func(value any) (any, error) {
		return h.decryptValue(ctx, value, fieldSchema, associatedData)
	}
	return h.processFieldAtPath(data, path, processor)
}

// redactFieldAtPath replaces the value at the given field path with nil.
func (h *SensitiveDataHandler) redactFieldAtPath(data map[string]any, path string) error {
	processor := func(any) (any, error) {
		return nil, nil
	}
	return h.processFieldAtPath(data, path, processor)
}

// processFieldAtPath traverses the data structure and applies the processor function to the field at the path.
func (h *SensitiveDataHandler) processFieldAtPath(data map[string]any, path string, processor func(any) (any, error)) error {
	if path == "" {
		return ErrInvalidFieldPath
	}

	segments := parseFieldPath(path)
	if len(segments) == 0 {
		return ErrInvalidFieldPath
	}

	return h.processPathSegments(data, segments, processor)
}

// processPathSegments recursively processes path segments to find and transform the target field.
func (h *SensitiveDataHandler) processPathSegments(current any, segments []pathSegment, processor func(any) (any, error)) error {
	if len(segments) == 0 {
		return nil
	}

	segment := segments[0]
	remainingSegments := segments[1:]

	switch segment.segmentType {
	case segmentTypeField:
		return h.processFieldSegment(current, segment.value, remainingSegments, processor)
	case segmentTypeWildcard:
		return h.processWildcardSegment(current, remainingSegments, processor)
	case segmentTypeIndex:
		return h.processIndexSegment(current, segment.value, remainingSegments, processor)
	default:
		return ErrInvalidFieldPath
	}
}

// processFieldSegment handles a regular field name segment in the path.
func (h *SensitiveDataHandler) processFieldSegment(current any, fieldName string, remainingSegments []pathSegment, processor func(any) (any, error)) error {
	dataMap, ok := current.(map[string]any)
	if !ok {
		return ErrFieldNotFound
	}

	value, exists := dataMap[fieldName]
	if !exists {
		return ErrFieldNotFound
	}

	// If this is the last segment, process the value
	if len(remainingSegments) == 0 {
		processed, err := processor(value)
		if err != nil {
			return err
		}
		dataMap[fieldName] = processed
		return nil
	}

	// Continue traversing
	return h.processPathSegments(value, remainingSegments, processor)
}

// processWildcardSegment handles [*] segments for arrays and maps.
func (h *SensitiveDataHandler) processWildcardSegment(current any, remainingSegments []pathSegment, processor func(any) (any, error)) error {
	// Handle array
	if arr, ok := current.([]any); ok {
		for i := range arr {
			if len(remainingSegments) == 0 {
				// Process each array element
				processed, err := processor(arr[i])
				if err != nil {
					return fmt.Errorf("index %d: %w", i, err)
				}
				arr[i] = processed
			} else {
				// Continue traversing into each element
				if err := h.processPathSegments(arr[i], remainingSegments, processor); err != nil {
					// Skip elements that don't have the field
					if !errors.Is(err, ErrFieldNotFound) {
						return fmt.Errorf("index %d: %w", i, err)
					}
				}
			}
		}
		return nil
	}

	// Handle map
	if dataMap, ok := current.(map[string]any); ok {
		for key := range dataMap {
			if len(remainingSegments) == 0 {
				// Process each map value
				processed, err := processor(dataMap[key])
				if err != nil {
					return fmt.Errorf("key %q: %w", key, err)
				}
				dataMap[key] = processed
			} else {
				// Continue traversing into each value
				if err := h.processPathSegments(dataMap[key], remainingSegments, processor); err != nil {
					// Skip elements that don't have the field
					if !errors.Is(err, ErrFieldNotFound) {
						return fmt.Errorf("key %q: %w", key, err)
					}
				}
			}
		}
		return nil
	}

	return ErrFieldNotFound
}

// processIndexSegment handles specific index segments like [0], [1], etc.
func (h *SensitiveDataHandler) processIndexSegment(current any, indexStr string, remainingSegments []pathSegment, processor func(any) (any, error)) error {
	arr, ok := current.([]any)
	if !ok {
		return ErrFieldNotFound
	}

	index, err := strconv.Atoi(indexStr)
	if err != nil {
		return fmt.Errorf("%w: invalid index %q", ErrInvalidFieldPath, indexStr)
	}

	if index < 0 || index >= len(arr) {
		return ErrFieldNotFound
	}

	if len(remainingSegments) == 0 {
		processed, err := processor(arr[index])
		if err != nil {
			return err
		}
		arr[index] = processed
		return nil
	}

	return h.processPathSegments(arr[index], remainingSegments, processor)
}

// encryptValue encrypts a single value, handling different types appropriately.
func (h *SensitiveDataHandler) encryptValue(value any, associatedData []byte) (any, error) {
	if value == nil {
		return nil, nil
	}

	var dataToEncrypt []byte
	var err error

	switch v := value.(type) {
	case string:
		if v == "" {
			return v, nil
		}
		dataToEncrypt = []byte(v)
	case map[string]any, []any:
		dataToEncrypt, err = json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal value: %w", err)
		}
	default:
		// For other types, convert to JSON
		dataToEncrypt, err = json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal value: %w", err)
		}
	}

	encrypted, err := h.encryptor.Encrypt(dataToEncrypt, associatedData)
	if err != nil {
		return nil, err
	}

	// Return as a structured object
	var result map[string]any
	if err := json.Unmarshal(encrypted, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// decryptValue decrypts a single value, restoring the original type using schema information if provided.
// If a keyProvider is available and the encrypted data contains a version, it will fetch the appropriate key.
func (h *SensitiveDataHandler) decryptValue(ctx context.Context, value any, fieldSchema map[string]any, associatedData []byte) (any, error) {
	if value == nil {
		return nil, nil
	}

	// Check if this looks like encrypted data
	encMap, ok := value.(map[string]any)
	if !ok {
		// Not encrypted data, return as-is
		return value, nil
	}

	_, hasEncrypted := encMap["encrypted"].(string)
	_, hasNonce := encMap["nonce"].(string)

	if !hasEncrypted || !hasNonce {
		// Not our encrypted format, return as-is
		return value, nil
	}

	// Convert back to JSON for decryption
	encryptedJSON, err := json.Marshal(encMap)
	if err != nil {
		return nil, err
	}

	// Get the appropriate encryptor based on the key version in the encrypted data
	encryptor, err := h.getEncryptorForDecryption(ctx, encryptedJSON)
	if err != nil {
		return nil, err
	}

	decrypted, err := encryptor.Decrypt(encryptedJSON, associatedData)
	if err != nil {
		return nil, err
	}

	// Determine the expected type from schema
	expectedType := getSchemaType(fieldSchema)

	// For string type, return as string directly
	if expectedType == "string" {
		return string(decrypted), nil
	}

	// For object/array types, unmarshal and apply type coercion based on schema
	var result any
	if err := json.Unmarshal(decrypted, &result); err != nil {
		// If not valid JSON, return as string
		return string(decrypted), nil
	}

	// Apply schema-based type coercion if schema is available
	if fieldSchema != nil && expectedType == "object" {
		if resultMap, ok := result.(map[string]any); ok {
			coerceTypesFromSchema(resultMap, fieldSchema)
		}
	}

	return result, nil
}

// buildAssociatedData constructs the associated data for AEAD encryption from the resource ID and field path.
// This binds the ciphertext to its context, preventing encrypted values from being moved between
// different resources or fields.
func buildAssociatedData(resourceID, fieldPath string) []byte {
	if resourceID == "" && fieldPath == "" {
		return nil
	}
	// Combine resource ID and field path with a separator
	// Format: "resourceID:fieldPath"
	return []byte(resourceID + ":" + fieldPath)
}

// getSchemaType returns the type from a schema, or empty string if not specified.
func getSchemaType(schema map[string]any) string {
	if schema == nil {
		return ""
	}
	if t, ok := schema["type"].(string); ok {
		return t
	}
	return ""
}

// getSchemaForPath retrieves the schema definition for a specific field path.
// It navigates through the schema following the path segments (supporting nested properties,
// array items via [*], and additionalProperties for maps).
func getSchemaForPath(schema map[string]any, path string) map[string]any {
	if schema == nil || path == "" {
		return nil
	}

	segments := parseFieldPath(path)
	current := schema

	for _, segment := range segments {
		switch segment.segmentType {
		case segmentTypeField:
			// Navigate to properties -> fieldName
			properties, ok := current["properties"].(map[string]any)
			if !ok {
				return nil
			}
			fieldSchema, ok := properties[segment.value].(map[string]any)
			if !ok {
				return nil
			}
			current = fieldSchema

		case segmentTypeWildcard:
			// Could be array items or additionalProperties
			if items, ok := current["items"].(map[string]any); ok {
				current = items
			} else if addProps, ok := current["additionalProperties"].(map[string]any); ok {
				current = addProps
			} else {
				return nil
			}

		case segmentTypeIndex:
			// Specific array index - use items schema
			if items, ok := current["items"].(map[string]any); ok {
				current = items
			} else {
				return nil
			}
		}
	}

	return current
}

// coerceTypesFromSchema recursively walks through a data map and coerces types
// to match the schema definition. This primarily handles converting float64 to int64
// for integer fields.
func coerceTypesFromSchema(data map[string]any, schema map[string]any) {
	if schema == nil {
		return
	}

	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return
	}

	for fieldName, fieldValue := range data {
		fieldSchema, ok := properties[fieldName].(map[string]any)
		if !ok {
			continue
		}

		fieldType := getSchemaType(fieldSchema)

		switch fieldType {
		case "integer":
			// Coerce float64 to int64
			if f, ok := fieldValue.(float64); ok {
				data[fieldName] = int64(f)
			}

		case "object":
			// Recursively coerce nested objects
			if nestedMap, ok := fieldValue.(map[string]any); ok {
				coerceTypesFromSchema(nestedMap, fieldSchema)
			}

		case "array":
			// Coerce array items if they have a schema
			if arr, ok := fieldValue.([]any); ok {
				itemSchema, _ := fieldSchema["items"].(map[string]any)
				if itemSchema != nil {
					itemType := getSchemaType(itemSchema)
					for i, item := range arr {
						if itemType == "integer" {
							if f, ok := item.(float64); ok {
								arr[i] = int64(f)
							}
						} else if itemType == "object" {
							if itemMap, ok := item.(map[string]any); ok {
								coerceTypesFromSchema(itemMap, itemSchema)
							}
						}
					}
				}
			}
		}

		// Handle additionalProperties for map types
		if addPropsSchema, ok := fieldSchema["additionalProperties"].(map[string]any); ok {
			if nestedMap, ok := fieldValue.(map[string]any); ok {
				addPropsType := getSchemaType(addPropsSchema)
				for key, val := range nestedMap {
					if addPropsType == "integer" {
						if f, ok := val.(float64); ok {
							nestedMap[key] = int64(f)
						}
					} else if addPropsType == "object" {
						if valMap, ok := val.(map[string]any); ok {
							coerceTypesFromSchema(valMap, addPropsSchema)
						}
					}
				}
			}
		}
	}
}

// pathSegment represents a segment of a field path.
type pathSegment struct {
	segmentType segmentType
	value       string
}

type segmentType int

const (
	segmentTypeField segmentType = iota
	segmentTypeWildcard
	segmentTypeIndex
)

// parseFieldPath parses a field path into segments.
// Examples:
//   - "credentials.password" -> [field:credentials, field:password]
//   - "secrets[*].value" -> [field:secrets, wildcard, field:value]
//   - "config[*]" -> [field:config, wildcard]
//   - "items[0].name" -> [field:items, index:0, field:name]
func parseFieldPath(path string) []pathSegment {
	var segments []pathSegment
	var current strings.Builder

	i := 0
	for i < len(path) {
		ch := path[i]

		switch ch {
		case '.':
			if current.Len() > 0 {
				segments = append(segments, pathSegment{segmentType: segmentTypeField, value: current.String()})
				current.Reset()
			}
			i++

		case '[':
			if current.Len() > 0 {
				segments = append(segments, pathSegment{segmentType: segmentTypeField, value: current.String()})
				current.Reset()
			}

			// Find the closing bracket
			end := strings.Index(path[i:], "]")
			if end == -1 {
				// Invalid path - unterminated bracket, return nil to signal error
				return nil
			}

			bracketContent := path[i+1 : i+end]
			if bracketContent == "*" {
				segments = append(segments, pathSegment{segmentType: segmentTypeWildcard})
			} else {
				segments = append(segments, pathSegment{segmentType: segmentTypeIndex, value: bracketContent})
			}
			i += end + 1

		default:
			current.WriteByte(ch)
			i++
		}
	}

	// Don't forget the last segment
	if current.Len() > 0 {
		segments = append(segments, pathSegment{segmentType: segmentTypeField, value: current.String()})
	}

	return segments
}
