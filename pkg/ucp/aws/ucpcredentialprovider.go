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
	// CredentialKind is AccessKey
	CredentialKindAccessKey = "AccessKey"
	// Token file path for IRSA
	TokenFilePath = "/var/run/secrets/eks.amazonaws.com/serviceaccount/token"
	// AWS STS Signing region
	awsSTSGlobalEndPointSigningRegion = "us-east-1"
	// AWS IRSA session name prefix
	sessionPrefix = "radius-ucp-"
	// Credential source
	credentialSource = "radiusucp"
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

	// STSEndpointRegion is the AWS region to use for the STS endpoint when retrieving
	// IRSA credentials. Using a regional STS endpoint (matching the target service region)
	// avoids token compatibility issues with some AWS services like CloudWatch Logs.
	// If empty, defaults to "us-east-1".
	STSEndpointRegion string
}

// NewUCPCredentialProvider creates UCPCredentialProvider provider to fetch Secret Access key using UCP credential APIs.
func NewUCPCredentialProvider(provider sdk_cred.CredentialProvider[sdk_cred.AWSCredential], expireDuration time.Duration, stsEndpointRegion string) *UCPCredentialProvider {
	if expireDuration == 0 {
		expireDuration = DefaultExpireDuration
	}

	if stsEndpointRegion == "" {
		stsEndpointRegion = awsSTSGlobalEndPointSigningRegion
	}

	o := UCPCredentialOptions{
		Provider:          provider,
		Duration:          expireDuration,
		STSEndpointRegion: stsEndpointRegion,
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
			Source:          credentialSource,
			CanExpire:       true,
			Expires:         time.Now().UTC().Add(c.options.Duration),
		}

	case CredentialKindIRSA:
		if s.IRSACredential == nil || s.IRSACredential.RoleARN == "" {
			return aws.Credentials{}, errors.New("invalid IRSA info. RoleARN is required")
		}
		logger.Info(fmt.Sprintf("Retrieved AWS Credential - RoleARN: %s", s.IRSACredential.RoleARN))

		stsRegion := c.options.STSEndpointRegion
		logger.Info(fmt.Sprintf("Using STS endpoint region: %s", stsRegion))

		awscfg, err := config.LoadDefaultConfig(ctx,
			config.WithRegion(stsRegion))
		if err != nil {
			return aws.Credentials{}, err
		}

		stsClient := sts.NewFromConfig(awscfg)

		// Step 1: Get web identity credentials via AssumeRoleWithWebIdentity.
		webIdentityProvider := stscreds.NewWebIdentityRoleProvider(
			stsClient,
			s.IRSACredential.RoleARN,
			stscreds.IdentityTokenFile(TokenFilePath),
			func(o *stscreds.WebIdentityRoleOptions) {
				o.RoleSessionName = sessionPrefix + "wi-" + uuid.New().String()
				if c.options.Duration > 0 {
					o.Duration = c.options.Duration
				}
			},
		)

		webIdentityCreds, err := webIdentityProvider.Retrieve(ctx)
		if err != nil {
			logger.Error(err, "Failed to retrieve web identity credentials")
			return aws.Credentials{}, err
		}
		logger.Info("Successfully retrieved web identity credentials")

		// Step 2: Re-assume the same role using regular AssumeRole (role chaining).
		// This converts the web identity federation session into a standard
		// AssumeRole session. Web identity sessions have restrictions on session
		// chaining that cause CloudControl's internal operations to fail with
		// "invalid security token" errors. A regular AssumeRole session does not
		// have these restrictions.
		// See: https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_terms-and-concepts.html#term-RoleChaining
		reAssumeClient := sts.NewFromConfig(awscfg, func(o *sts.Options) {
			o.Credentials = aws.CredentialsProviderFunc(
				func(ctx context.Context) (aws.Credentials, error) {
					return webIdentityCreds, nil
				},
			)
		})

		assumeRoleInput := &sts.AssumeRoleInput{
			RoleArn:         &s.IRSACredential.RoleARN,
			RoleSessionName: aws.String(sessionPrefix + uuid.New().String()),
		}
		// Role chaining sessions have a max duration of 1 hour.
		// Only set DurationSeconds if the value falls within the STS-accepted range.
		if c.options.Duration >= 15*time.Minute && c.options.Duration <= time.Hour {
			durationSeconds := int32(c.options.Duration / time.Second)
			assumeRoleInput.DurationSeconds = &durationSeconds
		}

		assumeRoleOutput, err := reAssumeClient.AssumeRole(ctx, assumeRoleInput)
		if err != nil {
			logger.Error(err, "Failed to re-assume role for clean session")
			return aws.Credentials{}, err
		}

		if assumeRoleOutput.Credentials == nil {
			return aws.Credentials{}, fmt.Errorf("AssumeRole returned nil credentials")
		}
		logger.Info("Successfully re-assumed role for clean session credentials")

		value = aws.Credentials{
			AccessKeyID:     aws.ToString(assumeRoleOutput.Credentials.AccessKeyId),
			SecretAccessKey:  aws.ToString(assumeRoleOutput.Credentials.SecretAccessKey),
			SessionToken:    aws.ToString(assumeRoleOutput.Credentials.SessionToken),
			Source:          credentialSource,
			CanExpire:       true,
			Expires:         aws.ToTime(assumeRoleOutput.Credentials.Expiration).UTC(),
		}
	default:
		return aws.Credentials{}, errors.New("invalid credential kind")
	}

	return value, nil
}
