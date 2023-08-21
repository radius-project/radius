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

package aws

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	sdk_cred "github.com/project-radius/radius/pkg/ucp/credentials"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
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

// Retrieve fetches credentials from an external provider, checks if they are valid, logs the AccessKeyID, and returns the
// credentials with an expiration time set. If the credentials are invalid, an error is returned.
func (c *UCPCredentialProvider) Retrieve(ctx context.Context) (aws.Credentials, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
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
