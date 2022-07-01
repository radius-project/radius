// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2020-10-01/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcemodel"
)

// NewARMHandler creates a ResourceHandler for 'generic' ARM resources.
func NewARMHandler(arm *armauth.ArmConfig) ResourceHandler {
	return &armHandler{arm: arm}
}

type armHandler struct {
	arm *armauth.ArmConfig
}

func (handler *armHandler) Put(ctx context.Context, resource *outputresource.OutputResource) error {
	// Do a GET just to validate that the resource exists.
	arm_resource, err := getByID(ctx, handler.arm.SubscriptionID, handler.arm.Auth, resource.Identity)
	if err != nil {
		return err
	}

	// Return the resource so renderers can use it for computed values.
	serialized, err := handler.serializeResource(*arm_resource)
	if err != nil {
		return err
	}
	resource.Resource = serialized

	return nil
}

func (handler *armHandler) GetResourceIdentity(ctx context.Context, resource outputresource.OutputResource) (resourcemodel.ResourceIdentity, error) {
	return resource.Identity, nil
}

func (handler *armHandler) GetResourceNativeIdentityKeyProperties(ctx context.Context, resource outputresource.OutputResource) (map[string]string, error) {
	return map[string]string{}, nil
}

func (handler *armHandler) Delete(ctx context.Context, resource outputresource.OutputResource) error {
	return nil
}

func (handler *armHandler) serializeResource(resource resources.GenericResource) (map[string]interface{}, error) {
	// We turn the resource into a weakly-typed representation. This is needed because JSON Pointer
	// will have trouble with the autorest embdedded types.
	b, err := json.Marshal(&resource)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal %T", resource)
	}

	data := map[string]interface{}{}
	err = json.Unmarshal(b, &data)
	if err != nil {
		return nil, errors.New("failed to umarshal resource data")
	}

	return data, nil
}

func getByID(ctx context.Context, subscriptionID string, auth autorest.Authorizer, identity resourcemodel.ResourceIdentity) (*resources.GenericResource, error) {
	id, apiVersion, err := identity.RequireARM()
	if err != nil {
		return nil, err
	}
	rc := clients.NewGenericResourceClient(subscriptionID, auth)
	resource, err := rc.GetByID(ctx, id, apiVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to access resource %q", id)
	}
	return &resource, nil
}
