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

package util

import (
	"strings"
	"unicode"
)

// NormalizeStringToLower takes in a string and returns a normalized version of the string with all non-alphanumeric
// characters replaced with hyphens and all letters converted to lowercase.
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
