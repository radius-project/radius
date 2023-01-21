// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armauth

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/go-autorest/autorest/to"

	aztoken "github.com/project-radius/radius/pkg/azure/tokencredentials"
	"github.com/project-radius/radius/pkg/sdk"
	ucpapi "github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	ucpsecretp "github.com/project-radius/radius/pkg/ucp/secret/provider"
)

const (
	credFetchPeriod = time.Minute * time.Duration(1)
)

var _ azcore.TokenCredential = (*UCPCredential)(nil)

// UCPCredential authenticates service principal using UCP credential APIs.
type UCPCredential struct {
	tenantID     string
	clientID     string
	clientSecret string
	secretName   string

	identityClient   azcore.TokenCredential
	ucpClient        *ucpapi.AzureCredentialClient
	clientMu         sync.RWMutex
	secretExpireTime time.Time

	secretProvider *ucpsecretp.SecretProvider
}

// NewUCPCredential creates a UCPCredential. Pass nil to accept default options.
func NewUCPCredential(options *Options) (*UCPCredential, error) {
	cli, err := ucpapi.NewAzureCredentialClient(&aztoken.AnonymousCredential{}, sdk.NewClientOptions(options.UCPConnection))
	if err != nil {
		return nil, err
	}

	return &UCPCredential{
		ucpClient:        cli,
		clientMu:         sync.RWMutex{},
		secretExpireTime: time.Time{},
		secretProvider:   options.SecretProvider,
	}, nil
}

func (c *UCPCredential) isExpired() bool {
	return c.secretExpireTime.Before(time.Now())
}

func (c *UCPCredential) refreshTokenClient(ctx context.Context) error {
	c.clientMu.Lock()
	defer c.clientMu.Unlock()

	if !c.isExpired() {
		return nil
	}

	err := c.updateIdentityOptions(ctx)
	if err != nil {
		return err
	}

	err = c.updateIdentityClient(ctx)
	if err != nil {
		return err
	}

	c.secretExpireTime = time.Now().Add(credFetchPeriod)
	return nil
}

func (c *UCPCredential) updateIdentityOptions(ctx context.Context) error {
	cred, err := c.ucpClient.Get(ctx, "azure", "azurecloud", "default", &ucpapi.AzureCredentialClientGetOptions{})
	if err != nil {
		return err
	}

	prop, ok := cred.Properties.(*ucpapi.AzureServicePrincipalProperties)
	if !ok {
		return errors.New("invalid AzureServicePrincipalProperties")
	}
	c.clientID = to.String(prop.ClientID)
	if c.clientID == "" {
		return errors.New("ClientID is not specified")
	}
	c.tenantID = to.String(prop.TenantID)
	if c.tenantID == "" {
		return errors.New("TenantID is not specified")
	}

	internal, ok := prop.Storage.(*ucpapi.InternalCredentialStorageProperties)
	if !ok {
		return errors.New("invalid InternalCredentialStorageProperties")
	}
	c.secretName = to.String(internal.SecretName)
	if c.secretName == "" {
		return errors.New("SecretName is not specified")
	}

	return nil
}

func (c *UCPCredential) updateIdentityClient(ctx context.Context) error {
	cli, err := c.secretProvider.GetClient(ctx)
	if err != nil {
		return err
	}

	secret, err := cli.Get(ctx, c.secretName)
	if err != nil {
		return err
	}

	// Do not instantiate new client unless the secret is rotated.
	if c.clientSecret == string(secret) {
		return nil
	}

	c.clientSecret = string(secret)
	c.identityClient, err = azidentity.NewClientSecretCredential(c.tenantID, c.clientID, c.clientSecret, nil)
	if err != nil {
		return err
	}
	return nil
}

// GetToken requests an access token from the hosting environment. This method is called automatically by Azure SDK clients.
func (c *UCPCredential) GetToken(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	if c.isExpired() {
		if err := c.refreshTokenClient(ctx); err != nil {
			return azcore.AccessToken{}, err
		}
	}

	c.clientMu.RLock()
	defer c.clientMu.RUnlock()
	return c.identityClient.GetToken(ctx, opts)
}
