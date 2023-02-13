// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package commonflags

import (
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/spf13/cobra"
)

const (
	SetEnvAzureFlag         = "set-azure"
	AzureSubscriptionIdFlag = "azure-subscriptionid"
	AzureResourceGroupFlag  = "azure-resourcegroup"
	SetEnvAWSFlag           = "set-aws"
	AWSRegionFlag           = "aws-region"
	AWSAccountIdFlag        = "aws-account"
	ClearEnvAzureFlag       = "clear-azure"
	ClearEnvAWSFlag         = "clear-aws"
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

func AddRecipeFlag(cmd *cobra.Command) {
	cmd.Flags().String("name", "", "The recipe name")
}

func AddAzureScopeFlags(cmd *cobra.Command) {
	cmd.Flags().Bool(SetEnvAzureFlag, false, "Specify if azure provider needs to be set on env")
	AddAzureSubscriptionFlag(cmd)
	AddAzureResourceGroupFlag(cmd)
	cmd.MarkFlagsRequiredTogether(SetEnvAzureFlag, AzureSubscriptionIdFlag, AzureResourceGroupFlag)
}

func AddAzureSubscriptionFlag(cmd *cobra.Command) {
	cmd.Flags().String(AzureSubscriptionIdFlag, "", "Subscription id of the azure app on env")
}

func AddAzureResourceGroupFlag(cmd *cobra.Command) {
	cmd.Flags().String(AzureResourceGroupFlag, "", "Resource group of the azure app")
}

func AddAWSScopeFlags(cmd *cobra.Command) {
	cmd.Flags().Bool(SetEnvAWSFlag, false, "Specify if aws provider needs to be set on env")
	AddAWSRegionFlag(cmd)
	AddAWSAccountFlag(cmd)
	cmd.MarkFlagsRequiredTogether(SetEnvAWSFlag, AWSRegionFlag, AWSAccountIdFlag)
}

func AddAWSRegionFlag(cmd *cobra.Command) {
	cmd.Flags().String(AWSRegionFlag, "", "Region of the aws app")
}

func AddAWSAccountFlag(cmd *cobra.Command) {
	cmd.Flags().String(AWSAccountIdFlag, "", "Account Id of the aws app")
}
