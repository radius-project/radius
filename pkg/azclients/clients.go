// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azclients

import (
	"github.com/Azure/azure-sdk-for-go/profiles/2017-03-09/resources/mgmt/features"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/containerregistry/mgmt/containerregistry"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/containerservice/mgmt/containerservice"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/keyvault/mgmt/keyvault"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/msi/mgmt/msi"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/operationalinsights/mgmt/operationalinsights"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/subscriptions"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/servicebus/mgmt/servicebus"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/storage/mgmt/storage"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/web/mgmt/web"
	"github.com/Azure/azure-sdk-for-go/profiles/preview/preview/authorization/mgmt/authorization"
	"github.com/Azure/azure-sdk-for-go/profiles/preview/preview/customproviders/mgmt/customproviders"
	"github.com/Azure/azure-sdk-for-go/profiles/preview/preview/subscription/mgmt/subscription"
	"github.com/Azure/azure-sdk-for-go/services/cosmos-db/mgmt/2021-03-15/documentdb"
	"github.com/Azure/azure-sdk-for-go/services/preview/sql/mgmt/2015-05-01-preview/sql"
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

func NewSubscriptionsClient(authorizer autorest.Authorizer) subscriptions.Client {
	sc := subscriptions.NewClient()
	sc.Authorizer = authorizer
	sc.PollingDuration = 0
	return sc
}

func NewCustomResourceProviderClient(subscriptionID string, authorizer autorest.Authorizer) customproviders.CustomResourceProviderClient {
	cpc := customproviders.NewCustomResourceProviderClient(subscriptionID)
	cpc.Authorizer = authorizer
	cpc.PollingDuration = 0
	return cpc
}

func NewManagedClustersClient(subscriptionID string, authorizer autorest.Authorizer) containerservice.ManagedClustersClient {
	mcc := containerservice.NewManagedClustersClient(subscriptionID)
	mcc.Authorizer = authorizer
	mcc.PollingDuration = 0
	return mcc
}

func NewFeaturesClient(subscriptionID string, authorizer autorest.Authorizer) features.Client {
	fc := features.NewClient(subscriptionID)
	fc.Authorizer = authorizer
	fc.PollingDuration = 0
	return fc
}

func NewProvidersClient(subscriptionID string, authorizer autorest.Authorizer) resources.ProvidersClient {
	pc := resources.NewProvidersClient(subscriptionID)
	pc.Authorizer = authorizer
	pc.PollingDuration = 0
	return pc
}

func NewRegistriesClient(subscriptionID string, authorizer autorest.Authorizer) containerregistry.RegistriesClient {
	crc := containerregistry.NewRegistriesClient(subscriptionID)
	crc.Authorizer = authorizer
	crc.PollingDuration = 0
	return crc
}

func NewWorkspacesClient(subscriptionID string, authorizer autorest.Authorizer) operationalinsights.WorkspacesClient {
	lwc := operationalinsights.NewWorkspacesClient(subscriptionID)
	lwc.Authorizer = authorizer
	lwc.PollingDuration = 0
	return lwc
}

func NewDeploymentsClient(subscriptionID string, authorizer autorest.Authorizer) resources.DeploymentsClient {
	dc := resources.NewDeploymentsClient(subscriptionID)
	dc.Authorizer = authorizer

	// Don't set a timeout, the user can cancel the command if they want a timeout.
	dc.PollingDuration = 0

	return dc
}

func NewWebClient(subscriptionID string, authorizer autorest.Authorizer) web.AppsClient {
	webc := web.NewAppsClient(subscriptionID)
	webc.Authorizer = authorizer
	webc.PollingDuration = 0
	return webc
}

func NewDatabaseAccountsClient(subscriptionID string, authorizer autorest.Authorizer) documentdb.DatabaseAccountsClient {
	cdbc := documentdb.NewDatabaseAccountsClient(subscriptionID)
	cdbc.Authorizer = authorizer
	cdbc.PollingDuration = 0
	return cdbc
}

func NewMongoDBResourcesClient(subscriptionID string, authorizer autorest.Authorizer) documentdb.MongoDBResourcesClient {
	mdbrc := documentdb.NewMongoDBResourcesClient(subscriptionID)
	mdbrc.Authorizer = authorizer
	mdbrc.PollingDuration = 0
	return mdbrc
}

func NewSQLResourcesClient(subscriptionID string, authorizer autorest.Authorizer) documentdb.SQLResourcesClient {
	sqlc := documentdb.NewSQLResourcesClient(subscriptionID)
	sqlc.Authorizer = authorizer
	sqlc.PollingDuration = 0
	return sqlc
}

func NewVaultsClient(subscriptionID string, authorizer autorest.Authorizer) keyvault.VaultsClient {
	vc := keyvault.NewVaultsClient(subscriptionID)
	vc.Authorizer = authorizer
	vc.PollingDuration = 0
	return vc
}

func NewUserAssignedIdentitiesClient(subscriptionID string, authorizer autorest.Authorizer) msi.UserAssignedIdentitiesClient {
	msic := msi.NewUserAssignedIdentitiesClient(subscriptionID)
	msic.Authorizer = authorizer
	msic.PollingDuration = 0
	return msic
}

func NewServiceBusNamespacesClient(subscriptionID string, authorizer autorest.Authorizer) servicebus.NamespacesClient {
	sbc := servicebus.NewNamespacesClient(subscriptionID)
	sbc.Authorizer = authorizer
	sbc.PollingDuration = 0
	return sbc
}

func NewTopicsClient(subscriptionID string, authorizer autorest.Authorizer) servicebus.TopicsClient {
	tc := servicebus.NewTopicsClient(subscriptionID)
	tc.Authorizer = authorizer
	tc.PollingDuration = 0
	return tc
}

func NewQueuesClient(subscriptionID string, authorizer autorest.Authorizer) servicebus.QueuesClient {
	tc := servicebus.NewQueuesClient(subscriptionID)
	tc.Authorizer = authorizer
	tc.PollingDuration = 0
	return tc
}

func NewDatabasesClient(subscriptionID string, authorizer autorest.Authorizer) sql.DatabasesClient {
	sqlc := sql.NewDatabasesClient(subscriptionID)
	sqlc.Authorizer = authorizer
	sqlc.PollingDuration = 0
	return sqlc
}

func NewServersClient(subscriptionID string, authorizer autorest.Authorizer) sql.ServersClient {
	sc := sql.NewServersClient(subscriptionID)
	sc.Authorizer = authorizer
	sc.PollingDuration = 0
	return sc
}

func NewAccountsClient(subscriptionID string, authorizer autorest.Authorizer) storage.AccountsClient {
	ac := storage.NewAccountsClient(subscriptionID)
	ac.Authorizer = authorizer
	ac.PollingDuration = 0
	return ac
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

func NewResourcesClient(subscriptionID string, authorizer autorest.Authorizer) resources.Client {
	rc := resources.NewClient(subscriptionID)
	rc.Authorizer = authorizer
	rc.PollingDuration = 0
	return rc
}
