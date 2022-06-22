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
	"sort"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/containerservice/mgmt/containerservice"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/azure-sdk-for-go/profiles/preview/preview/subscription/mgmt/subscription"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/azcli"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/cli"
	radazure "github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/project-radius/radius/pkg/featureflag"
	"github.com/project-radius/radius/pkg/handlers"
	"github.com/project-radius/radius/pkg/keys"
	"github.com/project-radius/radius/pkg/version"
)

func init() {
	envInitCmd.AddCommand(envInitAzureCmd)

	envInitAzureCmd.Flags().StringP("subscription-id", "s", "", "The subscription ID to use for the environment")
	envInitAzureCmd.Flags().StringP("resource-group", "g", "", "The resource group to use for the environment")
	envInitAzureCmd.Flags().StringP("location", "l", "", "The Azure location to use for the environment")
	envInitAzureCmd.Flags().String("container-registry", "", "Specify the name of an existing Azure Container Registry to grant the environment access to pull containers from the registry")
	envInitAzureCmd.Flags().String("loganalytics-workspace-id", "", "Specify the ARM resource ID of the log analytics workspace where the logs should be redirected to")

	// development support
	envInitAzureCmd.Flags().StringP("deployment-template", "t", "", "The file path to the deployment template - this can be used to override a custom build of the environment deployment ARM template for testing")
}

type azureArguments struct {
	sharedArgs
	Name                    string
	SubscriptionID          string
	ResourceGroup           string
	Location                string
	DeploymentTemplate      string
	ContainerRegistry       string
	LogAnalyticsWorkspaceID string
}

var supportedLocations = [5]string{
	"australiaeast",
	"eastus",
	"northeurope",
	"westeurope",
	"westus2",
}

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
	RunE: initAzureRadEnvironment,
}

func initAzureRadEnvironment(cmd *cobra.Command, args []string) error {
	a, err := validate(cmd, args)
	if err != nil {
		return err
	}

	if a.Interactive {
		authorizer, err := auth.NewAuthorizerFromCLI()
		if err != nil {
			return err
		}

		selectedSub, err := selectSubscription(cmd.Context(), authorizer)
		if err != nil {
			return err
		}
		a.SubscriptionID = selectedSub.SubscriptionID

		a.ResourceGroup, err = selectResourceGroup(cmd.Context(), authorizer, selectedSub)
		if err != nil {
			return err
		}

		a.Name, err = selectEnvironment(cmd, a.ResourceGroup, true)
		if err != nil {
			return err
		}
	}

	cliOptions := helm.ClusterOptions{
		Namespace: a.Namespace,
		Radius: helm.RadiusOptions{
			ChartPath: a.ChartPath,
			Image:     a.Image,
			Tag:       a.Tag,
		},
	}
	clusterOptions := helm.NewClusterOptions(cliOptions)

	clusterOptions.Radius.AzureProvider = &radazure.Provider{
		SubscriptionID:      a.SubscriptionID,
		ResourceGroup:       a.ResourceGroup,
		PodIdentitySelector: to.StringPtr("radius"),
	}

	// NOTE: the AKS configuration of the azure provider requires access to the cluster name, which we only have after deployment
	// so we configure that later.

	err = connect(cmd.Context(), a.Name, a.SubscriptionID, a.ResourceGroup, a.Location, a.DeploymentTemplate, a.ContainerRegistry, a.LogAnalyticsWorkspaceID, clusterOptions)
	if err != nil {
		return err
	}

	return nil
}

