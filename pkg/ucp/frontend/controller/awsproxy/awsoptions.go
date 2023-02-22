// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package awsproxy

import (
	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
)

type AWSOptions struct {
	AWSCloudControlClient   awsclient.AWSCloudControlClient
	AWSCloudFormationClient awsclient.AWSCloudFormationClient
}
