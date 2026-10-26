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

package radinit

import (
	"context"
	"fmt"

	"charm.land/bubbles/v2/textinput"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/radius-project/radius/pkg/cli/aws"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/prompt"
)

const (
	selectAWSRegionPrompt                 = "Select the region you would like to deploy AWS resources to:"
	selectAWSCredentialKindPrompt         = "Select a credential kind for the AWS credential:"
	enterAWSIAMAcessKeyIDPrompt           = "Enter the IAM access key id:"
	enterAWSRoleARNPrompt                 = "Enter the role ARN:"
	enterAWSRoleARNPlaceholder            = "Enter IAM role ARN..."
	enterAWSIAMAcessKeyIDPlaceholder      = "Enter IAM access key id..."
	enterAWSIAMSecretAccessKeyPrompt      = "Enter your IAM Secret Access Key:"
	enterAWSIAMSecretAccessKeyPlaceholder = "Enter IAM secret access key..."
	errNotEmptyTemplate                   = "%s cannot be empty"
	confirmAWSAccountIDPromptFmt          = "Use account id '%v'?"
	enterAWSAccountIDPrompt               = "Enter the account ID:"
	enterAWSAccountIDPlaceholder          = "Enter the account ID you want to use..."

	awsAccessKeysCreateInstructionFmt = "\nAWS IAM Access keys (Access key ID and Secret access key) are required to access and create AWS resources.\n\nFor example, you can create one using the following command:\n\033[36maws iam create-access-key\033[0m\n\nFor more information refer to https://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_access-keys.html.\n\n"
	awsIRSACredentialKind             = "IRSA"
	awsAccessKeyCredentialKind        = "Access Key"
)

// enterAWSCloudProvider prompts the user for AWS cloud provider configuration. When IRSA is
// selected it also enables IRSA in the Helm values.
func (r *Runner) enterAWSCloudProvider(ctx context.Context, options *initOptions) (*aws.Provider, error) {
	credentialKind, err := selectAWSCredentialKind(r.Prompter)
	if err != nil {
		return nil, err
	}

	switch credentialKind {
	case awsAccessKeyCredentialKind:
		r.Output.LogInfo(awsAccessKeysCreateInstructionFmt)

		accessKeyID, err := r.Prompter.GetTextInput(enterAWSIAMAcessKeyIDPrompt, prompt.TextInputOptions{Placeholder: enterAWSIAMAcessKeyIDPlaceholder})
		if err != nil {
			return nil, err
		}

		secretAccessKey, err := r.Prompter.GetTextInput(enterAWSIAMSecretAccessKeyPrompt, prompt.TextInputOptions{Placeholder: enterAWSIAMSecretAccessKeyPlaceholder, EchoMode: textinput.EchoPassword})
		if err != nil {
			return nil, err
		}

		accountID, err := getAWSAccountID(ctx, r.Prompter, r.awsClient)
		if err != nil {
			return nil, err
		}

		region, err := selectAWSRegion(ctx, r.Prompter, r.awsClient)
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
	case awsIRSACredentialKind:
		r.Output.LogInfo(awsAccessKeysCreateInstructionFmt)

		roleARN, err := r.Prompter.GetTextInput(enterAWSRoleARNPrompt, prompt.TextInputOptions{Placeholder: enterAWSRoleARNPlaceholder})
		if err != nil {
			return nil, err
		}

		accountID, err := getAWSAccountID(ctx, r.Prompter, r.awsClient)
		if err != nil {
			return nil, err
		}

		region, err := selectAWSRegion(ctx, r.Prompter, r.awsClient)
		if err != nil {
			return nil, err
		}

		// Set the value for the Helm chart.
		options.SetValues = append(options.SetValues, "global.aws.irsa.enabled=true")

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

// getAWSAccountID retrieves the AWS account ID via the configured AWS client and
// optionally allows the user to override it.
func getAWSAccountID(ctx context.Context, prompter prompt.Interface, awsClient aws.Client) (string, error) {
	callerIdentityOutput, err := awsClient.GetCallerIdentity(ctx)
	if err != nil {
		return "", clierrors.MessageWithCause(err, "AWS Cloud Provider setup failed, please use aws configure to set up the configuration. More information :https://docs.aws.amazon.com/cli/latest/userguide/cli-chap-configure.html")
	}

	if callerIdentityOutput.Account == nil {
		return "", clierrors.MessageWithCause(err, "AWS credential verification failed: Account ID is nil.")
	}

	accountID := *callerIdentityOutput.Account
	useDetectedAccount, err := prompt.YesOrNoPrompt(fmt.Sprintf(confirmAWSAccountIDPromptFmt, accountID), prompt.ConfirmYes, prompter)
	if err != nil {
		return "", err
	}

	if !useDetectedAccount {
		accountID, err = prompter.GetTextInput(enterAWSAccountIDPrompt, prompt.TextInputOptions{Placeholder: enterAWSAccountIDPlaceholder})
		if err != nil {
			return "", err
		}
	}

	return accountID, nil
}

// selectAWSRegion prompts the user to select an AWS region from the list of
// regions available to the configured AWS account.
func selectAWSRegion(ctx context.Context, prompter prompt.Interface, awsClient aws.Client) (string, error) {
	listRegionsOutput, err := awsClient.ListRegions(ctx)
	if err != nil {
		return "", clierrors.MessageWithCause(err, "Listing AWS regions failed.")
	}

	regions := buildAWSRegionsList(listRegionsOutput)
	selectedRegion, err := prompter.GetListInput(regions, selectAWSRegionPrompt)
	if err != nil {
		return "", err
	}

	return selectedRegion, nil
}

// buildAWSRegionsList extracts region names from the AWS DescribeRegions output.
func buildAWSRegionsList(listRegionsOutput *ec2.DescribeRegionsOutput) []string {
	regions := []string{}
	for _, region := range listRegionsOutput.Regions {
		regions = append(regions, *region.RegionName)
	}

	return regions
}

// selectAWSCredentialKind prompts the user to select an AWS credential kind.
func selectAWSCredentialKind(prompter prompt.Interface) (string, error) {
	return prompter.GetListInput(buildAWSCredentialKindList(), selectAWSCredentialKindPrompt)
}

// buildAWSCredentialKindList returns the list of supported AWS credential kinds.
func buildAWSCredentialKindList() []string {
	return []string{
		awsAccessKeyCredentialKind,
		awsIRSACredentialKind,
	}
}
