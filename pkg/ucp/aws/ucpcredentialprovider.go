// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package aws

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/go-logr/logr"

	sdk_cred "github.com/project-radius/radius/pkg/ucp/credentials"
)

var _ aws.CredentialsProvider = (*UCPCredentialProvider)(nil)

const (
	// DefaultExpireDuration is the default access key expiry duration.
	DefaultExpireDuration = time.Minute * time.Duration(15)
)

// UCPCredentialProvider is the implementation of aws.CredentialsProvider
// to retrieve credentials for AWS SDK via UCP credentials.
type UCPCredentialProvider struct {
	options UCPCredentialOptions
}

// UCPCredentialOptions is a configuration for UCPCredentialProvider.
type UCPCredentialOptions struct {
	// Provider is an UCP credential provider.
	Provider sdk_cred.CredentialProvider[sdk_cred.AWSCredential]

	// Duration is the duration for the secret keys.
	Duration time.Duration
}

// NewUCPCredentialProvider creates UCPCredentialProvider provider to fetch Secret Access key using UCP credential APIs.
func NewUCPCredentialProvider(provider sdk_cred.CredentialProvider[sdk_cred.AWSCredential], expireDuration time.Duration) *UCPCredentialProvider {
	if expireDuration == 0 {
		expireDuration = DefaultExpireDuration
	}

	o := UCPCredentialOptions{
		Provider: provider,
		Duration: expireDuration,
	}

	return &UCPCredentialProvider{options: o}
}

// Retrieve fetches the secret access key using UCP credential API.
func (c *UCPCredentialProvider) Retrieve(ctx context.Context) (aws.Credentials, error) {
	logger := logr.FromContextOrDiscard(ctx)
	s, err := c.options.Provider.Fetch(ctx, sdk_cred.AWSPublic, "default")
	if err != nil {
		return aws.Credentials{}, err
	}

	if s.AccessKeyID == "" || s.SecretAccessKey == "" {
		return aws.Credentials{}, errors.New("invalid access key info")
	}

	logger.Info(fmt.Sprintf("Retreived AWS Credential - AccessKeyID: %s", s.AccessKeyID))

	value := aws.Credentials{
		AccessKeyID:     s.AccessKeyID,
		SecretAccessKey: s.SecretAccessKey,
		Source:          "radiusucp",
		CanExpire:       true,
		// Enables AWS SDK to fetch (rotate) access keys by calling Retrieve() after Expires.
		Expires: time.Now().UTC().Add(c.options.Duration),
	}

	return value, nil
}
