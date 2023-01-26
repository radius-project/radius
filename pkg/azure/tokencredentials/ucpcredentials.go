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
	azureCreds ucpdatamodel.AzureCredentialProperties
	secretName string

	credentialClient azcore.TokenCredential
	ucpClient        *ucpapi.AzureCredentialClient
	clientMu         sync.RWMutex
	secretExpireTime atomic.Int64

	secretProvider *ucpsecretp.SecretProvider
}

// NewUCPCredential creates a UCPCredential. Pass nil to accept default options.
func NewUCPCredential(secretProvider *ucpsecretp.SecretProvider, ucpConn sdk.Connection) (*UCPCredential, error) {
	cli, err := ucpapi.NewAzureCredentialClient(&AnonymousCredential{}, sdk.NewClientOptions(ucpConn))
	if err != nil {
		return nil, err
	}

	return &UCPCredential{
		ucpClient:      cli,
		clientMu:       sync.RWMutex{},
		secretProvider: secretProvider,
	}, nil
}

func (c *UCPCredential) isExpired() bool {
	return c.secretExpireTime.Load() < time.Now().Unix()
}

func (c *UCPCredential) refreshTokenClient(ctx context.Context) error {
	c.clientMu.Lock()
	defer c.clientMu.Unlock()

	// When two requests fetch the token simultaneously, the second one does not need to refresh clients after the first one updates client successfully.
	if !c.isExpired() {
		return nil
	}

	err := c.updateIdentityOptions(ctx)
	if err != nil {
		return err
	}

	err = c.updatecredentialClient(ctx)
	if err != nil {
		return err
	}

	c.secretExpireTime.Store(time.Now().Add(credFetchPeriod).Unix())
	return nil
}

func (c *UCPCredential) updateIdentityOptions(ctx context.Context) error {
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

	return nil
}

func (c *UCPCredential) updatecredentialClient(ctx context.Context) error {
	cli, err := c.secretProvider.GetClient(ctx)
	if err != nil {
		return err
	}

	s, err := secret.GetSecret[ucpdatamodel.AzureCredentialProperties](ctx, cli, c.secretName)
	if err != nil {
		return err
	}

	// Do not instantiate new client unless the secret is rotated.
	if c.azureCreds.ClientSecret == s.ClientSecret && c.azureCreds.ClientID == s.ClientID && c.azureCreds.TenantID == s.TenantID {
		return nil
	}

	c.azureCreds = s
	cred, err := azidentity.NewClientSecretCredential(s.TenantID, s.ClientID, s.ClientSecret, nil)
	if err != nil {
		return err
	}

	c.credentialClient = cred
	return nil
}

// GetToken requests an access token from the hosting environment. This method is called automatically by Azure SDK clients.
func (c *UCPCredential) GetToken(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	logger := logr.FromContextOrDiscard(ctx)

	if c.isExpired() {
		err := c.refreshTokenClient(ctx)
		if err != nil {
			logger.Error(err, "failed to refresh credential client")
		}
	}

	c.clientMu.RLock()
	defer c.clientMu.RUnlock()

	if c.credentialClient == nil {
		return azcore.AccessToken{}, errors.New("credentialClient is not ready")
	}

	return c.credentialClient.GetToken(ctx, opts)
}
