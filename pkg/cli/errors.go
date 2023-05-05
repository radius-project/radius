// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cli

import (
	"fmt"
)

// FriendlyError is a type to use in the CLI codebase for errors that should be shown
// directly for the user. Use this for error conditions that are "expected" like file
// conflicts or missing data.
type FriendlyError struct {
	Message string
}

// # Function Explanation
// 
//	FriendlyError is a custom error type that allows callers to provide a custom error message to be returned when the error
//	 is raised. It implements the Error() method, which returns the custom error message provided when the error was 
//	created.
func (fe *FriendlyError) Error() string {
	return fe.Message
}

// # Function Explanation
// 
//	FriendlyError's Is() function compares the Message of the target error to the Message of the FriendlyError instance, and
//	 returns true if they are equal. This allows callers to check if a given error is a FriendlyError with a specific 
//	message.
func (fe *FriendlyError) Is(target error) bool {
	e, ok := target.(*FriendlyError)
	return ok && fe.Message == e.Message
}

// ClusterUnreachableError is an error type to be thrown when the kubernetes cluster
// is unreachable. The cluster could be gone, or we don't have access.
type ClusterUnreachableError struct {
	Err error
}

// # Function Explanation
// 
//	ClusterUnreachableError's Is() method checks if the given error is of type ClusterUnreachableError, and returns a 
//	boolean value indicating the result, allowing callers to handle the error accordingly.
func (e *ClusterUnreachableError) Is(target error) bool {
	_, ok := target.(*ClusterUnreachableError)
	return ok
}

// # Function Explanation
// 
//	ClusterUnreachableError's Error() function returns a formatted string describing an error encountered when trying to 
//	reach a Kubernetes cluster, which can be useful for callers of this function.
func (e *ClusterUnreachableError) Error() string {
	return fmt.Sprintf("kubernetes cluster unreachable: %s", e.Err.Error())
}
