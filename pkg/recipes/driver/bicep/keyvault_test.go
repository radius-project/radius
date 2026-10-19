/*
Copyright 2026 The Radius Authors.

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

package bicep

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/recipes/driver"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_Bicep_Delete_KeyVaultSecrets(t *testing.T) {
	const vaultID = "/subscriptions/test-sub/resourceGroups/test-rg/providers/Microsoft.KeyVault/vaults/old-vault"
	tests := []struct {
		name         string
		vaultID      string
		childID      string
		managedVault *bool
		cascade      bool
		vaultError   error
	}{
		{
			name:    "managed vault deletes its secrets",
			vaultID: vaultID, childID: vaultID + "/secrets/password",
			managedVault: new(true), cascade: true,
		},
		{
			name:    "Azure qualified IDs",
			vaultID: "/planes/azure/local" + vaultID, childID: "/planes/azure/local" + vaultID + "/secrets/password",
			managedVault: new(true), cascade: true,
		},
		{
			name:    "Azure ID matching is case insensitive",
			vaultID: strings.ToUpper(vaultID), childID: vaultID + "/secrets/password",
			managedVault: new(true), cascade: true,
		},
		{
			name:    "parent failure is not success",
			vaultID: vaultID, childID: vaultID + "/secrets/password",
			managedVault: new(true), cascade: true, vaultError: errors.New("vault deletion denied"),
		},
		{
			name:    "unmanaged parent cannot delete secrets",
			vaultID: vaultID, childID: vaultID + "/secrets/password",
			managedVault: new(false),
		},
		{
			name:    "parent with no ownership cannot delete secrets",
			vaultID: vaultID, childID: vaultID + "/secrets/password",
		},
		{
			name:    "other vault does not delete secrets",
			vaultID: vaultID + "-other", childID: vaultID + "/secrets/password",
			managedVault: new(true),
		},
		{
			name:    "other resource group does not delete secrets",
			vaultID: strings.Replace(vaultID, "test-rg", "other-rg", 1), childID: vaultID + "/secrets/password",
			managedVault: new(true),
		},
		{
			name:    "other subscription does not delete secrets",
			vaultID: strings.Replace(vaultID, "test-sub", "other-sub", 1), childID: vaultID + "/secrets/password",
			managedVault: new(true),
		},
		{
			name:         "non-Azure plane cannot cascade",
			vaultID:      "/planes/radius/local/resourceGroups/test-rg/providers/Microsoft.KeyVault/vaults/vault",
			childID:      "/planes/radius/local/resourceGroups/test-rg/providers/Microsoft.KeyVault/vaults/vault/secrets/password",
			managedVault: new(true),
		},
		{
			name:    "other child types keep their deletion",
			vaultID: vaultID, childID: vaultID + "/keys/key",
			managedVault: new(true),
		},
		{
			name:    "extension resource is not a vault secret",
			vaultID: vaultID, childID: vaultID + "/providers/Microsoft.KeyVault/vaults/extension/secrets/password",
			managedVault: new(true),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d, client, _, _ := setupDeleteInputs(t)
			d.options.DeleteRetryDelaySeconds = 0
			if tt.managedVault != nil && *tt.managedVault {
				client.EXPECT().Delete(gomock.Any(), resources.MustParse(tt.vaultID).String()).Return(tt.vaultError)
			}
			if !tt.cascade {
				client.EXPECT().Delete(gomock.Any(), tt.childID).Return(nil)
			}

			err := d.Delete(t.Context(), driver.DeleteOptions{
				OutputResources: []rpv1.OutputResource{
					{ID: resources.MustParse(tt.childID), RadiusManaged: new(true)},
					{ID: resources.MustParse(tt.vaultID), RadiusManaged: tt.managedVault},
				},
			})
			if tt.vaultError != nil {
				require.ErrorContains(t, err, tt.vaultError.Error())
				return
			}
			require.NoError(t, err)
		})
	}
}

func Test_Bicep_Delete_KeyVaultDeclarativeInventoryWaitsForVault(t *testing.T) {
	const vaultID = "/subscriptions/test-sub/resourceGroups/test-rg/providers/Microsoft.KeyVault/vaults/test-vault"
	const identityID = "/subscriptions/test-sub/resourceGroups/test-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/secrets"
	d, client, _, _ := setupDeleteInputs(t)
	outputs, err := d.getGCOutputResources(nil, []string{
		vaultID + "/secrets/username",
		vaultID + "/secrets/password",
		vaultID,
		identityID,
	})
	require.NoError(t, err)

	started := make(chan struct{})
	finish := make(chan struct{})
	t.Cleanup(func() { close(finish) })
	client.EXPECT().Delete(gomock.Any(), vaultID).DoAndReturn(func(context.Context, string) error {
		close(started)
		<-finish
		return nil
	})
	client.EXPECT().Delete(gomock.Any(), identityID).Return(nil)
	done := make(chan error, 1)
	go func() {
		done <- d.Delete(t.Context(), driver.DeleteOptions{OutputResources: outputs})
	}()

	<-started
	select {
	case err := <-done:
		t.Fatalf("deletion returned before vault deletion finished: %v", err)
	default:
	}
	finish <- struct{}{}
	require.NoError(t, <-done)
}

func Test_Bicep_Delete_KeyVaultRetainedDuringUpdateDoesNotSkipSecret(t *testing.T) {
	const vaultID = "/subscriptions/test-sub/resourceGroups/test-rg/providers/Microsoft.KeyVault/vaults/test-vault"
	const secretID = vaultID + "/secrets/password"
	d, client, _, _ := setupDeleteInputs(t)
	d.options.DeleteRetryDelaySeconds = 0
	outputs, err := d.getGCOutputResources([]string{vaultID}, []string{secretID, vaultID})
	require.NoError(t, err)
	client.EXPECT().Delete(gomock.Any(), secretID).Return(errors.New("405 DeleteNotSupported"))

	err = d.Delete(t.Context(), driver.DeleteOptions{OutputResources: outputs})
	require.ErrorContains(t, err, "405 DeleteNotSupported")
}
