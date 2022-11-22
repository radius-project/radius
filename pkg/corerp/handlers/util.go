// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"errors"
	"fmt"
	"strings"
)

func ValidateResourceIDsForResource(properties map[string]string, keys ...string) error {
	missing := []string{}
	for _, k := range keys {
		_, ok := properties[k]
		if !ok {
			// Surround with single-quotes for formatting later
			missing = append(missing, fmt.Sprintf("'%s'", k))
		}
	}

	if len(missing) == 0 {
		return nil
	}

	return fmt.Errorf("missing required properties %v for resource", strings.Join(missing, ", "))
}

// GetMapValue extracts the value for key from collection.
func GetMapValue[T any](collection any, key string) (T, error) {
	var defaultValue T
	switch c := collection.(type) {
	case map[string]T:
		val, ok := c[key]
		if !ok {
			return defaultValue, fmt.Errorf("%s not found", key)
		}
		return val, nil
	case map[string]any:
		val, ok := c[key]
		if !ok {
			return defaultValue, fmt.Errorf("%s not found", key)
		}
		s, ok := val.(T)
		if !ok {
			return defaultValue, fmt.Errorf("value is not %T type", *new(T))
		}
		return s, nil
	}
	return defaultValue, errors.New("unsupported type")
}
