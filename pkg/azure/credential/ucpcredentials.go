// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package credential

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	sdk_cred "github.com/project-radius/radius/pkg/ucp/credentials"
	"github.com/project-radius/radius/pkg/ucp/ucplog"

	"go.uber.org/atomic"
)

const (
	// DefaultExpireDuration is the default expiry duration.
	DefaultExpireDuration = time.Minute * time.Duration(15)
)

var _ azcore.TokenCredential = (*UCPCredential)(nil)

// UCPCredentialOptions is the options for UCP credential.
type UCPCredentialOptions struct {
	// Provider is an UCP credential provider.
	Provider sdk_cred.CredentialProvider[sdk_cred.AzureCredential]
	// Duration is the duration to refresh token client.
	Duration time.Duration

	// ClientOptions is the options for azure client.
	ClientOptions *azcore.ClientOptions
}

// UCPCredential authenticates service principal using UCP credential APIs.
type UCPCredential struct {
	options    UCPCredentialOptions
	credential *sdk_cred.AzureCredential

	tokenCred azcore.TokenCredential
	// tokenCredMu is the read write mutex to protect tokenCred.
	tokenCredMu sync.RWMutex

	// nextExpiry represents the time when the current UCP credential expires
	// or when it checks if credential is updated.
	nextExpiry atomic.Int64
}

// NewUCPCredential creates a UCPCredential. Pass nil to accept default options.
func NewUCPCredential(options UCPCredentialOptions) (*UCPCredential, error) {
	if options.Provider == nil {
		return nil, errors.New("undefined provider")
	}
	if options.Duration == 0 {
		options.Duration = DefaultExpireDuration
	}

	return &UCPCredential{
		options: options,
	}, nil
}

func (c *UCPCredential) isExpired() bool {
	return c.nextExpiry.Load() < time.Now().Unix()
}

func (c *UCPCredential) refreshExpiry() {
	c.nextExpiry.Store(time.Now().Add(c.options.Duration).Unix())
}

func (c *UCPCredential) refreshCredentials(ctx context.Context) error {
	logger := ucplog.FromContext(ctx)

	c.tokenCredMu.Lock()
	defer c.tokenCredMu.Unlock()

	// Ensure if credential refresh is not done by the previous request.
	if !c.isExpired() {
		return nil
	}

	s, err := c.options.Provider.Fetch(ctx, sdk_cred.AzureCloud, "default")
	if err != nil {
		return err
	}

	if s.ClientID == "" || s.ClientSecret == "" || s.TenantID == "" {
		return errors.New("invalid azure service principal credential info")
	}

	// Do not instantiate new client unless the secret is rotated.
	if c.credential != nil && c.credential.ClientSecret == s.ClientSecret &&
		c.credential.ClientID == s.ClientID && c.credential.TenantID == s.TenantID {
		c.refreshExpiry()
		return nil
	}

	logger.Info("Retreived Azure Credential - ClientID: " + s.ClientID)

	// Rotate credentials by creating new ClientSecretCredential.
	var opt *azidentity.ClientSecretCredentialOptions
	if c.options.ClientOptions != nil {
		opt = &azidentity.ClientSecretCredentialOptions{
			ClientOptions: *c.options.ClientOptions,
		}
	}

	azCred, err := azidentity.NewClientSecretCredential(s.TenantID, s.ClientID, s.ClientSecret, opt)
	if err != nil {
		return err
	}

	c.tokenCred = azCred
	c.credential = s

	c.refreshExpiry()
	return nil
}

// GetToken requests an access token from the hosting environment. This method is called automatically by Azure SDK clients.
func (c *UCPCredential) GetToken(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	logger := ucplog.FromContext(ctx)

	if c.isExpired() {
		err := c.refreshCredentials(ctx)
		if err != nil {
			logger.Error(err, "failed to refresh Azure service principal credential.")
		}
	}

	c.tokenCredMu.RLock()
	credentialAuth := c.tokenCred
	c.tokenCredMu.RUnlock()

	if credentialAuth == nil {
		return azcore.AccessToken{}, errors.New("azure service principal credential is not ready")
	}

	return credentialAuth.GetToken(ctx, opts)
}
