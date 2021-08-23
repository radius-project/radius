// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"

	"github.com/Azure/radius/pkg/radrp/db"
	"github.com/Azure/radius/pkg/radrp/outputresource"
)

type PutOptions struct {
	Application string
	Component   string
	Resource    *outputresource.OutputResource
	Existing    *db.DeploymentResource
}

type DeleteOptions struct {
	Application string
	Component   string
	Existing    db.DeploymentResource
}

// ResourceHandler interface defines the methods that every output resource will implement
//go:generate mockgen -destination=../../../mocks/mockhandlers/mock_resource_handler.go -package=mockhandlers github.com/Azure/radius/pkg/radrp/handlers ResourceHandler
type ResourceHandler interface {
	Put(ctx context.Context, options *PutOptions) (map[string]string, error)
	Delete(ctx context.Context, options DeleteOptions) error
}