func validate(cmd *cobra.Command, args []string) (azureArguments, error) {
	sharedArgs, err := parseArgs(cmd)
	if err != nil {
		return azureArguments{}, err
	}

	subscriptionID, err := cmd.Flags().GetString("subscription-id")
	if err != nil {
		return azureArguments{}, err
	}

	resourceGroup, err := cmd.Flags().GetString("resource-group")
	if err != nil {
		return azureArguments{}, err
	}

	name, err := selectEnvironment(cmd, resourceGroup, false)
	if err != nil {
		return azureArguments{}, err
	}

	location, err := cmd.Flags().GetString("location")
	if err != nil {
		return azureArguments{}, err
	}

	if sharedArgs.Interactive && (subscriptionID != "" || resourceGroup != "" || location != "" || name != "") {
		return azureArguments{}, errors.New("subcription id, resource group, location or environment name cannot be specified with interactive")
	}

	if !sharedArgs.Interactive {
		if subscriptionID == "" || resourceGroup == "" || location == "" {
			return azureArguments{}, errors.New("subscription id, resource group and location must be specified")
		}
		if name == "" {
			name = resourceGroup
			output.LogInfo("No environment name provided, using: %v", name)
		}
	}

	deploymentTemplate, err := cmd.Flags().GetString("deployment-template")
	if err != nil {
		return azureArguments{}, err
	}

	if deploymentTemplate != "" {
		_, err := os.Stat(deploymentTemplate)
		if err != nil {
			return azureArguments{}, fmt.Errorf("could not read deployment-template: %w", err)
		}
	}

	registryName, err := cmd.Flags().GetString("container-registry")
	if err != nil {
		return azureArguments{}, err
	}

	logAnalyticsWorkspaceID, err := cmd.Flags().GetString("loganalytics-workspace-id")
	if err != nil {
		return azureArguments{}, err
	}

	if location != "" && !isSupportedLocation(location) {
		return azureArguments{}, fmt.Errorf("the location '%s' is not supported. choose from: %s", location, strings.Join(supportedLocations[:], ", "))
	}

	return azureArguments{
		sharedArgs:              sharedArgs,
		Name:                    name,
		SubscriptionID:          subscriptionID,
		ResourceGroup:           resourceGroup,
		Location:                location,
		DeploymentTemplate:      deploymentTemplate,
		ContainerRegistry:       registryName,
		LogAnalyticsWorkspaceID: logAnalyticsWorkspaceID,
	}, nil
}

func selectSubscription(ctx context.Context, authorizer autorest.Authorizer) (radazure.Subscription, error) {
	subs, err := radazure.LoadSubscriptionsFromProfile()
	if err != nil {
		// Failed to load subscriptions from the user profile, fall back to online.
		subs, err = radazure.LoadSubscriptionsFromAzure(ctx, authorizer)
		if err != nil {
			return radazure.Subscription{}, err
		}
	}

	if subs.Default != nil {
		confirmed, err := prompt.ConfirmWithDefault(fmt.Sprintf("Use Subscription '%v'? [Y/n]", subs.Default.DisplayName), prompt.Yes)
		if err != nil {
			return radazure.Subscription{}, err
		}

		if confirmed {
			return *subs.Default, nil
		}
	}

	// build prompt to select from list
	sort.Slice(subs.Subscriptions, func(i, j int) bool {
		l := strings.ToLower(subs.Subscriptions[i].DisplayName)
		r := strings.ToLower(subs.Subscriptions[j].DisplayName)
		return l < r
	})
	names := make([]string, 0, len(subs.Subscriptions))
	for _, s := range subs.Subscriptions {
		names = append(names, s.DisplayName)
	}

	index, err := prompt.SelectWithDefault("Select Subscription:", &names[0], names)
	if err != nil {
		return radazure.Subscription{}, err
	}

	return subs.Subscriptions[index], nil
}

func selectResourceGroup(ctx context.Context, authorizer autorest.Authorizer, sub radazure.Subscription) (string, error) {

	rgc := clients.NewGroupsClient(sub.SubscriptionID, authorizer)
	name, err := promptUserForRgName(ctx, rgc)
	if err != nil {
		return "", err
	}
	resp, err := rgc.CheckExistence(ctx, name)
	if err != nil {
		return "", err
	} else if !resp.HasHTTPStatus(404) {
		// already exists
		return name, nil
	}
	output.LogInfo("Resource Group '%v' will be created...", name)

	location, err := promptUserForLocation(ctx, authorizer, sub)
	if err != nil {
		return "", err
	}
	_, err = rgc.CreateOrUpdate(ctx, name, resources.Group{
		Location: to.StringPtr(*location.Name),
	})
	if err != nil {
		return "", err
	}

	return name, nil
}

func promptUserForLocation(ctx context.Context, authorizer autorest.Authorizer, sub radazure.Subscription) (subscription.Location, error) {
	// Use the display name for the prompt
	// alphabetize so the list is stable and scannable
	subc := clients.NewSubscriptionClient(authorizer)

	locations, err := subc.ListLocations(ctx, sub.SubscriptionID)
	if err != nil {
		return subscription.Location{}, fmt.Errorf("cannot list locations: %w", err)
	}

	names := make([]string, 0, len(*locations.Value))
	nameToLocation := map[string]subscription.Location{}
	for _, loc := range *locations.Value {
		if !isSupportedLocation(*loc.Name) {
			continue
		}

		names = append(names, *loc.DisplayName)
		nameToLocation[*loc.DisplayName] = loc
	}
	sort.Strings(names)

	index, err := prompt.SelectWithDefault("Select a location:", &names[0], names)
	if err != nil {
		return subscription.Location{}, err
	}
	selected := names[index]
	return nameToLocation[selected], nil
}

