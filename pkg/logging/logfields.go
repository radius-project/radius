// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package logging

const (
	AppCoreLoggerName string = "applications.core"
	AppLinkLoggerName string = "applications.link"
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
	LogFieldOperationStatus    = "operationStatus"
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
)
