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

func (r *rp) ListApplications(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	return nil, errors.New("not implemented")
}

func (r *rp) GetApplication(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	return nil, errors.New("not implemented")
}

func (r *rp) UpdateApplication(ctx context.Context, id azresources.ResourceID, body []byte) (rest.Response, error) {
	return nil, errors.New("not implemented")
}

func (r *rp) DeleteApplication(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	return nil, errors.New("not implemented")
}

func (r *rp) ListResources(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	return nil, errors.New("not implemented")
}

func (r *rp) GetResource(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	return nil, errors.New("not implemented")
}

func (r *rp) UpdateResource(ctx context.Context, id azresources.ResourceID, body []byte) (rest.Response, error) {
	return nil, errors.New("not implemented")
}

func (r *rp) DeleteResource(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	return nil, errors.New("not implemented")
}

func (r *rp) GetOperation(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
	return nil, errors.New("not implemented")
}
