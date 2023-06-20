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

package commonflags

import (
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/spf13/cobra"
)

const (
	// AzureSubscriptionIdFlag provides azure subscription Id.
	AzureSubscriptionIdFlag = "azure-subscription-id"
	// AzureResourceGroupFlag provides azure resource group.
	AzureResourceGroupFlag = "azure-resource-group"
	// AWSRegionFlag provides aws region.
	AWSRegionFlag = "aws-region"
	// AWSAccountIdFlag provides aws accound id.
	AWSAccountIdFlag = "aws-account-id"
	// ClearEnvAzureFlag tells the command to clear azure scope on the environment it is configured.
	ClearEnvAzureFlag = "clear-azure"
	// ClearEnvAWSFlag tells the command to clear aws scope on the environment it is configured.
	ClearEnvAWSFlag = "clear-aws"
)

func AddOutputFlag(cmd *cobra.Command) {
	description := fmt.Sprintf("output format (supported formats are %s)", strings.Join(output.SupportedFormats(), ", "))
	cmd.Flags().StringP("output", "o", output.DefaultFormat, description)
}

func AddWorkspaceFlag(cmd *cobra.Command) {
	cmd.Flags().StringP("workspace", "w", "", "The workspace name")
}

func AddResourceGroupFlag(cmd *cobra.Command) {
	cmd.Flags().StringP("group", "g", "", "The resource group name")
}

func AddApplicationNameFlag(cmd *cobra.Command) {
	cmd.Flags().StringP("application", "a", "", "The application name")
}

func AddConfirmationFlag(cmd *cobra.Command) {
	cmd.Flags().BoolP("yes", "y", false, "The confirmation flag")
}

func AddEnvironmentNameFlag(cmd *cobra.Command) {
	cmd.Flags().StringP("environment", "e", "", "The environment name")
}

func AddNamespaceFlag(cmd *cobra.Command) {
	cmd.Flags().StringP("namespace", "n", "", "The Kubernetes namespace")
}

func AddParameterFlag(cmd *cobra.Command) {
	cmd.Flags().StringArrayP("parameters", "p", []string{}, "Specify parameters for the deployment")
}

func AddLinkTypeFlag(cmd *cobra.Command) {
	cmd.Flags().String("link-type", "", "Specify the type of the link this recipe can be consumed by")
}

func AddAzureScopeFlags(cmd *cobra.Command) {
	AddAzureSubscriptionFlag(cmd)
	AddAzureResourceGroupFlag(cmd)
	cmd.MarkFlagsRequiredTogether(AzureSubscriptionIdFlag, AzureResourceGroupFlag)
	cmd.MarkFlagsMutuallyExclusive(AzureSubscriptionIdFlag, ClearEnvAzureFlag)
	cmd.MarkFlagsMutuallyExclusive(AzureResourceGroupFlag, ClearEnvAzureFlag)
}

func AddAzureSubscriptionFlag(cmd *cobra.Command) {
	cmd.Flags().String(AzureSubscriptionIdFlag, "", "The subscription ID where Azure resources will be deployed")
}

func AddAzureResourceGroupFlag(cmd *cobra.Command) {
	cmd.Flags().String(AzureResourceGroupFlag, "", "The resource group where Azure resources will be deployed")
}

func AddAWSScopeFlags(cmd *cobra.Command) {
	AddAWSRegionFlag(cmd)
	AddAWSAccountFlag(cmd)
	cmd.MarkFlagsRequiredTogether(AWSRegionFlag, AWSAccountIdFlag)
	cmd.MarkFlagsMutuallyExclusive(AWSRegionFlag, ClearEnvAWSFlag)
	cmd.MarkFlagsMutuallyExclusive(AWSAccountIdFlag, ClearEnvAWSFlag)
}

func AddAWSRegionFlag(cmd *cobra.Command) {
	cmd.Flags().String(AWSRegionFlag, "", "The region where AWS resources will be deployed")
}

func AddAWSAccountFlag(cmd *cobra.Command) {
	cmd.Flags().String(AWSAccountIdFlag, "", "The account ID where AWS resources will be deployed")
}

func AddKubeContextFlagVar(cmd *cobra.Command, ref *string) {
	cmd.Flags().StringVar(ref, "kubecontext", "", "The Kubernetes context to use, will use the default if unset")
}
