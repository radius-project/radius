// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

const (
	ResourceTypeName                = "Applications.Core/provider"
	OperationStatusResourceTypeName = "Applications.Core/operationStatuses"

	// Supported operation names which are the unique names to process the operation request
	// in frontend API server and backend async operation process worker.
	OperationsGet        = "APPLICATIONSCORE.OPERATIONS.GET"
	OperationStatusesGet = "APPLICATIONSCORE.OPERATIONSTATUSES.GET"
	OperationResultGet   = "APPLICATIONSCORE.OPERATIONRESULT.PUT"
	SubscriptionsPut     = "APPLICATIONSCORE.SUBSCRIPTIONS.PUT"
)
