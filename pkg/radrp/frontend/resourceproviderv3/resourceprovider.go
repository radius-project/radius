// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resourceproviderv3

import (
	"context"
	"errors"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/radrp/rest"
)

//go:generate mockgen -destination=./mock_resourceprovider.go -package=resourceproviderv3 -self_package github.com/Azure/radius/pkg/radrp/frontend/resourceproviderv3 github.com/Azure/radius/pkg/radrp/frontend/resourceproviderv3 ResourceProvider

// ResourceProvider defines the business logic of the resource provider for Radius.
type ResourceProvider interface {
	ListApplications(ctx context.Context, id azresources.ResourceID) (rest.Response, error)
	GetApplication(ctx context.Context, id azresources.ResourceID) (rest.Response, error)
	UpdateApplication(ctx context.Context, id azresources.ResourceID, body []byte) (rest.Response, error)
	DeleteApplication(ctx context.Context, id azresources.ResourceID) (rest.Response, error)

	ListResources(ctx context.Context, id azresources.ResourceID) (rest.Response, error)
	GetResource(ctx context.Context, id azresources.ResourceID) (rest.Response, error)
	UpdateResource(ctx context.Context, id azresources.ResourceID, body []byte) (rest.Response, error)
	DeleteResource(ctx context.Context, id azresources.ResourceID) (rest.Response, error)

	GetOperation(ctx context.Context, id azresources.ResourceID) (rest.Response, error)
}

// NewResourceProvider creates a new ResourceProvider.
func NewResourceProvider() ResourceProvider {
	return &rp{}
}

type rp struct {
}

// TODO:
// Define - new database interface or new methods on existing interface
// Define - new database functionality in terms of weakly-type resource representation
//
// Conclusion: resources become strongly-typed when they need to go to a renderer.
// As part of this conversion we need to understand the connections (previously bindings) and pre-fetch their data.
//
// In the database we'll have 3 tables/collections:
// - Applications (v3 version)
// - Resources (children of applications)
// - Operations (same as existing)

func (r *rp) ListApplications(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	// Validate resource type

	// Read data from the database

	// Convert data to wire format

	// Return as rest.Response
	return nil, errors.New("not implemented")
}

func (r *rp) GetApplication(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	// Validate resource type

	// Read data from the database

	// Convert data to wire format

	// Return as rest.Response
	return nil, errors.New("not implemented")
}

func (r *rp) UpdateApplication(ctx context.Context, id azresources.ResourceID, body []byte) (rest.Response, error) {
	// Validate resource type

	// DeserializePayload

	// Convert to database format

	// Update database

	// Convert back to wire format

	// Return as rest.Response

	return nil, errors.New("not implemented")
}

func (r *rp) DeleteApplication(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	// Validate resource type

	// Check for existing & deployed resources & components
	// - if true return conflict

	// Delete from database

	// Return as rest.Response

	return nil, errors.New("not implemented")
}

func (r *rp) ListResources(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	// Validate resource type

	// Read data from the database

	// Convert data to wire format

	// Return as rest.Response
	return nil, errors.New("not implemented")
}

func (r *rp) GetResource(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	// Validate resource type

	// Read data from the database

	// Convert data to wire format

	// Return as rest.Response
	return nil, errors.New("not implemented")
}

func (r *rp) UpdateResource(ctx context.Context, id azresources.ResourceID, body []byte) (rest.Response, error) {
	// Validate resource type

	// DeserializePayload

	// Convert to database format

	// Compare revisions
	// - Is definition the same? return success

	// Update database
	// - Set provisioning state to creating/updating

	// Create async operation & kick off background work

	// Convert back to wire format

	// Return as rest.Response with long-running operation

	return nil, errors.New("not implemented")
}

func (r *rp) DeleteResource(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	// Validate resource type

	// Update database
	// - Set provisioning state to deleting

	// Create async operation & kick off background work

	// Return as rest.Response with long-running operation
	return nil, errors.New("not implemented")
}

func (r *rp) GetOperation(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	// Validate resource type

	// Read data from the database

	// Return as rest.Response
	return nil, errors.New("not implemented")
}
