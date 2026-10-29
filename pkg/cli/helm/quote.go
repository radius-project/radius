/*
Copyright 2026 The Radius Authors.

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

package helm

import "strings"

// QuoteFlagValue quotes a flag value for a command that Radius suggests. It uses double quotes, which bash, zsh,
// PowerShell, and cmd.exe all remove without changing the value, unless the value has a character that one of them
// treats specially inside double quotes. bash changes a backslash only before $, `, ", \, or a newline, so a value
// such as C:\charts\contour keeps its double quotes. Other values get single quotes, which only POSIX shells
// understand.
func QuoteFlagValue(value string) string {
	if !strings.ContainsAny(value, "$`\"!\n") && !strings.Contains(value, `\\`) && !strings.HasSuffix(value, `\`) {
		return `"` + value + `"`
	}

	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
