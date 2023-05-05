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

package workspaces

var _ error = (*NamedWorkspaceRequiredError)(nil)

// ErrNamedWorkspaceRequired is a value of NamedWorkspaceRequiredError.
var ErrNamedWorkspaceRequired error = &NamedWorkspaceRequiredError{}

// NamedWorkspaceRequiredError is an error used when a named workspace must be specified by the user.
type NamedWorkspaceRequiredError struct {
}

// # Function Explanation
// 
//	NamedWorkspaceRequiredError's Error() function returns a string explaining that a named workspace is required for this 
//	operation and how to specify it. This error is intended to be used by callers of the function to understand why the 
//	operation failed.
func (*NamedWorkspaceRequiredError) Error() string {
	return "This operation requires a named workspace. Specify a named workspace using the `--workspace` command line flag."
}

var _ error = (*EditableWorkspaceRequiredError)(nil)

// ErrNamedWorkspaceRequired is a value of EditableWorkspaceRequiredError.
var ErrEditableWorkspaceRequired error = &EditableWorkspaceRequiredError{}

// EditableWorkspaceRequiredError is an error used when an editable workspace must be specified by the user.
type EditableWorkspaceRequiredError struct {
}

// # Function Explanation
// 
//	The EditableWorkspaceRequiredError function returns an error message that informs the caller that a workspace is 
//	required for the operation. It provides instructions on how to scaffold a workspace in the local directory or how to 
//	specify a named workspace using the command line flag.
func (*EditableWorkspaceRequiredError) Error() string {
	return "This operation requires a workspace. Use `rad init` to scaffold a workspace in the local directory, or specify a named workspace using the `--workspace` command line flag."
}
