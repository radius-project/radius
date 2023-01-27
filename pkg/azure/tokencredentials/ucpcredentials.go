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
	"github.com/go-logr/logr"
	"go.uber.org/atomic"

	sdk "github.com/project-radius/radius/pkg/sdk/credentials"
)

const (
	// DefaultExpireDuration is the default expiry duration.
	DefaultExpireDuration = time.Minute * time.Duration(1)
)

var _ azcore.TokenCredential = (*UCPCredential)(nil)

// UCPCredential authenticates service principal using UCP credential APIs.
type UCPCredential struct {
	provider   sdk.CredentialProvider[sdk.AzureCredential]
	credential *sdk.AzureCredential

	tokenCred azcore.TokenCredential
	// tokenCredMu is the read write mutex to protect tokenCred.
	tokenCredMu sync.RWMutex
	nextRefresh atomic.Int64

	duration time.Duration
}

// NewUCPCredential creates a UCPCredential. Pass nil to accept default options.
func NewUCPCredential(provider sdk.CredentialProvider[sdk.AzureCredential], expireDuration time.Duration) (*UCPCredential, error) {
	return &UCPCredential{
		provider: provider,
		duration: expireDuration,
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

	s, err := c.provider.Fetch(ctx, sdk.AzureCloud, "default")
	if err != nil {
		return err
	}

	if s.ClientID == "" || s.ClientSecret == "" || s.TenantID == "" {
		return errors.New("invalid azure service principal credential info")
	}

	// Do not instantiate new client unless the secret is rotated.
	if c.credential != nil && c.credential.ClientSecret == s.ClientSecret &&
		c.credential.ClientID == s.ClientID && c.credential.TenantID == s.TenantID {
		return nil
	}

	// Rotate credentials by creating new ClientSecretCredential.
	azCred, err := azidentity.NewClientSecretCredential(s.TenantID, s.ClientID, s.ClientSecret, nil)
	if err != nil {
		return err
	}

	c.tokenCred = azCred
	c.credential = s

	c.nextRefresh.Store(time.Now().Add(c.duration).Unix())
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
