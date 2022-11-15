// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import "strings"

// MakeResourceName builds resource name based on name with prefix for azure resources.
func MakeResourceName(name string, prefix ...string) string {
	var sb strings.Builder

	for _, p := range prefix {
		_, _ = sb.WriteString(strings.ToLower(p))
		_, _ = sb.WriteString("-")
	}
	_, _ = sb.WriteString(strings.ToLower(name))
	return sb.String()
}