func promptUserForRgName(ctx context.Context, rgc resources.GroupsClient) (string, error) {
	var name string
	createNewRg, err := prompt.ConfirmWithDefault("Create a new Resource Group? [Y/n]", prompt.Yes)
	if err != nil {
		return "", err
	}
	if createNewRg {

		defaultRgName := "radius-rg"
		if resp, err := rgc.CheckExistence(ctx, defaultRgName); !resp.HasHTTPStatus(404) || err != nil {
			// only generate a random name if the default doesn't exist already or existence check fails
			defaultRgName = handlers.GenerateRandomName("radius", "rg")
		}

		promptStr := fmt.Sprintf("Enter a Resource Group name [%s]:", defaultRgName)
		name, err = prompt.TextWithDefault(promptStr, &defaultRgName, prompt.EmptyValidator)
		if err != nil {
			return "", err
		}
	} else {
		rgListResp, err := rgc.List(ctx, "", nil)
		if err != nil {
			return "", err
		}
		rgList := rgListResp.Values()
		sort.Slice(rgList, func(i, j int) bool { return strings.ToLower(*rgList[i].Name) < strings.ToLower(*rgList[j].Name) })
		names := make([]string, 0, len(rgList))
		for _, s := range rgList {
			names = append(names, *s.Name)
		}

		defaultRgName, _ := radazure.LoadDefaultResourceGroupFromConfig() // ignore errors resulting from being unable to read the config ini file
		index, err := prompt.SelectWithDefault("Select ResourceGroup:", &defaultRgName, names)
		if err != nil {
			return "", err
		}
		name = *rgList[index].Name
	}
	return name, nil
}

func connect(ctx context.Context, environmentName string, subscriptionID string, resourceGroup, location string, deploymentTemplate string, registryName string, logAnalyticsWorkspaceID string, clusterOptions helm.ClusterOptions) error {
	armauth, err := armauth.GetArmAuthorizer()
	if err != nil {
		return err
	}

	// Check for an cluster in the target resource group. This way we
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
			"Environment '%v' available at:\n%v\n", environmentName, envUrl)

		//TODO: Need to figure out here how to fetch the environment's resource group and scope
		err = storeEnvironment(ctx, armauth, environmentName, subscriptionID, resourceGroup, clusterName, fmt.Sprintf("%s-rg", environmentName), "", "")
		if err != nil {
			return err
		}
		// Merge credentials for the AKS cluster on client machine
		err = azcli.RunCLICommand("aks", "get-credentials", "--subscription", subscriptionID, "--resource-group", resourceGroup, "--name", clusterName)
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

	if registryName != "" {
		_, err = validateRegistry(ctx, armauth, subscriptionID, registryName)
		if err != nil {
			return err
		}
	}

	if logAnalyticsWorkspaceID != "" {
		_, err = validateLogAnalyticsWorkspace(ctx, armauth, subscriptionID, logAnalyticsWorkspaceID)
		if err != nil {
			return err
		}
	}

	if group != nil {
		if !isSupportedLocation(*group.Location) {
			return fmt.Errorf("the location '%s' of resource group '%s' is not supported. choose from: %s", *group.Location, *group.Name, strings.Join(supportedLocations[:], ", "))
		}

		location = *group.Location
	}

	params := deploymentParameters{
		ResourceGroup:           resourceGroup,
		Location:                location,
		DeploymentTemplate:      deploymentTemplate,
		RegistryName:            registryName,
		LogAnalyticsWorkspaceID: logAnalyticsWorkspaceID,
	}
	deployment, err := deployEnvironment(ctx, armauth, environmentName, subscriptionID, params)
	if err != nil {
		return err
	}

	clusterName, err = findClusterInDeployment(ctx, deployment)
	if err != nil {
		return err
	}

	// Merge credentials for the AKS cluster that just got created so we can install our components on it.
	err = azcli.RunCLICommand("aks", "get-credentials", "--subscription", subscriptionID, "--resource-group", params.ResourceGroup, "--name", clusterName)
	if err != nil {
		return err
	}

	// Now that we've deployed we know the cluster name.
	clusterOptions.Radius.AzureProvider.AKS = &radazure.AKSConfig{
		SubscriptionID: subscriptionID,
		ResourceGroup:  params.ResourceGroup,
		ClusterName:    clusterName,
	}

	client, runtimeClient, _, err := createKubernetesClients("")
	if err != nil {
		return err
	}

	err = helm.InstallOnCluster(ctx, clusterOptions, client, runtimeClient)
	if err != nil {
		return err
	}

	// As decided by the team we will have a temporary 1:1 correspondence between UCP resource group and environment
	ucpRgName := fmt.Sprintf("%s-rg", environmentName)
	var scope, ucpEnvId string
	if featureflag.EnableUnifiedControlPlane.IsActive() {
		ucpRgId, err := createUCPResourceGroup(clusterName, ucpRgName)
		if err != nil {
			return err
		}
		scope = ucpRgId
		ucpEnvId, err = createEnvironmentResource(ctx, clusterName, ucpRgName, environmentName)
		if err != nil {
			return err
		}
	}

	err = storeEnvironment(ctx, armauth, environmentName, subscriptionID, resourceGroup, clusterName, ucpRgName, scope, ucpEnvId)
	if err != nil {
		return err
	}

	return nil
}

