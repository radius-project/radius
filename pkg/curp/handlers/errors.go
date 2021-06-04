// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"fmt"
	"strings"
)

func ValidateResourceIDsForUnmanagedResource(properties map[string]string, keys ...string) error {
	if properties[ManagedKey] == "true" || properties[ManagedKey] == "" {
		return nil
	}

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

	return fmt.Errorf("missing required properties %v for an unmanaged resource", strings.Join(missing, ", "))
}
