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

	"github.com/radius-project/radius/pkg/cli/azure"
	"github.com/radius-project/radius/pkg/cli/cmd/radinit/common"
)

func (r *Runner) enterAzureCloudProvider(ctx context.Context, options *initOptions) (*azure.Provider, error) {
	provider, err := common.EnterAzureCloudProvider(ctx, r.Prompter, r.Output, r.azureClient)
	if err != nil {
		return nil, err
	}

	if provider.CredentialKind == azure.AzureCredentialKindWorkloadIdentity {
		// Set the value for the Helm chart.
		options.SetValues = append(options.SetValues, "global.azureWorkloadIdentity.enabled=true")
	}

	return provider, nil
}
