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
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"

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
		logger.Info(fmt.Sprintf("#<3##..#...Retrieved AWS Credential - RoleARN: %s", s.IRSACredential.RoleARN))
		logger.Info("Retrieved AWS Credential - TokenFile: NOT REALLY")

		// we have the rolearn in c's options but how can I inject that into the aws.Credentials?
		// we need to create a new aws.Credentials with the rolearn and token file infos

		//tokenFilePath := "/var/run/secrets/eks.amazonaws.com/serviceaccount/token" // Update this path as necessary

		/*loadOptions := []func(*config.LoadOptions) error{}
		regionLoadOption := config.WithRegion("us-west-2")
		loadOptions = append(loadOptions, regionLoadOption)

		/*assumeRoleLoadOption := config.WithAssumeRoleCredentialOptions(func(o *stscreds.AssumeRoleOptions) {
			logger.Info(fmt.Sprintf(".....<3.......Retrieved AWS Credential - RoleARN: %s", s.IRSACredential.RoleARN))
			o.RoleARN = s.IRSACredential.RoleARN // Specify the role ARN to assume
			o.RoleSessionName = "my-session"     // Optionally specify a session name
			// If you have an external ID, you can set it like this: o.ExternalID = aws.String("your-external-id")
		})

		awscfg, err := config.LoadDefaultConfig(ctx, loadOptions...)
		if err != nil {
			logger.Info(fmt.Sprintf("Failed to load AWS config ------------ %s", err.Error()))
			return aws.Credentials{}, err // Ensure to return the error to the caller
		}*/

		roleARN := "arn:aws:iam::817312594854:role/radius-role"
		tokenFilePath := os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE")

		if roleARN == "" || tokenFilePath == "" {
			panic("failed to load ENV")
		}

		awscfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-west-2"))
		if err != nil {
			panic("failed to load config, " + err.Error())
		}

		client := sts.NewFromConfig(awscfg)

		credsCache := aws.NewCredentialsCache(stscreds.NewWebIdentityRoleProvider(
			client,
			roleARN,
			stscreds.IdentityTokenFile(tokenFilePath),
			func(o *stscreds.WebIdentityRoleOptions) {
				o.RoleSessionName = "my-session"
			}))

		// use the credentials to list a s3 object

		value, err = credsCache.Retrieve(ctx)
		logger.Info(fmt.Sprintf("Retrieved AWS Credential IRSA - value is : %s, token is %s", value.AccessKeyID, value.SessionToken))
		/*client := sts.NewFromConfig(awscfg)

		//logger.Info(fmt.Sprintf("Created AWS STS client with region: %s", awscfg.Region))

			credsCache := aws.NewCredentialsCache(stscreds.NewWebIdentityRoleProvider(
				client,
				s.IRSACredential.RoleARN, // inject role-arn here
				stscreds.IdentityTokenFile(tokenFilePath), // Correctly use IdentityTokenFile here
				func(o *stscreds.WebIdentityRoleOptions) {
					o.RoleSessionName = "my-session" // Set RoleSessionName here if needed
				},
			))
			value, err = credsCache.Retrieve(ctx)
			if err != nil {
				return aws.Credentials{}, err
			}


		assumeRoleProvider := stscreds.NewAssumeRoleProvider(client, s.IRSACredential.RoleARN)
		logger.Info("Created AWS AssumeRoleProvider ")
		value, err = assumeRoleProvider.Retrieve(ctx)
		if err != nil {
			logger.Info(fmt.Sprintf("Failed to retrieve AWS Credential IRSA - %s", err.Error()))
			return aws.Credentials{}, err
		}
		*/

	default:
		return aws.Credentials{}, errors.New("invalid credential kind")
	}

	return value, nil
}
