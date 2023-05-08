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

package output

import "fmt"

// Step represents a logical step that's being executed.
type Step struct {
}

// LogInfo logs a message that is displayed to the user by default.
func LogInfo(format string, v ...any) {
	fmt.Printf(format, v...)
	fmt.Println()
}

// BeginStep starts execution of a step
func BeginStep(format string, v ...any) Step {
	LogInfo(format, v...)
	return Step{}
}

// CompleteStep ends execution of a step
func CompleteStep(step Step) {
}
