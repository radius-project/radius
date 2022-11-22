// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package applications

import (
	"context"
	"fmt"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	"github.com/project-radius/radius/pkg/corerp/frontend/controller/util"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/ucp/resources"

	rp_kube "github.com/project-radius/radius/pkg/rp/kube"
)

var _ ctrl.Controller = (*CreateOrUpdateApplication)(nil)

// CreateOrUpdateApplication is the controller implementation to create or update application resource.
type CreateOrUpdateApplication struct {
	ctrl.Operation[*datamodel.Application, datamodel.Application]
}

// NewCreateOrUpdateApplication creates a new instance of CreateOrUpdateApplication.
func NewCreateOrUpdateApplication(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateApplication{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.Application]{
				RequestConverter:  converter.ApplicationDataModelFromVersioned,
				ResponseConverter: converter.ApplicationDataModelToVersioned,
			},
		),
	}, nil
}

// Run executes CreateOrUpdateApplication operation.
func (a *CreateOrUpdateApplication) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	newResource, err := a.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	old, etag, err := a.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if r, err := a.PrepareResource(ctx, req, newResource, old, etag); r != nil || err != nil {
		return r, err
	}

	if old != nil {
		oldProp := &old.Properties.BasicResourceProperties
		newProp := &newResource.Properties.BasicResourceProperties
		if !oldProp.EqualLinkedResource(newProp) {
			return rest.NewLinkedResourceUpdateErrorResponse(serviceCtx.ResourceID, oldProp, newProp), nil
		}
	}

	kubeNamespace := ""
	ext := newResource.Properties.FindExtension(datamodel.KubernetesNamespaceOverride)
	if ext != nil {
		// Override environment namespace.
		kubeNamespace = ext.KubernetesNamespaceOverride.Namespace
	} else {
		// Construct namespace using the namespace specified by environment resource.
		envNamespace, err := rp_kube.FindNamespaceByEnvID(ctx, a.DataProvider(), newResource.Properties.Environment)
		if err != nil {
			return rest.NewBadRequestResponse(fmt.Sprintf("Environment could not be constructed: %s", err.Error())), nil
		}
		kubeNamespace = kubernetes.NormalizeResourceName(fmt.Sprintf("%s-%s", envNamespace, serviceCtx.ResourceID.Name()))
	}

	// Check if another environment resource is using namespace
	envID, err := resources.ParseResource(newResource.Properties.Environment)
	if err != nil {
		return rest.NewBadRequestResponse(fmt.Sprintf("Environment %s for application %s could not be found", envID.Name(), serviceCtx.ResourceID.Name())), nil
	}

	result, err := util.FindNamespaceResources(ctx, envID.RootScope(), envID.Type(), "properties.compute.kubernetes.namespace", kubeNamespace, a.StorageClient())
	if err != nil {
		return nil, err
	}
	if len(result.Items) > 0 {
		return rest.NewConflictResponse(fmt.Sprintf("Environment %s with the same namespace (%s) already exists", envID.Name(), kubeNamespace)), nil
	}

	// Check if another application resource is using namespace
	result, err = util.FindNamespaceResources(ctx, serviceCtx.ResourceID.RootScope(), serviceCtx.ResourceID.Type(), "appInternal.kubernetesNamespace", kubeNamespace, a.StorageClient())
	if err != nil {
		return nil, err
	}
	if len(result.Items) > 0 {
		app := &datamodel.Application{}
		if err := result.Items[0].As(app); err != nil {
			return nil, err
		}

		// If a different resource has the same namespace, return a conflict
		// Otherwise, continue and update the resource
		if old == nil || app.ID != old.ID {
			return rest.NewConflictResponse(fmt.Sprintf("Application %s with the same namespace (%s) already exists", app.ID, kubeNamespace)), nil
		}
	}

	// Populate kubernetes namespace to internal metadata property.
	newResource.AppInternal.KubernetesNamespace = kubeNamespace

	newResource.SetProvisioningState(v1.ProvisioningStateSucceeded)
	newEtag, err := a.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	return a.ConstructSyncResponse(ctx, req.Method, newEtag, newResource)
}
