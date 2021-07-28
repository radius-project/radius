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
	"github.com/Azure/radius/pkg/azclients"
	"github.com/Azure/radius/pkg/azresources"
	"github.com/Azure/radius/pkg/keys"
	"github.com/Azure/radius/pkg/rad/namegenerator"
	"github.com/Azure/radius/pkg/radrp/armauth"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	radresources "github.com/Azure/radius/pkg/radrp/resources"
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

func (handler *azureKeyVaultHandler) Put(ctx context.Context, options PutOptions) (map[string]string, error) {
	properties := mergeProperties(options.Resource, options.Existing)

	// This assertion is important so we don't start creating/modifying an unmanaged resource
	err := ValidateResourceIDsForUnmanagedResource(properties, KeyVaultIDKey)
	if err != nil {
		return nil, err
	}

	if properties[KeyVaultIDKey] == "" {
		// If we have already created this resource we would have stored the name and ID.
		vaultName := namegenerator.GenerateName("kv")

		kv, err := handler.CreateKeyVault(ctx, vaultName, options)
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
	properties := options.Existing.Properties
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
	parsed, err := radresources.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("failed to parse KeyVault resource id: %w", err)
	}

	kvc := azclients.NewVaultsClient(handler.arm.SubscriptionID, handler.arm.Auth)

	kv, err := kvc.Get(ctx, parsed.ResourceGroup, parsed.Types[0].Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get KeyVault: %w", err)
	}

	return &kv, nil
}

func (handler *azureKeyVaultHandler) CreateKeyVault(ctx context.Context, vaultName string, options PutOptions) (*keyvault.Vault, error) {
	kvc := azclients.NewVaultsClient(handler.arm.SubscriptionID, handler.arm.Auth)

	sc := azclients.NewSubscriptionsClient(handler.arm.Auth)

	s, err := sc.Get(ctx, handler.arm.SubscriptionID)
	if err != nil {
		return nil, fmt.Errorf("unable to find subscription: %w", err)
	}
	tenantID, err := uuid.FromString(*s.TenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to convert tenantID to UUID: %w", err)
	}

	location, err := getResourceGroupLocation(ctx, handler.arm)
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
	kvClient := azclients.NewVaultsClient(handler.arm.SubscriptionID, handler.arm.Auth)

	_, err := kvClient.Delete(ctx, handler.arm.ResourceGroup, vaultName)
	if err != nil {
		return fmt.Errorf("failed to DELETE keyvault: %w", err)
	}

	return nil
}
