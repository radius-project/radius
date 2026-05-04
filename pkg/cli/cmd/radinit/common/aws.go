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

package common

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/radius-project/radius/pkg/cli/aws"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
)

const (
	SelectAWSRegionPrompt                 = "Select the region you would like to deploy AWS resources to:"
	SelectAWSCredentialKindPrompt         = "Select a credential kind for the AWS credential:"
	EnterAWSIAMAcessKeyIDPrompt           = "Enter the IAM access key id:"
	EnterAWSRoleARNPrompt                 = "Enter the role ARN:"
	EnterAWSRoleARNPlaceholder            = "Enter IAM role ARN..."
	EnterAWSIAMAcessKeyIDPlaceholder      = "Enter IAM access key id..."
	EnterAWSIAMSecretAccessKeyPrompt      = "Enter your IAM Secret Access Key:"
	EnterAWSIAMSecretAccessKeyPlaceholder = "Enter IAM secret access key..."
	ErrNotEmptyTemplate                   = "%s cannot be empty"
	ConfirmAWSAccountIDPromptFmt          = "Use account id '%v'?"
	EnterAWSAccountIDPrompt               = "Enter the account ID:"
	EnterAWSAccountIDPlaceholder          = "Enter the account ID you want to use..."

	AWSAccessKeysCreateInstructionFmt = "\nAWS IAM Access keys (Access key ID and Secret access key) are required to access and create AWS resources.\n\nFor example, you can create one using the following command:\n\033[36maws iam create-access-key\033[0m\n\nFor more information refer to https://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_access-keys.html.\n\n"
	AWSIRSACredentialKind             = "IRSA"
	AWSAccessKeyCredentialKind        = "Access Key"
)

// EnterAWSCloudProvider prompts the user for AWS cloud provider configuration.
// The caller is responsible for any post-processing such as enabling IRSA Helm
// values based on the returned provider's CredentialKind.
func EnterAWSCloudProvider(ctx context.Context, prompter prompt.Interface, out output.Interface, awsClient aws.Client) (*aws.Provider, error) {
	credentialKind, err := SelectAWSCredentialKind(prompter)
	if err != nil {
		return nil, err
	}

	switch credentialKind {
	case AWSAccessKeyCredentialKind:
		out.LogInfo(AWSAccessKeysCreateInstructionFmt)

		accessKeyID, err := prompter.GetTextInput(EnterAWSIAMAcessKeyIDPrompt, prompt.TextInputOptions{Placeholder: EnterAWSIAMAcessKeyIDPlaceholder})
		if err != nil {
			return nil, err
		}

		secretAccessKey, err := prompter.GetTextInput(EnterAWSIAMSecretAccessKeyPrompt, prompt.TextInputOptions{Placeholder: EnterAWSIAMSecretAccessKeyPlaceholder, EchoMode: textinput.EchoPassword})
		if err != nil {
			return nil, err
		}

		accountID, err := GetAWSAccountID(ctx, prompter, awsClient)
		if err != nil {
			return nil, err
		}

		region, err := SelectAWSRegion(ctx, prompter, awsClient)
		if err != nil {
			return nil, err
		}

		return &aws.Provider{
			AccessKey: &aws.AccessKeyCredential{
				AccessKeyID:     accessKeyID,
				SecretAccessKey: secretAccessKey,
			},
			CredentialKind: aws.AWSCredentialKindAccessKey,
			AccountID:      accountID,
			Region:         region,
		}, nil
	case AWSIRSACredentialKind:
		out.LogInfo(AWSAccessKeysCreateInstructionFmt)

		roleARN, err := prompter.GetTextInput(EnterAWSRoleARNPrompt, prompt.TextInputOptions{Placeholder: EnterAWSRoleARNPlaceholder})
		if err != nil {
			return nil, err
		}

		accountID, err := GetAWSAccountID(ctx, prompter, awsClient)
		if err != nil {
			return nil, err
		}

		region, err := SelectAWSRegion(ctx, prompter, awsClient)
		if err != nil {
			return nil, err
		}

		return &aws.Provider{
			AccountID:      accountID,
			Region:         region,
			CredentialKind: aws.AWSCredentialKindIRSA,
			IRSA: &aws.IRSACredential{
				RoleARN: roleARN,
			},
		}, nil
	default:
		return nil, clierrors.Message("Invalid AWS credential kind: %s", credentialKind)
	}
}

// GetAWSAccountID retrieves the AWS account ID via the configured AWS client and
// optionally allows the user to override it.
func GetAWSAccountID(ctx context.Context, prompter prompt.Interface, awsClient aws.Client) (string, error) {
	callerIdentityOutput, err := awsClient.GetCallerIdentity(ctx)
	if err != nil {
		return "", clierrors.MessageWithCause(err, "AWS Cloud Provider setup failed, please use aws configure to set up the configuration. More information :https://docs.aws.amazon.com/cli/latest/userguide/cli-chap-configure.html")
	}

	if callerIdentityOutput.Account == nil {
		return "", clierrors.MessageWithCause(err, "AWS credential verification failed: Account ID is nil.")
	}

	accountID := *callerIdentityOutput.Account
	useDetectedAccount, err := prompt.YesOrNoPrompt(fmt.Sprintf(ConfirmAWSAccountIDPromptFmt, accountID), prompt.ConfirmYes, prompter)
	if err != nil {
		return "", err
	}

	if !useDetectedAccount {
		accountID, err = prompter.GetTextInput(EnterAWSAccountIDPrompt, prompt.TextInputOptions{Placeholder: EnterAWSAccountIDPlaceholder})
		if err != nil {
			return "", err
		}
	}

	return accountID, nil
}

// SelectAWSRegion prompts the user to select an AWS region from the list of
// regions available to the configured AWS account.
func SelectAWSRegion(ctx context.Context, prompter prompt.Interface, awsClient aws.Client) (string, error) {
	listRegionsOutput, err := awsClient.ListRegions(ctx)
	if err != nil {
		return "", clierrors.MessageWithCause(err, "Listing AWS regions failed.")
	}

	regions := BuildAWSRegionsList(listRegionsOutput)
	selectedRegion, err := prompter.GetListInput(regions, SelectAWSRegionPrompt)
	if err != nil {
		return "", err
	}

	return selectedRegion, nil
}

// BuildAWSRegionsList extracts region names from the AWS DescribeRegions output.
func BuildAWSRegionsList(listRegionsOutput *ec2.DescribeRegionsOutput) []string {
	regions := []string{}
	for _, region := range listRegionsOutput.Regions {
		regions = append(regions, *region.RegionName)
	}

	return regions
}

// SelectAWSCredentialKind prompts the user to select an AWS credential kind.
func SelectAWSCredentialKind(prompter prompt.Interface) (string, error) {
	return prompter.GetListInput(BuildAWSCredentialKindList(), SelectAWSCredentialKindPrompt)
}

// BuildAWSCredentialKindList returns the list of supported AWS credential kinds.
func BuildAWSCredentialKindList() []string {
	return []string{
		AWSAccessKeyCredentialKind,
		AWSIRSACredentialKind,
	}
}
