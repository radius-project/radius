/*
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
*/

// Package s3 has the native handlers for S3 settings that have no CloudFormation type: a bucket's Requester Pays
// setting (AWS::S3::BucketRequestPayment) and the routes of a Multi-Region Access Point
// (AWS::S3::MultiRegionAccessPointRoutes). Each is a setting of a resource another template creates, so a delete sets
// it back to AWS's default. Every call takes effect at once, so the handlers never wait for a state.
package s3

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3control"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

// API is the part of the S3 client the handlers use.
type API interface {
	GetBucketRequestPayment(ctx context.Context, params *s3.GetBucketRequestPaymentInput, optFns ...func(*s3.Options)) (*s3.GetBucketRequestPaymentOutput, error)
	PutBucketRequestPayment(ctx context.Context, params *s3.PutBucketRequestPaymentInput, optFns ...func(*s3.Options)) (*s3.PutBucketRequestPaymentOutput, error)
}

// ControlAPI is the part of the S3 Control client the handlers use.
type ControlAPI interface {
	GetMultiRegionAccessPointRoutes(ctx context.Context, params *s3control.GetMultiRegionAccessPointRoutesInput, optFns ...func(*s3control.Options)) (*s3control.GetMultiRegionAccessPointRoutesOutput, error)
	SubmitMultiRegionAccessPointRoutes(ctx context.Context, params *s3control.SubmitMultiRegionAccessPointRoutesInput, optFns ...func(*s3control.Options)) (*s3control.SubmitMultiRegionAccessPointRoutesOutput, error)
}

var inRegion = awsapi.InRegion[s3.Options]

// rejected returns a client failure AWS applied none of, with message.
func rejected(format string, args ...any) error {
	return &native.Failure{Message: fmt.Sprintf(format, args...), ClientError: true, Outcome: native.OutcomeRejected}
}
