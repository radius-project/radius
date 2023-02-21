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
	LogFieldAppID              = "applicationID"
	LogFieldAppName            = "applicationName"
	LogFieldArmResourceID      = "armResourceID"
	LogFieldDeploymentID       = "deploymentID"
	LogFieldDeploymentName     = "deploymentName"
	LogFieldKind               = "kind"
	LogFieldLocalID            = "localID"
	LogFieldNamespace          = "namespace"
	LogFieldOperationID        = "operationID"
	LogFieldOperationType      = "operationType"
	LogFieldAttributes         = "attributes"
	LogFieldOperationStatus    = "operationStatus"
	LogFieldDequeueCount       = "dequeueCount"
	LogFieldResourceGroup      = "resourceGroup"
	LogFieldResourceID         = "resourceID"
	LogFieldResourceName       = "resourceName"
	LogFieldResourceProperties = "resourceProperties"
	LogFieldResourceType       = "resourceType"
	LogFieldRPIdentifier       = "rpIdentifier"
	LogFieldScopeName          = "scopeName"
	LogFieldSubscriptionID     = "subscriptionID"
	LogFieldResourceKind       = "resourceKind"
	LogHTTPStatusCode          = "statusCode"
	LogFieldRPHostName         = "rpHostName"
)
