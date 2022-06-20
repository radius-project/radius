// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"errors"
	"fmt"

	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/spf13/cobra"

	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
)

var envInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a RAD environment",
	Long:  `Create a RAD environment`,
}

func init() {
	envCmd.AddCommand(envInitCmd)
	envInitCmd.PersistentFlags().BoolP("interactive", "i", false, "Collect values for required command arguments through command line interface prompts")
	envInitCmd.PersistentFlags().StringP("namespace", "n", "default", "Specify the namespace to use for the environment into which application resources are deployed")
	envInitCmd.PersistentFlags().String("chart", "", "Specify a file path to a helm chart to install radius from")
	envInitCmd.PersistentFlags().String("image", "", "Specify the radius controller image to use")
	envInitCmd.PersistentFlags().String("tag", "", "Specify the radius controller tag to use")
	envInitCmd.PersistentFlags().String("appcore-image", "", "Specify Application.Core RP image to use")
	envInitCmd.PersistentFlags().String("appcore-tag", "", "Specify Application.Core RP image tag to use")
}

type sharedArgs struct {
	Interactive  bool
	Namespace    string
	ChartPath    string
	Image        string
	Tag          string
	AppCoreImage string
	AppCoreTag   string
}

type EnvKind int

const (
	Azure EnvKind = iota
	Kubernetes
	Dev
)

func parseArgs(cmd *cobra.Command) (sharedArgs, error) {
	// the below function call should never errors given a default is defined
	interactive, err := cmd.Flags().GetBool("interactive")
	if err != nil {
		return sharedArgs{}, err
	}
	namespace, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return sharedArgs{}, err
	}
	chartPath, err := cmd.Flags().GetString("chart")
	if err != nil {
		return sharedArgs{}, err
	}
	image, err := cmd.Flags().GetString("image")
	if err != nil {
		return sharedArgs{}, err
	}
	tag, err := cmd.Flags().GetString("tag")
	if err != nil {
		return sharedArgs{}, err
	}
	appcoreImage, err := cmd.Flags().GetString("appcore-image")
	if err != nil {
		return sharedArgs{}, err
	}

	appcoreTag, err := cmd.Flags().GetString("appcore-tag")
	if err != nil {
		return sharedArgs{}, err
	}

	return sharedArgs{
		Interactive: interactive,
		Namespace:   namespace,
		ChartPath:   chartPath,
		Image:       image,
		Tag:         tag,
		AppCoreImage: appcoreImage,
		AppCoreTag: appcoreTag,
	}, nil
}

func parseAzureProviderFromArgs(cmd *cobra.Command, interactive bool) (*azure.Provider, error) {
	if interactive {
		return parseAzureProviderInteractive(cmd)
	}
	return parseAzureProviderNonInteractive(cmd)
}

func parseAzureProviderInteractive(cmd *cobra.Command) (*azure.Provider, error) {
	authorizer, err := auth.NewAuthorizerFromCLI()
	if err != nil {
		return nil, err
	}

	addAzureSPN, err := prompt.ConfirmWithDefault("Add Azure provider for cloud resources [y/N]?", prompt.No)
	if err != nil {
		return nil, err
	}
	if !addAzureSPN {
		return &azure.Provider{}, nil
	}

	subscription, err := selectSubscription(cmd.Context(), authorizer)
	if err != nil {
		return nil, err
	}
	resourceGroup, err := selectResourceGroup(cmd.Context(), authorizer, subscription)
	if err != nil {
		return nil, err
	}

	fmt.Printf(
		"\nA Service Principal Name (SPN) with a corresponding role assignment and scope for your resource group is required to create Azure resources.\n\nFor example, you can create one using the following command:\n\033[36maz ad sp create-for-rbac --role Owner --scope /subscriptions/%s/resourceGroups/%s\033[0m\n\nFor more information, see: https://docs.microsoft.com/cli/azure/ad/sp?view=azure-cli-latest#az-ad-sp-create-for-rbac and https://aka.ms/azadsp-more\n\n",
		subscription.SubscriptionID,
		resourceGroup,
	)

	clientID, err := prompt.Text(
		"Enter the `appId` of the service principal used to create Azure resources:",
		prompt.UUIDv4Validator,
	)
	if err != nil {
		return nil, err
	}

	clientSecret, err := prompt.Text(
		"Enter the `password` of the service principal used to create Azure resources:",
		prompt.EmptyValidator,
	)
	if err != nil {
		return nil, err
	}

	tenantID, err := prompt.Text(
		"Enter the `tenant` of the service principal used to create Azure resources:",
		prompt.UUIDv4Validator,
	)
	if err != nil {
		return nil, err
	}

	return &azure.Provider{
		SubscriptionID: subscription.SubscriptionID,
		ResourceGroup:  resourceGroup,
		ServicePrincipal: &azure.ServicePrincipal{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			TenantID:     tenantID,
		},
	}, nil
}

