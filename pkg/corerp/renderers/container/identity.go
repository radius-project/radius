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

package container

// RoleAssignmentData describes how to configure role assignment permissions based on the kind of
// connection.
type RoleAssignmentData struct {
	// RoleNames contains the names of the IAM roles to grant.
	RoleNames []string

	// LocalID contains the LocalID of an output resource that can be resolved to find the underlying
	// cloud resource.
	LocalID string
}
