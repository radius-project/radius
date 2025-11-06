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

package v20250801preview

import (
	"context"
	"fmt"
	"net/http"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/datamodel/converter"
	"github.com/radius-project/radius/pkg/corerp/frontend/controller/util"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ ctrl.Controller = (*CreateOrUpdateEnvironmentv20250801preview)(nil)

// CreateOrUpdateEnvironmentv20250801preview is the controller implementation to create or update Radius.Core/environments resource.
type CreateOrUpdateEnvironmentv20250801preview struct {
	ctrl.Operation[*datamodel.Environment_v20250801preview, datamodel.Environment_v20250801preview]
}

// NewCreateOrUpdateEnvironmentv20250801preview creates a new controller for creating or updating a Radius.Core/environments resource.
func NewCreateOrUpdateEnvironmentv20250801preview(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateEnvironmentv20250801preview{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.Environment_v20250801preview]{
				RequestConverter:  converter.Environment20250801DataModelFromVersioned,
				ResponseConverter: converter.Environment20250801DataModelToVersioned,
			},
		),
	}, nil
}

// Run creates or updates a Radius.Core/environments resource.
func (e *CreateOrUpdateEnvironmentv20250801preview) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	newResource, err := e.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	old, etag, err := e.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if resp, err := e.PrepareResource(ctx, req, newResource, old, etag); resp != nil || err != nil {
		return resp, err
	}

	// Create Query filter to query kubernetes namespace used by the other environment resources.
	if newResource.Properties.Providers != nil && newResource.Properties.Providers.Kubernetes != nil {
		namespace := newResource.Properties.Providers.Kubernetes.Namespace
		result, err := util.FindResources(ctx, serviceCtx.ResourceID.RootScope(), serviceCtx.ResourceID.Type(), "properties.providers.kubernetes.namespace", namespace, e.DatabaseClient())
		if err != nil {
			return nil, err
		}

		if len(result.Items) > 0 {
			env := &datamodel.Environment_v20250801preview{}
			if err := result.Items[0].As(env); err != nil {
				return nil, err
			}

			// If a different resource has the same namespace, return a conflict
			// Otherwise, continue and update the resource
			if old == nil || env.ID != old.ID {
				return rest.NewConflictResponse(fmt.Sprintf("Environment %s with the same namespace (%s) already exists", env.ID, namespace)), nil
			}
		}

		ns := &corev1.Namespace{}
		err = e.Options().KubeClient.Get(ctx, client.ObjectKey{Name: namespace}, ns)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return rest.NewBadRequestResponse(fmt.Sprintf("Namespace '%s' does not exist in the Kubernetes cluster. Please create it before proceeding.", namespace)), nil
			}
			return nil, err
		}
	}

	newResource.SetProvisioningState(v1.ProvisioningStateSucceeded)
	newEtag, err := e.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	return e.ConstructSyncResponse(ctx, req.Method, newEtag, newResource)
}
