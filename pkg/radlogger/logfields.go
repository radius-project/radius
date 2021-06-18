// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package radlogger

// Field names for structured logging
const (
	LogFieldTimeStamp          = "timestamp"
	LogFieldAppName            = "applicationName"
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
)

const ContextLoggerField = "logger"
