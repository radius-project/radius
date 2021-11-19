// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cli

// FriendlyError is a type to use in the CLI codebase for errors that should be shown
// directly for the user. Use this for error conditions that are "expected" like file
// conflicts or missing data.
type FriendlyError struct {
	Message string
}

func (fe *FriendlyError) Error() string {
	return fe.Message
}

func (fe *FriendlyError) Is(target error) bool {
	_, ok := target.(*FriendlyError)
	return ok
}
