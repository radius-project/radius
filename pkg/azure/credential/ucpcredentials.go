/*
Copyright 2023 The Radius Authors.

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

package credential

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	sdk_cred "github.com/radius-project/radius/pkg/ucp/credentials"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/ucplog"

	"go.uber.org/atomic"
)

const (
	// DefaultExpireDuration is the default expiry duration.
	DefaultExpireDuration = time.Second * time.Duration(30)
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

// NewUCPCredential creates a new UCPCredential with the given options and returns it, or returns an error if the
// provider is not defined. If Duration is 0, it is set to DefaultDuration. Pass nil to accept default options.
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
	logger := ucplog.FromContextOrDiscard(ctx)

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

	switch s.Kind {
	case datamodel.AzureServicePrincipalCredentialKind:
		azureServicePrincipalCredential := s.ServicePrincipal
		if azureServicePrincipalCredential.ClientID == "" || azureServicePrincipalCredential.ClientSecret == "" || azureServicePrincipalCredential.TenantID == "" {
			return errors.New("empty clientID, tenantID, or client secret provided for Azure service principal")
		}

		// Do not instantiate new client unless the secret is rotated.
		if c.credential != nil &&
			c.credential.ServicePrincipal != nil &&
			c.credential.ServicePrincipal.ClientSecret == azureServicePrincipalCredential.ClientSecret &&
			c.credential.ServicePrincipal.ClientID == azureServicePrincipalCredential.ClientID &&
			c.credential.ServicePrincipal.TenantID == azureServicePrincipalCredential.TenantID {
			c.refreshExpiry()
			return nil
		}

		logger.Info("Retreived Azure Credential - ClientID: " + azureServicePrincipalCredential.ClientID)

		// Rotate credentials by creating new ClientSecretCredential.
		var opt *azidentity.ClientSecretCredentialOptions
		if c.options.ClientOptions != nil {
			opt = &azidentity.ClientSecretCredentialOptions{
				ClientOptions: *c.options.ClientOptions,
			}
		}

		azCred, err := azidentity.NewClientSecretCredential(azureServicePrincipalCredential.TenantID, azureServicePrincipalCredential.ClientID, azureServicePrincipalCredential.ClientSecret, opt)
		if err != nil {
			return err
		}

		c.tokenCred = azCred
		c.credential = s

		c.refreshExpiry()
		return nil
	case datamodel.AzureWorkloadIdentityCredentialKind:
		azureWorkloadIdentityCredential := s.WorkloadIdentity
		if azureWorkloadIdentityCredential.ClientID == "" || azureWorkloadIdentityCredential.TenantID == "" {
			return errors.New("empty clientID or tenantID provided for Azure workload identity")
		}

		// Do not instantiate new client unless clientId and tenantId are changed.
		if c.credential != nil &&
			c.credential.WorkloadIdentity != nil &&
			c.credential.WorkloadIdentity.ClientID == azureWorkloadIdentityCredential.ClientID &&
			c.credential.WorkloadIdentity.TenantID == azureWorkloadIdentityCredential.TenantID {
			c.refreshExpiry()
			return nil
		}

		logger.Info("Retreived Azure Credential - ClientID: " + azureWorkloadIdentityCredential.ClientID)

		var opt *azidentity.DefaultAzureCredentialOptions
		if c.options.ClientOptions != nil {
			opt = &azidentity.DefaultAzureCredentialOptions{
				ClientOptions: *c.options.ClientOptions,
			}
		}

		azCred, err := azidentity.NewDefaultAzureCredential(opt)
		if err != nil {
			return err
		}

		c.tokenCred = azCred
		c.credential = s

		c.refreshExpiry()
		return nil
	default:
		return errors.New(fmt.Sprintf("unknown Azure credential kind, expected ServicePrincipal or WorkloadIdentity (got %s)", s.Kind))
	}
}

// GetToken attempts to refresh the Azure credential if it is expired and then returns an
// access token if the credential is ready. This method is called automatically by Azure SDK clients.
func (c *UCPCredential) GetToken(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	if c.isExpired() {
		err := c.refreshCredentials(ctx)
		if err != nil {
			logger.Error(err, "failed to refresh Azure credential.")
		}
	}

	c.tokenCredMu.RLock()
	credentialAuth := c.tokenCred
	c.tokenCredMu.RUnlock()

	if credentialAuth == nil {
		return azcore.AccessToken{}, errors.New("azure credential is not ready")
	}

	return credentialAuth.GetToken(ctx, opts)
}
