// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clientv2"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

const (
	// AzureIdentityTypeKey is the key to represent azure identity type.
	AzureIdentityTypeKey = "AzureIdentityType"
	// AzureIdentityIDKey is the key to represent azure identity resource id.
	AzureIdentityIDKey = "AzureIdentityID"
	// AzureIdentityClientIDKey is the key to represent the client id of identity.
	AzureIdentityClientIDKey = "clientID"
	// AzureIdentityTenantIDKey is the key to represent the tenant id of identity.
	AzureIdentityTenantIDKey = "tenantID"
	// AzureFederatedIdentityAudience represents the Azure AD OIDC target audience.
	AzureFederatedIdentityAudience = "api://AzureADTokenExchange"

	// FederatedIdentityNameKey is the key to represent the federated identity credential name (aka workload identity).
	FederatedIdentityNameKey = "federatedidentityname"
	// FederatedIdentityIssuerKey is the key to represent the oidc issuer.
	FederatedIdentityIssuerKey = "federatedidentityissuer"
	// FederatedIdentitySubjectKey is the key to represent the identity subject.
	FederatedIdentitySubjectKey = "federatedidentitysubject"
)

var (
	ErrInvalidIdentity = errors.New("invalid identity property")
)

// GetKubeAzureSubject constructs the federated identity subject with Kuberenetes namespace and service account name.
func GetKubeAzureSubject(namespace, saName string) string {
	return fmt.Sprintf("system:serviceaccount:%s:%s", namespace, saName)
}

// NewAzureFederatedIdentity initializes a new handler for federated identity resource.
func NewAzureFederatedIdentity(arm *armauth.ArmConfig) ResourceHandler {
	return &azureFederatedIdentityHandler{arm: arm}
}

type azureFederatedIdentityHandler struct {
	arm *armauth.ArmConfig
}

// Put creates or updates the federated identity resource of the azure identity.
func (handler *azureFederatedIdentityHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	logger := radlogger.GetLogger(ctx)

	rs := options.Resource.Resource

	federatedName, err := GetString(rs, FederatedIdentityNameKey)
	if err != nil {
		return nil, err
	}
	issuer, err := GetString(rs, FederatedIdentityIssuerKey)
	if err != nil {
		return nil, err
	}
	subject, err := GetString(rs, FederatedIdentitySubjectKey)
	if err != nil {
		return nil, err
	}
	identityID, err := GetString(rs, UserAssignedIdentityNameKey)
	if err != nil {
		return nil, err
	}

	_, err = resources.ParseResource(identityID)
	if err != nil {
		return nil, err
	}

	identity, ok := options.Resource.Identity.Data.(resourcemodel.AzureFederatedIdentity)
	if !ok {
		return nil, ErrInvalidIdentity
	}

	params := armmsi.FederatedIdentityCredential{
		Properties: &armmsi.FederatedIdentityCredentialProperties{
			Audiences: []*string{to.Ptr(AzureFederatedIdentityAudience)},
			Issuer:    to.Ptr(identity.OIDCIssuer),
			Subject:   to.Ptr(identity.Subject),
		},
	}

	rID, err := resources.ParseResource(identity.Resource)
	if err != nil {
		return nil, err
	}

	subID := rID.FindScope(resources.SubscriptionsSegment)
	rgName := rID.FindScope(resources.ResourceGroupsSegment)

	client, err := clientv2.NewFederatedIdentityClient(subID, &handler.arm.ClientOption)
	if err != nil {
		return nil, err
	}

	// Populating the federated identity credendial changes takes some time. Therefore, POD will take some time to start.
	// User will encounter the error when the given identity has more than 20 federated identity credentials.
	_, err = client.CreateOrUpdate(ctx, rgName, rID.Name(), identity.Name, params, nil)
	if err != nil {
		return nil, err
	}

	// WORKAROUND: Ensure that federal identity credential is populated. (Why not they provide async api?)
	_, err = client.Get(ctx, rgName, rID.Name(), identity.Name, nil)
	if err != nil {
		return nil, err
	}

	options.Resource.Identity = resourcemodel.ResourceIdentity{
		ResourceType: &resourcemodel.ResourceType{
			Type:     resourcekinds.AzureFederatedIdentity,
			Provider: resourcemodel.ProviderAzure,
		},
		Data: resourcemodel.AzureFederatedIdentity{
			Resource:   identityID,
			OIDCIssuer: issuer,
			Subject:    subject,
			Audience:   AzureFederatedIdentityAudience,
			Name:       federatedName,
		},
	}

	logger.WithValues(
		radlogger.LogFieldResourceID, identity,
		radlogger.LogFieldLocalID, outputresource.LocalIDFederatedIdentity).Info("Created federated identity for Azure AD identity.")

	return map[string]string{}, nil
}

func (handler *azureFederatedIdentityHandler) Delete(ctx context.Context, options *DeleteOptions) error {
	identityID, err := GetString(options.Resource.Identity.Data, "resource")
	if err != nil {
		return err
	}
	name, err := GetString(options.Resource.Identity.Data, "name")
	if err != nil {
		return err
	}

	rID, err := resources.ParseResource(identityID)
	if err != nil {
		return err
	}

	client, err := clientv2.NewFederatedIdentityClient(rID.FindScope(resources.SubscriptionsSegment), &handler.arm.ClientOption)
	if err != nil {
		return err
	}

	_, err = client.Delete(ctx, rID.FindScope(resources.ResourceGroupsSegment), rID.Name(), name, nil)
	return err
}
