// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/keyvault/mgmt/keyvault"
	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/azure/clients"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/keys"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/gofrs/uuid"
)

const (
	KeyVaultURIKey  = "uri"
	KeyVaultNameKey = "keyvaultname"
	KeyVaultIDKey   = "keyvaultid"
)

func NewAzureKeyVaultHandler(arm armauth.ArmConfig) ResourceHandler {
	return &azureKeyVaultHandler{arm: arm}
}

type azureKeyVaultHandler struct {
	arm armauth.ArmConfig
}

func (handler *azureKeyVaultHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	properties := mergeProperties(*options.Resource, options.Existing, options.ExistingOutputResource)

	// This assertion is important so we don't start creating/modifying an unmanaged resource
	err := ValidateResourceIDsForUnmanagedResource(properties, KeyVaultIDKey)
	if err != nil {
		return nil, err
	}

	if properties[KeyVaultIDKey] == "" {
		// If we have already created this resource we would have stored the name and ID.
		vaultName := GenerateRandomName("kv")

		kv, err := handler.CreateKeyVault(ctx, vaultName, *options)
		if err != nil {
			return nil, err
		}

		// store vault so we can use later
		properties[KeyVaultNameKey] = *kv.Name
		properties[KeyVaultIDKey] = *kv.ID
	} else {
		// This is mostly called for the side-effect of verifying that the keyvault exists.
		_, err := handler.GetKeyVaultByID(ctx, properties[KeyVaultIDKey])
		if err != nil {
			return nil, err
		}
	}

	if options.Resource.Deployed {
		return properties, nil
	}

	options.Resource.Info = outputresource.ARMInfo{
		ID:           properties[KeyVaultIDKey],
		ResourceType: azresources.KeyVaultVaults,
		APIVersion:   keyvault.Version(),
	}

	return properties, nil
}

func (handler *azureKeyVaultHandler) Delete(ctx context.Context, options DeleteOptions) error {
	var properties map[string]string
	if options.ExistingOutputResource == nil {
		properties = options.Existing.Properties
	} else {
		properties = options.ExistingOutputResource.PersistedProperties
	}

	if properties[ManagedKey] != "true" {
		// For an 'unmanaged' resource we don't need to do anything, just forget it.
		return nil
	}

	vaultName := properties[KeyVaultNameKey]

	err := handler.DeleteKeyVault(ctx, vaultName)
	if err != nil {
		return err
	}

	return nil
}

func (handler *azureKeyVaultHandler) GetKeyVaultByID(ctx context.Context, id string) (*keyvault.Vault, error) {
	parsed, err := azresources.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("failed to parse KeyVault resource id: %w", err)
	}

	kvc := clients.NewVaultsClient(handler.arm.SubscriptionID, handler.arm.Auth)

	kv, err := kvc.Get(ctx, parsed.ResourceGroup, parsed.Types[0].Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get KeyVault: %w", err)
	}

	return &kv, nil
}

func (handler *azureKeyVaultHandler) CreateKeyVault(ctx context.Context, vaultName string, options PutOptions) (*keyvault.Vault, error) {
	kvc := clients.NewVaultsClient(handler.arm.SubscriptionID, handler.arm.Auth)

	sc := clients.NewSubscriptionsClient(handler.arm.Auth)

	s, err := sc.Get(ctx, handler.arm.SubscriptionID)
	if err != nil {
		return nil, fmt.Errorf("unable to find subscription: %w", err)
	}
	tenantID, err := uuid.FromString(*s.TenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to convert tenantID to UUID: %w", err)
	}

	location, err := clients.GetResourceGroupLocation(ctx, handler.arm)
	if err != nil {
		return nil, err
	}

	vaultsFuture, err := kvc.CreateOrUpdate(
		ctx,
		handler.arm.ResourceGroup,
		vaultName,
		keyvault.VaultCreateOrUpdateParameters{
			Location: location,
			Properties: &keyvault.VaultProperties{
				TenantID: &tenantID,
				Sku: &keyvault.Sku{
					Family: to.StringPtr("A"),
					Name:   keyvault.Standard,
				},
				EnableRbacAuthorization: to.BoolPtr(true),
			},
			Tags: keys.MakeTagsForRadiusComponent(options.Application, options.Component),
		},
	)

	if err != nil {
		return nil, fmt.Errorf("failed to PUT keyvault: %w", err)
	}

	err = vaultsFuture.WaitForCompletionRef(ctx, kvc.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to PUT keyvault: %w", err)
	}

	kv, err := vaultsFuture.Result(kvc)
	if err != nil {
		return nil, fmt.Errorf("failed to PUT keyvault: %w", err)
	}

	return &kv, nil
}

func (handler *azureKeyVaultHandler) DeleteKeyVault(ctx context.Context, vaultName string) error {
	kvClient := clients.NewVaultsClient(handler.arm.SubscriptionID, handler.arm.Auth)

	_, err := kvClient.Delete(ctx, handler.arm.ResourceGroup, vaultName)
	if err != nil {
		return fmt.Errorf("failed to DELETE keyvault: %w", err)
	}

	return nil
}

func NewAzureKeyVaultHealthHandler(arm armauth.ArmConfig) HealthHandler {
	return &azureKeyVaultHealthHandler{arm: arm}
}

type azureKeyVaultHealthHandler struct {
	arm armauth.ArmConfig
}

func (handler *azureKeyVaultHealthHandler) GetHealthOptions(ctx context.Context) healthcontract.HealthCheckOptions {
	return healthcontract.HealthCheckOptions{}
}
