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

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	ucpresources "github.com/project-radius/radius/pkg/ucp/resources"
)

// NewARMHandler creates a ResourceHandler for 'generic' ARM resources.
func NewARMHandler(arm *armauth.ArmConfig) ResourceHandler {
	return &armHandler{arm: arm}
}

type armHandler struct {
	arm *armauth.ArmConfig
}

func (handler *armHandler) Put(ctx context.Context, resource *outputresource.OutputResource) (outputResourceIdentity resourcemodel.ResourceIdentity, properties map[string]string, err error) {
	// Do a GET just to validate that the resource exists.
	res, err := getByID(ctx, handler.arm.Auth, resource.Identity)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	// Return the resource so renderers can use it for computed values.
	// TODO: it may not require such serialization.
	serialized, err := handler.serializeResource(*res)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}
	resource.Resource = serialized

	return resourcemodel.ResourceIdentity{}, map[string]string{}, nil
}

func (handler *armHandler) Delete(ctx context.Context, resource *outputresource.OutputResource, identity *resourcemodel.ResourceIdentity) error {
	if resource != nil {
		identity = &resource.Identity
	}
	err := deleteByID(ctx, handler.arm.Auth, *identity)
	if err != nil {
		return err
	}
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

func getByID(ctx context.Context, auth autorest.Authorizer, identity resourcemodel.ResourceIdentity) (*resources.GenericResource, error) {
	id, apiVersion, err := identity.RequireARM()
	if err != nil {
		return nil, err
	}

	parsed, err := ucpresources.Parse(id)
	if err != nil {
		return nil, err
	}

	rc := clients.NewGenericResourceClient(parsed.FindScope(ucpresources.SubscriptionsSegment), auth)
	resource, err := rc.GetByID(ctx, id, apiVersion)
	if err != nil {
		if clients.Is404Error(err) {
			return nil, conv.NewClientErrInvalidRequest(fmt.Sprintf("provided Azure resource %q does not exist", id))
		}
		return nil, fmt.Errorf("failed to access resource %q", id)
	}
	return &resource, nil
}

func deleteByID(ctx context.Context, auth autorest.Authorizer, identity resourcemodel.ResourceIdentity) error {
	id, apiVersion, err := identity.RequireARM()
	if err != nil {
		return err
	}

	parsed, err := ucpresources.Parse(id)
	if err != nil {
		return err
	}

	rc := clients.NewGenericResourceClient(parsed.FindScope(ucpresources.SubscriptionsSegment), auth)
	_, err = rc.DeleteByID(ctx, id, apiVersion)
	if err != nil {
		if clients.Is404Error(err) {
			return fmt.Errorf("provided Azure resource %q does not exist", id)
		}
		return fmt.Errorf("failed to delete resource %q", id)
	}
	return nil
}
