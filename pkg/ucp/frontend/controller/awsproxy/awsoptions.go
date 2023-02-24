// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package awsproxy

import (
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
)

// AWSOptions is the options for AWS proxy controller.
type AWSOptions struct {
	// Options is the base options.
	ctrl.Options

	// AWSCloudControlClient is the AWS Cloud Control client.
	AWSCloudControlClient awsclient.AWSCloudControlClient

	// AWSCloudFormationClient is the AWS Cloud Formation client.
	AWSCloudFormationClient awsclient.AWSCloudFormationClient
}
