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

func (fe *FriendlyError) Error() string {
	return fe.Message
}

func (fe *FriendlyError) Is(target error) bool {
	_, ok := target.(*FriendlyError)
	return ok
}

// ClusterUnreachable is an error type to be thrown when the kubernetes cluster
// is unreachable. The cluster the cluster could gone, or we don't have access.
type ClusterUnreachable struct {
	Err error
}

func (e *ClusterUnreachable) Is(target error) bool {
	_, ok := target.(*ClusterUnreachable)
	return ok
}

func (e *ClusterUnreachable) Error() string {
	return fmt.Sprintf("kubernetes cluster unreachable: %s", e.Err)
}
