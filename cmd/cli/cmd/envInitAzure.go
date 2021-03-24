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
	"github.com/Azure/azure-sdk-for-go/profiles/preview/preview/subscription/mgmt/subscription"
	containerserviceclient "github.com/Azure/azure-sdk-for-go/services/containerservice/mgmt/2018-03-31/containerservice"
	"github.com/Azure/azure-sdk-for-go/services/preview/customproviders/mgmt/2018-09-01-preview/customproviders"
	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2019-05-01/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/Azure/radius/cmd/cli/utils"
	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/rad/azure"
	"github.com/Azure/radius/pkg/rad/logger"
	"github.com/Azure/radius/pkg/rad/prompt"
	"github.com/Azure/radius/pkg/rad/util"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const armTemplateURI = "https://radiuspublic.blob.core.windows.net/environment/edge/rp-full.json"
const clusterInitScriptURI = "https://radiuspublic.blob.core.windows.net/environment/edge/initialize-cluster.sh"

var envInitAzureCmd = &cobra.Command{
	Use:   "azure",
	Short: "Create a RAD environment on Azure",
	Long:  `Create a RAD environment on Azure`,
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

		err = connect(cmd.Context(), a.Name, a.SubscriptionID, a.ResourceGroup, a.Location, a.DeploymentTemplate)
		if err != nil {
			return err
		}

		return nil
	},
}

func init() {
	envInitCmd.AddCommand(envInitAzureCmd)

	envInitAzureCmd.Flags().String("name", "azure", "The environment name")
	envInitAzureCmd.Flags().String("subscription-id", "", "The subscription ID to use for the environment")
	envInitAzureCmd.Flags().String("resource-group", "", "The resource group to use for the environment")
	envInitAzureCmd.Flags().String("location", "", "The Azure location to use for the environment")
	envInitAzureCmd.Flags().BoolP("interactive", "i", false, "Specify interactive to choose subscription and resource group interactively")

	// development support
	envInitAzureCmd.Flags().String("deployment-template", "", "The file path to the deployment template - this can be used to override a custom build of the environment deployment ARM template for testing")
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

	// We need to find the list of locations where custom resource providers are supported
	locations, err := getSupportedLocations(ctx, authorizer, sub)
	if err != nil {
		return "", fmt.Errorf("cannot find supported locations: %w", err)
	}

	index, err := prompt.Select("Select a location:", locations)
	if err != nil {
		return "", err
	}

	location := locations[index]
	_, err = rgc.CreateOrUpdate(ctx, name, resources.Group{
		Location: to.StringPtr(location),
	})
	if err != nil {
		return "", err
	}

	return name, nil
}

