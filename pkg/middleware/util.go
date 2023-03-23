// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import "strings"

// GetRelativePath trims the prefix basePath from path
func GetRelativePath(basePath string, path string) string {
	trimmedPath := strings.TrimPrefix(path, basePath)
	return trimmedPath
}