func parseAzureProviderNonInteractive(cmd *cobra.Command) (*azure.Provider, error) {
	subscriptionID, err := cmd.Flags().GetString("provider-azure-subscription")
	if err != nil {
		return nil, err
	}
	resourceGroup, err := cmd.Flags().GetString("provider-azure-resource-group")
	if err != nil {
		return nil, err
	}

	addAzureSPN, err := cmd.Flags().GetBool("provider-azure")
	if err != nil {
		return nil, err
	}
	if !addAzureSPN {
		if subscriptionID == "" && resourceGroup == "" {
			return nil, nil
		}
		return &azure.Provider{
			SubscriptionID: subscriptionID,
			ResourceGroup:  resourceGroup,
		}, nil
	}
	clientID, err := cmd.Flags().GetString("provider-azure-client-id")
	if err != nil {
		return nil, err
	}
	clientSecret, err := cmd.Flags().GetString("provider-azure-client-secret")
	if err != nil {
		return nil, err
	}
	tenantID, err := cmd.Flags().GetString("provider-azure-tenant-id")
	if err != nil {
		return nil, err
	}
	if isValid, _ := prompt.UUIDv4Validator(subscriptionID); !isValid {
		return nil, fmt.Errorf("--provider-azure-subscription is required to configure Azure provider for cloud resources")
	}
	if resourceGroup == "" {
		return nil, fmt.Errorf("--provider-azure-resource-group is required to configure Azure provider for cloud resources")
	}
	if isValid, _ := prompt.UUIDv4Validator(clientID); !isValid {
		return nil, errors.New("--provider-azure-client-id parameter is required to configure Azure provider for cloud resources")
	}
	if clientSecret == "" {
		return nil, errors.New("--provider-azure-client-secret parameter is required to configure Azure provider for cloud resources")
	}
	if isValid, _ := prompt.UUIDv4Validator(tenantID); !isValid {
		return nil, errors.New("--provider-azure-tenant-id parameter is required to configure Azure provider for cloud resources")
	}
	if (subscriptionID != "") != (resourceGroup != "") {
		return nil, fmt.Errorf("to use the Azure provider both --provider-azure-subscription and --provider-azure-resource-group must be provided")
	}

	return &azure.Provider{
		SubscriptionID: subscriptionID,
		ResourceGroup:  resourceGroup,
		ServicePrincipal: &azure.ServicePrincipal{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			TenantID:     tenantID,
		},
	}, nil
}

func selectNamespace(cmd *cobra.Command, defaultVal string, interactive bool) (string, error) {
	var val string
	var err error
	if interactive {
		promptMsg := fmt.Sprintf("Enter a namespace name to deploy apps into [%s]:", defaultVal)
		val, err = prompt.TextWithDefault(promptMsg, &defaultVal, prompt.EmptyValidator)
		if err != nil {
			return "", err
		}
		fmt.Printf("Using %s as namespace name\n", val)
	} else {
		val, _ = cmd.Flags().GetString("namespace")
		if val == "" {
			output.LogInfo("No namespace name provided, using: %v", defaultVal)
			val = defaultVal
		}
	}
	return val, nil
}

func selectEnvironment(cmd *cobra.Command, defaultVal string, interactive bool) (string, error) {
	var val string
	var err error
	if interactive {
		promptMsg := fmt.Sprintf("Enter an environment name [%s]:", defaultVal)
		val, err = prompt.TextWithDefault(promptMsg, &defaultVal, prompt.EmptyValidator)
		if err != nil {
			return "", err
		}
		fmt.Printf("Using %s as environment name\n", val)
	} else {
		val, _ = cmd.Flags().GetString("environment")
		if val == "" {
			output.LogInfo("No environment name provided, using: %v", defaultVal)
			val = defaultVal
		}
	}
	return val, nil
}

// Setup flags to configure Azure provider for cloud resources
func registerAzureProviderFlags(cmd *cobra.Command) {
	cmd.Flags().BoolP("provider-azure", "", false, "Add Azure provider for cloud resources")
	cmd.Flags().String("provider-azure-subscription", "", "Azure subscription for cloud resources")
	cmd.Flags().String("provider-azure-resource-group", "", "Azure resource-group for cloud resources")
	cmd.Flags().StringP("provider-azure-client-id", "", "", "The client id for the service principal")
	cmd.Flags().StringP("provider-azure-client-secret", "", "", "The client secret for the service principal")
	cmd.Flags().StringP("provider-azure-tenant-id", "", "", "The tenant id for the service principal")
}
