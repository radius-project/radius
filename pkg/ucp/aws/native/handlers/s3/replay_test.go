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

package s3

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3control"

	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// Test_Replay_BucketRequestPayment turns Requester Pays on for a bucket, and deletes the setting, which turns it off
// again, against AWS's responses.
func Test_Replay_BucketRequestPayment(t *testing.T) {
	s := awsreplay.New(t, "testdata/bucketrequestpayment.json")
	bucket := s.Input("RADIUS_REC_S3_BUCKET")
	nativetest.Lifecycle{
		Handler:    NewBucketRequestPayment(s3.NewFromConfig(s.Config(t.Context()))),
		Session:    s,
		Properties: payment(bucket, "Requester"),
		Poll:       time.Second,
	}.Run(t)
}

// Test_Replay_MultiRegionAccessPointRoutes makes one of two buckets of a Multi-Region Access Point passive, fails over
// to it, and deletes the routes, which makes both active again, against AWS's responses.
func Test_Replay_MultiRegionAccessPointRoutes(t *testing.T) {
	s := awsreplay.New(t, "testdata/multiregionaccesspointroutes.json")
	mrap := s.Input("RADIUS_REC_S3_MRAP_ARN")
	west, east := s.Input("RADIUS_REC_S3_MRAP_BUCKET_US_WEST_2"), s.Input("RADIUS_REC_S3_MRAP_BUCKET_US_EAST_1")
	routes := func(westDial int, eastDial int, regions bool) map[string]any {
		westRoute := map[string]any{"Bucket": west, "TrafficDialPercentage": westDial}
		eastRoute := map[string]any{"Bucket": east, "TrafficDialPercentage": eastDial}
		if regions {
			westRoute["Region"], eastRoute["Region"] = "us-west-2", "us-east-1"
		}
		return map[string]any{"Mrap": mrap, "Routes": []any{westRoute, eastRoute}}
	}
	nativetest.Lifecycle{
		Handler:    NewMultiRegionAccessPointRoutes(s3control.NewFromConfig(s.Config(t.Context()))),
		Session:    s,
		Properties: routes(100, 0, false),
		Update:     routes(0, 100, false),
		// AWS fills in each bucket's Region.
		Want:        routes(100, 0, true),
		WantUpdated: routes(0, 100, true),
		Poll:        time.Second,
	}.Run(t)
}
