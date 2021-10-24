// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package iam

// RoleAssignmentData describes how to configure role assignment permissions based on the kind of
// connection.
type RoleAssignmentData struct {
	// RoleNames contains the names of the IAM roles to grant.
	RoleNames []string

	// LocalID contains the LocalID of an output resource that can be resolved to find the underlying
	// cloud resource.
	LocalID string
}

type RoleAssignmentProvider struct {
	// RoleAssignmentMap is an optional map of connection kind -> []Role Assignment. Used to configure managed
	// identity permissions for cloud resources. This will be nil in environments that don't support role assignments.
	RoleAssignmentMap map[string]RoleAssignmentData
}

func (r RoleAssignmentProvider) IsIdentitySupported(connectionKind string) bool {
	if r.RoleAssignmentMap == nil {
		return false
	}

	_, ok := r.RoleAssignmentMap[connectionKind]
	return ok
}
