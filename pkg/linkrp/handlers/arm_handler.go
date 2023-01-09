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
	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/logging"
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
	id, apiVersion, err := resource.Identity.RequireARM()
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	// Do a GET just to validate that the resource exists.
	res, err := getByID(ctx, handler.arm.Auth, id, apiVersion)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	// Return the resource so renderers can use it for computed values.
	// TODO: it may not require such serialization.
	serialized, err := serializeResource(*res)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}
	resource.Resource = serialized

	return resourcemodel.ResourceIdentity{}, map[string]string{}, nil
}

func (handler *armHandler) Delete(ctx context.Context, resource *outputresource.OutputResource) error {
	id, apiVersion, err := resource.Identity.RequireARM()
	if err != nil {
		return err
	}

	logger := logr.FromContextOrDiscard(ctx).WithValues(logging.LogFieldArmResourceID, id)
	logger.Info("Deleting ARM resource")
	parsed, err := ucpresources.ParseResource(id)
	if err != nil {
		return err
	}

	rc := clients.NewGenericResourceClient(parsed.FindScope(ucpresources.SubscriptionsSegment), handler.arm.Auth)
	_, err = rc.DeleteByID(ctx, id, apiVersion)
	if err != nil {
		if !clients.Is404Error(err) {
			return fmt.Errorf("failed to delete resource %q: %w", id, err)
		}
		logger.Info(fmt.Sprintf("Resource %s does not exist: %s", id, err.Error()))
	}

	return nil
}

func serializeResource(resource resources.GenericResource) (map[string]any, error) {
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

func getByID(ctx context.Context, auth autorest.Authorizer, id, apiVersion string) (*resources.GenericResource, error) {
	parsed, err := ucpresources.ParseResource(id)
	if err != nil {
		return nil, err
	}

	logger := logr.FromContextOrDiscard(ctx).WithValues(logging.LogFieldArmResourceID, id)
	logger.Info("Fetching arm resource by id")
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
