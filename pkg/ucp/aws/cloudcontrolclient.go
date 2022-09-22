// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
)

// Didn't see an interface for aws-sdk-go-v2, v1 had: https://pkg.go.dev/github.com/aws/aws-sdk-go/service/cloudcontrolapi/cloudcontrolapiiface
// This is most likely due to using json schemas to define types rather than crafting by hand. There are significantly less functions in v2, so a small mock.
//
//go:generate mockgen -destination=./mock_awsclient.go -package=aws -self_package github.com/project-radius/radius/pkg/ucp/aws github.com/project-radius/radius/pkg/ucp/aws AWSClient
type AWSClient interface {
	GetResource(ctx context.Context, params *cloudcontrol.GetResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.GetResourceOutput, error)
	ListResources(ctx context.Context, params *cloudcontrol.ListResourcesInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.ListResourcesOutput, error)
	DeleteResource(ctx context.Context, params *cloudcontrol.DeleteResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.DeleteResourceOutput, error)
	UpdateResource(ctx context.Context, params *cloudcontrol.UpdateResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.UpdateResourceOutput, error)
	CreateResource(ctx context.Context, params *cloudcontrol.CreateResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.CreateResourceOutput, error)
	GetResourceRequestStatus(ctx context.Context, params *cloudcontrol.GetResourceRequestStatusInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.GetResourceRequestStatusOutput, error)
	CancelResourceRequest(ctx context.Context, params *cloudcontrol.CancelResourceRequestInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.CancelResourceRequestOutput, error)
	ListResourceRequests(ctx context.Context, params *cloudcontrol.ListResourceRequestsInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.ListResourceRequestsOutput, error)
}

var _ = AWSClient(&cloudcontrol.Client{})
