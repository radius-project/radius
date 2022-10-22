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

// GetStringProperty gets value for key in collection.
func GetStringProperty(collection any, key string) (string, error) {
	switch c := collection.(type) {
	case map[string]string:
		val, ok := c[key]
		if !ok {
			return "", fmt.Errorf("%s not found", key)
		}
		return val, nil
	case map[string]any:
		val, ok := c[key]
		if !ok {
			return "", fmt.Errorf("%s not found", key)
		}
		s, ok := val.(string)
		if !ok {
			return "", errors.New("value is not string type")
		}
		return s, nil
	}
	return "", errors.New("unsupported type")
}
