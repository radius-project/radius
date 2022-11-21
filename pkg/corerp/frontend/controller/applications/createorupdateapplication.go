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
	"github.com/project-radius/radius/pkg/ucp/store"
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

	// Create Query filter to query kubernetes namespace used by the other application resources.
	// TODO : Refactor for loop and add case for default namespace (i.e. env-ns-myapp case)
	var namespace string
	for index, entry := range newResource.Properties.Extensions {
		if entry.Kind == datamodel.KubernetesNamespaceOverride {
			namespace = newResource.Properties.Extensions[index].KubernetesNamespaceOverride.Namespace
		}
	}

	if namespace == "" {
		// TODO : update to environment namespace not just env name
		namespace = newResource.Properties.Environment + "-ns-" + newResource.Name
	}

	// Create Query filter to query kubernetes namespace used by the other application resources.
	// TODO: update based on helper code
	// namespace, err := kube.FindNamespaceByAppID(ctx, a.DataProvider(), newResource.ID)
	// if err != nil {
	// 	return nil, err
	// }

	namespaceQuery := store.Query{
		RootScope:    serviceCtx.ResourceID.RootScope(),
		ResourceType: serviceCtx.ResourceID.Type(),
		Filters: []store.QueryFilter{
			{
				Field: "appInternal.kubernetesNamespace",
				Value: namespace,
			},
		},
	}

	// Check if application with this namespace already exists
	result, err := a.StorageClient().Query(ctx, namespaceQuery)
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
		if app.ID != old.ID {
			return rest.NewConflictResponse(fmt.Sprintf("Application %s with the same namespace (%s) already exists", app.ID, namespace)), nil
		}
	}

	newResource.SetProvisioningState(v1.ProvisioningStateSucceeded)
	newEtag, err := a.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	return a.ConstructSyncResponse(ctx, req.Method, newEtag, newResource)
}
