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

	"github.com/Azure/azure-sdk-for-go/profiles/2017-03-09/resources/mgmt/features"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/containerservice/mgmt/containerservice"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/azure-sdk-for-go/profiles/preview/preview/customproviders/mgmt/customproviders"
	"github.com/Azure/azure-sdk-for-go/profiles/preview/preview/subscription/mgmt/subscription"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/Azure/radius/cmd/cli/utils"
	radresources "github.com/Azure/radius/pkg/curp/resources"
	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/rad/azure"
	"github.com/Azure/radius/pkg/rad/logger"
	"github.com/Azure/radius/pkg/rad/prompt"
	"github.com/Azure/radius/pkg/rad/util"
	"github.com/Azure/radius/pkg/version"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

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
rad env init azure --name myenv --subscription-id SUB-ID-GUID --resource-group RG-NAME --location westus2
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		a, err := validate(cmd, args)
		if err != nil {
			return err
		}

		if a.Interactive {
			a.SubscriptionID, a.ResourceGroup, err = choose(cmd.Context())
			if err != nil {
				return err
			}
		}

		if a.Name == "" {
			a.Name = a.ResourceGroup
		}

		err = connect(cmd.Context(), a.Name, a.SubscriptionID, a.ResourceGroup, a.Location, a.DeploymentTemplate)
		if err != nil {
			return err
		}

		return nil
	},
}

func init() {
	envInitCmd.AddCommand(envInitAzureCmd)

	envInitAzureCmd.Flags().StringP("name", "n", "", "The environment name")
	envInitAzureCmd.Flags().StringP("subscription-id", "s", "", "The subscription ID to use for the environment")
	envInitAzureCmd.Flags().StringP("resource-group", "g", "", "The resource group to use for the environment")
	envInitAzureCmd.Flags().StringP("location", "l", "", "The Azure location to use for the environment")
	envInitAzureCmd.Flags().BoolP("interactive", "i", false, "Specify interactive to choose subscription and resource group interactively")

	// development support
	envInitAzureCmd.Flags().StringP("deployment-template", "t", "", "The file path to the deployment template - this can be used to override a custom build of the environment deployment ARM template for testing")
}

type arguments struct {
	Name               string
	Interactive        bool
	SubscriptionID     string
	ResourceGroup      string
	Location           string
	DeploymentTemplate string
}

func validate(cmd *cobra.Command, args []string) (arguments, error) {
	interactive, err := cmd.Flags().GetBool("interactive")
	if err != nil {
		return arguments{}, err
	}

	name, err := cmd.Flags().GetString("name")
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

	if location != "" && !isSupportedLocation(location) {
		return arguments{}, fmt.Errorf("the location '%s' is not supported. choose from: %s", location, strings.Join(supportedLocations[:], ", "))
	}

	return arguments{
		Name:               name,
		Interactive:        interactive,
		SubscriptionID:     subscriptionID,
		ResourceGroup:      resourceGroup,
		Location:           location,
		DeploymentTemplate: deploymentTemplate,
	}, nil
}

func choose(ctx context.Context) (string, string, error) {
	authorizer, err := auth.NewAuthorizerFromCLI()
	if err != nil {
		return "", "", err
	}

	sub, err := selectSubscription(ctx, authorizer)
	if err != nil {
		return "", "", err
	}

	resourceGroup, err := selectResourceGroup(ctx, authorizer, sub)
	if err != nil {
		return "", "", err
	}

	return sub.SubscriptionID, resourceGroup, nil
}

func selectSubscription(ctx context.Context, authorizer autorest.Authorizer) (azure.Subscription, error) {
	subc := subscription.NewSubscriptionsClient()
	subc.Authorizer = authorizer

	subs, err := azure.LoadSubscriptionsFromProfile()
	if err != nil {
		// Failed to load subscriptions from the user profile, fall back to online.
		subs, err = azure.LoadSubscriptionsFromAzure(ctx, authorizer)
		if err != nil {
			return azure.Subscription{}, err
		}
	}

	if subs.Default != nil {
		confirmed, err := prompt.Confirm(fmt.Sprintf("Use Subscription '%v' [y/n]?", subs.Default.DisplayName))
		if err != nil {
			return azure.Subscription{}, err
		}

		if confirmed {
			return *subs.Default, nil
		}
	}

	// build prompt to select from list
	names := []string{}
	for _, s := range subs.Subscriptions {
		names = append(names, s.DisplayName)
	}

	index, err := prompt.Select("Select Subscription:", names)
	if err != nil {
		return azure.Subscription{}, err
	}

	return subs.Subscriptions[index], nil
}

