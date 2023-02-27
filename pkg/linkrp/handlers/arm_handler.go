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

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clientv2"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	ucpresources "github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

// NewARMHandler creates a ResourceHandler for 'generic' ARM resources.
func NewARMHandler(arm *armauth.ArmConfig) ResourceHandler {
	return &armHandler{arm: arm}
}

type armHandler struct {
	arm *armauth.ArmConfig
}

func (handler *armHandler) Put(ctx context.Context, resource *rpv1.OutputResource) (outputResourceIdentity resourcemodel.ResourceIdentity, properties map[string]string, err error) {
	id, apiVersion, err := resource.Identity.RequireARM()
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	// Do a GET just to validate that the resource exists.
	res, err := getByID(ctx, &handler.arm.ClientOptions, id, apiVersion)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	// Return the resource so renderers can use it for computed values.
	// TODO: it may not require such serialization.
	serialized, err := serializeResource(res)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}
	resource.Resource = serialized

	return resourcemodel.ResourceIdentity{}, map[string]string{}, nil
}

func (handler *armHandler) Delete(ctx context.Context, resource *rpv1.OutputResource) error {
	if resource.RadiusManaged == nil || !*resource.RadiusManaged {
		return nil
	}
	id, apiVersion, err := resource.Identity.RequireARM()
	if err != nil {
		return err
	}

	logger := ucplog.FromContext(ctx)
	logger.Info("Deleting ARM resource")
	parsed, err := ucpresources.ParseResource(id)
	if err != nil {
		return err
	}

	client, err := clientv2.NewGenericResourceClient(parsed.FindScope(ucpresources.SubscriptionsSegment), &handler.arm.ClientOptions)
	if err != nil {
		return err
	}

	poller, err := client.BeginDeleteByID(ctx, id, apiVersion, &armresources.ClientBeginDeleteByIDOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete resource %q: %w", id, err)
	}

	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to delete resource %q: %w", id, err)
	}

	return nil
}

func serializeResource(resource *armresources.GenericResource) (map[string]any, error) {
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

func getByID(ctx context.Context, options *clientv2.Options, id, apiVersion string) (*armresources.GenericResource, error) {
	parsed, err := ucpresources.ParseResource(id)
	if err != nil {
		return nil, err
	}

	logger := ucplog.FromContext(ctx)
	logger.Info("Fetching arm resource by id")

	client, err := clientv2.NewGenericResourceClient(parsed.FindScope(ucpresources.SubscriptionsSegment), options)
	if err != nil {
		return nil, err
	}

	resource, err := client.GetByID(ctx, id, apiVersion, &armresources.ClientGetByIDOptions{})
	if clientv2.Is404Error(err) {
		return nil, fmt.Errorf("provided Azure resource %q does not exist", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to access resource %q", id)
	}

	return &resource.GenericResource, nil
}
