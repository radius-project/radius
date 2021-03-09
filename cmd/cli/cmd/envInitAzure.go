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

	"github.com/Azure/azure-sdk-for-go/profiles/latest/authorization/mgmt/authorization"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/containerservice/mgmt/containerservice"
	"github.com/Azure/azure-sdk-for-go/profiles/preview/preview/subscription/mgmt/subscription"
	"github.com/Azure/azure-sdk-for-go/services/graphrbac/1.6/graphrbac"
	"github.com/Azure/azure-sdk-for-go/services/preview/customproviders/mgmt/2018-09-01-preview/customproviders"
	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2019-05-01/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/Azure/go-autorest/autorest/date"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/Azure/radius/cmd/cli/utils"
	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/rad/azure"
	"github.com/Azure/radius/pkg/rad/logger"
	"github.com/Azure/radius/pkg/rad/prompt"
	"github.com/Azure/radius/pkg/rad/util"
	"github.com/Azure/radius/pkg/rad/util/ssh"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const armTemplateURI = "https://radiuspublic.blob.core.windows.net/environment/rp-full.json"
const clusterInitScriptURI = "https://radiuspublic.blob.core.windows.net/environment/initialize-cluster.sh"

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
		confirmed, err := prompt.Confirm(fmt.Sprintf("Use Subcription '%v' [y/n]?", subs.Default.DisplayName))
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
	location, err := prompt.Text("Enter a valid Azure location:", func(input string) (bool, error) {
		sc := subscription.NewSubscriptionsClient()
		sc.Authorizer = authorizer

		locres, err := sc.ListLocations(ctx, sub.SubscriptionID)
		if err != nil {
			return false, fmt.Errorf("failed to list locations: %v", err)
		}

		locations := *locres.Value
		for _, loc := range locations {
			if strings.EqualFold(*loc.Name, input) {
				return true, nil
			}
		}

		return false, nil
	})
	if err != nil {
		return "", err
	}

	_, err = rgc.CreateOrUpdate(ctx, name, resources.Group{
		Location: to.StringPtr(location),
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

	graphauth, err := utils.GetGraphEndpointAuthorizer()
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

	tenantID, rgExists, err := validateSubscription(ctx, armauth, subscriptionID, resourceGroup)
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

	isServicePrincipalConfigured, err := utils.IsServicePrincipalConfigured()
	if err != nil {
		return err
	}
	var sp servicePrincipal
	if isServicePrincipalConfigured {
		sp, err = getServicePrincipal(ctx, armauth, tenantID)
		if err != nil {
			return err
		}
	} else {
		sp, err := createServicePrincipal(ctx, graphauth, tenantID)
		if err != nil {
			return err
		}

		err = configurePermissions(ctx, armauth, sp, subscriptionID)
		if err != nil {
			return err
		}
	}

	deployment, err := deployEnvironment(ctx, armauth, sp, subscriptionID, resourceGroup, deploymentTemplate)
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

func validateSubscription(ctx context.Context, authorizer autorest.Authorizer, subscriptionID string, resourceGroup string) (string, bool, error) {
	step := logger.BeginStep("Validating Subscription...")

	sc := subscription.NewSubscriptionsClient()
	sc.Authorizer = authorizer

	_, err := sc.Get(ctx, subscriptionID)
	if err != nil {
		return "", false, fmt.Errorf("cannot find subscription with id '%v'", subscriptionID)
	}

	rgc := resources.NewGroupsClient(subscriptionID)
	rgc.Authorizer = authorizer

	rgExists := true
	resp, err := rgc.CheckExistence(ctx, resourceGroup)
	if err != nil || resp.HasHTTPStatus(404) {
		rgExists = false
	}

	tc := subscription.NewTenantsClient()
	tc.Authorizer = authorizer

	// TODO: this lists the tenants and just returns the first one. This might be the cause of #32
	tenants, err := tc.ListComplete(ctx)
	if err != nil {
		return "", rgExists, err
	}

	if tenants.Value().TenantID == nil {
		return "", rgExists, errors.New("cannot find tenant ID")
	}

	logger.CompleteStep(step)
	return *tenants.Value().TenantID, rgExists, nil
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

func createServicePrincipal(ctx context.Context, authorizer autorest.Authorizer, tenantID string) (servicePrincipal, error) {
	step := logger.BeginStep("Creating Service Principal...")
	appc := graphrbac.NewApplicationsClient(tenantID)
	appc.Authorizer = authorizer

	id := fmt.Sprintf("https://%v-%06v", "rad-rp", uuid.New().String())
	app, err := appc.Create(ctx, graphrbac.ApplicationCreateParameters{
		AvailableToOtherTenants: to.BoolPtr(false),
		DisplayName:             to.StringPtr("rad-rp"),
		Homepage:                to.StringPtr("http://azure.com"),
		IdentifierUris: &[]string{
			id,
		},
	})
	if err != nil {
		return servicePrincipal{}, err
	}

	rbacc := graphrbac.NewServicePrincipalsClient(tenantID)
	rbacc.Authorizer = authorizer

	clientSecret := uuid.New().String()

	sp, err := rbacc.Create(ctx, graphrbac.ServicePrincipalCreateParameters{
		AccountEnabled: to.BoolPtr(true),
		AppID:          app.AppID,
		PasswordCredentials: &[]graphrbac.PasswordCredential{
			{
				StartDate: &date.Time{Time: time.Now()},
				EndDate:   &date.Time{Time: time.Now().AddDate(1, 0, 0)}, // One year from now
				Value:     to.StringPtr(clientSecret),
				KeyID:     to.StringPtr(uuid.New().String()),
			},
		},
	})
	if err != nil {
		return servicePrincipal{}, err
	}

	logger.CompleteStep(step)
	return servicePrincipal{Principal: sp, ClientSecret: clientSecret}, nil
}

func getServicePrincipal(ctx context.Context, authorizer autorest.Authorizer, tenantID string) (servicePrincipal, error) {
	step := logger.BeginStep("Getting Service Principal...")
	settings, err := auth.GetSettingsFromEnvironment()
	if err != nil {
		return servicePrincipal{}, err
	}

	var sp graphrbac.ServicePrincipal
	clientID := settings.Values[auth.ClientID]
	sp.AppID = &clientID

	logger.CompleteStep(step)

	return servicePrincipal{Principal: sp, ClientSecret: settings.Values[auth.ClientSecret]}, nil
}

func configurePermissions(ctx context.Context, authorizer autorest.Authorizer, sp servicePrincipal, subscriptionID string) error {
	step := logger.BeginStep("Configuring Permissions...")
	rdc := authorization.NewRoleDefinitionsClient(subscriptionID)
	rdc.Authorizer = authorizer

	roles, err := rdc.ListComplete(ctx, fmt.Sprintf("/subscriptions/%v", subscriptionID), "roleName eq 'Contributor'")
	if err != nil {
		return err
	}

	if roles.Value().Name == nil {
		return errors.New("failed to find Contributor role")
	}

	rac := authorization.NewRoleAssignmentsClient(subscriptionID)
	rac.Authorizer = authorizer

	// We'll sometimes see this call fail due to the service principal not being propagated yet
	for i := 0; i < 30; i++ {
		_, err = rac.Create(
			ctx,
			fmt.Sprintf("/subscriptions/%v", subscriptionID),
			uuid.New().String(),
			authorization.RoleAssignmentCreateParameters{
				Properties: &authorization.RoleAssignmentProperties{
					PrincipalID:      to.StringPtr(*sp.Principal.ObjectID),
					RoleDefinitionID: to.StringPtr(*roles.Value().ID),
				},
			})

		if detailed, ok := util.ExtractDetailedError(err); ok && detailed.StatusCode == 400 {
			if service, ok := util.ExtractServiceError(err); ok && service.Code == "PrincipalNotFound" {
				fmt.Println("Waiting for permissions...")
				time.Sleep(5 * time.Second)
				continue
			}
		}

		if err != nil {
			return err
		}

		logger.CompleteStep(step)
		return nil
	}

	return errors.New("timed out waiting for service principal to show up")
}

func deployEnvironment(ctx context.Context, authorizer autorest.Authorizer, sp servicePrincipal, subscriptionID string, resourceGroup string, deploymentTemplate string) (resources.DeploymentExtended, error) {
	step := logger.BeginStep("Deploying Environment...")
	dc := resources.NewDeploymentsClient(subscriptionID)
	dc.Authorizer = authorizer

	// Don't set a timeout, the user can cancel the command if they want a timeout.
	dc.PollingDuration = 0

	// Poll faster for completion when possible. The default is 60 seconds which is a long time to wait.
	dc.PollingDelay = time.Second * 15

	key, err := ssh.GenerateKey(ssh.DefaultSize)
	if err != nil {
		return resources.DeploymentExtended{}, err
	}

	// see https://github.com/Azure-Samples/azure-sdk-for-go-samples/blob/master/resources/testdata/parameters.json
	// for an example
	parameters := map[string]interface{}{
		"servicePrincipalClientId": map[string]interface{}{
			"value": *sp.Principal.AppID,
		},
		"servicePrincipalClientSecret": map[string]interface{}{
			"value": sp.ClientSecret,
		},
		"sshRSAPublicKey": map[string]interface{}{
			"value": string(key),
		},
		"_scriptUri": map[string]interface{}{
			"value": clusterInitScriptURI,
		},
	}

	deploymentProperties := &resources.DeploymentProperties{
		Parameters: parameters,
		Mode:       resources.Incremental,
	}

	if deploymentTemplate == "" {
		deploymentProperties.TemplateLink = &resources.TemplateLink{
			URI: to.StringPtr(armTemplateURI),
		}
	} else {
		logger.LogInfo("overriding deployment template: %v", deploymentTemplate)
		templateContent, err := ioutil.ReadFile(deploymentTemplate)
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

	// See https://github.com/Azure/AKS/issues/1206 - propagation of service principals can cause issues during
	// validation. We need retries to work around that.
	name := fmt.Sprintf("rad-create-environment-%v", uuid.New().String())
	for i := 0; i < 30; i++ {
		op, err := dc.CreateOrUpdate(ctx, resourceGroup, name, resources.Deployment{
			Properties: deploymentProperties,
		})
		if detailed, ok := util.ExtractDetailedError(err); ok && detailed.StatusCode == 400 {
			if service, ok := util.ExtractServiceError(err); ok {
				if service.Code == "InvalidTemplateDeployment" {
					return resources.DeploymentExtended{}, fmt.Errorf("encountered a fatal error while creating environment: %v", err)
				}

				fmt.Printf("error encountered: %+v", service)
				time.Sleep(5 * time.Second)
				continue
			} else {
				return resources.DeploymentExtended{}, err
			}
		} else if err != nil {
			return resources.DeploymentExtended{}, err
		}

		// This will not return a detailed error, it's always a service error
		err = op.WaitForCompletionRef(ctx, dc.Client)
		if service, ok := util.ExtractServiceError(err); ok {
			fmt.Printf("error encountered: %+v", service)
			time.Sleep(5 * time.Second)
			continue
		} else if err != nil {
			return resources.DeploymentExtended{}, err
		}

		dep, err := op.Result(dc)
		if err != nil {
			return resources.DeploymentExtended{}, err
		}

		logger.CompleteStep(step)
		return dep, nil
	}

	return resources.DeploymentExtended{}, errors.New("timed out waiting for creation to succeeed")
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

type servicePrincipal struct {
	Principal    graphrbac.ServicePrincipal
	ClientSecret string
}
