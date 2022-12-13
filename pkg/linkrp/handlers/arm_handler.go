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

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clientv2"
	"github.com/project-radius/radius/pkg/radlogger"
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
	res, err := getByID(ctx, handler.arm.TokenCredential, id, apiVersion)
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

	logger := radlogger.GetLogger(ctx).WithValues(radlogger.LogFieldArmResourceID, id)
	logger.Info("Deleting ARM resource")
	parsed, err := ucpresources.ParseResource(id)
	if err != nil {
		return err
	}

	subscriptionID := parsed.FindScope(ucpresources.SubscriptionsSegment)

	client, err := armresources.NewClient(subscriptionID, handler.arm.TokenCredential, &arm.ClientOptions{})
	if err != nil {
		return err
	}
	_, err = client.BeginDeleteByID(ctx, id, apiVersion, &armresources.ClientBeginDeleteByIDOptions{})
	if err != nil {
		if !clientv2.Is404Error(err) {
			return fmt.Errorf("failed to delete resource %q: %w", id, err)
		}
		logger.Info(fmt.Sprintf("Resource %s does not exist: %s", id, err.Error()))
	}

	return nil
}

func serializeResource(resource armresources.GenericResource) (map[string]interface{}, error) {
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

func getByID(ctx context.Context, cred azcore.TokenCredential, id, apiVersion string) (*armresources.GenericResource, error) {
	parsed, err := ucpresources.ParseResource(id)
	if err != nil {
		return nil, err
	}

	logger := radlogger.GetLogger(ctx).WithValues(radlogger.LogFieldArmResourceID, id)
	logger.Info("Fetching arm resource by id")

	subscriptionID := parsed.FindScope(ucpresources.SubscriptionsSegment)

	client, err := armresources.NewClient(subscriptionID, cred, &arm.ClientOptions{})
	if err != nil {
		return nil, err
	}

	resp, err := client.GetByID(ctx, id, apiVersion, &armresources.ClientGetByIDOptions{})
	if err != nil {
		// FIXME: Do we need this?
		if clientv2.Is404Error(err) {
			return nil, conv.NewClientErrInvalidRequest(fmt.Sprintf("provided Azure resource %q does not exist", id))
		}
		return nil, fmt.Errorf("failed to access resource %q", id)
	}

	return &resp.GenericResource, nil
}
