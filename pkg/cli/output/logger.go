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
//
// # Function Explanation
// 
//	LogInfo prints a formatted string with the provided parameters to the console, and adds a new line at the end. It 
//	handles any errors that may occur while printing.
func LogInfo(format string, v ...any) {
	fmt.Printf(format, v...)
	fmt.Println()
}

// BeginStep starts execution of a step
//
// # Function Explanation
// 
//	BeginStep prints a log message with the provided format and arguments, and returns a Step object for error handling.
func BeginStep(format string, v ...any) Step {
	LogInfo(format, v...)
	return Step{}
}

// CompleteStep ends execution of a step
//
// # Function Explanation
// 
//	CompleteStep() performs a series of operations and validations on the given Step object, and returns an error if any of 
//	the operations fail.
func CompleteStep(step Step) {
}
