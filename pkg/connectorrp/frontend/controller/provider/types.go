// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

const (
	Namespace        = "Applications.Connector"
	ResourceTypeName = Namespace + "/provider"

	// Supported operation names which are the unique names to process the operation request
	// in frontend API server and backend async operation process worker.
	OperationsGet    = "APPLICATIONSCONNECTOR.OPERATIONS.GET"
	SubscriptionsPut = "APPLICATIONSCONNECTOR.SUBSCRIPTIONS.PUT"
)
