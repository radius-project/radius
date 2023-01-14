// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"context"
	"errors"
	"fmt"

	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/handlers"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/project-radius/radius/pkg/ucp/resources"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	azureWorkloadIdentityClientID = "azure.workload.identity/client-id"
	azureWorkloadIdentityTenantID = "azure.workload.identity/tenant-id"
)

// MakeManagedIdentity builds a user-assigned managed identity output resource.
func MakeManagedIdentity(name string, cloudProvider *datamodel.Providers) (*outputresource.OutputResource, error) {
	var rID resources.ID
	var err error
	if cloudProvider != nil && cloudProvider.Azure.Scope != "" {
		rID, err = resources.Parse(cloudProvider.Azure.Scope)
		if err != nil || rID.FindScope(resources.SubscriptionsSegment) == "" || rID.FindScope(resources.ResourceGroupsSegment) == "" {
			return nil, fmt.Errorf("invalid environment Azure Provider scope: %s", cloudProvider.Azure.Scope)
		}
	} else {
		return nil, errors.New("environment providers are not specified")
	}

	return &outputresource.OutputResource{
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.AzureUserAssignedManagedIdentity,
			Provider: resourcemodel.ProviderAzure,
		},
		LocalID:  outputresource.LocalIDUserAssignedManagedIdentity,
		Deployed: false,
		Resource: map[string]string{
			handlers.UserAssignedIdentityNameKey:        name,
			handlers.UserAssignedIdentitySubscriptionID: rID.FindScope(resources.SubscriptionsSegment),
			handlers.UserAssignedIdentityResourceGroup:  rID.FindScope(resources.ResourceGroupsSegment),
		},
	}, nil
}

// MakeRoleAssignments assigns roles/permissions to a specific resource for the managed identity resource.
func MakeRoleAssignments(azResourceID string, roleNames []string) ([]outputresource.OutputResource, []outputresource.Dependency) {
	deps := []outputresource.Dependency{}
	outputResources := []outputresource.OutputResource{}
	for _, roleName := range roleNames {
		roleAssignment := outputresource.OutputResource{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureRoleAssignment,
				Provider: resourcemodel.ProviderAzure,
			},
			LocalID:  outputresource.GenerateLocalIDForRoleAssignment(azResourceID, roleName),
			Deployed: false,
			Resource: map[string]string{
				handlers.RoleNameKey:         roleName,
				handlers.RoleAssignmentScope: azResourceID,
			},
			Dependencies: []outputresource.Dependency{
				{
					LocalID: outputresource.LocalIDUserAssignedManagedIdentity,
				},
			},
		}
		deps = append(deps, outputresource.Dependency{LocalID: roleAssignment.LocalID})
		outputResources = append(outputResources, roleAssignment)
	}

	return outputResources, deps
}

// MakeFederatedIdentity builds azure federated identity (aka workload identity) for User assignmend managed identity.
func MakeFederatedIdentity(name string, envOpt *renderers.EnvironmentOptions) (*outputresource.OutputResource, error) {
	if envOpt.Identity == nil || envOpt.Identity.OIDCIssuer == "" {
		return nil, errors.New("OIDC Issuer URL is not specified")
	}

	if envOpt.Namespace == "" {
		return nil, errors.New("namespace is not specified")
	}

	subject := handlers.GetKubeAzureSubject(envOpt.Namespace, name)
	return &outputresource.OutputResource{
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.AzureFederatedIdentity,
			Provider: resourcemodel.ProviderAzure,
		},
		LocalID:  outputresource.LocalIDFederatedIdentity,
		Deployed: false,
		Resource: map[string]string{
			handlers.FederatedIdentityNameKey:    name,
			handlers.FederatedIdentitySubjectKey: subject,
			handlers.FederatedIdentityIssuerKey:  envOpt.Identity.OIDCIssuer,
		},
		Dependencies: []outputresource.Dependency{
			{
				LocalID: outputresource.LocalIDUserAssignedManagedIdentity,
			},
		},
	}, nil
}

// TransformFederatedIdentitySA mutates Kubernetes ServiceAccount type resource.
func TransformFederatedIdentitySA(ctx context.Context, options *handlers.PutOptions) error {
	sa, ok := options.Resource.Resource.(*corev1.ServiceAccount)
	if !ok {
		return errors.New("invalid output resource type")
	}

	clientID, tenantID, err := extractIdentityInfo(options)
	if err != nil {
		return err
	}

	sa.Annotations[azureWorkloadIdentityClientID] = clientID
	sa.Annotations[azureWorkloadIdentityTenantID] = tenantID

	return nil
}

func extractIdentityInfo(options *handlers.PutOptions) (clientID string, tenantID string, err error) {
	mi := options.DependencyProperties[outputresource.LocalIDUserAssignedManagedIdentity]
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

// MakeFederatedIdentitySA builds service account for the federated identity.
func MakeFederatedIdentitySA(appName, name, namespace string, resource *datamodel.ContainerResource) *outputresource.OutputResource {
	labels := kubernetes.MakeDescriptiveLabels(appName, resource.Name, resource.Type)
	labels["azure.workload.identity/use"] = "true"

	sa := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubernetes.NormalizeResourceName(name),
			Namespace: namespace,
			Labels:    labels,
			Annotations: map[string]string{
				// ResourceTransformer transforms these values before deploying resource.
				azureWorkloadIdentityClientID: "placeholder",
				azureWorkloadIdentityTenantID: "placeholder",
			},
		},
	}

	or := outputresource.NewKubernetesOutputResource(
		resourcekinds.ServiceAccount,
		outputresource.LocalIDServiceAccount,
		sa,
		sa.ObjectMeta)

	or.Dependencies = []outputresource.Dependency{
		{
			LocalID: outputresource.LocalIDFederatedIdentity,
		},
	}

	return &or
}
