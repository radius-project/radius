/*
------------------------------------------------------------
Copyright 2023 The Radius Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
------------------------------------------------------------
*/

package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
)

// Didn't see an interface for aws-sdk-go-v2, v1 had: https://pkg.go.dev/github.com/aws/aws-sdk-go/service/cloudcontrolapi/cloudcontrolapiiface
// This is most likely due to using json schemas to define types rather than crafting by hand. There are significantly less functions in v2, so a small mock.
//
//go:generate mockgen -destination=./mock_awscloudcontrolclient.go -package=aws -self_package github.com/project-radius/radius/pkg/ucp/aws github.com/project-radius/radius/pkg/ucp/aws AWSCloudControlClient
type AWSCloudControlClient interface {
	GetResource(ctx context.Context, params *cloudcontrol.GetResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.GetResourceOutput, error)
	ListResources(ctx context.Context, params *cloudcontrol.ListResourcesInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.ListResourcesOutput, error)
	DeleteResource(ctx context.Context, params *cloudcontrol.DeleteResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.DeleteResourceOutput, error)
	UpdateResource(ctx context.Context, params *cloudcontrol.UpdateResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.UpdateResourceOutput, error)
	CreateResource(ctx context.Context, params *cloudcontrol.CreateResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.CreateResourceOutput, error)
	GetResourceRequestStatus(ctx context.Context, params *cloudcontrol.GetResourceRequestStatusInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.GetResourceRequestStatusOutput, error)
	CancelResourceRequest(ctx context.Context, params *cloudcontrol.CancelResourceRequestInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.CancelResourceRequestOutput, error)
	ListResourceRequests(ctx context.Context, params *cloudcontrol.ListResourceRequestsInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.ListResourceRequestsOutput, error)
}

var _ = AWSCloudControlClient(&cloudcontrol.Client{})
