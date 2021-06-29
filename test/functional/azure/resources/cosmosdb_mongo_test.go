// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resources_test

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/cosmos-db/mgmt/documentdb"
	"github.com/Azure/radius/test/azuretest"
	"github.com/Azure/radius/test/validation"
	"github.com/stretchr/testify/require"
)

func Test_CosmosDBMongoManaged(t *testing.T) {
	application := "azure-resources-cosmosdb-mongo-managed"
	template := "testdata/azure-resources-cosmosdb-mongo-managed.bicep"
	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor: azuretest.NewDeployStepExecutor(template),
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sObjectForComponent(application, "todoapp"),
					},
				},
			},
			SkipARMResources: true,
			SkipComponents:   true,
		},
	})

	test.Test(t)
}

func Test_CosmosDBMongoUnmanaged(t *testing.T) {
	application := "azure-resources-cosmosdb-mongo-unmanaged"
	template := "testdata/azure-resources-cosmosdb-mongo-unmanaged.bicep"
	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor: azuretest.NewDeployStepExecutor(template),
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sObjectForComponent(application, "todoapp"),
					},
				},
			},
			SkipARMResources: true,
			SkipComponents:   true,
		},
	})

	// This test has additional 'unmanaged' resources that are deployed in the same template but not managed
	// by Radius.
	//
	// We don't need to delete these, they will be deleted as part of the resource group cleanup.
	test.PostDeleteVerify = func(ctx context.Context, t *testing.T, at azuretest.ApplicationTest) {
		// Verify that the cosmosdb resources were not deleted
		ac := documentdb.NewDatabaseAccountsClient(at.Options.Environment.SubscriptionID)
		ac.Authorizer = at.Options.ARMAuthorizer

		// We have to use a generated name due to uniqueness requirements, so lookup based on tags
		var account *documentdb.DatabaseAccountGetResults
		list, err := ac.ListByResourceGroup(context.Background(), at.Options.Environment.ResourceGroup)
		require.NoErrorf(t, err, "failed to list database accounts")

		for _, value := range *list.Value {
			if value.Tags["radiustest"] != nil {
				temp := value
				account = &temp
				break
			}
		}

		require.NotNilf(t, account, "failed to find database account with 'radiustest' tag")

		dbc := documentdb.NewMongoDBResourcesClient(at.Options.Environment.SubscriptionID)
		dbc.Authorizer = at.Options.ARMAuthorizer

		_, err = dbc.GetMongoDBDatabase(context.Background(), at.Options.Environment.ResourceGroup, *account.Name, "mydb")
		require.NoErrorf(t, err, "failed to find mongo database")
	}

	test.Test(t)
}
