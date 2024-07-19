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
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/google/uuid"

	sdk_cred "github.com/radius-project/radius/pkg/ucp/credentials"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

var _ aws.CredentialsProvider = (*UCPCredentialProvider)(nil)

const (
	// DefaultExpireDuration is the default access key expiry duration.
	DefaultExpireDuration = time.Minute * time.Duration(15)

	// CredentialKind is IRSA
	CredentialKindIRSA = "IRSA"
	// CredentialKindAccessKey is AccessKey
	CredentialKindAccessKey = "AccessKey"
	// Token file path for IRSA
	tokenFilePath = "/var/run/secrets/eks.amazonaws.com/serviceaccount/token"
	// AWS STS Signing region
	awsSTSGlobalEndPointSigningRegion = "us-east-1"
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

	var value aws.Credentials
	switch s.Kind {
	case CredentialKindAccessKey:
		if s.AccessKeyCredential == nil || s.AccessKeyCredential.AccessKeyID == "" || s.AccessKeyCredential.SecretAccessKey == "" {
			return aws.Credentials{}, errors.New("invalid access key info")
		}
		logger.Info(fmt.Sprintf("Retrieved AWS Credential - AccessKeyID: %s", s.AccessKeyCredential.AccessKeyID))

		value = aws.Credentials{
			AccessKeyID:     s.AccessKeyCredential.AccessKeyID,
			SecretAccessKey: s.AccessKeyCredential.SecretAccessKey,
			Source:          "radiusucp",
			CanExpire:       true,
			Expires:         time.Now().UTC().Add(c.options.Duration),
		}

	case CredentialKindIRSA:
		if s.IRSACredential == nil || s.IRSACredential.RoleARN == "" {
			return aws.Credentials{}, errors.New("invalid IRSA info. RoleARN is required")
		}
		logger.Info(fmt.Sprintf("Retrieved AWS Credential - RoleARN: %s", s.IRSACredential.RoleARN))

		// Radius requests will first be routed to STS endpoint,
		// where it will be validated and then the request to the specific service (such as S3) will be made using
		// the bearer token from the STS response.
		// Based on the https://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_temp_enable-regions.html,
		// STS endpoint should be region based, and in the same region as
		// Radius instance to minimize latency associated eith STS call and thereby improve performance.
		// We should provide the user with ability to configure the STS endpoint region.
		// For now, we are using the global STS endpoint.
		awscfg, err := config.LoadDefaultConfig(context.TODO(),
			config.WithRegion(awsSTSGlobalEndPointSigningRegion))

		if err != nil {
			return aws.Credentials{}, err
		}

		client := sts.NewFromConfig(awscfg)

		credsCache := aws.NewCredentialsCache(stscreds.NewWebIdentityRoleProvider(
			client,
			s.IRSACredential.RoleARN,
			stscreds.IdentityTokenFile(tokenFilePath),
			func(o *stscreds.WebIdentityRoleOptions) {
				o.RoleSessionName = "radius-ucp-" + uuid.New().String()
			},
		))

		value, err = credsCache.Retrieve(ctx)
		if err != nil {
			logger.Info(fmt.Sprintf("Failed to retrieve AWS Credential IRSA - %s", err.Error()))
			return aws.Credentials{}, err
		}
		value.Source = "radiusucp"
		value.CanExpire = true
		value.Expires = time.Now().UTC().Add(c.options.Duration)
	default:
		return aws.Credentials{}, errors.New("invalid credential kind")
	}

	return value, nil
}
