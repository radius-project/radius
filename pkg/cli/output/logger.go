// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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
