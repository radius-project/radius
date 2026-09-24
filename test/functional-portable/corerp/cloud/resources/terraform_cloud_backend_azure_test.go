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

package resource_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources/v3"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage/v4"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/google/uuid"
	"github.com/radius-project/radius/test/step"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	azureProvisioningTimeout = 10 * time.Minute
	azurePropagationTimeout  = 10 * time.Minute
)

func newAzureBackendFixture(ctx context.Context, t *testing.T, name string) cloudBackendFixture {
	t.Helper()
	provisionCtx, cancelProvision := context.WithTimeout(ctx, azureProvisioningTimeout)
	defer cancelProvision()
	subscription := requiredCloudEnv(t, "AZURE_SUBSCRIPTION_ID")
	group := requiredCloudEnv(t, "INTEGRATION_TEST_RESOURCE_GROUP_NAME")
	location := requiredCloudEnv(t, "AZURE_LOCATION")
	credential, err := azidentity.NewDefaultAzureCredential(nil)
	require.NoError(t, err)
	accounts, err := armstorage.NewAccountsClient(subscription, credential, nil)
	require.NoError(t, err)
	containers, err := armstorage.NewBlobContainersClient(subscription, credential, nil)
	require.NoError(t, err)
	roles, err := armauthorization.NewRoleAssignmentsClient(subscription, credential, nil)
	require.NoError(t, err)
	groups, err := armresources.NewResourceGroupsClient(subscription, credential, nil)
	require.NoError(t, err)
	account := "tf" + strings.ReplaceAll(uuid.NewString(), "-", "")[:20]
	poller, err := accounts.BeginCreate(provisionCtx, group, account, armstorage.AccountCreateParameters{
		Kind: new(armstorage.KindStorageV2), Location: new(location),
		SKU: &armstorage.SKU{Name: new(armstorage.SKUNameStandardLRS)},
		Properties: &armstorage.AccountPropertiesCreateParameters{
			AllowBlobPublicAccess: new(false), AllowSharedKeyAccess: new(false),
			EnableHTTPSTrafficOnly: new(true), MinimumTLSVersion: new(armstorage.MinimumTLSVersionTLS12),
		},
		Tags: map[string]*string{"radiustest": new("terraform-cloud-backend")},
	}, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), cloudBackendTimeout)
		defer cancel()
		// Finish any outstanding allocation before deleting its unique account.
		if !poller.Done() {
			if _, err := poller.PollUntilDone(cleanupCtx, nil); err != nil {
				t.Errorf("finish test storage account allocation: %v", err)
			}
		}
		_, err := accounts.Delete(cleanupCtx, group, account, nil)
		if err != nil && !azureNotFound(err) {
			t.Errorf("delete test storage account: %v", err)
		}
	})
	accountResponse, err := poller.PollUntilDone(provisionCtx, nil)
	require.NoError(t, err)
	require.NotNil(t, accountResponse.ID)
	t.Logf("Test-owned Terraform state storage account: %s", *accountResponse.ID)
	const container = "tfstate"
	_, err = containers.Create(provisionCtx, group, account, container, armstorage.BlobContainer{
		ContainerProperties: &armstorage.ContainerProperties{PublicAccess: new(armstorage.PublicAccessNone)},
	}, nil)
	require.NoError(t, err)

	// CI's runner and Radius WI use the same Entra application. Existing CI RBAC
	// administrator permission permits this container-scoped, temporary grant.
	token, err := credential.GetToken(provisionCtx, policy.TokenRequestOptions{Scopes: []string{"https://management.azure.com/.default"}})
	require.NoError(t, err)
	principal, err := azureTokenPrincipal(token.Token)
	require.NoError(t, err)
	scope := *accountResponse.ID + "/blobServices/default/containers/" + container
	assignment := uuid.NewString()
	_, err = roles.Create(provisionCtx, scope, assignment, armauthorization.RoleAssignmentCreateParameters{
		Properties: &armauthorization.RoleAssignmentProperties{
			PrincipalID: new(principal), PrincipalType: new(armauthorization.PrincipalTypeServicePrincipal),
			RoleDefinitionID: new("/subscriptions/" + subscription + "/providers/Microsoft.Authorization/roleDefinitions/ba92f5b4-2d11-453d-a403-e96b0029c9fe"),
		},
	}, nil)
	require.NoError(t, err, "grant Storage Blob Data Contributor on the test container")
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), cloudBackendTimeout)
		defer cancel()
		_, err := roles.Delete(cleanupCtx, scope, assignment, nil)
		if err != nil && !azureNotFound(err) {
			t.Errorf("delete test container role assignment: %v", err)
		}
	})
	blobClient, err := azblob.NewClient("https://"+account+".blob.core.windows.net/", credential, nil)
	require.NoError(t, err)
	cancelProvision()
	propagationCtx, cancelPropagation := context.WithTimeout(ctx, azurePropagationTimeout)
	defer cancelPropagation()
	err = waitForAzureBlobAccess(propagationCtx, func(ctx context.Context) error {
		_, err := blobClient.ServiceClient().NewContainerClient(container).GetProperties(ctx, nil)
		return err
	})
	require.NoError(t, err)

	return cloudBackendFixture{
		settings: map[string]any{
			"type": "azurerm", "storageAccountName": account, "containerName": container, "keyPrefix": "e2e/" + name,
		},
		prefix:       "e2e/" + name,
		providers:    map[string]any{"azure": map[string]any{"subscriptionId": subscription, "resourceGroupName": group}},
		parameters:   map[string]any{"location": location},
		resourceType: "azurerm_resource_group",
		resourceID:   func(name string) string { return "/subscriptions/" + subscription + "/resourceGroups/" + name },
		read: func(ctx context.Context, key string) ([]byte, error) {
			response, err := blobClient.DownloadStream(ctx, container, key, nil)
			if err != nil {
				return nil, err
			}
			return readCloudStateBody(response.Body)
		},
		keys: func(ctx context.Context) ([]string, error) {
			var keys []string
			pager := blobClient.NewListBlobsFlatPager(container, nil)
			for pager.More() {
				page, err := pager.NextPage(ctx)
				if err != nil {
					return nil, err
				}
				for _, blob := range page.Segment.BlobItems {
					keys = append(keys, *blob.Name)
				}
			}
			return keys, nil
		},
		verifyObject: func(ctx context.Context, name, revision string) (bool, error) {
			response, err := groups.Get(ctx, name, nil)
			if azureNotFound(err) {
				return revision == "", nil
			}
			if err != nil {
				return false, err
			}
			tag := response.Tags["revision"]
			return revision != "" && tag != nil && *tag == revision, nil
		},
		cleanupObject: func(ctx context.Context, name string) error {
			poller, err := groups.BeginDelete(ctx, name, nil)
			if azureNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			_, err = poller.PollUntilDone(ctx, nil)
			return err
		},
	}
}

