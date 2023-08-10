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

package gateway

type ErrFQDNOrPrefixRequired struct {
}

// # Function Explanation
//
// Error returns the error string for when either a prefix or fully qualified hostname is not provided.
func (e *ErrFQDNOrPrefixRequired) Error() string {
	return "must provide either prefix or fullyQualifiedHostname if hostname is specified"
}

// # Function Explanation
//
// Is checks if the target error is of the same type as the ErrFQDNOrPrefixRequired error.
func (e *ErrFQDNOrPrefixRequired) Is(target error) bool {
	_, ok := target.(*ErrFQDNOrPrefixRequired)
	return ok
}

type ErrNoPublicEndpoint struct {
}

// # Function Explanation
//
// Error returns an error string when there is no public endpoint available.
func (e *ErrNoPublicEndpoint) Error() string {
	return "no public endpoint available"
}

// # Function Explanation
//
// Is checks if the target error is of type ErrNoPublicEndpoint.
func (e *ErrNoPublicEndpoint) Is(target error) bool {
	_, ok := target.(*ErrNoPublicEndpoint)
	return ok
}
