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
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"
	"uuid"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources/v3"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage/v4"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/radius-project/radius/test/step"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	azureProvisioningTimeout = 10 * time.Minute
	azurePropagationTimeout  = 10 * time.Minute
	// azureReadinessProbeKey is written and removed by the data-plane readiness probe. It is
	// deliberately outside any state prefix so a surviving object would be obvious.
	azureReadinessProbeKey = "radius-readiness-probe"
)

func newAzureBackendFixture(ctx context.Context, t *testing.T, name string) cloudBackendFixture {
	t.Helper()
	provisionCtx, cancelProvision := context.WithTimeout(ctx, azureProvisioningTimeout)
	defer cancelProvision()
	subscription := requiredCloudEnv(t, "AZURE_SUBSCRIPTION_ID")
	group := requiredCloudEnv(t, "INTEGRATION_TEST_RESOURCE_GROUP_NAME")
	location := requiredCloudEnv(t, "AZURE_LOCATION")
	credential := azureTestCredential(provisionCtx, t)
	accounts, err := armstorage.NewAccountsClient(subscription, credential, nil)
	require.NoError(t, err)
	containers, err := armstorage.NewBlobContainersClient(subscription, credential, nil)
	require.NoError(t, err)
	roles, err := armauthorization.NewRoleAssignmentsClient(subscription, credential, nil)
	require.NoError(t, err)
	groups, err := armresources.NewResourceGroupsClient(subscription, credential, nil)
	require.NoError(t, err)
	account := "tf" + strings.ReplaceAll(uuid.New().String(), "-", "")[:20]
	var poller *runtime.Poller[armstorage.AccountsClientCreateResponse]
	// Registered before the account is requested. Azure can accept the allocation and still return
	// an error, so registering after require.NoError below would leak the account on that path.
	// The account name is unique to this test, so deleting by name is safe even if it never existed.
	t.Cleanup(func() {
		base := context.WithoutCancel(t.Context())
		// Finish any outstanding allocation before deleting its unique account. Polling and
		// deletion get separate budgets, otherwise a slow allocation consumes the whole deadline
		// and the delete below is issued with an already-expired context.
		if poller != nil && !poller.Done() {
			pollCtx, cancelPoll := context.WithTimeout(base, cloudBackendTimeout)
			_, err := poller.PollUntilDone(pollCtx, nil)
			cancelPoll()
			if err != nil {
				t.Errorf("finish test storage account allocation: %v", err)
			}
		}
		cleanupCtx, cancel := context.WithTimeout(base, cloudBackendTimeout)
		defer cancel()
		_, err := accounts.Delete(cleanupCtx, group, account, nil)
		if err != nil && !isNotFoundResponse(err) {
			t.Errorf("delete test storage account: %v", err)
		}
	})
	poller, err = accounts.BeginCreate(provisionCtx, group, account, armstorage.AccountCreateParameters{
		Kind: new(armstorage.KindStorageV2), Location: new(location),
		SKU: &armstorage.SKU{Name: new(armstorage.SKUNameStandardLRS)},
		Properties: &armstorage.AccountPropertiesCreateParameters{
			AllowBlobPublicAccess: new(false), AllowSharedKeyAccess: new(false),
			EnableHTTPSTrafficOnly: new(true), MinimumTLSVersion: new(armstorage.MinimumTLSVersionTLS12),
		},
		Tags: map[string]*string{"radiustest": new("terraform-cloud-backend")},
	}, nil)
	require.NoError(t, err)
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
	assignment := uuid.New().String()
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
		if err != nil && !isNotFoundResponse(err) {
			t.Errorf("delete test container role assignment: %v", err)
		}
	})
	blobClient, err := azblob.NewClient("https://"+account+".blob.core.windows.net/", credential, nil)
	require.NoError(t, err)
	cancelProvision()
	propagationCtx, cancelPropagation := context.WithTimeout(ctx, azurePropagationTimeout)
	defer cancelPropagation()
	err = waitForAzureBlobAccess(propagationCtx, func(ctx context.Context) error {
		// Exercise the data plane rather than container metadata. The CI principal already holds
		// subscription Contributor, which grants the container metadata read that GetProperties
		// performs, so that call can succeed before the Storage Blob Data Contributor assignment
		// above has propagated. Blob read/write are DataActions that Contributor does not grant,
		// and the suite's first real operation is a blob write, so probe exactly that.
		//
		// The probe object is removed before returning: keys() lists the whole container, so a
		// survivor would corrupt the final "only its own key was deleted" assertion.
		if _, err := blobClient.UploadBuffer(ctx, container, azureReadinessProbeKey, []byte("ready"), nil); err != nil {
			return err
		}
		if _, err := blobClient.DeleteBlob(ctx, container, azureReadinessProbeKey, nil); err != nil &&
			!bloberror.HasCode(err, bloberror.BlobNotFound) {
			return err
		}
		return nil
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
		write: func(ctx context.Context, key string, body []byte) error {
			_, err := blobClient.UploadBuffer(ctx, container, key, body, nil)
			return err
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
			if isNotFoundResponse(err) {
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
			if isNotFoundResponse(err) {
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

// azureCredentialRetries bounds how often a credential failure is retried. A few attempts absorb a
// transient token service or Azure CLI hiccup, while a real misconfiguration still fails in
// seconds instead of consuming the whole propagation budget.
const azureCredentialRetries = 3

func waitForAzureBlobAccess(ctx context.Context, probe func(context.Context) error) error {
	lastObservation := "no completed request"
	credentialAttempts := 0
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
		classification := azureBlobReadinessError(err)
		lastObservation = classification.observation
		retry := classification.retry
		if classification.credential {
			credentialAttempts++
			retry = retry && credentialAttempts <= azureCredentialRetries
		}
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

// azureReadinessClassification describes how the readiness probe should treat a failure.
type azureReadinessClassification struct {
	// retry reports whether the failure is expected to clear on its own.
	retry bool
	// credential reports whether the failure came from acquiring a token. The caller bounds those
	// separately, so a real misconfiguration cannot consume the whole propagation budget.
	credential bool
	// observation is a redacted description, safe to log.
	observation string
}

// Only the new-account permission probe uses this classification. Diagnostics
// deliberately exclude raw SDK messages, response bodies, URLs and tokens.
func azureBlobReadinessError(err error) azureReadinessClassification {
	classify := func(retry bool, observation string) azureReadinessClassification {
		return azureReadinessClassification{retry: retry, observation: observation}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return classify(false, "request context ended")
	}
	// Checked before the response branches below, because a credential failure can carry its own
	// HTTP response and would otherwise be classified by that response's status code.
	if code, ok := azureCredentialFailure(err); ok {
		observation := "credential acquisition failed"
		if code != "" {
			observation += " (" + code + ")"
		}
		return azureReadinessClassification{retry: true, credential: true, observation: observation}
	}
	var response *azcore.ResponseError
	if errors.As(err, &response) {
		description := fmt.Sprintf("HTTP %d", response.StatusCode)
		switch response.StatusCode {
		case 403:
			return classify(response.ErrorCode == "AuthorizationPermissionMismatch" || response.ErrorCode == "AuthorizationFailure", description)
		case 408, 429, 500, 502, 503, 504:
			return classify(true, description)
		default:
			return classify(false, description)
		}
	}
	var dnsError *net.DNSError
	if errors.As(err, &dnsError) {
		return classify(dnsError.IsNotFound || dnsError.IsTimeout || dnsError.IsTemporary, "DNS resolution failed")
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return classify(true, "network timeout")
	}
	if errors.Is(err, io.EOF) || errors.Is(err, syscall.ECONNREFUSED) || step.IsTransientConnectionError(err) {
		return classify(true, "transient connection failure")
	}
	return classify(false, fmt.Sprintf("error type %T", err))
}

// azureEntraStatusCode matches the Entra status code in a credential failure. Only this code is
// reported: the surrounding message can carry account URLs, object IDs and token material.
var azureEntraStatusCode = regexp.MustCompile(`AADSTS\d+`)

// azureEntraExchangeAudience is the audience Entra requires when exchanging a federated assertion.
const azureEntraExchangeAudience = "api://AzureADTokenExchange"

// azureTestCredential builds the credential used for the test's own Azure calls.
//
// In CI `azure/login` caches the GitHub OIDC assertion it was handed at login. That assertion
// expires within minutes, so acquiring a token for a scope that was not already cached - the Blob
// data plane, for example - fails later in the job with AADSTS700024, even though the ARM token
// obtained at login is still valid. Requesting a fresh assertion per acquisition avoids that,
// because GitHub issues ID tokens for the whole lifetime of the job. Outside CI, or when the
// federated inputs are absent, this falls back to the default credential chain.
func azureTestCredential(ctx context.Context, t *testing.T) azcore.TokenCredential {
	t.Helper()
	requestURL := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
	requestToken := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
	tenant, client := azureFederatedIdentity(ctx, t)
	if requestURL == "" || requestToken == "" || tenant == "" || client == "" {
		t.Log("Azure credential: default chain (no federated inputs available)")
		credential, err := azidentity.NewDefaultAzureCredential(nil)
		require.NoError(t, err)
		return credential
	}
	t.Log("Azure credential: federated assertion, minted per token acquisition")
	credential, err := azidentity.NewClientAssertionCredential(tenant, client, func(ctx context.Context) (string, error) {
		return azureFederatedAssertion(ctx, requestURL, requestToken)
	}, nil)
	require.NoError(t, err)
	return credential
}

// azureFederatedIdentity resolves the tenant and client to exchange the assertion for.
//
// The workflow env is preferred, but this suite runs under `pull_request_target`, where the
// workflow definition comes from the base branch - so a pull request that adds those variables
// cannot observe them until it merges. The Azure CLI is already logged in by `azure/login` in that
// job, so its account record is used as a fallback and keeps the test self-sufficient.
func azureFederatedIdentity(ctx context.Context, t *testing.T) (string, string) {
	t.Helper()
	tenant := os.Getenv("AZURE_SP_TESTS_TENANTID")
	client := os.Getenv("AZURE_SP_TESTS_APPID")
	if tenant != "" && client != "" {
		return tenant, client
	}
	output, err := exec.CommandContext(ctx, "az", "account", "show", "--output", "json").Output()
	if err != nil {
		// Expected off CI, where the Azure CLI may be absent or signed out.
		t.Log("Azure credential: could not read the Azure CLI account record")
		return "", ""
	}
	tenant, client, err = azureCLIIdentity(output)
	if err != nil {
		t.Logf("Azure credential: %s", err)
		return "", ""
	}
	return tenant, client
}

// azureCLIIdentity extracts the tenant and service principal from `az account show` output. The
// account record carries subscription details, so parse failures deliberately stay generic.
func azureCLIIdentity(data []byte) (string, string, error) {
	var account struct {
		TenantID string `json:"tenantId"`
		User     struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"user"`
	}
	if err := json.Unmarshal(data, &account); err != nil {
		return "", "", errors.New("could not decode the Azure CLI account record")
	}
	if account.User.Type != "servicePrincipal" {
		return "", "", fmt.Errorf("Azure CLI is signed in as %q, not a service principal", account.User.Type)
	}
	if account.TenantID == "" || account.User.Name == "" {
		return "", "", errors.New("Azure CLI account record has no tenant or service principal")
	}
	return account.TenantID, account.User.Name, nil
}

// azureFederatedAssertion requests a fresh GitHub OIDC token. Diagnostics deliberately exclude the
// response body and the token itself.
func azureFederatedAssertion(ctx context.Context, requestURL string, requestToken string) (string, error) {
	endpoint, err := url.Parse(requestURL)
	if err != nil {
		return "", errors.New("malformed GitHub OIDC token request URL")
	}
	query := endpoint.Query()
	query.Set("audience", azureEntraExchangeAudience)
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return "", errors.New("could not build the GitHub OIDC token request")
	}
	request.Header.Set("Authorization", "Bearer "+requestToken)

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", errors.New("could not reach the GitHub OIDC token endpoint")
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub OIDC token request returned HTTP %d", response.StatusCode)
	}
	var payload struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, azureFederatedTokenLimit)).Decode(&payload); err != nil {
		return "", errors.New("could not decode the GitHub OIDC token response")
	}
	if payload.Value == "" {
		return "", errors.New("GitHub OIDC token response contained no token")
	}
	return payload.Value, nil
}

// azureFederatedTokenLimit bounds the OIDC response read so a malformed endpoint cannot stream
// without end.
const azureFederatedTokenLimit = 1 << 20

// azureCredentialUnavailableType is the error DefaultAzureCredential returns when it cannot supply
// a token. azidentity does not export the type, so it is compared against an instance the package
// constructs. The chain converts every underlying credential failure into this one error, so the
// type alone cannot say whether the cause is transient.
var azureCredentialUnavailableType = reflect.TypeOf(azidentity.NewCredentialUnavailableError(""))

// azureCredentialFailure reports whether err came from acquiring a token, and returns the bare
// Entra status code when the message carries one. The code is empty when it does not.
func azureCredentialFailure(err error) (string, bool) {
	credential := false
	var authenticationFailed *azidentity.AuthenticationFailedError
	if errors.As(err, &authenticationFailed) {
		credential = true
	}
	for unwrapped := err; unwrapped != nil && !credential; unwrapped = errors.Unwrap(unwrapped) {
		credential = reflect.TypeOf(unwrapped) == azureCredentialUnavailableType
	}
	if !credential {
		return "", false
	}
	return azureEntraStatusCode.FindString(err.Error()), true
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
