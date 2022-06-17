// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containerv1alpha3

// RoleAssignmentData describes how to configure role assignment permissions based on the kind of
// connection.
type RoleAssignmentData struct {
	// RoleNames contains the names of the IAM roles to grant.
	RoleNames []string

	// LocalID contains the LocalID of an output resource that can be resolved to find the underlying
	// cloud resource.
	LocalID string
}