func getSupportedLocations(ctx context.Context, authorizer autorest.Authorizer, sub azure.Subscription) ([]string, error) {
	pc := resources.NewProvidersClient(sub.SubscriptionID)
	pc.Authorizer = authorizer

	provider, err := pc.Get(ctx, "Microsoft.CustomProviders", "")
	if err != nil {
		return nil, fmt.Errorf("failed to find Microsoft.CustomProviders namespace: %w", err)
	}

	if provider.ResourceTypes == nil {
		return nil, errors.New("provider response does not include resource types")
	}

	for _, t := range *provider.ResourceTypes {
		if *t.ResourceType != "resourceProviders" {
			continue
		}

		if t.Locations == nil {
			return nil, errors.New("locations were nil for resourceProviders")
		}

		// alphabetize so the list is stable and scannable
		sort.Strings(*t.Locations)
		return *t.Locations, nil
	}

	return nil, errors.New("could not find resourceProviders resource type")
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

	if exists {
		// We already have a provider in this resource group
		logger.LogInfo("Found existing environment...")
		err = storeEnvironment(ctx, armauth, name, subscriptionID, resourceGroup, clusterName)
		if err != nil {
			return err
		}

		return nil
	}

	rgExists, err := validateSubscription(ctx, armauth, subscriptionID, resourceGroup)
	if err != nil {
		return err
	}

	if !rgExists {
		// Resource group specified was not found. Create it
		err := createResourceGroup(ctx, subscriptionID, resourceGroup, location)
		if err != nil {
			return err
		}
	}

	kubernetesVersion, err := findKubernetesVersion(ctx, armauth, subscriptionID, resourceGroup)
	if err != nil {
		return err
	}

	params := deploymentParameters{DeploymentTemplate: deploymentTemplate, KubernetesVersion: kubernetesVersion}
	deployment, err := deployEnvironment(ctx, armauth, subscriptionID, resourceGroup, params)
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

		// For SOME REASON the value 'true' in a tag gets normalized to 'True'
		tag, ok := list.Value().Tags["rad-environment"]
		if ok && strings.EqualFold(*tag, "true") {
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

func validateSubscription(ctx context.Context, authorizer autorest.Authorizer, subscriptionID string, resourceGroup string) (bool, error) {
	step := logger.BeginStep("Validating Subscription...")

	sc := subscription.NewSubscriptionsClient()
	sc.Authorizer = authorizer

	_, err := sc.Get(ctx, subscriptionID)
	if err != nil {
		return false, fmt.Errorf("cannot find subscription with id '%v'", subscriptionID)
	}

	rgc := resources.NewGroupsClient(subscriptionID)
	rgc.Authorizer = authorizer

	rgExists := true
	resp, err := rgc.CheckExistence(ctx, resourceGroup)
	if err != nil || resp.HasHTTPStatus(404) {
		rgExists = false
	}

	logger.CompleteStep(step)
	return rgExists, nil
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

func findKubernetesVersion(ctx context.Context, authorizer autorest.Authorizer, subscriptionID string, resourceGroup string) (string, error) {
	rgc := resources.NewGroupsClient(subscriptionID)
	rgc.Authorizer = authorizer

	group, err := rgc.Get(ctx, resourceGroup)
	if err != nil {
		return "", fmt.Errorf("cannot get resource group %v: %w", resourceGroup, err)
	}

	k8sc := containerserviceclient.NewContainerServicesClient(subscriptionID)
	k8sc.Authorizer = authorizer

	result, err := k8sc.ListOrchestrators(ctx, *group.Location, "")
	if err != nil {
		return "", fmt.Errorf("cannot get AKS version: %w", err)
	}

	if result.OrchestratorVersionProfileProperties == nil || result.OrchestratorVersionProfileProperties.Orchestrators == nil {
		return "", errors.New("AKS version response has missing data")
	}

	for _, v := range *result.OrchestratorVersionProfileProperties.Orchestrators {
		if v.Default == nil || !*v.Default {
			continue
		}

		return *v.OrchestratorVersion, nil
	}

	return "", errors.New("could not find a default version for Kubernetes")
}

func deployEnvironment(ctx context.Context, authorizer autorest.Authorizer, subscriptionID string, resourceGroup string, params deploymentParameters) (resources.DeploymentExtended, error) {
	step := logger.BeginStep("Deploying Environment...")
	dc := resources.NewDeploymentsClient(subscriptionID)
	dc.Authorizer = authorizer

	// Don't set a timeout, the user can cancel the command if they want a timeout.
	dc.PollingDuration = 0

	// Poll faster for completion when possible. The default is 60 seconds which is a long time to wait.
	dc.PollingDelay = time.Second * 15

	// see https://github.com/Azure-Samples/azure-sdk-for-go-samples/blob/master/resources/testdata/parameters.json
	// for an example
	parameters := map[string]interface{}{
		"_scriptUri": map[string]interface{}{
			"value": clusterInitScriptURI,
		},
		"kubernetesVersion": map[string]interface{}{
			"value": params.KubernetesVersion,
		},
	}

	deploymentProperties := &resources.DeploymentProperties{
		Parameters: parameters,
		Mode:       resources.Incremental,
	}

	if params.DeploymentTemplate == "" {
		deploymentProperties.TemplateLink = &resources.TemplateLink{
			URI: to.StringPtr(armTemplateURI),
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
	name := fmt.Sprintf("rad-create-environment-%v", uuid.New().String())
	op, err := dc.CreateOrUpdate(ctx, resourceGroup, name, resources.Deployment{
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

type deploymentParameters struct {
	DeploymentTemplate string
	KubernetesVersion  string
}
