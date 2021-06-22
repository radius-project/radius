// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package radlogger

// Field names for structured logging
const (
	LogFieldTimeStamp          = "timestamp"
	LogFieldAppName            = "applicationName"
	LogFieldScopeName          = "scopeName"
	LogFieldAppID              = "applicationID"
	LogFieldComponentName      = "componentName"
	LogFieldSubscriptionID     = "subscriptionId"
	LogFieldResourceGroup      = "resourceGroup"
	LogFieldAction             = "action"
	LogFieldComponentKind      = "componentKind"
	LogFieldDeploymentName     = "deploymentName"
	LogFieldDeploymentID       = "deploymentID"
	LogFieldErrors             = "errors"
	LogFieldResourceType       = "resourceType"
	LogFieldResourceProperties = "resourceProperties"
	LogFieldWorkLoadKind       = "workloadKind"
	LogFieldWorkLoadName       = "workloadName"
	LogFieldResourceID         = "resourceID"
	LogFieldResourceName       = "resourceName"
	LogFieldOperationID        = "operationID"
	LogFieldOperationStatus    = "operationStatus"
	LogFieldOperationFilter    = "operationfilter"
	LogFieldLocalID            = "localID"
)

const ContextLoggerField = "logger"
