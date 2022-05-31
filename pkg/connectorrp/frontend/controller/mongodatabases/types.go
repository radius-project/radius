// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodatabases

const (
	ResourceTypeName = "Applications.Connector/mongoDatabases"

	// Supported operation names which are the unique names to process the operation request
	// in frontend API server and backend async operation process worker.
	MongoDatabasesList       = "APPLICATIONSCONNECTOR.MONGODATABASES.LIST"
	MongoDatabasesGet        = "APPLICATIONSCONNECTOR.MONGODATABASES.GET"
	MongoDatabasesPut        = "APPLICATIONSCONNECTOR.MONGODATABASES.PUT"
	MongoDatabasesPatch      = "APPLICATIONSCONNECTOR.MONGODATABASES.PATCH"
	MongoDatabasesDelete     = "APPLICATIONSCONNECTOR.MONGODATABASES.DELETE"
	MongoDatabasesListSecret = "APPLICATIONSCONNECTOR.MONGODATABASES.LISTSECRET"
)
