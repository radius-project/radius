// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/authorization/mgmt/authorization"
	"github.com/Azure/azure-sdk-for-go/profiles/preview/preview/subscription/mgmt/subscription"
	"github.com/Azure/azure-sdk-for-go/services/containerservice/mgmt/2019-02-01/containerservice"
	"github.com/Azure/azure-sdk-for-go/services/graphrbac/1.6/graphrbac"
	"github.com/Azure/azure-sdk-for-go/services/preview/customproviders/mgmt/2018-09-01-preview/customproviders"
	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2019-05-01/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/Azure/go-autorest/autorest/date"
	"github.com/Azure/go-autorest/autorest/to"
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

		err = connect(cmd.Context(), a.Name, a.SubscriptionID, a.ResourceGroup)
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
	envInitAzureCmd.Flags().BoolP("interactive", "i", false, "Specify interactive to choose subscription and resource group interactively")
}

type arguments struct {
	Name           string
	Interactive    bool
	SubscriptionID string
	ResourceGroup  string
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

	if interactive && (subscriptionID != "" || resourceGroup != "") {
		return arguments{}, errors.New("subcription id and resource group cannot be specified with interactive")
	}

	if !interactive && (subscriptionID == "" || resourceGroup == "") {
		return arguments{}, errors.New("subscription id and resource group must be specified")
	}

	return arguments{
		Name:           name,
		Interactive:    interactive,
		SubscriptionID: subscriptionID,
		ResourceGroup:  resourceGroup,
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

func connect(ctx context.Context, name string, subscriptionID string, resourceGroup string) error {
	settings, err := auth.GetSettingsFromEnvironment()
	if err != nil {
		return err
	}

	armauth, err := auth.NewAuthorizerFromCLIWithResource(settings.Environment.ResourceManagerEndpoint)
	if err != nil {
		return err
	}

	graphauth, err := auth.NewAuthorizerFromCLIWithResource(settings.Environment.GraphEndpoint)
	if err != nil {
		return err
	}

	// Check for an existing RP in the target resource group. This way we
	// can use a single command to bind to an existing environment
	exists, err := lookForExisting(ctx, armauth, subscriptionID, resourceGroup)
	if err != nil {
		return err
	}

	if exists {
		// We already have a provider in this resource group
		logger.LogInfo("Found existing environment...")
		err = storeEnvironment(ctx, armauth, name, subscriptionID, resourceGroup)
		if err != nil {
			return err
		}

		return nil
	}

	tenantID, err := validateSubscription(ctx, armauth, subscriptionID, resourceGroup)
	if err != nil {
		return err
	}

	sp, err := createServicePrincipal(ctx, graphauth, tenantID)
	if err != nil {
		return err
	}

	err = configurePermissions(ctx, armauth, sp, subscriptionID)
	if err != nil {
		return err
	}

	_, err = deployEnvironment(ctx, armauth, sp, subscriptionID, resourceGroup)
	if err != nil {
		return err
	}

	err = storeEnvironment(ctx, armauth, name, subscriptionID, resourceGroup)
	if err != nil {
		return err
	}

	return nil
}

func lookForExisting(ctx context.Context, authorizer autorest.Authorizer, subscriptionID string, resourceGroup string) (bool, error) {
	cpc := customproviders.NewCustomResourceProviderClient(subscriptionID)
	cpc.Authorizer = authorizer

	_, err := cpc.Get(ctx, resourceGroup, "radius")
	if detail, ok := util.ExtractDetailedError(err); ok && detail.StatusCode == 404 {
		// not found - will need to be created
		return false, nil
	} else if err != nil {
		return false, err
	} else {
		// already exists
		return true, nil
	}
}

func validateSubscription(ctx context.Context, authorizer autorest.Authorizer, subscriptionID string, resourceGroup string) (string, error) {
	step := logger.BeginStep("Validating Subscription...")

	sc := subscription.NewSubscriptionsClient()
	sc.Authorizer = authorizer

	_, err := sc.Get(ctx, subscriptionID)
	if err != nil {
		return "", fmt.Errorf("cannot find subscription with id '%v'", subscriptionID)
	}

	rgc := resources.NewGroupsClient(subscriptionID)
	rgc.Authorizer = authorizer

	resp, err := rgc.CheckExistence(ctx, resourceGroup)
	if err != nil {
		return "", fmt.Errorf("cannot find resource group named '%v'", resourceGroup)
	} else if resp.HasHTTPStatus(404) {
		return "", fmt.Errorf("cannot find resource group named '%v'", resourceGroup)
	}

	tc := subscription.NewTenantsClient()
	tc.Authorizer = authorizer

	tenants, err := tc.ListComplete(ctx)
	if err != nil {
		return "", err
	}

	if tenants.Value().TenantID == nil {
		return "", errors.New("cannot find tenant ID")
	}

	logger.CompleteStep(step)
	return *tenants.Value().TenantID, nil
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

func deployEnvironment(ctx context.Context, authorizer autorest.Authorizer, sp servicePrincipal, subscriptionID string, resourceGroup string) (resources.DeploymentExtended, error) {
	step := logger.BeginStep("Deploying Environment...")
	dc := resources.NewDeploymentsClient(subscriptionID)
	dc.Authorizer = authorizer

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

	// See https://github.com/Azure/AKS/issues/1206 - propagation of service principals can cause issues during
	// validation.
	name := fmt.Sprintf("rad-create-environment-%v", uuid.New().String())
	for i := 0; i < 30; i++ {
		op, err := dc.CreateOrUpdate(ctx, resourceGroup, name, resources.Deployment{
			Properties: &resources.DeploymentProperties{
				TemplateLink: &resources.TemplateLink{
					URI: to.StringPtr(armTemplateURI),
				},
				Parameters: parameters,
				Mode:       resources.Incremental,
			},
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

func storeEnvironment(ctx context.Context, authorizer autorest.Authorizer, name string, subscriptionID string, resourceGroup string) error {
	step := logger.BeginStep("Updating Config...")

	mcc := containerservice.NewManagedClustersClient(subscriptionID)
	mcc.Authorizer = authorizer

	iter, err := mcc.ListByResourceGroupComplete(ctx, resourceGroup)
	if err != nil {
		return err
	}

	var cluster *containerservice.ManagedCluster
	for {
		tag, ok := iter.Value().Tags["rad-environment"]
		if ok && *tag == "true" {
			temp := iter.Value()
			cluster = &temp
			break
		}

		if !iter.NotDone() {
			break
		}

		err := iter.NextWithContext(ctx)
		if err != nil {
			return fmt.Errorf("cannot read AKS clusters: %w", err)
		}
	}

	if cluster == nil {
		return fmt.Errorf("could not find an AKS instance in resource group '%v'", resourceGroup)
	}

	v := viper.GetViper()
	env, err := rad.ReadEnvironmentSection(v)
	if err != nil {
		return err
	}

	env.Items[name] = map[string]interface{}{
		"kind":           "azure",
		"subscriptionId": subscriptionID,
		"resourceGroup":  resourceGroup,
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
