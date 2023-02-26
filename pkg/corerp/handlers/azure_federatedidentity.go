// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"

	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clientv2"
	"github.com/project-radius/radius/pkg/logging"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

const (
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
	// Federated identity is still in preview.
	// The below regions are not supported.
	// Reference: https://learn.microsoft.com/en-us/azure/active-directory/develop/workload-identity-federation-considerations#unsupported-regions-user-assigned-managed-identities
	federatedUnsupportedRegions = []string{
		"Germany North",
		"Sweden South",
		"Sweden Central",
		"Switzerland West",
		"Brazil Southeast",
		"East Asia",
		"Southeast Asia",
		"Switzerland West",
		"South Africa West",
		"Qatar Central",
		"Australia Central",
		"Australia Central2",
		"Norway West",
	}
)

func isFederatedIdentitySupported(region string) bool {
	for _, r := range federatedUnsupportedRegions {
		if strings.EqualFold(r, region) {
			return false
		}
	}
	return true
}

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
	logger := ucplog.FromContext(ctx)

	// Get dependencies
	identityProp, ok := options.DependencyProperties[rpv1.LocalIDUserAssignedManagedIdentity]
	if !ok {
		return nil, errors.New("missing dependency: a user assigned identity is required to create role assignment")
	}

	identityID, err := GetMapValue[string](identityProp, UserAssignedIdentityIDKey)
	if err != nil {
		return nil, errors.New("fails to get identity resource id")
	}

	rs := options.Resource.Resource
	federatedName, err := GetMapValue[string](rs, FederatedIdentityNameKey)
	if err != nil {
		return nil, err
	}
	issuer, err := GetMapValue[string](rs, FederatedIdentityIssuerKey)
	if err != nil {
		return nil, err
	}
	subject, err := GetMapValue[string](rs, FederatedIdentitySubjectKey)
	if err != nil {
		return nil, err
	}

	_, err = resources.ParseResource(identityID)
	if err != nil {
		return nil, err
	}

	params := armmsi.FederatedIdentityCredential{
		Properties: &armmsi.FederatedIdentityCredentialProperties{
			Audiences: []*string{to.Ptr(AzureFederatedIdentityAudience)},
			Issuer:    to.Ptr(issuer),
			Subject:   to.Ptr(subject),
		},
	}

	rID, err := resources.ParseResource(identityID)
	if err != nil {
		return nil, err
	}

	subID := rID.FindScope(resources.SubscriptionsSegment)
	rgName := rID.FindScope(resources.ResourceGroupsSegment)

	client, err := clientv2.NewFederatedIdentityClient(subID, &handler.arm.ClientOptions)
	if err != nil {
		return nil, err
	}

	// Populating the federated identity credential changes takes some time. Therefore, POD will take some time to start.
	_, err = client.CreateOrUpdate(ctx, rgName, rID.Name(), federatedName, params, nil)
	if err != nil {
		return nil, err
	}

	// WORKAROUND: Ensure that federal identity credential is populated. (Why not they provide async api?)
	_, err = client.Get(ctx, rgName, rID.Name(), federatedName, nil)
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
		logging.LogFieldResourceID, identityID,
		logging.LogFieldLocalID, rpv1.LocalIDFederatedIdentity).Info("Created federated identity for Azure AD identity.")

	return map[string]string{}, nil
}

// Delete deletes the federated identity credential.
func (handler *azureFederatedIdentityHandler) Delete(ctx context.Context, options *DeleteOptions) error {
	fi := &resourcemodel.AzureFederatedIdentity{}
	if err := store.DecodeMap(options.Resource.Identity.Data, fi); err != nil {
		return err
	}

	rID, err := resources.ParseResource(fi.Resource)
	if err != nil {
		return err
	}

	subscriptionID := rID.FindScope(resources.SubscriptionsSegment)

	client, err := clientv2.NewFederatedIdentityClient(subscriptionID, &handler.arm.ClientOptions)
	if err != nil {
		return err
	}

	_, err = client.Delete(ctx, rID.FindScope(resources.ResourceGroupsSegment), rID.Name(), fi.Name, nil)
	return err
}
