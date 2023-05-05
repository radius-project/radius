// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package tools

var _ error = (ErrToolNotFound)(ErrToolNotFound{})

type ErrToolNotFound struct {
	Tool    string
	Message string
}

// # Function Explanation
// 
//	ErrToolNotFound is an error type that is returned by the function GetTool when the requested tool is not found. It 
//	contains a message that can be used to inform the caller of the error.
func (e ErrToolNotFound) Error() string {
	return e.Message
}
