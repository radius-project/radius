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

package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/renderers/container"
	"github.com/radius-project/radius/pkg/corerp/renderers/gateway"
	"github.com/radius-project/radius/pkg/corerp/renderers/volume"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

var _ ctrl.Controller = (*CreateOrUpdateResource)(nil)

// CreateOrUpdateResource is the async operation controller to create or update Applications.Core/Containers resource.
type CreateOrUpdateResource struct {
	ctrl.BaseController
}

// NewCreateOrUpdateResource creates a new CreateOrUpdateResource controller.
func NewCreateOrUpdateResource(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateResource{ctrl.NewBaseAsyncController(opts)}, nil
}

func getDataModel(id resources.ID) (v1.DataModelInterface, error) {
	resourceType := strings.ToLower(id.Type())
	switch resourceType {
	case strings.ToLower(container.ResourceType):
		return &datamodel.ContainerResource{}, nil
	case strings.ToLower(gateway.ResourceType):
		return &datamodel.Gateway{}, nil
	case strings.ToLower(volume.ResourceType):
		return &datamodel.VolumeResource{}, nil
	default:
		return nil, fmt.Errorf("invalid resource type: %q for dependent resource ID: %q", resourceType, id.String())
	}
}

// Run checks if the resource exists, renders the resource, deploys the resource, applies the
// deployment output to the resource, deletes any resources that are no longer needed, and saves the resource.
func (c *CreateOrUpdateResource) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {

	// use id.ExtractStorageParts to get ResourceType
	// get the resourcetyperesurce from the database
	// see if you can get the value of recipe enabled
	// pass it down to deployment
	// myid, err := resources.Parse(request.ResourceID)
	//if err != nil {
	//	return ctrl.Result{}, err
	// }
	//	pre, root, routing, resourceType := databaseutil.ExtractStorageParts(myid)

	//	resourceTypeResourceObj, _ := c.DatabaseClient().Get(ctx, pre+"/"+root+"/"+routing+"/"+resourceType)

	resourceTypeResourceObj, err := c.DatabaseClient().Get(ctx, "/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Core/resourceTypes/containers")

	if err != nil {
		return ctrl.Result{}, err
	}
	fmt.Print(resourceTypeResourceObj)

	data := resourceTypeResourceObj.Data
	if data == nil {
		return ctrl.Result{}, errors.New("data is nil")
	}

	// data is a map[string]interface{}. chexk if key capabilities is present
	// and has "recipeEnabled"
	properties, ok := data.(map[string]interface{})["properties"]
	if !ok {
		return ctrl.Result{}, errors.New("properties not found")
	}

	capabilities, ok := properties.(map[string]interface{})["capabilities"]
	if !ok {
		return ctrl.Result{}, errors.New("capabilities not found")
	}

	for _, capability := range capabilities.([]interface{}) {
		if capability == "SupportsRecipes" {
			fmt.Print("recipe enabled")
		}

	}

	obj, err := c.DatabaseClient().Get(ctx, request.ResourceID)
	if err != nil && !errors.Is(&database.ErrNotFound{ID: request.ResourceID}, err) {
		return ctrl.Result{}, err
	}

	isNewResource := false
	if errors.Is(&database.ErrNotFound{ID: request.ResourceID}, err) {

		isNewResource = true
	}

	opType, _ := v1.ParseOperationType(request.OperationType)
	if opType.Method == http.MethodPatch && errors.Is(&database.ErrNotFound{ID: request.ResourceID}, err) {
		return ctrl.Result{}, err
	}

	// This code is general and we might be processing an async job for a resource or a scope, so using the general Parse function.
	id, err := resources.Parse(request.ResourceID)
	if err != nil {
		return ctrl.Result{}, err
	}

	dataModel, err := getDataModel(id)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err = obj.As(dataModel); err != nil {
		return ctrl.Result{}, err
	}

	// here pass if recipe enabled or not in Render
	rendererOutput, err := c.DeploymentProcessor().Render(ctx, id, dataModel)
	if err != nil {
		return ctrl.Result{}, err
	}

	deploymentOutput, err := c.DeploymentProcessor().Deploy(ctx, id, rendererOutput)
	if err != nil {
		return ctrl.Result{}, err
	}

	deploymentDataModel, ok := dataModel.(rpv1.DeploymentDataModel)
	if !ok {
		return ctrl.NewFailedResult(v1.ErrorDetails{Message: "deployment data model conversion error"}), err
	}

	oldOutputResources := deploymentDataModel.OutputResources()

	err = deploymentDataModel.ApplyDeploymentOutput(deploymentOutput)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !isNewResource {
		diff := rpv1.GetGCOutputResources(deploymentDataModel.OutputResources(), oldOutputResources)
		err = c.DeploymentProcessor().Delete(ctx, id, diff)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	nr := &database.Object{
		Metadata: database.Metadata{
			ID: request.ResourceID,
		},
		Data: deploymentDataModel,
	}
	err = c.DatabaseClient().Save(ctx, nr, database.WithETag(obj.ETag))
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, err
}
