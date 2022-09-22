// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resources

import "strings"

func ToARN(id ID) string {
	parts := []string{}
	// AWS ARNs use :: as separator.
	for _, segment := range id.TypeSegments() {
		parts = append(parts, strings.ReplaceAll(strings.ReplaceAll(segment.Type, ".", "::"), "/", "::"))
	}
	arn := strings.Join(parts, "::")
	return arn
}
