// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"

	"github.com/Azure/radius/pkg/radrp/db"
	"github.com/Azure/radius/pkg/workloads"
)

type PutOptions struct {
	Application string
	Component   string
	Resource    workloads.OutputResource
	Existing    *db.DeploymentResource
}

type DeleteOptions struct {
	Application string
	Component   string
	Existing    db.DeploymentResource
}

type ResourceHandler interface {
	Put(ctx context.Context, options PutOptions) (map[string]string, error)
	Delete(ctx context.Context, options DeleteOptions) error
}
