// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/project-radius/radius/pkg/armrpc/hostoptions"
	"github.com/project-radius/radius/pkg/corerp/backend/deployment"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/model"
	"github.com/project-radius/radius/pkg/corerp/renderers/httproute"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	controller_runtime "sigs.k8s.io/controller-runtime/pkg/client"
	csidriver "sigs.k8s.io/secrets-store-csi-driver/apis/v1alpha1"
)

var _ ctrl.Controller = (*UpdateHttpRoute)(nil)

// UpdateHttpRoute is the async operation controller to create or update Applications.Core/httpRoutes resource.
type UpdateHttpRoute struct {
	ctrl.BaseController
	Options hostoptions.HostOptions
}

// NewUpdateHttpRoute creates the UpdateHttpRoute controller instance.
func NewUpdateHttpRoute(store store.StorageClient, options hostoptions.HostOptions) (ctrl.Controller, error) {
	return &UpdateHttpRoute{ctrl.NewBaseAsyncController(store), options}, nil
}

func (c *UpdateHttpRoute) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	// TODO: Integration with modified new backend controller
	// Note: mentioning here in this current placeholder controller to show the flow, it will not be checked in,
	// The job of backend controller is to do two major operations 1. Render and 2. Deploy

	scheme := clientgoscheme.Scheme
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(csidriver.AddToScheme(scheme))
	utilruntime.Must(contourv1.AddToScheme(scheme))

	k8s, err := controller_runtime.New(c.Options.K8sConfig, controller_runtime.Options{Scheme: scheme})

	if err != nil {
		return ctrl.Result{}, err
	}
	appModel, err := model.NewApplicationModel(c.Options.Arm, k8s)
	if err != nil {
		return ctrl.Result{}, err
	}

	dp := deployment.NewDeploymentProcessor(appModel, dataprovider.NewStorageProvider(dataprovider.StorageProviderOptions{}), nil, nil)

	fmt.Println("In update httproute")
	// Get the resource
	existingResource := &datamodel.HTTPRoute{}
	etag, err := c.GetResource(ctx, request.ResourceID, existingResource)
	if err != nil && !errors.Is(&store.ErrNotFound{}, err) {
		return ctrl.Result{}, err
	}
	id, err := resources.Parse(request.ResourceID)
	if err != nil {
		return ctrl.Result{}, err
	}

	bytes, _ := json.Marshal(existingResource)
	fmt.Println(string(bytes))

	// Render the resource
	rendererOutput, err := dp.Render(ctx, id, existingResource, httproute.Renderer{}) // TODO role assignment map?
	if err != nil {
		fmt.Println("Error in render " + err.Error())
		return ctrl.Result{}, err
	}

	// Deploy the resource
	deploymentOutput, err := dp.Deploy(ctx, id, rendererOutput)
	if err != nil {
		fmt.Println(err.Error())
		return ctrl.Result{}, err
	}

	// Update the resource with deployed outputResources
	existingResource.Properties.BasicResourceProperties.Status.OutputResources = deploymentOutput.DeployedOutputResources
	existingResource.InternalMetadata.ComputedValues = deploymentOutput.ComputedValues
	existingResource.InternalMetadata.SecretValues = deploymentOutput.SecretValues

	// Save the resource
	_, err = c.SaveResource(ctx, request.ResourceID, existingResource, etag)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
