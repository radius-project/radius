// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/armcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	radresources "github.com/Azure/radius/pkg/curp/resources"
	"github.com/Azure/radius/pkg/rad/bicep"
	"github.com/Azure/radius/pkg/radclient"
	"github.com/Azure/radius/pkg/radtest"
	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:   "delete [path]",
	Short: "Deletes the resources in the provided template",
	Long:  "Deletes the resources in the provided template",
	RunE:  runDelete,
	Args:  cobra.MaximumNArgs(1),
}

func init() {
	RootCmd.AddCommand(deleteCmd)

	deleteCmd.Flags().String("host", "localhost:5000", "specify the hostname (defaults to localhost:5000)")
	deleteCmd.Flags().BoolP("verbose", "v", false, "output verbose logging output")
}

func runDelete(cmd *cobra.Command, args []string) error {
	hostname, err := cmd.Flags().GetString("host")
	if err != nil {
		return err
	}

	verbose, err := cmd.Flags().GetBool("verbose")
	if err != nil {
		return err
	}

	if verbose {
		azcore.Log().SetListener(func(lc azcore.LogClassification, s string) {
			fmt.Printf("RADCLIENT SDK %s: %s\n", lc, s)
		})
	}

	file, err := validate(args)
	if err != nil {
		return err
	}

	fmt.Printf("Building '%s'...\n", file)
	template, err := bicep.Build(file)
	if err != nil {
		return err
	}

	fmt.Printf("Building template...\n")
	resources, err := radtest.Parse(template)
	if err != nil {
		return err
	}

	options := &armcore.ConnectionOptions{Logging: azcore.LogOptions{IncludeBody: verbose}}
	connection := armcore.NewConnection(fmt.Sprintf("http://%s/", hostname), &radtest.AnonymousCredential{}, options)

	fmt.Printf("Starting deletion...\n")

	// Deletion can proceed in reverse dependency order
	for i := len(resources) - 1; i >= 0; i-- {
		resource := resources[i]
		fmt.Printf("Deleting %s %s...\n", resource.Type, resource.Name)
		response, err := delete(cmd.Context(), connection, resource)
		if err != nil {
			return fmt.Errorf("failed to DELETE resource %s %s: %w", resource.Type, resource.Name, err)
		}

		fmt.Printf("succeed with status code %d\n", response.StatusCode)
	}

	return nil
}

func delete(ctx context.Context, connection *armcore.Connection, resource radtest.Resource) (*http.Response, error) {
	if resource.Type == radresources.ApplicationResourceType.Type() {
		return deleteApplication(ctx, connection, resource)
	} else if resource.Type == radresources.ComponentResourceType.Type() {
		return deleteComponent(ctx, connection, resource)
	} else if resource.Type == radresources.DeploymentResourceType.Type() {
		return deleteDeployment(ctx, connection, resource)
	}

	return nil, fmt.Errorf("unsupported resource type '%s'. radtest only supports radius types", resource.Type)
}

func deleteApplication(ctx context.Context, connection *armcore.Connection, resource radtest.Resource) (*http.Response, error) {
	client := radclient.NewApplicationClient(connection, radtest.TestSubscriptionID)

	names := strings.Split(resource.Name, "/")
	if len(names) != 2 {
		return nil, fmt.Errorf("expected name in format of 'radius/<application>'. was '%s'", resource.Name)
	}

	response, err := client.Delete(ctx, radtest.TestResourceGroup, names[1], nil)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func deleteComponent(ctx context.Context, connection *armcore.Connection, resource radtest.Resource) (*http.Response, error) {
	client := radclient.NewComponentClient(connection, radtest.TestSubscriptionID)

	names := strings.Split(resource.Name, "/")
	if len(names) != 3 {
		return nil, fmt.Errorf("expected name in format of 'radius/<application>/<component>'. was '%s'", resource.Name)
	}

	response, err := client.Delete(ctx, radtest.TestResourceGroup, names[1], names[2], nil)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func deleteDeployment(ctx context.Context, connection *armcore.Connection, resource radtest.Resource) (*http.Response, error) {
	client := radclient.NewDeploymentClient(connection, radtest.TestSubscriptionID)

	names := strings.Split(resource.Name, "/")
	if len(names) != 3 {
		return nil, fmt.Errorf("expected name in format of 'radius/<application>/<deployment>'. was '%s'", resource.Name)
	}

	poller, err := client.BeginDelete(ctx, radtest.TestResourceGroup, names[1], names[2], nil)
	if err != nil {
		return nil, err
	}

	response, err := poller.PollUntilDone(ctx, radclient.PollInterval)
	if err != nil {
		return nil, err
	}

	return response, nil
}