func selectResourceGroup(ctx context.Context, authorizer autorest.Authorizer, sub azure.Subscription) (string, error) {
	rgc := resources.NewGroupsClient(sub.SubscriptionID)
	rgc.Authorizer = authorizer

	name, err := prompt.Text("Enter a Resource Group name:", prompt.EmptyValidator)
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

	logger.LogInfo("Resource Group '%v' will be created...", name)

	subc := subscription.NewSubscriptionsClient()
	subc.Authorizer = authorizer

	locations, err := subc.ListLocations(ctx, sub.SubscriptionID)
	if err != nil {
		return "", fmt.Errorf("cannot list locations: %w", err)
	}

	names := []string{}
	nameToLocation := map[string]subscription.Location{}
	for _, loc := range *locations.Value {
		if !isSupportedLocation(*loc.Name) {
			continue
		}

		// Use the display name for the prompt
		names = append(names, *loc.DisplayName)
		nameToLocation[*loc.DisplayName] = loc
	}

	// alphabetize so the list is stable and scannable
	sort.Strings(names)

	index, err := prompt.Select("Select a location:", names)
	if err != nil {
		return "", err
	}

	selected := names[index]
	location := nameToLocation[selected]
	_, err = rgc.CreateOrUpdate(ctx, name, resources.Group{
		Location: to.StringPtr(*location.Name),
	})
	if err != nil {
		return "", err
	}

	return name, nil
}

