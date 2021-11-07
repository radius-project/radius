// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/containerservice/mgmt/containerservice"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/azure/clients"
	"github.com/Azure/radius/pkg/cli"
	radazure "github.com/Azure/radius/pkg/cli/azure"
	"github.com/Azure/radius/pkg/cli/azure/selector"
	"github.com/Azure/radius/pkg/cli/output"
	"github.com/Azure/radius/pkg/keys"
	"github.com/Azure/radius/pkg/version"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

// Placeholder is for the 'channel'
const armTemplateURIFormat = "https://radiuspublic.blob.core.windows.net/environment/%s/rp-full.json"

var requiredFeatures = map[string]string{
	"EnablePodIdentityPreview": "Microsoft.ContainerService",
}

var envInitAzureCmd = &cobra.Command{
	Use:   "azure",
	Short: "Create a Radius environment on Azure",
	Long:  `Create a Radius environment and deploy to a specified Azure resource group and subscription.`,
	Example: `
# Create a Radius environment in interactive mode
## If an environment of the same name, resource group, and subscription
## already exists, Radius will attach to it instead and update the
## local config with these details.
rad env init azure -i

# Create a Radius environment using flags
## If an environment of the same name, resource group, and subscription
## already exists Radius will connect to it instead of deploying a new one.
rad env init azure -e myenv --subscription-id SUB-ID-GUID --resource-group RG-NAME --location westus2
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		a, err := validate(cmd, args)
		if err != nil {
			return err
		}

		if a.Interactive {
			a.SubscriptionID, a.ResourceGroup, err = selector.Select(cmd.Context())
			if err != nil {
				return err
			}
		}

		if a.Name == "" {
			a.Name = a.ResourceGroup
		}

		err = connect(cmd.Context(), a.Name, a.SubscriptionID, a.ResourceGroup, a.Location, a.DeploymentTemplate, a.ContainerRegistry, a.LogAnalyticsWorkspaceID)
		if err != nil {
			return err
		}

		return nil
	},
}

func init() {
	envInitCmd.AddCommand(envInitAzureCmd)

	envInitAzureCmd.Flags().StringP("subscription-id", "s", "", "The subscription ID to use for the environment")
	envInitAzureCmd.Flags().StringP("resource-group", "g", "", "The resource group to use for the environment")
	envInitAzureCmd.Flags().StringP("location", "l", "", "The Azure location to use for the environment")
	envInitAzureCmd.Flags().BoolP("interactive", "i", false, "Specify interactive to choose subscription and resource group interactively")
	envInitAzureCmd.Flags().String("container-registry", "", "Specify the name of an existing Azure Container Registry to grant the environment access to pull containers from the registry")
	envInitAzureCmd.Flags().String("loganalytics-workspace-id", "", "Specify the ARM resource ID of the log analytics workspace where the logs should be redirected to")

	// development support
	envInitAzureCmd.Flags().StringP("deployment-template", "t", "", "The file path to the deployment template - this can be used to override a custom build of the environment deployment ARM template for testing")
}

type arguments struct {
	Name                    string
	Interactive             bool
	SubscriptionID          string
	ResourceGroup           string
	Location                string
	DeploymentTemplate      string
	ContainerRegistry       string
	LogAnalyticsWorkspaceID string
}

func validate(cmd *cobra.Command, args []string) (arguments, error) {
	interactive, err := cmd.Flags().GetBool("interactive")
	if err != nil {
		return arguments{}, err
	}

	name, err := cmd.Flags().GetString("environment")
	if err != nil {
		return arguments{}, err
	}

	subscriptionID, err := cmd.Flags().GetString("subscription-id")
	if err != nil {
		return arguments{}, err
	}

	resourceGroup, err := cmd.Flags().GetString("resource-group")
	if err != nil {
		return arguments{}, err
	}

	location, err := cmd.Flags().GetString("location")
	if err != nil {
		return arguments{}, err
	}

	if interactive && (subscriptionID != "" || resourceGroup != "" || location != "") {
		return arguments{}, errors.New("subcription id, resource group or location cannot be specified with interactive")
	}

	if !interactive && (subscriptionID == "" || resourceGroup == "" || location == "") {
		return arguments{}, errors.New("subscription id, resource group and location must be specified")
	}

	deploymentTemplate, err := cmd.Flags().GetString("deployment-template")
	if err != nil {
		return arguments{}, err
	}

	if deploymentTemplate != "" {
		_, err := os.Stat(deploymentTemplate)
		if err != nil {
			return arguments{}, fmt.Errorf("could not read deployment-template: %w", err)
		}
	}

	registryName, err := cmd.Flags().GetString("container-registry")
	if err != nil {
		return arguments{}, err
	}

	logAnalyticsWorkspaceID, err := cmd.Flags().GetString("loganalytics-workspace-id")
	if err != nil {
		return arguments{}, err
	}

	if location != "" && !selector.IsSupportedLocation(location) {
		return arguments{}, fmt.Errorf("the location '%s' is not supported. choose from: %s", location, strings.Join(selector.SupportedLocations[:], ", "))
	}

	return arguments{
		Name:                    name,
		Interactive:             interactive,
		SubscriptionID:          subscriptionID,
		ResourceGroup:           resourceGroup,
		Location:                location,
		DeploymentTemplate:      deploymentTemplate,
		ContainerRegistry:       registryName,
		LogAnalyticsWorkspaceID: logAnalyticsWorkspaceID,
	}, nil
}

func connect(ctx context.Context, name string, subscriptionID string, resourceGroup, location string, deploymentTemplate string, registryName string, logAnalyticsWorkspaceID string) error {
	armauth, err := armauth.GetArmAuthorizer()
	if err != nil {
		return err
	}

	// Check for an existing RP in the target resource group. This way we
	// can use a single command to bind to an existing environment
	exists, clusterName, err := findExistingEnvironment(ctx, armauth, subscriptionID, resourceGroup)
	if err != nil {
		return err
	}

	envUrl, err := radazure.GenerateAzureEnvUrl(subscriptionID, resourceGroup)
	if err != nil {
		return err
	}

	if exists {
		// We already have a provider in this resource group
		output.LogInfo("Found existing environment...\n\n"+
			"Environment '%v' available at:\n%v\n", name, envUrl)
		err = storeEnvironment(ctx, armauth, name, subscriptionID, resourceGroup, radazure.GetControlPlaneResourceGroup(resourceGroup), clusterName)
		if err != nil {
			return err
		}

		return nil
	}

	group, err := validateSubscription(ctx, armauth, subscriptionID, resourceGroup)
	if err != nil {
		return err
	}

	// Register the subscription for required features
	err = registerSubscription(ctx, armauth, subscriptionID)
	if err != nil {
		return err
	}

	registryID := ""
	if registryName != "" {
		registryID, err = validateRegistry(ctx, armauth, subscriptionID, registryName)
		if err != nil {
			return err
		}
	}

	logAnalyticsWorkspaceName := ""
	if logAnalyticsWorkspaceID != "" {
		logAnalyticsWorkspaceName, err = validateLogAnalyticsWorkspace(ctx, armauth, subscriptionID, logAnalyticsWorkspaceID)
		if err != nil {
			return err
		}
	}

	if group != nil {
		if !selector.IsSupportedLocation(*group.Location) {
			return fmt.Errorf("the location '%s' of resource group '%s' is not supported. choose from: %s", *group.Location, *group.Name, strings.Join(selector.SupportedLocations[:], ", "))
		}

		location = *group.Location
	}

	params := deploymentParameters{
		ResourceGroup:             resourceGroup,
		ControlPlaneResourceGroup: radazure.GetControlPlaneResourceGroup(resourceGroup),
		Location:                  location,
		DeploymentTemplate:        deploymentTemplate,
		RegistryID:                registryID,
		RegistryName:              registryName,
		LogAnalyticsWorkspaceName: logAnalyticsWorkspaceName,
		LogAnalyticsWorkspaceID:   logAnalyticsWorkspaceID,
	}
	deployment, err := deployEnvironment(ctx, armauth, name, subscriptionID, params)
	if err != nil {
		return err
	}

	clusterName, err = findClusterInDeployment(ctx, deployment)
	if err != nil {
		return err
	}

	err = storeEnvironment(ctx, armauth, name, subscriptionID, resourceGroup, params.ControlPlaneResourceGroup, clusterName)
	if err != nil {
		return err
	}

	return nil
}

func findExistingEnvironment(ctx context.Context, authorizer autorest.Authorizer, subscriptionID string, resourceGroup string) (bool, string, error) {
	cpc := clients.NewCustomResourceProviderClient(subscriptionID, authorizer)

	_, err := cpc.Get(ctx, resourceGroup, "radius")
	if clients.Is404Error(err) {
		// not found - will need to be created
		return false, "", nil
	} else if err != nil {
		return false, "", err
	}

	_, err = cpc.Get(ctx, resourceGroup, "radiusv3")
	if clients.Is404Error(err) {
		// not found - will need to be created
		return false, "", nil
	} else if err != nil {
		return false, "", err
	}

	// Custom Provider already exists, find the cluster...
	mcc := clients.NewManagedClustersClient(subscriptionID, authorizer)

	var cluster *containerservice.ManagedCluster
	for list, err := mcc.ListByResourceGroupComplete(ctx, radazure.GetControlPlaneResourceGroup(resourceGroup)); list.NotDone(); err = list.NextWithContext(ctx) {
		if err != nil {
			return false, "", fmt.Errorf("cannot read AKS clusters: %w", err)
		}

		if keys.HasRadiusEnvironmentTag(list.Value().Tags) {
			temp := list.Value()
			cluster = &temp
			break
		}
	}

	if cluster == nil {
		return false, "", fmt.Errorf("could not find an AKS instance in resource group '%v'", radazure.GetControlPlaneResourceGroup(resourceGroup))
	}

	return true, *cluster.Name, nil
}

func validateSubscription(ctx context.Context, authorizer autorest.Authorizer, subscriptionID string, resourceGroup string) (*resources.Group, error) {
	step := output.BeginStep("Validating Subscription...")

	sc := clients.NewSubscriptionClient(authorizer)

	_, err := sc.Get(ctx, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("cannot find subscription with id '%v'", subscriptionID)
	}

	rgc := clients.NewGroupsClient(subscriptionID, authorizer)

	group, err := rgc.Get(ctx, resourceGroup)
	if group.StatusCode == 404 {
		// Ignore the NotFound error for the resource group as it will get created if it does not exist.
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	output.CompleteStep(step)
	return &group, nil
}

func registerSubscription(ctx context.Context, authorizer autorest.Authorizer, subscriptionID string) error {
	step := output.BeginStep("Registering Subscription for required features...")
	fc := clients.NewFeaturesClient(subscriptionID, authorizer)

	providerClient := clients.NewProvidersClient(subscriptionID, authorizer)

	for feature, namespace := range requiredFeatures {
		_, err := fc.Register(ctx, namespace, feature)
		if err != nil {
			return fmt.Errorf("failed to register subscription: %v for feature: %v/%v: %w", subscriptionID, namespace, feature, err)
		}

		// See: https://github.com/Azure/radius/issues/520
		// We've seen users still hitting issues where they see the error:
		// "PodIdentity addon is not allowed since feature 'Microsoft.ContainerService/EnablePodIdentityPreview' is not enabled"
		// Our working theory is that we need to force the provider to be registered again,
		// causing the RP to refresh it's info about features.
		_, err = providerClient.Register(ctx, namespace)
		if err != nil {
			return fmt.Errorf("failed to register subscription: %v for provider %v: %w", subscriptionID, namespace, err)
		}

		output.LogInfo("Sucessfully registered subscriptionid: %v for feature: %v/%v", subscriptionID, namespace, feature)
	}

	output.CompleteStep(step)
	return nil
}

func validateRegistry(ctx context.Context, authorizer autorest.Authorizer, subscriptionID string, registryName string) (string, error) {
	step := output.BeginStep("Validating Container Registry for %s...", registryName)
	crc := clients.NewRegistriesClient(subscriptionID, authorizer)

	for list, err := crc.ListComplete(ctx); list.NotDone(); err = list.NextWithContext(ctx) {
		if err != nil {
			return "", fmt.Errorf("failed while searching container registries: %w", err)
		}

		if *list.Value().Name == registryName {
			output.CompleteStep(step)
			return *list.Value().ID, nil
		}
	}

	return "", fmt.Errorf("failed to find registry %s in subscription %s. The container registry must be in the same subscription as the environment", registryName, subscriptionID)
}

func validateLogAnalyticsWorkspace(ctx context.Context, authorizer autorest.Authorizer, subscriptionID string, logAnalyticsWorkspaceID string) (string, error) {
	step := output.BeginStep("Validating Log Analytics Workspace ID for %s...", logAnalyticsWorkspaceID)
	resource, err := azure.ParseResourceID(logAnalyticsWorkspaceID)
	if err != nil {
		return "", fmt.Errorf("invalid log analytics workspace id: %w", err)
	}
	lwc := clients.NewWorkspacesClient(resource.SubscriptionID, authorizer)
	_, err = lwc.Get(ctx, resource.ResourceGroup, resource.ResourceName)
	if err != nil {
		return "", fmt.Errorf("could not retrieve log analytics workspace: %w", err)
	}
	output.CompleteStep(step)
	return resource.ResourceName, nil
}

func deployEnvironment(ctx context.Context, authorizer autorest.Authorizer, name string, subscriptionID string, params deploymentParameters) (resources.DeploymentExtended, error) {
	envUrl, err := radazure.GenerateAzureEnvUrl(subscriptionID, params.ResourceGroup)
	if err != nil {
		return resources.DeploymentExtended{}, err
	}

	step := output.BeginStep(fmt.Sprintf("Deploying Environment from channel %s...\n\n"+
		"New Environment '%v' with Resource Group '%v' will be available at:\n%v\n\n"+
		"Deployment In Progress...", version.Channel(), name, params.ResourceGroup, envUrl))
	dc := clients.NewDeploymentsClient(subscriptionID, authorizer)
	dc.Authorizer = authorizer

	// Poll faster for completion when possible. The default is 60 seconds which is a long time to wait.
	dc.PollingDelay = time.Second * 15

	// see https://github.com/Azure-Samples/azure-sdk-for-go-samples/blob/master/resources/testdata/parameters.json
	// for an example
	parameters := map[string]interface{}{
		"channel": map[string]interface{}{
			"value": version.Channel(),
		},
		"resourceGroup": map[string]interface{}{
			"value": params.ResourceGroup,
		},
		"controlPlaneResourceGroup": map[string]interface{}{
			"value": params.ControlPlaneResourceGroup,
		},
		"location": map[string]interface{}{
			"value": params.Location,
		},
		"registryId": map[string]interface{}{
			"value": params.RegistryID,
		},
		"registryName": map[string]interface{}{
			"value": params.RegistryName,
		},
		"logAnalyticsWorkspaceName": map[string]interface{}{
			"value": params.LogAnalyticsWorkspaceName,
		},
		"logAnalyticsWorkspaceID": map[string]interface{}{
			"value": params.LogAnalyticsWorkspaceID,
		},
	}

	deploymentProperties := &resources.DeploymentProperties{
		Parameters: parameters,
		Mode:       resources.DeploymentModeIncremental,
	}

	if params.DeploymentTemplate == "" {
		deploymentProperties.TemplateLink = &resources.TemplateLink{
			URI: to.StringPtr(fmt.Sprintf(armTemplateURIFormat, version.Channel())),
		}
	} else {
		output.LogInfo("overriding deployment template: %v", params.DeploymentTemplate)
		templateContent, err := ioutil.ReadFile(params.DeploymentTemplate)
		if err != nil {
			return resources.DeploymentExtended{}, fmt.Errorf("could not read deployment template: %w", err)
		}

		data := map[string]interface{}{}
		err = json.Unmarshal(templateContent, &data)
		if err != nil {
			return resources.DeploymentExtended{}, fmt.Errorf("could not read deployment template as JSON: %w", err)
		}

		deploymentProperties.Template = data
	}
	deploymentName := fmt.Sprintf("rad-create-environment-%v", uuid.New().String())
	op, err := dc.CreateOrUpdateAtSubscriptionScope(ctx, deploymentName, resources.Deployment{
		Location:   &params.Location,
		Properties: deploymentProperties,
	})
	if err != nil {
		return resources.DeploymentExtended{}, err
	}

	err = op.WaitForCompletionRef(ctx, dc.Client)
	if err != nil {
		return resources.DeploymentExtended{}, err
	}

	dep, err := op.Result(dc)
	if err != nil {
		return resources.DeploymentExtended{}, err
	}

	output.CompleteStep(step)
	return dep, nil
}

func findClusterInDeployment(ctx context.Context, deployment resources.DeploymentExtended) (string, error) {
	obj := deployment.Properties.Outputs
	outputs, ok := obj.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("deployment outputs has unexpected type: %T", obj)
	}

	obj, ok = outputs["clusterName"]
	if !ok {
		return "", fmt.Errorf("deployment outputs does not contain cluster name: %v", outputs)
	}

	clusterNameOutput, ok := obj.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("cluster name was of unexpected type: %T", obj)
	}

	obj, ok = clusterNameOutput["value"]
	if !ok {
		return "", fmt.Errorf("cluster name output does not contain value: %v", outputs)
	}

	clusterName, ok := obj.(string)
	if !ok {
		return "", fmt.Errorf("cluster name was of unexpected type: %T", obj)
	}

	return clusterName, nil
}

func storeEnvironment(ctx context.Context, authorizer autorest.Authorizer, name string, subscriptionID string, resourceGroup string, controlPlaneResourceGroup string, clusterName string) error {
	step := output.BeginStep("Updating Config...")

	config := ConfigFromContext(ctx)
	env, err := cli.ReadEnvironmentSection(config)
	if err != nil {
		return err
	}

	env.Items[name] = map[string]interface{}{
		"kind":                      "azure",
		"subscriptionId":            subscriptionID,
		"resourceGroup":             resourceGroup,
		"controlPlaneResourceGroup": controlPlaneResourceGroup,
		"clusterName":               clusterName,
	}
	if len(env.Items) == 1 {
		env.Default = name
	}
	cli.UpdateEnvironmentSection(config, env)

	err = cli.SaveConfig(config)
	if err != nil {
		return err
	}

	output.CompleteStep(step)
	return nil
}

type deploymentParameters struct {
	ResourceGroup             string
	ControlPlaneResourceGroup string
	Location                  string
	DeploymentTemplate        string
	RegistryID                string
	RegistryName              string
	LogAnalyticsWorkspaceName string
	LogAnalyticsWorkspaceID   string
}
