// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resources_test

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/servicebus/mgmt/servicebus"
	"github.com/Azure/radius/pkg/azclients"
	"github.com/Azure/radius/test/azuretest"
	"github.com/Azure/radius/test/validation"
	"github.com/stretchr/testify/require"
)

func Test_DaprPubSubServiceBusManaged(t *testing.T) {
	application := "azure-resources-dapr-pubsub-servicebus-managed"
	template := "testdata/azure-resources-dapr-pubsub-servicebus-managed.bicep"
	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor: azuretest.NewDeployStepExecutor(template),
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sObjectForComponent(application, "nodesubscriber"),
						validation.NewK8sObjectForComponent(application, "pythonpublisher"),
					},
				},
			},
			SkipARMResources: true,
			SkipComponents:   true,
		},
	})

	test.Test(t)
}

func Test_DaprPubSubServiceBusUnmanaged(t *testing.T) {
	application := "azure-resources-dapr-pubsub-servicebus-unmanaged"
	template := "testdata/azure-resources-dapr-pubsub-servicebus-unmanaged.bicep"
	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor: azuretest.NewDeployStepExecutor(template),
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sObjectForComponent(application, "nodesubscriber"),
						validation.NewK8sObjectForComponent(application, "pythonpublisher"),
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
		// Verify that the servicebus resources were not deleted
		nsc := azclients.NewServiceBusNamespacesClient(at.Options.Environment.SubscriptionID, at.Options.ARMAuthorizer)
		// We have to use a generated name due to uniqueness requirements, so lookup based on tags
		var ns *servicebus.SBNamespace
		list, err := nsc.ListByResourceGroup(context.Background(), at.Options.Environment.ResourceGroup)
		require.NoErrorf(t, err, "failed to list servicebus namespaces")

	outer:
		for ; list.NotDone(); err = list.Next() {
			require.NoErrorf(t, err, "failed to list servicebus namespaces")

			for _, value := range list.Values() {
				if value.Tags["radiustest"] != nil {
					temp := value
					ns = &temp
					break outer
				}
			}
		}

		require.NotNilf(t, ns, "failed to find servicebus namespace with 'radiustest' tag")

		tc := azclients.NewTopicsClient(at.Options.Environment.SubscriptionID, at.Options.ARMAuthorizer)

		_, err = tc.Get(context.Background(), at.Options.Environment.ResourceGroup, *ns.Name, "TOPIC_A")
		require.NoErrorf(t, err, "failed to find servicebus topic")
	}

	test.Test(t)
}
