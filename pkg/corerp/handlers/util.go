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
