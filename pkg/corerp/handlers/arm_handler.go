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
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/resourcemodel"
	ucpresources "github.com/project-radius/radius/pkg/ucp/resources"
)

// NewARMHandler creates a ResourceHandler for 'generic' ARM resources.
func NewARMHandler(arm *armauth.ArmConfig) ResourceHandler {
	return &armHandler{arm: arm}
}

type armHandler struct {
	arm *armauth.ArmConfig
}

func (handler *armHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	// Do a GET just to validate that the resource exists.
	res, err := getByID(ctx, handler.arm.Auth, options.Resource.Identity)
	if err != nil {
		return nil, err
	}

	// Return the resource so renderers can use it for computed values.
	serialized, err := handler.serializeResource(*res)
	if err != nil {
		return nil, err
	}
	options.Resource.Resource = serialized

	return map[string]string{}, nil
}

func (handler *armHandler) Delete(ctx context.Context, options *DeleteOptions) error {
	return nil
}

func (handler *armHandler) serializeResource(resource resources.GenericResource) (map[string]any, error) {
	// We turn the resource into a weakly-typed representation. This is needed because JSON Pointer
	// will have trouble with the autorest embdedded types.
	b, err := json.Marshal(&resource)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal %T", resource)
	}

	data := map[string]any{}
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

	parsed, err := ucpresources.ParseResource(id)
	if err != nil {
		return nil, err
	}

	rc := clients.NewGenericResourceClient(parsed.FindScope(ucpresources.SubscriptionsSegment), auth)
	resource, err := rc.GetByID(ctx, id, apiVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to access resource %q", id)
	}
	return &resource, nil
}