func connect(ctx context.Context, name string, subscriptionID string, resourceGroup, location string, deploymentTemplate string) error {
	armauth, err := utils.GetResourceManagerEndpointAuthorizer()
	if err != nil {
		return err
	}

	// Check for an existing RP in the target resource group. This way we
	// can use a single command to bind to an existing environment
	exists, clusterName, err := findExistingEnvironment(ctx, armauth, subscriptionID, resourceGroup)
	if err != nil {
		return err
	}

	envUrl, err := azure.GenerateAzureEnvUrl(subscriptionID, resourceGroup)
	if err != nil {
		return err
	}

	if exists {
		// We already have a provider in this resource group
		logger.LogInfo("Found existing environment...\n\n"+
			"Environment '%v' available at:\n%v\n", name, envUrl)
		err = storeEnvironment(ctx, armauth, name, subscriptionID, resourceGroup, clusterName)
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

	logger.LogInfo("%v", group == nil)

	if group == nil {
		// Resource group specified was not found. Create it
		err := createResourceGroup(ctx, subscriptionID, resourceGroup, location)
		if err != nil {
			return err
		}
	} else if !isSupportedLocation(*group.Location) {
		return fmt.Errorf("the location '%s' of resource group '%s' is not supported. choose from: %s", *group.Location, *group.Name, strings.Join(supportedLocations[:], ", "))
	}

	params := deploymentParameters{DeploymentTemplate: deploymentTemplate}
	deployment, err := deployEnvironment(ctx, armauth, name, subscriptionID, resourceGroup, params)
	if err != nil {
		return err
	}

	clusterName, err = findClusterInDeployment(ctx, deployment)
	if err != nil {
		return err
	}

	err = storeEnvironment(ctx, armauth, name, subscriptionID, resourceGroup, clusterName)
	if err != nil {
		return err
	}

	return nil
}

func findExistingEnvironment(ctx context.Context, authorizer autorest.Authorizer, subscriptionID string, resourceGroup string) (bool, string, error) {
	cpc := customproviders.NewCustomResourceProviderClient(subscriptionID)
	cpc.Authorizer = authorizer

	_, err := cpc.Get(ctx, resourceGroup, "radius")
	if detail, ok := util.ExtractDetailedError(err); ok && detail.StatusCode == 404 {
		// not found - will need to be created
		return false, "", nil
	} else if err != nil {
		return false, "", err
	}

	// Custom Provider already exists, find the cluster...
	mcc := containerservice.NewManagedClustersClient(subscriptionID)
	mcc.Authorizer = authorizer

	var cluster *containerservice.ManagedCluster
	for list, err := mcc.ListByResourceGroupComplete(ctx, resourceGroup); list.NotDone(); err = list.NextWithContext(ctx) {
		if err != nil {
			return false, "", fmt.Errorf("cannot read AKS clusters: %w", err)
		}

		if radresources.HasRadiusEnvironmentTag(list.Value().Tags) {
			temp := list.Value()
			cluster = &temp
			break
		}
	}

	if cluster == nil {
		return false, "", fmt.Errorf("could not find an AKS instance in resource group '%v'", resourceGroup)
	}

	return true, *cluster.Name, nil
}

func validateSubscription(ctx context.Context, authorizer autorest.Authorizer, subscriptionID string, resourceGroup string) (*resources.Group, error) {
	step := logger.BeginStep("Validating Subscription...")

	sc := subscription.NewSubscriptionsClient()
	sc.Authorizer = authorizer

	_, err := sc.Get(ctx, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("cannot find subscription with id '%v'", subscriptionID)
	}

	rgc := resources.NewGroupsClient(subscriptionID)
	rgc.Authorizer = authorizer

	group, err := rgc.Get(ctx, resourceGroup)
	if group.StatusCode == 404 {
		// Ignore the NotFound error for the resource group as it will get created if it does not exist.
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	logger.CompleteStep(step)
	return &group, nil
}

func registerSubscription(ctx context.Context, authorizer autorest.Authorizer, subscriptionID string) error {
	step := logger.BeginStep("Registering Subscription for required features...")
	fc := features.NewClient(subscriptionID)
	fc.Authorizer = authorizer
	for feature, namespace := range requiredFeatures {
		_, err := fc.Register(ctx, namespace, feature)
		if err != nil {
			return fmt.Errorf("Failed to register subscription: %v for feature: %v/%v: %w", subscriptionID, namespace, feature, err)
		}
		logger.LogInfo("Sucessfully registered subscriptionid: %v for feature: %v/%v", subscriptionID, namespace, feature)
	}
	logger.CompleteStep(step)
	return nil
}

func createResourceGroup(ctx context.Context, subscriptionID, resourceGroupName, location string) error {
	groupsClient := resources.NewGroupsClient(subscriptionID)
	a, err := utils.GetResourceManagerEndpointAuthorizer()
	if err != nil {
		return err
	}
	groupsClient.Authorizer = a

	_, err = groupsClient.CreateOrUpdate(
		ctx,
		resourceGroupName,
		resources.Group{
			Location: to.StringPtr(location),
		})

	if err != nil {
		return err
	}

	return nil
}

func deployEnvironment(ctx context.Context, authorizer autorest.Authorizer, name string, subscriptionID string, resourceGroup string, params deploymentParameters) (resources.DeploymentExtended, error) {
	envUrl, err := azure.GenerateAzureEnvUrl(subscriptionID, resourceGroup)
	if err != nil {
		return resources.DeploymentExtended{}, err
	}

	step := logger.BeginStep(fmt.Sprintf("Deploying Environment from channel %s...\n\n"+
		"New Environment '%v' with Resource Group '%v' will be available at:\n%v\n\n"+
		"Deployment In Progress...", name, version.Channel(), resourceGroup, envUrl))
	dc := resources.NewDeploymentsClient(subscriptionID)
	dc.Authorizer = authorizer

	// Don't set a timeout, the user can cancel the command if they want a timeout.
	dc.PollingDuration = 0

	// Poll faster for completion when possible. The default is 60 seconds which is a long time to wait.
	dc.PollingDelay = time.Second * 15

	// see https://github.com/Azure-Samples/azure-sdk-for-go-samples/blob/master/resources/testdata/parameters.json
	// for an example
	parameters := map[string]interface{}{
		"channel": map[string]interface{}{
			"value": version.Channel(),
		},
	}

	deploymentProperties := &resources.DeploymentProperties{
		Parameters: parameters,
		Mode:       resources.Incremental,
	}

	if params.DeploymentTemplate == "" {
		deploymentProperties.TemplateLink = &resources.TemplateLink{
			URI: to.StringPtr(fmt.Sprintf(armTemplateURIFormat, version.Channel())),
		}
	} else {
		logger.LogInfo("overriding deployment template: %v", params.DeploymentTemplate)
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
	op, err := dc.CreateOrUpdate(ctx, resourceGroup, deploymentName, resources.Deployment{
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

	logger.CompleteStep(step)
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

func storeEnvironment(ctx context.Context, authorizer autorest.Authorizer, name string, subscriptionID string, resourceGroup string, clusterName string) error {
	step := logger.BeginStep("Updating Config...")

	v := viper.GetViper()
	env, err := rad.ReadEnvironmentSection(v)
	if err != nil {
		return err
	}

	env.Items[name] = map[string]interface{}{
		"kind":           "azure",
		"subscriptionId": subscriptionID,
		"resourceGroup":  resourceGroup,
		"clusterName":    clusterName,
	}
	if len(env.Items) == 1 {
		env.Default = name
	}
	rad.UpdateEnvironmentSection(v, env)

	err = saveConfig()
	if err != nil {
		return err
	}

	logger.CompleteStep(step)
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

type deploymentParameters struct {
	DeploymentTemplate string
}
