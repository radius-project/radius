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

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/project-radius/radius/pkg/cli/aws"
	"github.com/project-radius/radius/pkg/cli/clierrors"
	"github.com/project-radius/radius/pkg/cli/prompt"
)

const (
	DefaultRegion = "us-east-1"

	selectAWSRegionPrompt                 = "Select the region you would like to deploy AWS resources to:"
	enterAWSIAMAcessKeyIDPrompt           = "Enter the IAM access key id:"
	enterAWSIAMAcessKeyIDPlaceholder      = "Enter IAM access key id..."
	enterAWSIAMSecretAccessKeyPrompt      = "Enter your IAM Secret Access Key:"
	enterAWSIAMSecretAccessKeyPlaceholder = "Enter IAM secret access key..."
	errNotEmptyTemplate                   = "%s cannot be empty"

	awsAccessKeysCreateInstructionFmt = "\nAWS IAM Access keys (Access key ID and Secret access key) are required to access and create AWS resources.\n\nFor example, you can create one using the following command:\n\033[36maws iam create-access-key\033[0m\n\nFor more information refer to https://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_access-keys.html.\n\n"
)

func (r *Runner) enterAWSCloudProvider(ctx context.Context, options *initOptions) (*aws.Provider, error) {
	r.Output.LogInfo(awsAccessKeysCreateInstructionFmt)

	accessKeyID, err := r.Prompter.GetTextInput(enterAWSIAMAcessKeyIDPrompt, prompt.TextInputOptions{Placeholder: enterAWSIAMAcessKeyIDPlaceholder})
	if err != nil {
		return nil, err
	}

	secretAccessKey, err := r.Prompter.GetTextInput(enterAWSIAMSecretAccessKeyPrompt, prompt.TextInputOptions{Placeholder: enterAWSIAMSecretAccessKeyPlaceholder, EchoMode: textinput.EchoPassword})
	if err != nil {
		return nil, err
	}

	accountId, err := r.getAccountId(ctx, DefaultRegion, accessKeyID, secretAccessKey)
	if err != nil {
		return nil, err
	}

	region, err := r.selectAWSRegion(ctx, DefaultRegion, accessKeyID, secretAccessKey)
	if err != nil {
		return nil, err
	}

	return &aws.Provider{
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		AccountID:       accountId,
		Region:          region,
	}, nil
}

func (r *Runner) getAccountId(ctx context.Context, region, accessKeyID, secretAccessKey string) (string, error) {
	callerIdentityOutput, err := r.awsClient.GetCallerIdentity(ctx, DefaultRegion, accessKeyID, secretAccessKey)
	if err != nil {
		return "", clierrors.MessageWithCause(err, "AWS credential verification failed.")
	}

	if callerIdentityOutput.Account == nil {
		return "", clierrors.MessageWithCause(err, "AWS credential verification failed: Account ID is nil.")
	}

	return *callerIdentityOutput.Account, nil
}

func (r *Runner) selectAWSRegion(ctx context.Context, region, accessKeyID, secretAccessKey string) (string, error) {
	listRegionsOutput, err := r.awsClient.ListRegions(ctx, region, accessKeyID, secretAccessKey)
	if err != nil {
		return "", clierrors.MessageWithCause(err, "Listing AWS regions failed.")
	}

	regions := r.buildAWSRegionsList(listRegionsOutput)
	selectedRegion, err := r.Prompter.GetListInput(regions, selectAWSRegionPrompt)
	if err != nil {
		return "", err
	}

	return selectedRegion, nil
}

func (r *Runner) buildAWSRegionsList(listRegionsOutput *ec2.DescribeRegionsOutput) []string {
	regions := []string{}
	for _, region := range listRegionsOutput.Regions {
		regions = append(regions, *region.RegionName)
	}

	return regions
}
