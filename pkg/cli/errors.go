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
	e, ok := target.(*FriendlyError)
	return ok && fe.Message == e.Message
}

// ClusterUnreachableError is an error type to be thrown when the kubernetes cluster
// is unreachable. The cluster could be gone, or we don't have access.
type ClusterUnreachableError struct {
	Err error
}

func (e *ClusterUnreachableError) Is(target error) bool {
	_, ok := target.(*ClusterUnreachableError)
	return ok
}

func (e *ClusterUnreachableError) Error() string {
	return fmt.Sprintf("kubernetes cluster unreachable: %s", e.Err.Error())
}
