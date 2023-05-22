// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package awsproxy

import (
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
)

// CloudControlRegionOption sets the region for the CloudControl client.
func CloudControlRegionOption(region string) func(*cloudcontrol.Options) {
	return func(o *cloudcontrol.Options) {
		o.Region = region
	}
}

// CloudFormationRegionOption sets the region for the CloudFormation client.
func CloudFormationWithRegionOption(region string) func(*cloudformation.Options) {
	return func(o *cloudformation.Options) {
		o.Region = region
	}
}
