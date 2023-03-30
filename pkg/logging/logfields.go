// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package logging

const (
	AppCoreLoggerName string = "applications.core"
	AppLinkLoggerName string = "applications.link"

	ServiceName string = "rp"
)

// Field names for structured logging
const (
	LogFieldAction             = "action"
	LogFieldAppID              = "applicationId"
	LogFieldAppName            = "applicationName"
	LogFieldCorrelationID      = "correlationId"
	LogFieldDeploymentID       = "deploymentId"
	LogFieldDeploymentName     = "deploymentName"
	LogFieldKind               = "kind"
	LogFieldLocalID            = "localId"
	LogFieldNamespace          = "namespace"
	LogFieldOperationID        = "operationId"
	LogFieldOperationType      = "operationType"
	LogFieldDequeueCount       = "dequeueCount"
	LogFieldOperationStatus    = "operationStatus"
	LogFieldResourceGroup      = "resourceGroup"
	LogFieldResourceID         = "resourceId"
	LogFieldResourceName       = "resourceName"
	LogFieldResourceProperties = "resourceProperties"
	LogFieldResourceType       = "resourceType"
	LogFieldRPIdentifier       = "rpIdentifier"
	LogFieldScopeName          = "scopeName"
	LogFieldSubscriptionID     = "subscriptionId"
	LogFieldResourceKind       = "resourceKind"
	LogHTTPStatusCode          = "statusCodetest"
)
