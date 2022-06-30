// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package util

import (
	"strings"
	"unicode"
)

// NormalizeStringToLower converts string to safe string by removing non digit and non letter and replace '/' with '-'
func NormalizeStringToLower(s string) string {
	if s == "" {
		return s
	}

	sb := strings.Builder{}
	for _, ch := range s {
		if ch == '/' {
			sb.WriteString("-")
		} else if unicode.IsDigit(ch) || unicode.IsLetter(ch) {
			sb.WriteRune(ch)
		}
	}

	return strings.ToLower(sb.String())
}
