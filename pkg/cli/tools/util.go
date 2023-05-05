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
