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
	resource, err := getByID(ctx, &handler.arm.ClientOptions, options.Resource.Identity)
	if err != nil {
		return nil, err
	}

	// Return the resource so renderers can use it for computed values.
	serialized, err := handler.serializeResource(resource)
	if err != nil {
		return nil, err
	}
	options.Resource.Resource = serialized

	return map[string]string{}, nil
}

func (handler *armHandler) Delete(ctx context.Context, options *DeleteOptions) error {
	return nil
}

func (handler *armHandler) serializeResource(resource *armresources.GenericResource) (map[string]interface{}, error) {
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

func getByID(ctx context.Context, options *clientv2.Options, identity resourcemodel.ResourceIdentity) (*armresources.GenericResource, error) {
	id, apiVersion, err := identity.RequireARM()
	if err != nil {
		return nil, err
	}

	parsed, err := ucpresources.ParseResource(id)
	if err != nil {
		return nil, err
	}

	client, err := clientv2.NewGenericResourceClient(parsed.FindScope(ucpresources.SubscriptionsSegment), options, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.GetByID(ctx, id, apiVersion, &armresources.ClientGetByIDOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to access resource %q", id)
	}

	return &resp.GenericResource, nil
}