func findExistingEnvironment(ctx context.Context, authorizer autorest.Authorizer, subscriptionID string, resourceGroup string) (bool, string, error) {
	mcc := clients.NewManagedClustersClient(subscriptionID, authorizer)

	var cluster *containerservice.ManagedCluster
	for list, err := mcc.ListByResourceGroupComplete(ctx, resourceGroup); list.NotDone(); err = list.NextWithContext(ctx) {
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
		return false, "", nil
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

		// See: https://github.com/project-radius/radius/issues/520
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

type deploymentParameters struct {
	ResourceGroup           string
	Location                string
	DeploymentTemplate      string
	RegistryName            string
	LogAnalyticsWorkspaceID string
}

func deployEnvironment(ctx context.Context, authorizer autorest.Authorizer, name string, subscriptionID string, params deploymentParameters) (resources.DeploymentExtended, error) {
	// We have to create the resource group in case it doesn't exist.
	groupc := clients.NewGroupsClient(subscriptionID, authorizer)
	resp, err := groupc.CheckExistence(ctx, params.ResourceGroup)
	if err != nil {
		return resources.DeploymentExtended{}, err
	}

	if resp.StatusCode == 404 {
		_, err = groupc.CreateOrUpdate(ctx, params.ResourceGroup, resources.Group{
			Name:     &params.ResourceGroup,
			Location: &params.Location,
		})
		if err != nil {
			return resources.DeploymentExtended{}, err
		}
	}

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
		"registryName": map[string]interface{}{
			"value": params.RegistryName,
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
	op, err := dc.CreateOrUpdate(ctx, params.ResourceGroup, deploymentName, resources.Deployment{
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

func storeEnvironment(ctx context.Context, authorizer autorest.Authorizer, envName, subId, azureRg, clusterName, ucpRgName, scope, ucpEnvId string) error {
	step := output.BeginStep("Updating Config...")

	config := ConfigFromContext(ctx)
	env, err := cli.ReadEnvironmentSection(config)
	if err != nil {
		return err
	}

	env.Items[envName] = map[string]interface{}{
		"kind":           "azure",
		"subscriptionId": subId,
		"resourceGroup":  azureRg,
		"clusterName":    clusterName,
		"context":        clusterName,
		"namespace":      "default",
		"enableucp":      featureflag.EnableUnifiedControlPlane.IsActive(),
	}
	if featureflag.EnableUnifiedControlPlane.IsActive() {
		env.Items[envName]["ucpresourcegroupname"] = ucpRgName
		env.Items[envName]["scope"] = scope
		env.Items[envName]["id"] = ucpEnvId
	}

	err = cli.SaveConfigOnLock(ctx, config, cli.UpdateEnvironmentWithLatestConfig(env, cli.MergeInitEnvConfig(envName)))
	if err != nil {
		return err
	}

	output.CompleteStep(step)
	return nil
}

// operates on the *programmatic* location name ('eastus' not 'East US')
func isSupportedLocation(name string) bool {
	for _, loc := range supportedLocations {
		if strings.EqualFold(name, loc) {
			return true
		}
	}

	return false
}
