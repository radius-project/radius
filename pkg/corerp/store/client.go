// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package store

import (
	"context"
)

type StorageClient interface {
	Query(ctx context.Context, query Query, options ...QueryOptions) ([]Object, error)
	Get(ctx context.Context, id string, options ...GetOptions) (*Object, error)
	Delete(ctx context.Context, id string, options ...DeleteOptions) error
	Save(ctx context.Context, obj *Object, options ...SaveOptions) error
}

// Query specifies the structure of a query. RootScope is required and other fields are optional.
type Query struct {
	// Scope sets the root scope of the query. This will be the fully-qualified root scope. This can be a
	// UCP scope ('/planes/...') or an ARM scope as long as the data-store is self-consistent.
	//
	// Example:
	//	/planes/radius/local/resourceGroups/cool-group/
	RootScope string

	// ScopeRecursive determines whether the root scope is applied recursively.
	//
	// Example: If 'true' the following value of Scope would match all of the provided root scopes.
	//	/planes/radius/local/ ->
	//		/planes/radius/local/
	//		/planes/radius/local/resourceGroups/cool-group
	//		/planes/radius/local/resourceGroups/cool-group2
	ScopeRecursive bool

	// ResourceType is the optional resource type used to filter the query. ResourceType must be a fully-qualified
	// type if it is provided.
	//
	// Example:
	//	Applications.Core/applications
	ResourceType string

	// RoutingScopePrefix is the optional routing scope used to filter the query. RoutingScopePrefix should be the prefix
	// of the desired resources (types and names). RoutingScopePrefix should have a resource name as its last segment
	// not a type.
	//
	// Example:
	//	/Applications.Core/applications/my-app/
	RoutingScopePrefix string
}

type QueryOptions interface {
}

type GetOptions interface {
}

type DeleteOptions interface {
}

type SaveOptions interface {
}
