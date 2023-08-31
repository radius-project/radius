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

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/radius-project/radius/pkg/azure/armauth"
	"github.com/radius-project/radius/pkg/azure/clientv2"
	"github.com/radius-project/radius/pkg/logging"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_azure "github.com/radius-project/radius/pkg/ucp/resources/azure"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

const (
	IdentityProperties                 = "identityproperties"
	UserAssignedIdentityNameKey        = "userassignedidentityname"
	UserAssignedIdentityIDKey          = "userassignedidentityid"
	UserAssignedIdentityPrincipalIDKey = "userassignedidentityprincipalid"
	UserAssignedIdentityClientIDKey    = "userassignedidentityclientid"
	UserAssignedIdentityTenantIDKey    = "userassignedidentitytenantid"
	UserAssignedIdentitySubscriptionID = "userassignedidentitysubscriptionid"
	UserAssignedIdentityResourceGroup  = "userassignedidentityresourcegroup"
)

// NewAzureUserAssignedManagedIdentityHandler creates a new ResourceHandler for Azure User Assigned Managed Identity.
func NewAzureUserAssignedManagedIdentityHandler(arm *armauth.ArmConfig) ResourceHandler {
	return &azureUserAssignedManagedIdentityHandler{arm: arm}
}

type azureUserAssignedManagedIdentityHandler struct {
	arm *armauth.ArmConfig
}

// Put creates or updates a user assigned managed identity in the specified resource group and returns the identity's
// properties. It returns an error if the region does not support federated identity or if the creation or update fails.
func (handler *azureUserAssignedManagedIdentityHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	properties, ok := options.Resource.CreateResource.Data.(map[string]string)
	if !ok {
		return properties, fmt.Errorf("invalid required properties for resource")
	}

	identityName, err := GetMapValue[string](properties, UserAssignedIdentityNameKey)
	if err != nil {
		return nil, err
	}

	subID, err := GetMapValue[string](properties, UserAssignedIdentitySubscriptionID)
	if err != nil {
		return nil, err
	}

	rgName, err := GetMapValue[string](properties, UserAssignedIdentityResourceGroup)
	if err != nil {
		return nil, err
	}

	rgLocation, err := clientv2.GetResourceGroupLocation(ctx, subID, rgName, &handler.arm.ClientOptions)
	if err != nil {
		return properties, err
	}

	resourceLocation := *rgLocation

	// Federated identity is in preview. Some region doesn't support federated identity.
	// Reference: https://learn.microsoft.com/en-us/azure/active-directory/develop/workload-identity-federation-considerations#unsupported-regions-user-assigned-managed-identities
	// TODO: Remove when all regions support Federated identity.
	if !isFederatedIdentitySupported(resourceLocation) {
		return nil, fmt.Errorf("azure federated identity does not support %s region now. unsupported regions: %q", resourceLocation, federatedUnsupportedRegions)
	}

	msiClient, err := clientv2.NewUserAssignedIdentityClient(subID, &handler.arm.ClientOptions)
	if err != nil {
		return nil, err
	}

	identity, err := msiClient.CreateOrUpdate(ctx, rgName, identityName, armmsi.Identity{Location: &resourceLocation}, nil)
	if err != nil {
		return properties, fmt.Errorf("failed to create user assigned managed identity: %w", err)
	}

	properties[UserAssignedIdentityIDKey] = to.String(identity.ID)
	properties[UserAssignedIdentityPrincipalIDKey] = to.String(identity.Properties.PrincipalID)
	properties[UserAssignedIdentityClientIDKey] = to.String(identity.Properties.ClientID)
	properties[UserAssignedIdentityTenantIDKey] = to.String(identity.Properties.TenantID)

	id, err := resources.ParseResource(properties[UserAssignedIdentityIDKey])
	if err != nil {
		return nil, err
	}

	options.Resource.ID = id
	logger.Info("Created managed identity for KeyVault access", logging.LogFieldLocalID, rpv1.LocalIDUserAssignedManagedIdentity)

	return properties, nil
}

// Delete deletes a user assigned managed identity from Azure using the provided DeleteOptions.
func (handler *azureUserAssignedManagedIdentityHandler) Delete(ctx context.Context, options *DeleteOptions) error {

	subscriptionID := options.Resource.ID.FindScope(resources_azure.ScopeSubscriptions)
	msiClient, err := clientv2.NewUserAssignedIdentityClient(subscriptionID, &handler.arm.ClientOptions)
	if err != nil {
		return err
	}

	_, err = msiClient.Delete(ctx, options.Resource.ID.FindScope(resources_azure.ScopeResourceGroups), options.Resource.ID.Name(), nil)

	if err != nil {
		return fmt.Errorf("failed to delete user assigned managed identity: %w", err)
	}

	return nil
}
