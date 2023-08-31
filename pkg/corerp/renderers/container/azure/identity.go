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

package azure

import (
	"context"
	"errors"
	"fmt"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/handlers"
	"github.com/radius-project/radius/pkg/corerp/renderers"
	"github.com/radius-project/radius/pkg/resourcemodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_azure "github.com/radius-project/radius/pkg/ucp/resources/azure"

	corev1 "k8s.io/api/core/v1"
)

const (
	azureWorkloadIdentityClientID = "azure.workload.identity/client-id"
	azureWorkloadIdentityTenantID = "azure.workload.identity/tenant-id"

	// AzureWorkloadIdentityUseKey represents the key of azure workload identity to enable in Pod and SA.
	// https://azure.github.io/azure-workload-identity/docs/topics/service-account-labels-and-annotations.html?highlight=azure.workload.identity#pod
	AzureWorkloadIdentityUseKey = "azure.workload.identity/use"
)

// MakeManagedIdentity parses the Azure Provider scope and creates an OutputResource with the parsed subscription ID and
// resource group, and the given name. It returns an error if the scope is invalid or if the environment providers are not specified.
func MakeManagedIdentity(name string, cloudProvider *datamodel.Providers) (*rpv1.OutputResource, error) {
	var rID resources.ID
	var err error
	if cloudProvider != nil && cloudProvider.Azure.Scope != "" {
		rID, err = resources.Parse(cloudProvider.Azure.Scope)
		if err != nil || rID.FindScope(resources_azure.ScopeSubscriptions) == "" || rID.FindScope(resources_azure.ScopeResourceGroups) == "" {
			return nil, fmt.Errorf("invalid environment Azure Provider scope: %s", cloudProvider.Azure.Scope)
		}
	} else {
		return nil, errors.New("environment providers are not specified")
	}

	return &rpv1.OutputResource{
		LocalID: rpv1.LocalIDUserAssignedManagedIdentity,
		CreateResource: &rpv1.Resource{
			Data: map[string]string{
				handlers.UserAssignedIdentityNameKey:        name,
				handlers.UserAssignedIdentitySubscriptionID: rID.FindScope(resources_azure.ScopeSubscriptions),
				handlers.UserAssignedIdentityResourceGroup:  rID.FindScope(resources_azure.ScopeResourceGroups),
			},
			ResourceType: resourcemodel.ResourceType{
				Type:     resources_azure.ResourceTypeManagedIdentityUserAssignedManagedIdentity,
				Provider: resourcemodel.ProviderAzure,
			},
		},
	}, nil
}

// MakeRoleAssignments creates OutputResources and Dependencies for each roleName in the roleNames slice, and adds them to
// the outputResources and deps slices respectively.
func MakeRoleAssignments(azResourceID string, roleNames []string) ([]rpv1.OutputResource, []string) {
	deps := []string{}
	outputResources := []rpv1.OutputResource{}
	for _, roleName := range roleNames {
		roleAssignment := rpv1.OutputResource{
			LocalID: rpv1.NewLocalID(rpv1.LocalIDRoleAssignmentPrefix, azResourceID, roleName),
			CreateResource: &rpv1.Resource{
				Data: map[string]string{
					handlers.RoleNameKey:         roleName,
					handlers.RoleAssignmentScope: azResourceID,
				},
				ResourceType: resourcemodel.ResourceType{
					Type:     resources_azure.ResourceTypeAuthorizationRoleAssignment,
					Provider: resourcemodel.ProviderAzure,
				},
				Dependencies: []string{rpv1.LocalIDUserAssignedManagedIdentity},
			},
		}
		deps = append(deps, roleAssignment.LocalID)
		outputResources = append(outputResources, roleAssignment)
	}

	return outputResources, deps
}

// MakeFederatedIdentity creates an OutputResource object with the necessary fields to create a Federated Identity in
// Azure (aka workload identity), and returns an error if the OIDC Issuer URL or namespace is not specified.
func MakeFederatedIdentity(name string, envOpt *renderers.EnvironmentOptions) (*rpv1.OutputResource, error) {
	if envOpt.Identity == nil || envOpt.Identity.OIDCIssuer == "" {
		return nil, errors.New("OIDC Issuer URL is not specified")
	}

	if envOpt.Namespace == "" {
		return nil, errors.New("namespace is not specified")
	}

	subject := handlers.GetKubeAzureSubject(envOpt.Namespace, name)
	return &rpv1.OutputResource{
		LocalID: rpv1.LocalIDFederatedIdentity,
		CreateResource: &rpv1.Resource{
			Data: map[string]string{
				handlers.FederatedIdentityNameKey:    name,
				handlers.FederatedIdentitySubjectKey: subject,
				handlers.FederatedIdentityIssuerKey:  envOpt.Identity.OIDCIssuer,
			},
			ResourceType: resourcemodel.ResourceType{
				Type:     resources_azure.ResourceTypeManagedIdentityUserAssignedManagedIdentityFederatedIdentityCredential,
				Provider: resourcemodel.ProviderAzure,
			},
			Dependencies: []string{rpv1.LocalIDUserAssignedManagedIdentity},
		},
	}, nil
}

// TransformFederatedIdentitySA extracts the identity info from the request and adds it to the ServiceAccount annotations.
func TransformFederatedIdentitySA(ctx context.Context, options *handlers.PutOptions) error {
	sa, ok := options.Resource.CreateResource.Data.(*corev1.ServiceAccount)
	if !ok {
		return errors.New("invalid output resource type")
	}

	clientID, tenantID, err := extractIdentityInfo(options)
	if err != nil {
		return err
	}

	if clientID != "" && tenantID != "" {
		sa.Annotations[azureWorkloadIdentityClientID] = clientID
		sa.Annotations[azureWorkloadIdentityTenantID] = tenantID
	}

	return nil
}

func extractIdentityInfo(options *handlers.PutOptions) (clientID string, tenantID string, err error) {
	mi, ok := options.DependencyProperties[rpv1.LocalIDUserAssignedManagedIdentity]
	if !ok {
		return "", "", nil
	}

	if mi == nil {
		err = errors.New("cannot find LocalIDUserAssignedManagedIdentity")
		return
	}

	clientID = mi[handlers.UserAssignedIdentityClientIDKey]
	if clientID == "" {
		err = errors.New("cannot extract Client ID of user assigned managed identity")
		return
	}
	tenantID = mi[handlers.UserAssignedIdentityTenantIDKey]
	if tenantID == "" {
		err = errors.New("cannot extract Tenant ID of user assigned managed identity")
		return
	}

	return
}

// SetWorkloadIdentityServiceAccount creates a ServiceAccount with descriptive labels and placeholder annotations for Azure Workload
// Identity, and returns an OutputResource with the ServiceAccount and a dependency on the FederatedIdentity.
func SetWorkloadIdentityServiceAccount(base *corev1.ServiceAccount) *rpv1.OutputResource {
	base.ObjectMeta.Labels[AzureWorkloadIdentityUseKey] = "true"
	base.ObjectMeta.Annotations[azureWorkloadIdentityClientID] = "placeholder"
	base.ObjectMeta.Annotations[azureWorkloadIdentityTenantID] = "placeholder"

	or := rpv1.NewKubernetesOutputResource(rpv1.LocalIDServiceAccount, base, base.ObjectMeta)
	or.CreateResource.Dependencies = []string{rpv1.LocalIDFederatedIdentity}

	return &or
}
