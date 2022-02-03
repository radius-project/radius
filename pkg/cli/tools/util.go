// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package tools

import "runtime"

func GetExecutableName(tool string) string {
	switch runtime.GOOS {
	case "windows":
		return tool + ".exe"
	default:
		return tool
	}
}
