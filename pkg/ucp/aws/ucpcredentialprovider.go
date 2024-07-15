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
	// TODO: AWS STS endpoint region (should I set AWS_REGION env variable instead?)
	awsSTSEndPointRegion = "us-west-2"
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
			return aws.Credentials{}, errors.New("invalid IRSA info")
		}
		logger.Info(fmt.Sprintf("Retrieved AWS Credential - RoleARN: %s", s.IRSACredential.RoleARN))

		//TODO: is it a good idea to make this a env variable for AWS_REGION?
		awscfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(awsSTSEndPointRegion))
		if err != nil {
			panic("failed to load config, " + err.Error())
		}

		client := sts.NewFromConfig(awscfg)

		credsCache := aws.NewCredentialsCache(stscreds.NewWebIdentityRoleProvider(
			client,
			s.IRSACredential.RoleARN,
			stscreds.IdentityTokenFile(tokenFilePath),

			func(o *stscreds.WebIdentityRoleOptions) {
				// TODO: How can we use this?
				o.RoleSessionName = "radius-ucp-" + uuid.New().String()
			},
		))

		// use the credentials to list a s3 object

		value, err = credsCache.Retrieve(ctx)
		if err != nil {
			logger.Info(fmt.Sprintf("Failed to retrieve AWS Credential IRSA - %s", err.Error()))
			return aws.Credentials{}, err
		}
		value.Source = "radiusucp"
		value.CanExpire = true
		value.Expires = time.Now().UTC().Add(c.options.Duration)
		logger.Info(fmt.Sprintf("Retrieved AWS Credential IRSA - value is : %s, token is %s", value.AccessKeyID, value.SessionToken))

	default:
		return aws.Credentials{}, errors.New("invalid credential kind")
	}

	return value, nil
}
