// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deployment

import (
	"context"
	"errors"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/radrp/db"
)

//go:generate mockgen -destination=./mock_deploymentprocessor.go -package=deployment -self_package github.com/Azure/radius/pkg/radrp/backend/deployment github.com/Azure/radius/pkg/radrp/backend/deployment DeploymentProcessor

type DeploymentProcessor interface {
	// NOTE: the DeploymentProcessor returns errors but they are just for logging, since it's called
	// asynchronously.

	Deploy(ctx context.Context, id azresources.ResourceID, resource db.RadiusResource) error
	Delete(ctx context.Context, id azresources.ResourceID, resource db.RadiusResource) error
}

func NewDeploymentProcessor() DeploymentProcessor {
	return &deploymentProcessor{}
}

var _ DeploymentProcessor = (*deploymentProcessor)(nil)

type deploymentProcessor struct {
}

func (d *deploymentProcessor) Deploy(ctx context.Context, id azresources.ResourceID, resource db.RadiusResource) error {
	return errors.New("not implemented")
}
func (d *deploymentProcessor) Delete(ctx context.Context, id azresources.ResourceID, resource db.RadiusResource) error {
	return errors.New("not implemented")
}
