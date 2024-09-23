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

package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
)

// jsonPropertyPattern is the pattern for a valid JSON property name.
var jsonPropertyPattern = "[a-zA-Z$_][a-zA-Z0-9$_]*"

// fieldRegex is the regular expression to validate a field path in a query. It matches:
// - A single property OR
// - Multople properties separated by a '.'
var fieldRegex = regexp.MustCompile(fmt.Sprintf(`^(%s)(\.%s)*$`, jsonPropertyPattern, jsonPropertyPattern))

//go:generate mockgen -typed -destination=./mock_storageClient.go -package=store -self_package github.com/radius-project/radius/pkg/ucp/store github.com/radius-project/radius/pkg/ucp/store StorageClient

type StorageClient interface {
	Query(ctx context.Context, query Query, options ...QueryOptions) (*ObjectQueryResult, error)
	Get(ctx context.Context, id string, options ...GetOptions) (*Object, error)
	Delete(ctx context.Context, id string, options ...DeleteOptions) error
	Save(ctx context.Context, obj *Object, options ...SaveOptions) error
}

// Query specifies the structure of a query. RootScope and ResourceType are required and other fields are optional.
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

	// IsScopeQuery is used to determine whether to query scopes (true) or resources (false).
	// Example: To query all "plane"
	// 	set RootScope to /planes and both ScopeRecursive and IsScopeQuery to True.
	// If ScopeQuery is False, we would be querying for resources that match RootScope and other optional
	// query field values.
	// Example: To query all resources in a radius local plane scope
	// 	set RootScope to /planes/radius/local and ScopeRecursive = True and IsScopeQuery to False.
	IsScopeQuery bool

	// TODO: Revisit filter design

	// Filters is an query filter to filter the specific property value.
	Filters []QueryFilter
}

// Validate validates the Query.
func (q Query) Validate() error {
	var err error
	if q.RootScope == "" {
		err = errors.Join(err, &ErrInvalid{Message: "RootScope is required"})
	}

	if q.ResourceType == "" {
		err = errors.Join(err, &ErrInvalid{Message: "ResourceType is required"})
	}

	if q.IsScopeQuery && q.RoutingScopePrefix != "" {
		err = errors.Join(err, &ErrInvalid{Message: "RoutingScopePrefix' is not supported for scope queries"})
	}

	for _, filter := range q.Filters {
		err = errors.Join(filter.Validate())
	}

	return err
}

// QueryFilter is the filter which filters property in resource entity.
type QueryFilter struct {
	// Field specifies the property name to filter.
	//
	// Field can be a simple property name of a '.' separated property path.
	// Examples:
	//	- "location"
	//	- "properties.application"
	Field string

	// Value specifies the value to filter. The value must be a string and will be
	// compared case-insentively with the property value.
	Value string
}

// Validate validates the QueryFilter.
func (f QueryFilter) Validate() error {
	var err error
	if f.Field == "" {
		err = errors.Join(err, &ErrInvalid{Message: fmt.Sprintf("Field is required in filter: %+v", f)})
	}

	if !fieldRegex.Match([]byte(f.Field)) {
		err = errors.Join(err, &ErrInvalid{Message: fmt.Sprintf("Field is invalid in filter: %+v", f)})
	}

	// Value can be blank. If it is blank, the filter will match the empty string in the target property.

	return err
}
