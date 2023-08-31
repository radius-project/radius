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
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/radius-project/radius/pkg/azure/armauth"
	"github.com/radius-project/radius/pkg/azure/clientv2"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_azure "github.com/radius-project/radius/pkg/ucp/resources/azure"
)

// NewARMHandler creates a new ARMHandler instance with the given ARM configuration, for 'generic' ARM resources.
func NewARMHandler(arm *armauth.ArmConfig) ResourceHandler {
	return &armHandler{arm: arm}
}

type armHandler struct {
	arm *armauth.ArmConfig
}

// Put validates that the resource exists. It returns an error if the resource does not exist.
func (handler *armHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	// Do a GET just to validate that the resource exists.
	_, err := handler.getByID(ctx, options.Resource.ID)
	if err != nil {
		return nil, err
	}

	return map[string]string{}, nil
}

// No-op - just returns nil.
func (handler *armHandler) Delete(ctx context.Context, options *DeleteOptions) error {
	return nil
}

func (handler *armHandler) getByID(ctx context.Context, id resources.ID) (*armresources.GenericResource, error) {
	client, err := clientv2.NewGenericResourceClient(id.FindScope(resources_azure.ScopeSubscriptions), &handler.arm.ClientOptions, nil)
	if err != nil {
		return nil, err
	}

	apiVersion, err := handler.lookupARMAPIVersion(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to access resource %q", id)
	}

	resp, err := client.GetByID(ctx, id.String(), apiVersion, &armresources.ClientGetByIDOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to access resource %q", id)
	}

	return &resp.GenericResource, nil
}

func (handler *armHandler) lookupARMAPIVersion(ctx context.Context, id resources.ID) (string, error) {
	client, err := clientv2.NewProvidersClient(id.FindScope(resources_azure.ScopeSubscriptions), &handler.arm.ClientOptions, nil)
	if err != nil {
		return "", err
	}

	resp, err := client.Get(ctx, id.ProviderNamespace(), nil)
	if err != nil {
		return "", err
	}

	// We need to match on the resource type name without the provider namespace.
	shortType := strings.TrimPrefix(id.TypeSegments()[0].Type, id.ProviderNamespace()+"/")
	for _, rt := range resp.ResourceTypes {
		if !strings.EqualFold(shortType, *rt.ResourceType) {
			continue
		}
		if rt.DefaultAPIVersion != nil {
			return *rt.DefaultAPIVersion, nil
		}

		if len(rt.APIVersions) > 0 {
			return *rt.APIVersions[0], nil
		}

		return "", fmt.Errorf("could not find API version for type %q, no supported API versions", id.Type())

	}

	return "", fmt.Errorf("could not find API version for type %q, type was not found", id.Type())
}
