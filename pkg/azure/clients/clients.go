// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clients

import (
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/servicebus/mgmt/servicebus"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/storage/mgmt/storage"

	"github.com/Azure/azure-sdk-for-go/profiles/preview/preview/authorization/mgmt/authorization"
	"github.com/Azure/azure-sdk-for-go/profiles/preview/preview/subscription/mgmt/subscription"
	"github.com/Azure/go-autorest/autorest"
)

// Contains all Azure Clients we want to use in radius.
// Allows us to set an infinite timeout by default for each client
// and maintain all client versions in a single place.

// All Azure Clients should be put in this file. If you see "New(.*)Client" elsewhere,
// please move it to this file.

func NewGroupsClient(subscriptionID string, authorizer autorest.Authorizer) resources.GroupsClient {
	rgc := resources.NewGroupsClient(subscriptionID)
	rgc.Authorizer = authorizer

	// Don't timeout, let the user cancel
	rgc.PollingDuration = 0
	return rgc
}

func NewSubscriptionClient(authorizer autorest.Authorizer) subscription.SubscriptionsClient {
	sc := subscription.NewSubscriptionsClient()
	sc.Authorizer = authorizer
	sc.PollingDuration = 0
	return sc
}

func NewGenericResourceClient(subscriptionID string, authorizer autorest.Authorizer) resources.Client {
	rc := resources.NewClient(subscriptionID)
	rc.Authorizer = authorizer
	rc.PollingDuration = 0
	return rc
}

func NewCustomActionClient(subscriptionID string, authorizer autorest.Authorizer) CustomActionClient {
	cac := CustomActionClient{resources.NewWithBaseURI(resources.DefaultBaseURI, subscriptionID)}
	cac.Authorizer = authorizer
	cac.PollingDuration = 0
	return cac
}

func NewProvidersClient(subscriptionID string, authorizer autorest.Authorizer) resources.ProvidersClient {
	pc := resources.NewProvidersClient(subscriptionID)
	pc.Authorizer = authorizer
	pc.PollingDuration = 0
	return pc
}

func NewDeploymentsClient(subscriptionID string, authorizer autorest.Authorizer) resources.DeploymentsClient {
	dc := resources.NewDeploymentsClient(subscriptionID)
	dc.Authorizer = authorizer

	// Don't set a timeout, the user can cancel the command if they want a timeout.
	dc.PollingDuration = 0

	return dc
}

func NewDeploymentsClientWithBaseURI(uri string, subscriptionID string) resources.DeploymentsClient {
	dc := resources.NewDeploymentsClientWithBaseURI(uri, subscriptionID)
	// Don't set a timeout, the user can cancel the command if they want a timeout.
	dc.PollingDuration = 0
	dc.RetryDuration = 3 * time.Second

	return dc
}

func NewServiceBusNamespacesClient(subscriptionID string, authorizer autorest.Authorizer) servicebus.NamespacesClient {
	sbc := servicebus.NewNamespacesClient(subscriptionID)
	sbc.Authorizer = authorizer
	sbc.PollingDuration = 0
	return sbc
}

func NewAccountsClient(subscriptionID string, authorizer autorest.Authorizer) storage.AccountsClient {
	ac := storage.NewAccountsClient(subscriptionID)
	ac.Authorizer = authorizer
	ac.PollingDuration = 0
	return ac
}

func NewOperationsClientWithBaseUri(uri string, subscriptionID string) resources.DeploymentOperationsClient {
	doc := resources.NewDeploymentOperationsClientWithBaseURI(uri, subscriptionID)
	doc.PollingDuration = 0
	return doc
}

func NewRoleDefinitionsClient(subscriptionID string, authorizer autorest.Authorizer) authorization.RoleDefinitionsClient {
	rc := authorization.NewRoleDefinitionsClient(subscriptionID)
	rc.Authorizer = authorizer
	rc.PollingDuration = 0
	return rc
}

func NewRoleAssignmentsClient(subscriptionID string, authorizer autorest.Authorizer) authorization.RoleAssignmentsClient {
	rc := authorization.NewRoleAssignmentsClient(subscriptionID)
	rc.Authorizer = authorizer
	rc.PollingDuration = 0
	return rc
}
