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

package extenders

import (
	"context"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
)

var _ ctrl.Controller = (*ListSecretsExtender)(nil)

// ListSecretsExtender is the controller implementation to list secrets for the extender to access the connected extender resource with the resource id passed in the request body.
type ListSecretsExtender struct {
	ctrl.Operation[*datamodel.Extender, datamodel.Extender]
}

// NewListSecretsExtender creates a new instance of ListSecretsExtender.
func NewListSecretsExtender(opts ctrl.Options) (ctrl.Controller, error) {
	return &ListSecretsExtender{
		Operation: ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.Extender]{
				RequestConverter:  converter.ExtenderDataModelFromVersioned,
				ResponseConverter: converter.ExtenderDataModelToVersioned,
			}),
	}, nil
}

// Run returns secrets values for the specified Extender resource.
func (ctrl *ListSecretsExtender) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	sCtx := v1.ARMRequestContextFromContext(ctx)

	// Request route for listsecrets has name of the operation as suffix which should be removed to get the resource id.
	// route id format: subscriptions/<subscription_id>/resourceGroups/<resource_group>/providers/Applications.Core/extenders/<resource_name>/listsecrets
	parsedResourceID := sCtx.ResourceID.Truncate()
	resource, _, err := ctrl.GetResource(ctx, parsedResourceID)
	if err != nil {
		return nil, err
	}

	if resource == nil {
		return rest.NewNotFoundResponse(sCtx.ResourceID), nil
	}

	secrets := map[string]string{}
	for key, secret := range resource.SecretValues {
		secrets[key] = secret.Value
	}

	return rest.NewOKResponse(secrets), nil
}
