// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/keyvault/mgmt/keyvault"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/subscriptions"
	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/Azure/radius/pkg/curp/armauth"
	radresources "github.com/Azure/radius/pkg/curp/resources"
	"github.com/Azure/radius/pkg/rad/namegenerator"
	"github.com/gofrs/uuid"
)

const (
	KeyVaultURIKey  = "uri"
	KeyVaultNameKey = "keyvaultname"
)

func NewAzureKeyVaultHandler(arm armauth.ArmConfig) ResourceHandler {
	return &azureKeyVaultHandler{arm: arm}
}

type azureKeyVaultHandler struct {
	arm armauth.ArmConfig
}

func (handler *azureKeyVaultHandler) Put(ctx context.Context, options PutOptions) (map[string]string, error) {
	properties := mergeProperties(options.Resource, options.Existing)

	// If we have already created this resource we would have stored the name.
	vaultName, ok := properties[KeyVaultNameKey]
	if !ok {
		// No name stored, generate a new one
		vaultName = namegenerator.GenerateName("kv")
	}

	rgc := resources.NewGroupsClient(handler.arm.SubscriptionID)
	rgc.Authorizer = handler.arm.Auth

	g, err := rgc.Get(ctx, handler.arm.ResourceGroup)
	if err != nil {
		return nil, fmt.Errorf("failed to PUT keyvault: %w", err)
	}

	kvc := keyvault.NewVaultsClient(handler.arm.SubscriptionID)
	kvc.Authorizer = handler.arm.Auth
	if err != nil {
		return nil, err
	}

	sc := subscriptions.NewClient()
	sc.Authorizer = handler.arm.Auth
	s, err := sc.Get(ctx, handler.arm.SubscriptionID)
	if err != nil {
		return nil, fmt.Errorf("unable to find subscription: %w", err)
	}
	tenantID, err := uuid.FromString(*s.TenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to convert tenantID to UUID: %w", err)
	}

	vaultsFuture, err := kvc.CreateOrUpdate(
		ctx,
		handler.arm.ResourceGroup,
		vaultName,
		keyvault.VaultCreateOrUpdateParameters{
			Location: g.Location,
			Properties: &keyvault.VaultProperties{
				TenantID: &tenantID,
				Sku: &keyvault.Sku{
					Family: to.StringPtr("A"),
					Name:   keyvault.Standard,
				},
				EnableRbacAuthorization: to.BoolPtr(true),
			},
			Tags: map[string]*string{
				radresources.TagRadiusApplication: &options.Application,
			},
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

	// store vault so we can use later
	properties[KeyVaultNameKey] = *kv.Name

	return properties, nil
}

func (handler *azureKeyVaultHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.Existing.Properties
	vaultName := properties[KeyVaultNameKey]

	kvClient := keyvault.NewVaultsClient(handler.arm.SubscriptionID)
	kvClient.Authorizer = handler.arm.Auth

	_, err := kvClient.Delete(ctx, handler.arm.ResourceGroup, vaultName)
	if err != nil {
		return fmt.Errorf("failed to DELETE keyvault: %w", err)
	}

	return nil
}
