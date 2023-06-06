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
	"errors"
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/aws"
	"github.com/project-radius/radius/pkg/cli/prompt"
)

const (
	enterAWSRegionPrompt                  = "Enter the region you would like to use to deploy AWS resources:"
	enterAWSRegionPlaceholder             = "Enter a region..."
	enterAWSIAMAcessKeyIDPrompt           = "Enter the IAM access key id:"
	enterAWSIAMAcessKeyIDPlaceholder      = "Enter IAM access key id..."
	enterAWSIAMSecretAccessKeyPrompt      = "Enter your IAM Secret Access Keys:"
	enterAWSIAMSecretAccessKeyPlaceholder = "Enter IAM access key..."
	errNotEmptyTemplate                   = "%s cannot be empty"
)

func (r *Runner) enterAWSCloudProvider(ctx context.Context, options *initOptions) (*aws.Provider, error) {
	region, err := r.Prompter.GetTextInput(enterAWSRegionPrompt, prompt.TextInputOptions{Placeholder: enterAWSRegionPlaceholder})
	if errors.Is(err, &prompt.ErrExitConsole{}) {
		return nil, &cli.FriendlyError{Message: err.Error()}
	} else if err != nil {
		return nil, err
	}

	accessKeyID, err := r.Prompter.GetTextInput(enterAWSIAMAcessKeyIDPrompt, prompt.TextInputOptions{Placeholder: enterAWSIAMAcessKeyIDPlaceholder})
	if errors.Is(err, &prompt.ErrExitConsole{}) {
		return nil, &cli.FriendlyError{Message: err.Error()}
	} else if err != nil {
		return nil, err
	}

	secretAccessKey, err := r.Prompter.GetTextInput(enterAWSIAMSecretAccessKeyPrompt, prompt.TextInputOptions{Placeholder: enterAWSIAMSecretAccessKeyPlaceholder, EchoMode: textinput.EchoPassword})
	if errors.Is(err, &prompt.ErrExitConsole{}) {
		return nil, &cli.FriendlyError{Message: err.Error()}
	} else if err != nil {
		return nil, err
	}

	result, err := r.awsClient.GetCallerIdentity(ctx, region, accessKeyID, secretAccessKey)
	if err != nil {
		return nil, &cli.FriendlyError{Message: fmt.Sprintf("AWS credential verification failed: %s", err.Error())}
	}

	return &aws.Provider{
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		Region:          region,
		AccountID:       *result.Account,
	}, nil
}
