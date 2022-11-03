// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"context"
	"errors"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/handlers"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
)

const (
	miTestResource = "/subscriptions/testSub/resourcegroups/testGroup/providers/Microsoft.ManagedIdentity/userAssignedIdentities/radius-mi-app"
)

func TestMakeManagedIdentity(t *testing.T) {
	t.Run("invalid-provider", func(t *testing.T) {
		provider := &datamodel.Providers{}
		_, err := MakeManagedIdentity("mi", provider)
		require.Error(t, err)
	})

	t.Run("invalid-scope", func(t *testing.T) {
		provider := &datamodel.Providers{
			Azure: datamodel.ProvidersAzure{
				Scope: "/resourceGroups/test-group",
			},
		}
		_, err := MakeManagedIdentity("mi", provider)
		require.Error(t, err)
	})

	t.Run("valid-scope", func(t *testing.T) {
		provider := &datamodel.Providers{
			Azure: datamodel.ProvidersAzure{
				Scope: "/subscriptions/test-sub-id/resourceGroups/test-group",
			},
		}
		or, err := MakeManagedIdentity("mi", provider)
		require.NoError(t, err)
		require.Equal(t, &outputresource.OutputResource{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureUserAssignedManagedIdentity,
				Provider: resourcemodel.ProviderAzure,
			},
			LocalID:  outputresource.LocalIDUserAssignedManagedIdentity,
			Deployed: false,
			Resource: map[string]string{
				handlers.UserAssignedIdentityNameKey:        "mi",
				handlers.UserAssignedIdentitySubscriptionID: "test-sub-id",
				handlers.UserAssignedIdentityResourceGroup:  "test-group",
			},
		}, or)
	})
}

func TestMakeRoleAssignments(t *testing.T) {
	roleNames := []string{
		"Role1",
		"Role2",
	}

	or, ra := MakeRoleAssignments(miTestResource, roleNames)

	require.Len(t, or, 2)
	require.Len(t, ra, 2)

	require.Equal(t, outputresource.LocalIDUserAssignedManagedIdentity, or[0].Dependencies[0].LocalID)
	require.Equal(t, outputresource.LocalIDUserAssignedManagedIdentity, or[1].Dependencies[0].LocalID)
	require.NotEqual(t, or[0].LocalID, or[1].LocalID)
	require.Equal(t, map[string]string{
		handlers.RoleNameKey:         "Role1",
		handlers.RoleAssignmentScope: miTestResource,
	}, or[0].Resource)
	require.Equal(t, map[string]string{
		handlers.RoleNameKey:         "Role2",
		handlers.RoleAssignmentScope: miTestResource,
	}, or[1].Resource)
}

func TestMakeFederatedIdentitySA(t *testing.T) {
	fi := MakeFederatedIdentitySA("app", "sa", "default", &datamodel.ContainerResource{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				Name: "test-cntr",
				Type: "applications.core/containers",
			},
		},
	})

	putOptions := &handlers.PutOptions{
		Resource: fi,
		DependencyProperties: map[string]map[string]string{
			// output properties of managed identity
			outputresource.LocalIDUserAssignedManagedIdentity: {
				handlers.UserAssignedIdentityClientIDKey: "newClientID",
				handlers.UserAssignedIdentityTenantIDKey: "newTenantID",
			},
		},
	}

	// Transform outputresource
	err := TransformFederatedIdentitySA(context.Background(), putOptions)
	require.NoError(t, err)
	sa := fi.Resource.(*corev1.ServiceAccount)

	require.Equal(t, sa.Annotations[azureWorkloadIdentityClientID], "newClientID")
	require.Equal(t, sa.Annotations[azureWorkloadIdentityTenantID], "newTenantID")
	require.Equal(t, outputresource.LocalIDFederatedIdentity, fi.Dependencies[0].LocalID)
}

func TestTransformFederatedIdentitySA_Validation(t *testing.T) {
	tests := []struct {
		desc     string
		resource any
		dep      map[string]map[string]string
		err      error
	}{
		{
			desc:     "invalid resource",
			resource: struct{}{},
			err:      errors.New("invalid output resource type"),
		},
		{
			desc:     "missing user managed identity",
			resource: &corev1.ServiceAccount{},
			err:      errors.New("cannot find LocalIDUserAssignedManagedIdentity"),
		},
		{
			desc:     "missing client ID",
			resource: &corev1.ServiceAccount{},
			dep: map[string]map[string]string{
				outputresource.LocalIDUserAssignedManagedIdentity: {
					handlers.UserAssignedIdentityTenantIDKey: "tenantID",
				},
			},
			err: errors.New("cannot extract Client ID of user assigned managed identity"),
		},
		{
			desc:     "missing tenant ID",
			resource: &corev1.ServiceAccount{},
			dep: map[string]map[string]string{
				outputresource.LocalIDUserAssignedManagedIdentity: {
					handlers.UserAssignedIdentityClientIDKey: "clientID",
				},
			},
			err: errors.New("cannot extract Tenant ID of user assigned managed identity"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			err := TransformFederatedIdentitySA(context.Background(), &handlers.PutOptions{
				Resource:             &outputresource.OutputResource{Resource: tc.resource},
				DependencyProperties: tc.dep,
			})

			require.ErrorContains(t, err, tc.err.Error())
		})
	}
}

func TestMakeFederatedIdentity(t *testing.T) {
	t.Run("invalid environment option", func(t *testing.T) {
		envOpt := &renderers.EnvironmentOptions{
			Identity: &rp.IdentitySettings{
				Kind: rp.AzureIdentityWorkload,
			},
		}

		_, err := MakeFederatedIdentity("fi", envOpt)
		require.Error(t, err)
	})

	t.Run("valid federated identity", func(t *testing.T) {
		envOpt := &renderers.EnvironmentOptions{
			Namespace: "default",
			Identity: &rp.IdentitySettings{
				Kind:       rp.AzureIdentityWorkload,
				OIDCIssuer: "https://radiusoidc/00000000-0000-0000-0000-000000000000",
			},
		}

		or, err := MakeFederatedIdentity("fi", envOpt)

		require.NoError(t, err)
		require.Equal(t, outputresource.LocalIDFederatedIdentity, or.LocalID)
		require.Equal(t, outputresource.LocalIDUserAssignedManagedIdentity, or.Dependencies[0].LocalID)
		require.Equal(t, map[string]string{
			handlers.FederatedIdentityNameKey:    "fi",
			handlers.FederatedIdentitySubjectKey: "system:serviceaccount:default:fi",
			handlers.FederatedIdentityIssuerKey:  "https://radiusoidc/00000000-0000-0000-0000-000000000000",
		}, or.Resource)
	})
}
