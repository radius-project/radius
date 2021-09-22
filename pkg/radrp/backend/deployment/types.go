// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deployment

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/model"
	"github.com/Azure/radius/pkg/radrp/db"
)

//go:generate mockgen -destination=./mock_deploymentprocessor.go -package=deployment -self_package github.com/Azure/radius/pkg/radrp/backend/deployment github.com/Azure/radius/pkg/radrp/backend/deployment DeploymentProcessor

type DeploymentProcessor interface {
	// NOTE: the DeploymentProcessor returns errors but they are just for logging, since it's called
	// asynchronously.

	Deploy(ctx context.Context, id azresources.ResourceID, resource db.RadiusResource) error
	Delete(ctx context.Context, id azresources.ResourceID, resource db.RadiusResource) error
}

func NewDeploymentProcessor(appmodel model.ApplicationModel) DeploymentProcessor {
	return &deploymentProcessor{appmodel: appmodel}
}

var _ DeploymentProcessor = (*deploymentProcessor)(nil)

type deploymentProcessor struct {
	appmodel model.ApplicationModel
}

func (dp *deploymentProcessor) Deploy(ctx context.Context, id azresources.ResourceID, resource db.RadiusResource) error {
	// Locate Renderer
	componentKind, err := dp.appmodel.LookupComponent(resource.Definition["kind"].(string))
	if err != nil {
		return err
	}

	renderer := componentKind.Renderer()
	// resources, err := componentKind.Renderer().Render(ctx, w)
	// if err != nil {
	// 	return resources, fmt.Errorf("could not render workload of kind %v: %v", w.Workload.Kind, err)
	// }
	fmt.Println("found renderer ", renderer)

	// Gather Renderer Inputs ******

	// Render

	// Foreach OutputResource

	// 		PUT Resource

	// 		Update Database

	//		Register Health

	// Update Operation

	// Update Resource & set computed values
	return errors.New("not implemented")
}
func (d *deploymentProcessor) Delete(ctx context.Context, id azresources.ResourceID, resource db.RadiusResource) error {
	return errors.New("not implemented")
}
