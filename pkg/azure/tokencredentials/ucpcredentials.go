// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package tokencredentials

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/go-logr/logr"
	"go.uber.org/atomic"

	"github.com/project-radius/radius/pkg/sdk"
	ucpapi "github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	ucpdatamodel "github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/secret"
	ucpsecretp "github.com/project-radius/radius/pkg/ucp/secret/provider"
)

const (
	credFetchPeriod = time.Minute * time.Duration(1)
)

var _ azcore.TokenCredential = (*UCPCredential)(nil)

// UCPCredential authenticates service principal using UCP credential APIs.
type UCPCredential struct {
	secretProvider    *ucpsecretp.SecretProvider
	azUCPClient       *ucpapi.AzureCredentialClient
	currentCredential ucpdatamodel.AzureCredentialProperties

	tokenCred azcore.TokenCredential
	// tokenCredMu is the read write mutex to protect tokenCred.
	tokenCredMu sync.RWMutex
	nextRefresh atomic.Int64
}

// NewUCPCredential creates a UCPCredential. Pass nil to accept default options.
func NewUCPCredential(secretProvider *ucpsecretp.SecretProvider, ucpConn sdk.Connection) (*UCPCredential, error) {
	cli, err := ucpapi.NewAzureCredentialClient(&AnonymousCredential{}, sdk.NewClientOptions(ucpConn))
	if err != nil {
		return nil, err
	}

	if secretProvider == nil {
		return nil, errors.New("secretProvider is not ready")
	}

	return &UCPCredential{
		azUCPClient:    cli,
		secretProvider: secretProvider,
	}, nil
}

func (c *UCPCredential) isRefreshRequired() bool {
	return c.nextRefresh.Load() < time.Now().Unix()
}

func (c *UCPCredential) refreshCredentials(ctx context.Context) error {
	c.tokenCredMu.Lock()
	defer c.tokenCredMu.Unlock()

	// Ensure if credential refresh is not done by the previous request.
	if !c.isRefreshRequired() {
		return nil
	}

	// 1. Fetch the secret name of Azure service principal credentials from UCP.
	cred, err := c.azUCPClient.Get(ctx, "azure", "azurecloud", "default", &ucpapi.AzureCredentialClientGetOptions{})
	if err != nil {
		return err
	}

	storage, ok := cred.Properties.GetCredentialResourceProperties().Storage.(*ucpapi.InternalCredentialStorageProperties)
	if !ok {
		return errors.New("invalid InternalCredentialStorageProperties")
	}

	secretName := to.String(storage.SecretName)
	if secretName == "" {
		return errors.New("unspecified SecretName for internal storage")
	}

	// 2. Fetch the credential from internal storage (e.g. Kubernetes secret store)
	cli, err := c.secretProvider.GetClient(ctx)
	if err != nil {
		return err
	}

	s, err := secret.GetSecret[ucpdatamodel.AzureCredentialProperties](ctx, cli, secretName)
	if err != nil {
		return errors.New("failed to get credential info: " + err.Error())
	}

	if s.ClientID == "" || s.ClientSecret == "" || s.TenantID == "" {
		return errors.New("invalid azure service principal credential info")
	}

	// Do not instantiate new client unless the secret is rotated.
	if c.currentCredential.ClientSecret == s.ClientSecret &&
		c.currentCredential.ClientID == s.ClientID &&
		c.currentCredential.TenantID == s.TenantID {
		return nil
	}

	// Rotate credentials by creating new ClientSecretCredential.
	azCred, err := azidentity.NewClientSecretCredential(s.TenantID, s.ClientID, s.ClientSecret, nil)
	if err != nil {
		return err
	}

	c.tokenCred = azCred
	c.currentCredential = s

	c.nextRefresh.Store(time.Now().Add(credFetchPeriod).Unix())
	return nil
}

// GetToken requests an access token from the hosting environment. This method is called automatically by Azure SDK clients.
func (c *UCPCredential) GetToken(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	logger := logr.FromContextOrDiscard(ctx)

	if c.isRefreshRequired() {
		err := c.refreshCredentials(ctx)
		if err != nil {
			logger.Error(err, "failed to refresh Azure service principal credential.")
		}
	}

	c.tokenCredMu.RLock()
	defer c.tokenCredMu.RUnlock()

	if c.tokenCred == nil {
		return azcore.AccessToken{}, errors.New("azure service principal credential is not ready")
	}

	return c.tokenCred.GetToken(ctx, opts)
}
