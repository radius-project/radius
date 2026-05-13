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

package preview

import (
	"context"

	"github.com/radius-project/radius/pkg/cli/aws"
	"github.com/radius-project/radius/pkg/cli/azure"
	"github.com/radius-project/radius/pkg/cli/cmd/radinit/common"
)

const (
	confirmCloudProviderBackNavigationSentinel = common.ConfirmCloudProviderBackNavigationSentinel
	confirmCloudProviderPrompt                 = common.ConfirmCloudProviderPrompt
	confirmCloudProviderAdditionalPrompt       = common.ConfirmCloudProviderAdditionalPrompt
	selectCloudProviderPrompt                  = common.SelectCloudProviderPrompt
)

func (r *Runner) enterCloudProviderOptions(ctx context.Context, options *initOptions) error {
	result, err := common.EnterCloudProviderOptions(
		r.Prompter,
		r.Full,
		options.Environment.Create,
		func() (*azure.Provider, error) { return r.enterAzureCloudProvider(ctx, options) },
		func() (*aws.Provider, error) { return r.enterAWSCloudProvider(ctx, options) },
	)
	if err != nil {
		return err
	}
	options.CloudProviders.Azure = result.Azure
	options.CloudProviders.AWS = result.AWS
	return nil
}
