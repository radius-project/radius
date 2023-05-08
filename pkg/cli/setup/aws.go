/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

package setup

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/spf13/cobra"

	"github.com/project-radius/radius/pkg/cli"
	radAWS "github.com/project-radius/radius/pkg/cli/aws"
	"github.com/project-radius/radius/pkg/cli/prompt"
)

const (
	AWSProviderFlagName                = "provider-aws"
	AWSProviderAccessKeyIdFlagName     = "provider-aws-access-key-id"
	AWSProviderSecretAccessKeyFlagName = "provider-aws-secret-access-key"
	AWSProviderRegionFlagName          = "provider-aws-region"
)

var (
	errNotEmptyTemplate = "%s cannot be empty"
)

func RegisterPersistentAWSProviderArgs(cmd *cobra.Command) {
	cmd.PersistentFlags().BoolP(
		AWSProviderFlagName,
		"",
		false,
		"Add AWS provider for cloud resources",
	)
	cmd.PersistentFlags().String(
		AWSProviderAccessKeyIdFlagName,
		"",
		"Specifies an AWS access key associated with an IAM user or role",
	)
	cmd.PersistentFlags().String(
		AWSProviderSecretAccessKeyFlagName,
		"",
		"Specifies the secret key associated with the access key. This is essentially the \"password\" for the access key",
	)
	cmd.PersistentFlags().String(
		AWSProviderRegionFlagName,
		"",
		"Specifies the region to be used for resources deployed by this provider",
	)
}

// ParseAWSProviderArgs parses AWS args from user cmd line and returns an aws provider.
func ParseAWSProviderArgs(cmd *cobra.Command, interactive bool, prompter prompt.Interface) (*radAWS.Provider, error) {
	if interactive {
		return parseAWSProviderInteractive(cmd, prompter)
	}
	return parseAWSProviderNonInteractive(cmd)

}

func parseAWSProviderInteractive(cmd *cobra.Command, prompter prompt.Interface) (*radAWS.Provider, error) {
	ctx := cmd.Context()

	region, err := prompter.GetTextInput("Enter the region you would like to use to deploy AWS resources:", "Enter a region...")
	if err != nil {
		return nil, err
	}
	if region == "" {
		return nil, &cli.FriendlyError{Message: fmt.Sprintf(errNotEmptyTemplate, "aws region")}
	}

	keyID, err := prompter.GetTextInput("Enter the IAM Access Key ID:", "Enter IAM access KeyId...")
	if err != nil {
		return nil, err
	}
	if keyID == "" {
		return nil, &cli.FriendlyError{Message: fmt.Sprintf(errNotEmptyTemplate, "aws keyId")}
	}

	secretAccessKey, err := prompter.GetTextInput("Enter your IAM Secret Access Keys:", "Enter IAM access key...")
	if err != nil {
		return nil, err
	}
	if secretAccessKey == "" {
		return nil, &cli.FriendlyError{Message: fmt.Sprintf(errNotEmptyTemplate, "iam access key")}
	}

	return verifyAWSCredentials(ctx, keyID, secretAccessKey, region)
}

func parseAWSProviderNonInteractive(cmd *cobra.Command) (*radAWS.Provider, error) {
	ctx := cmd.Context()

	addAWSProvider, err := cmd.Flags().GetBool(AWSProviderFlagName)
	if err != nil {
		return nil, err
	}
	if !addAWSProvider {
		return nil, nil
	}

	keyID, err := cmd.Flags().GetString(AWSProviderAccessKeyIdFlagName)
	if err != nil {
		return nil, err
	}

	secretAccessKey, err := cmd.Flags().GetString(AWSProviderSecretAccessKeyFlagName)
	if err != nil {
		return nil, err
	}

	region, err := cmd.Flags().GetString(AWSProviderRegionFlagName)
	if err != nil {
		return nil, err
	}

	return verifyAWSCredentials(ctx, keyID, secretAccessKey, region)
}

func verifyAWSCredentials(ctx context.Context, keyID string, secretAccessKey string, region string) (*radAWS.Provider, error) {
	credentialsProvider := credentials.NewStaticCredentialsProvider(keyID, secretAccessKey, "")
	stsClient := sts.New(sts.Options{
		Region:      region,
		Credentials: credentialsProvider,
	})
	result, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, fmt.Errorf("AWS credential verification failed: %s", err.Error())
	}

	return &radAWS.Provider{
		AccessKeyId:     keyID,
		SecretAccessKey: secretAccessKey,
		TargetRegion:    region,
		AccountId:       *result.Account,
	}, nil
}
