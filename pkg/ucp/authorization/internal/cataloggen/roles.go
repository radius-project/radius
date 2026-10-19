// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package cataloggen

import (
	"slices"
)

// These are release-reviewed type lists, deliberately independent of catalog
// discovery. Adding Swagger types or actions never expands these lists.
var applicationTypes = []string{
	"Radius.Core/applications",
	"Applications.Core/applications", "Applications.Core/containers",
	"Applications.Core/gateways", "Applications.Core/volumes",
	"Applications.Core/secretStores", "Applications.Core/extenders",
	"Applications.Dapr/configurationStores", "Applications.Dapr/pubSubBrokers",
	"Applications.Dapr/secretStores", "Applications.Dapr/stateStores",
	"Applications.Datastores/mongoDatabases", "Applications.Datastores/redisCaches",
	"Applications.Datastores/sqlDatabases", "Applications.Messaging/rabbitMQQueues",
	"Radius.Compute/containers", "Radius.Compute/containerImages",
	"Radius.Compute/persistentVolumes", "Radius.Compute/routes",
	"Radius.Data/mySqlDatabases", "Radius.Data/mongoDatabases",
	"Radius.Data/postgreSqlDatabases", "Radius.Data/redisCaches",
	"Radius.Data/sqlServerDatabases", "Radius.Security/secrets",
}

// ApplicationTypes returns the reviewed built-in application resource set.
func ApplicationTypes() []string {
	return slices.Clone(applicationTypes)
}

var platformTypes = []string{
	"System.Resources/resourceGroups",
	"Radius.Core/environments", "Applications.Core/environments",
	"Microsoft.Resources/deployments",
}

var registrationTypes = []string{
	"System.Resources/resourceProviders",
	"System.Resources/resourceProviders/locations",
	"System.Resources/resourceProviders/resourceTypes",
	"System.Resources/resourceProviders/resourceTypes/apiVersions",
}

var auditPermissions = []string{
	"Radius.Core/roleDefinitions/read", "Radius.Core/roleAssignments/read",
	"Radius.Core/authorizationSettings/read", "Radius.Core/permissions/read",
	"Radius.Core/accessChanges/read", "Radius.Core/checkAccess/action",
	"Radius.Core/explainAccess/action", "Radius.Core/effectiveAccess/read",
}

func typePermissions(types []string, operations ...string) []string {
	var permissions []string
	for _, typ := range types {
		for _, operation := range operations {
			permissions = append(permissions, typ+"/"+operation)
		}
	}
	return permissions
}

// ReleaseRoles expands only reviewed lists, not catalog-discovered types/actions.
// The superuser is intentionally resolved against the current catalog at runtime.
func ReleaseRoles() map[string][]string {
	app := typePermissions(applicationTypes, "read", "write", "delete", "connect/action")
	app = append(app, typePermissions([]string{"Microsoft.Resources/deployments"}, "read", "write", "delete")...)
	platform := typePermissions(platformTypes, "read", "write", "delete")
	platform = append(platform,
		"Radius.Core/environments/use/action", "Applications.Core/environments/use/action",
		"Radius.Core/recipePacks/read", "Radius.Core/recipePacks/use/action",
		"System.Azure/credentials/use/action", "System.AWS/credentials/use/action")
	access := typePermissions([]string{"Radius.Core/roleDefinitions", "Radius.Core/roleAssignments"}, "read", "write", "delete")
	access = append(access, "Radius.Core/permissions/read", "Radius.Core/checkAccess/action",
		"Radius.Core/explainAccess/action", "Radius.Core/effectiveAccess/read", "System.Resources/resourceGroups/read")
	readerTypes := slices.Concat(applicationTypes, platformTypes, registrationTypes, []string{
		"Radius.Core/recipePacks", "Radius.Core/bicepSettings", "Radius.Core/terraformSettings",
		"System.Radius/planes", "System.Azure/planes", "System.AWS/planes",
		"System.Resources/planes", "System.Resources/providers", "System.Resources/resources",
		// New namespaces do not enter applicationResources without a reviewed change.
		"Radius.Messaging/kafka", "Radius.Messaging/rabbitMQ", "Radius.AI/search",
		"Radius.AI/models", "Radius.Storage/objectStorage",
	})
	reader := typePermissions(readerTypes, "read")
	reader = append(reader, "Radius.Core/applications/getGraph/action", "Applications.Core/applications/getGraph/action",
		"Applications.Core/environments/getMetadata/action")
	registrar := typePermissions(registrationTypes, "read", "write", "delete")
	registrar = append(registrar, "System.Resources/resourceProviders/resourceTypes/joinPermissionSet/action")
	definitions := map[string][]string{
		"access-administrator":   access,
		"platform-administrator": platform,
		"application-developer":  app,
		"environment-deployer": {
			"Radius.Core/environments/read", "Radius.Core/environments/use/action",
			"Applications.Core/environments/read", "Applications.Core/environments/use/action",
		},
		"recipe-pack-administrator":   typePermissions([]string{"Radius.Core/recipePacks"}, "read", "write", "delete", "use/action"),
		"resource-type-administrator": registrar,
		"reader":                      reader,
		"auditor":                     slices.Clone(auditPermissions),
	}
	for name, permissions := range definitions {
		slices.Sort(permissions)
		definitions[name] = slices.Compact(permissions)
	}
	return definitions
}
