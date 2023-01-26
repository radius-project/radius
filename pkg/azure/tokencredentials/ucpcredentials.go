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
	secretProvider *ucpsecretp.SecretProvider

	azureCreds ucpdatamodel.AzureCredentialProperties
	secretName string

	ucpClient *ucpapi.AzureCredentialClient

	credClient   azcore.TokenCredential
	credClientMu sync.RWMutex

	once sync.Once
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
		ucpClient:      cli,
		secretProvider: secretProvider,
	}, nil
}

// StartFetcher starts azure service principal credential fetcher worker. This must be
// called during startup with the cancellable context.
func (c *UCPCredential) StartFetcher(ctx context.Context) {
	c.once.Do(func() {
		go func(ctx context.Context) {
			logger := logr.FromContextOrDiscard(ctx)
			ticker := time.NewTicker(credFetchPeriod)

			logger.Info("Starting Azure service principal credential fetcher.")
			for {
				select {
				case <-ticker.C:
					if err := c.refreshClient(ctx); err != nil {
						logger.Error(err, "failed to refresh azure service principal credential client")
					}
				case <-ctx.Done():
					logger.Info("Exiting Azure service principal credential fetcher gracefully.")
					return
				}
			}
		}(ctx)
	})
}

func (c *UCPCredential) refreshClient(ctx context.Context) error {
	c.credClientMu.Lock()
	defer c.credClientMu.Unlock()

	cred, err := c.ucpClient.Get(ctx, "azure", "azurecloud", "default", &ucpapi.AzureCredentialClientGetOptions{})
	if err != nil {
		return err
	}

	storage, ok := cred.Properties.GetCredentialResourceProperties().Storage.(*ucpapi.InternalCredentialStorageProperties)
	if !ok {
		return errors.New("invalid InternalCredentialStorageProperties")
	}

	c.secretName = to.String(storage.SecretName)
	if c.secretName == "" {
		return errors.New("SecretName is not specified")
	}

	cli, err := c.secretProvider.GetClient(ctx)
	if err != nil {
		return err
	}

	s, err := secret.GetSecret[ucpdatamodel.AzureCredentialProperties](ctx, cli, c.secretName)
	if err != nil {
		return errors.New("azure service principal credential may not set: " + err.Error())
	}

	if s.ClientID == "" || s.ClientSecret == "" || s.TenantID == "" {
		return errors.New("invalid azure service principal credential info")
	}

	// Do not instantiate new client unless the secret is rotated.
	if c.azureCreds.ClientSecret == s.ClientSecret &&
		c.azureCreds.ClientID == s.ClientID &&
		c.azureCreds.TenantID == s.TenantID {
		return nil
	}

	c.azureCreds = s
	azCred, err := azidentity.NewClientSecretCredential(s.TenantID, s.ClientID, s.ClientSecret, nil)
	if err != nil {
		return err
	}

	c.credClient = azCred
	return nil
}

// GetToken requests an access token from the hosting environment. This method is called automatically by Azure SDK clients.
func (c *UCPCredential) GetToken(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	c.credClientMu.RLock()
	defer c.credClientMu.RUnlock()

	if c.credClient == nil {
		return azcore.AccessToken{}, errors.New("credClient is not ready")
	}

	return c.credClient.GetToken(ctx, opts)
}
