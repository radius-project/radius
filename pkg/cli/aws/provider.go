// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package aws

// Provider specifies the properties required to configure Azure provider for cloud resources
type Provider struct {
	PrincipalKeyId string
	PrincipalAccessKey string
	TargetRegion string
}