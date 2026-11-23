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
	"context"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3control"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const region = "us-west-2"

func readFile(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/handwritten/" + file)
	require.NoError(t, err)
	return string(b)
}

func payment(bucket string, payer string) map[string]any {
	return map[string]any{"Bucket": bucket, "RequestPaymentConfiguration": map[string]any{"Payer": payer}}
}

func Test_Suite_BucketRequestPayment(t *testing.T) {
	nativetest.Run(t, nativetest.Suite{
		TypeName:    bucketRequestPaymentType,
		Schema:      readFile(t, "aws-s3-bucketrequestpayment.json"),
		Metadata:    readFile(t, "aws-s3-bucketrequestpayment.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return NewBucketRequestPayment(newFakeAPI(store))
		},
		IdentifierProperty: "Bucket",
		Properties:         payment("app-bucket", "Requester"),
		Identifier:         "app-bucket",
		Changed:            payment("app-bucket", "BucketOwner"),
		CreateOnlyChange:   payment("other-bucket", "Requester"),
		ResetTo:            payment("app-bucket", "BucketOwner"),
		Invalid:            []map[string]any{payment("app-bucket", "Nobody")},
		AlwaysPresent:      true,
		API:                bucketRequestPaymentAPI,
	})
}

// routesProperties returns the routes of the Multi-Region Access Point, with the dials of bucket-east, bucket-west, and
// bucket-eu.
func routesProperties(mrap string, east int, west int, eu int) map[string]any {
	return map[string]any{
		"Mrap": mrapARN(mrap),
		"Routes": []any{
			map[string]any{"Bucket": "bucket-east", "Region": "us-east-1", "TrafficDialPercentage": east},
			map[string]any{"Bucket": "bucket-west", "Region": "us-west-2", "TrafficDialPercentage": west},
			map[string]any{"Bucket": "bucket-eu", "Region": "eu-west-1", "TrafficDialPercentage": eu},
		},
	}
}

func Test_Suite_MultiRegionAccessPointRoutes(t *testing.T) {
	properties := routesProperties(testMrap, 100, 0, 0)
	reordered := routesProperties(testMrap, 100, 0, 0)
	list := reordered["Routes"].([]any)
	reordered["Routes"] = []any{list[2], list[0], list[1]}
	nativetest.Run(t, nativetest.Suite{
		TypeName:    routesType,
		Schema:      readFile(t, "aws-s3-multiregionaccesspointroutes.json"),
		Metadata:    readFile(t, "aws-s3-multiregionaccesspointroutes.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return NewMultiRegionAccessPointRoutes(newFakeAPI(store))
		},
		IdentifierProperty: "Mrap",
		Properties:         properties,
		Identifier:         mrapARN(testMrap),
		// A failover from bucket-east to bucket-west, in one call.
		Changed:          routesProperties(testMrap, 0, 100, 0),
		CreateOnlyChange: routesProperties("other.mrap", 100, 0, 0),
		ResetTo:          routesProperties(testMrap, 100, 100, 100),
		// AWS returns the routes in its own order.
		OtherForm: map[string]any{"Routes": reordered["Routes"]},
		Invalid: []map[string]any{
			{"Mrap": "mfzwi23gnjvgw.mrap", "Routes": properties["Routes"]},
			withRoute(properties, map[string]any{"TrafficDialPercentage": 0}),
			withRoute(properties, map[string]any{"Bucket": "bucket-east", "TrafficDialPercentage": 101}),
			withRoute(properties, map[string]any{"Bucket": "bucket-east", "TrafficDialPercentage": 50}),
			withRoute(properties, map[string]any{"Bucket": "bucket-east"}),
			{"Mrap": properties["Mrap"], "Routes": []any{
				map[string]any{"Bucket": "bucket-east", "TrafficDialPercentage": 100},
				map[string]any{"Bucket": "bucket-east", "TrafficDialPercentage": 0},
				map[string]any{"Bucket": "bucket-eu", "TrafficDialPercentage": 0},
			}},
		},
		AlwaysPresent: true,
		API:           routesAPI,
	})
}

// withRoute returns properties with route in place of the first route.
func withRoute(properties map[string]any, route map[string]any) map[string]any {
	routes := append([]any{route}, properties["Routes"].([]any)[1:]...)
	return map[string]any{"Mrap": properties["Mrap"], "Routes": routes}
}

func Test_ControlRegion(t *testing.T) {
	for _, r := range controlRegions {
		require.Equal(t, r, controlRegion(r))
	}
	require.Equal(t, "us-east-1", controlRegion("eu-north-1"))

	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewMultiRegionAccessPointRoutes(&regionRecorder{fakeAPI: api})
	_, err := h.Read(t.Context(), "eu-north-1", mrapARN(testMrap))
	require.NoError(t, err, "a request in a Region that doesn't take route calls goes to us-east-1")
	require.Equal(t, []string{"us-east-1"}, h.api.(*regionRecorder).regions)
}

// regionRecorder records the Region of each route call.
type regionRecorder struct {
	*fakeAPI
	regions []string
}

func (r *regionRecorder) GetMultiRegionAccessPointRoutes(ctx context.Context, in *s3control.GetMultiRegionAccessPointRoutesInput, optFns ...func(*s3control.Options)) (*s3control.GetMultiRegionAccessPointRoutesOutput, error) {
	options := s3control.Options{}
	for _, fn := range optFns {
		fn(&options)
	}
	r.regions = append(r.regions, options.Region)
	return r.fakeAPI.GetMultiRegionAccessPointRoutes(ctx, in, optFns...)
}
