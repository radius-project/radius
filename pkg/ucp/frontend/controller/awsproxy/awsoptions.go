// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package awsproxy

import (
	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
)

// AWSOptions is the options for AWS proxy controller.
type AWSOptions struct {
	// AWSCloudControlClient is the AWS Cloud Control client.
	AWSCloudControlClient awsclient.AWSCloudControlClient

	// AWSCloudFormationClient is the AWS Cloud Formation client.
	AWSCloudFormationClient awsclient.AWSCloudFormationClient
}
