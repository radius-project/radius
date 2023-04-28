// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package workspaces

var _ error = (*NamedWorkspaceRequiredError)(nil)

// ErrNamedWorkspaceRequired is a value of NamedWorkspaceRequiredError.
var ErrNamedWorkspaceRequired error = &NamedWorkspaceRequiredError{}

// NamedWorkspaceRequiredError is an error used when a named workspace must be specified by the user.
type NamedWorkspaceRequiredError struct {
}

func (*NamedWorkspaceRequiredError) Error() string {
	return "This operation requires a named workspace. Specify a named workspace using the `--workspace` command line flag."
}

var _ error = (*EditableWorkspaceRequiredError)(nil)

// ErrNamedWorkspaceRequired is a value of EditableWorkspaceRequiredError.
var ErrEditableWorkspaceRequired error = &EditableWorkspaceRequiredError{}

// EditableWorkspaceRequiredError is an error used when an editable workspace must be specified by the user.
type EditableWorkspaceRequiredError struct {
}

func (*EditableWorkspaceRequiredError) Error() string {
	return "This operation requires a workspace. Use `rad init` to scaffold a workspace in the local directory, or specify a named workspace using the `--workspace` command line flag."
}
