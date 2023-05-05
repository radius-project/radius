// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package tools

import "runtime"

// # Function Explanation
// 
//	GetExecutableName() returns the name of the executable for the given tool, depending on the operating system. It handles
//	 errors by returning the tool name as-is for any operating system other than Windows.
func GetExecutableName(tool string) string {
	switch runtime.GOOS {
	case "windows":
		return tool + ".exe"
	default:
		return tool
	}
}