func waitForAzureBlobAccess(ctx context.Context, probe func(context.Context) error) error {
	lastObservation := "no completed request"
	err := wait.PollUntilContextCancel(ctx, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		err := probe(ctx)
		if contextErr := ctx.Err(); contextErr != nil {
			return false, contextErr
		}
		if err == nil {
			return true, nil
		}
		var retry bool
		retry, lastObservation = azureBlobReadinessError(err)
		if retry {
			return false, nil
		}
		if errors.Is(err, context.Canceled) {
			return false, context.Canceled
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return false, context.DeadlineExceeded
		}
		return false, errors.New("non-retryable Blob readiness error")
	})
	if err != nil {
		return fmt.Errorf("waiting for new Azure container access (last observation: %s): %w", lastObservation, err)
	}
	return nil
}

// Only the new-account permission probe uses this classification. Diagnostics
// deliberately exclude raw SDK messages, response bodies, URLs and tokens.
func azureBlobReadinessError(err error) (bool, string) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false, "request context ended"
	}
	var response *azcore.ResponseError
	if errors.As(err, &response) {
		description := fmt.Sprintf("HTTP %d", response.StatusCode)
		switch response.StatusCode {
		case 403:
			return response.ErrorCode == "AuthorizationPermissionMismatch" || response.ErrorCode == "AuthorizationFailure", description
		case 408, 429, 500, 502, 503, 504:
			return true, description
		default:
			return false, description
		}
	}
	var dnsError *net.DNSError
	if errors.As(err, &dnsError) {
		return dnsError.IsNotFound || dnsError.IsTimeout || dnsError.IsTemporary, "DNS resolution failed"
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return true, "network timeout"
	}
	if errors.Is(err, io.EOF) || errors.Is(err, syscall.ECONNREFUSED) || step.IsTransientConnectionError(err) {
		return true, "transient connection failure"
	}
	return false, fmt.Sprintf("error type %T", err)
}

// The token comes directly from the SDK credential, not caller-supplied input.
// Only extract its object ID; never log or persist the token or payload.
func azureTokenPrincipal(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", errors.New("Azure SDK returned a non-JWT token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", errors.New("Azure SDK returned an invalid JWT payload")
	}
	var claims struct {
		ObjectID string `json:"oid"`
	}
	if json.Unmarshal(payload, &claims) != nil {
		return "", errors.New("Azure SDK token claims could not be decoded")
	}
	if _, err := uuid.Parse(claims.ObjectID); err != nil {
		return "", errors.New("Azure SDK token has no valid principal object ID")
	}
	return claims.ObjectID, nil
}
