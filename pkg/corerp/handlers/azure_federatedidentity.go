// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/msi/mgmt/msi"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/azure/clientv2"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

const (
	FederatedIdentityNameKey    = "federatedidentityname"
	FederatedIdentityIssuerKey  = "federatedidentityissuer"
	FederatedIdentitySubjectKey = "federatedidentitysubject"
	FederatedIdentityAudience   = "api://AzureADTokenExchange"
)

// NewAzFederatedIdentity initializes a new handler for federated identity resource.
func NewAzFederatedIdentity(arm *armauth.ArmConfig) ResourceHandler {
	return &azFederatedIdentityHandler{arm: arm}
}

type azFederatedIdentityHandler struct {
	arm *armauth.ArmConfig
}

func (handler *azFederatedIdentityHandler) Put(ctx context.Context, resource *outputresource.OutputResource) error {
	logger := radlogger.GetLogger(ctx)
	resource_identity, err := handler.GetResourceIdentity(ctx, *resource)
	if err != nil {
		return err
	}

	resource.Identity = resource_identity
	id := resource_identity.Data.(resourcemodel.ARMIdentity)
	logger.WithValues(
		radlogger.LogFieldResourceID, id,
		radlogger.LogFieldLocalID, outputresource.LocalIDUserAssignedManagedIdentity).Info("Created managed identity for KeyVault access")

	return nil
}

func (handler *azFederatedIdentityHandler) GetResourceIdentity(ctx context.Context, resource outputresource.OutputResource) (resourcemodel.ResourceIdentity, error) {
	properties, err := handler.GetResourceNativeIdentityKeyProperties(ctx, resource)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, err
	}

	identity := resourcemodel.NewARMIdentity(&resource.ResourceType, properties[UserAssignedIdentityIDKey], clients.GetAPIVersionFromUserAgent(msi.UserAgent()))

	return identity, nil
}

func (handler *azFederatedIdentityHandler) GetResourceNativeIdentityKeyProperties(ctx context.Context, resource outputresource.OutputResource) (map[string]string, error) {
	properties, ok := resource.Resource.(map[string]string)
	if !ok {
		return properties, fmt.Errorf("invalid required properties for resource")
	}

	identityID := properties[UserAssignedIdentityNameKey]
	rID, err := resources.ParseResource(identityID)
	if err != nil {
		return nil, err
	}

	client, err := clientv2.NewFederatedIdentityClient(rID.FindScope(resources.SubscriptionsSegment), &handler.arm.ClientOption)
	if err != nil {
		return nil, err
	}

	federatedName, ok := properties[FederatedIdentityNameKey]
	if !ok {
		return nil, errors.New("invalid federated name")
	}
	audience, ok := properties[FederatedIdentityAudience]
	if !ok {
		return nil, errors.New("invalid audience")
	}
	issuer, ok := properties[FederatedIdentityIssuerKey]
	if !ok {
		return nil, errors.New("invalid issuer url")
	}
	subject, ok := properties[FederatedIdentitySubjectKey]
	if !ok {
		return nil, errors.New("invalid subject")
	}

	params := armmsi.FederatedIdentityCredential{
		Properties: &armmsi.FederatedIdentityCredentialProperties{
			Audiences: []*string{&audience},
			Issuer:    &issuer,
			Subject:   &subject,
		},
	}

	_, err = client.CreateOrUpdate(ctx, rID.FindScope(resources.ResourceGroupsSegment), rID.Name(), federatedName, params, nil)
	if err != nil {
		return nil, err
	}

	return properties, nil
}

func (handler *azFederatedIdentityHandler) Delete(ctx context.Context, resource outputresource.OutputResource) error {
	properties, ok := resource.Resource.(map[string]string)
	if !ok {
		return fmt.Errorf("invalid required properties for resource")
	}

	identityID := properties[UserAssignedIdentityNameKey]
	rID, err := resources.ParseResource(identityID)
	if err != nil {
		return err
	}

	federatedName, ok := properties[FederatedIdentityNameKey]
	if !ok {
		return errors.New("invalid audience")
	}
	client, err := clientv2.NewFederatedIdentityClient(rID.FindScope(resources.SubscriptionsSegment), &handler.arm.ClientOption)
	if err != nil {
		return err
	}

	_, err = client.Delete(ctx, rID.FindScope(resources.ResourceGroupsSegment), rID.Name(), federatedName, nil)
	return err
}
