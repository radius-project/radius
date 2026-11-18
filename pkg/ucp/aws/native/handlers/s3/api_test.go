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
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/s3control"
	"github.com/aws/aws-sdk-go-v2/service/s3control/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	apiReference = "https://docs.aws.amazon.com/AmazonS3/latest/API/"

	// errorCodes is S3's list of error codes.
	errorCodes = "https://docs.aws.amazon.com/AmazonS3/latest/developerguide/ErrorResponses.html"

	// failoverErrors is the page of errors route calls return.
	failoverErrors = "https://docs.aws.amazon.com/AmazonS3/latest/userguide/mrap-failover-errors.html"

	// routeType is the page of the MultiRegionAccessPointRoute data type.
	routeType = apiReference + "API_control_MultiRegionAccessPointRoute.html"

	// testMrap is the alias of the Multi-Region Access Point the rules use.
	testMrap = "mfzwi23gnjvgw.mrap"
)

func getPayer(ctx context.Context, store *awsfake.CloudControl, bucket string) (s3types.Payer, error) {
	out, err := newFakeAPI(store).GetBucketRequestPayment(ctx, &s3.GetBucketRequestPaymentInput{Bucket: aws.String(bucket)})
	if err != nil {
		return "", err
	}
	return out.Payer, nil
}

func putPayer(ctx context.Context, store *awsfake.CloudControl, bucket string, configuration *s3types.RequestPaymentConfiguration) error {
	_, err := newFakeAPI(store).PutBucketRequestPayment(ctx, &s3.PutBucketRequestPaymentInput{Bucket: aws.String(bucket), RequestPaymentConfiguration: configuration})
	return err
}

// bucketRequestPaymentAPI records the S3 calls BucketRequestPayment makes.
var bucketRequestPaymentAPI = map[string]nativetest.Call{
	"GetBucketRequestPayment": {
		Doc: apiReference + "API_GetBucketRequestPayment.html",
		Rules: []nativetest.Rule{
			{
				Rule: "the bucket owner pays by default, so a bucket nobody set returns BucketOwner",
				Doc:  apiReference + "API_PutBucketRequestPayment.html",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					payer, err := getPayer(ctx, store, "app-bucket")
					if err != nil {
						return err
					}
					if payer != s3types.PayerBucketOwner {
						return fmt.Errorf("a new bucket's payer is %q", payer)
					}
					return nil
				},
			},
			{
				Rule: "the caller must be the bucket owner",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := getPayer(ctx, store, otherOwnerPrefix+"-bucket")
					return err
				},
				Code: accessDeniedCode,
			},
			{
				Rule: "the bucket exists",
				Doc:  errorCodes,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := getPayer(ctx, store, missingPrefix+"-bucket")
					return err
				},
				Code: noSuchBucketCode,
			},
		},
		Unclear: "The page says only the bucket owner can call it, without naming the error; the fake returns " +
			"AccessDenied, the code S3's error list gives for 403 Forbidden. It isn't supported for directory buckets, " +
			"without an error named; the fake has none. ExpectedBucketOwner isn't sent.",
	},
	"PutBucketRequestPayment": {
		Doc: apiReference + "API_PutBucketRequestPayment.html",
		Rules: []nativetest.Rule{
			{
				Rule: "RequestPaymentConfiguration is required",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return putPayer(ctx, store, "app-bucket", nil)
				},
				Code: malformedXMLCode,
			},
			{
				Rule: "Payer is Requester or BucketOwner",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return putPayer(ctx, store, "app-bucket", &s3types.RequestPaymentConfiguration{Payer: "Nobody"})
				},
				Code: malformedXMLCode,
			},
			{
				Rule: "only the bucket owner can set the payer",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return putPayer(ctx, store, otherOwnerPrefix+"-bucket", &s3types.RequestPaymentConfiguration{Payer: s3types.PayerRequester})
				},
				Code: accessDeniedCode,
			},
			{
				Rule: "the bucket exists",
				Doc:  errorCodes,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return putPayer(ctx, store, missingPrefix+"-bucket", &s3types.RequestPaymentConfiguration{Payer: s3types.PayerRequester})
				},
				Code: noSuchBucketCode,
			},
			{
				Rule: "the call replaces the payer, so setting BucketOwner again turns Requester Pays off",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					for _, payer := range []s3types.Payer{s3types.PayerRequester, s3types.PayerBucketOwner} {
						if err := putPayer(ctx, store, "app-bucket", &s3types.RequestPaymentConfiguration{Payer: payer}); err != nil {
							return err
						}
						got, err := getPayer(ctx, store, "app-bucket")
						if err != nil {
							return err
						}
						if got != payer {
							return fmt.Errorf("the payer is %q after setting %q", got, payer)
						}
					}
					return nil
				},
			},
		},
		Unclear: "The page names no error for a missing or invalid Payer; the fake returns MalformedXML, S3's error for " +
			"a body that doesn't validate against its schema. It says only the bucket owner can set the payer, without " +
			"naming the error; the fake returns AccessDenied. Requester Pays buckets can't be a logging target, or log " +
			"to one, but the page doesn't say that PutBucketRequestPayment checks it, so the fake doesn't. It isn't " +
			"supported for directory buckets, without an error named; the fake has none.",
	},
}

func getRoutes(ctx context.Context, store *awsfake.CloudControl, region string, accountID string, mrap string) ([]types.MultiRegionAccessPointRoute, error) {
	out, err := newFakeAPI(store).GetMultiRegionAccessPointRoutes(ctx, &s3control.GetMultiRegionAccessPointRoutesInput{
		AccountId: aws.String(accountID),
		Mrap:      aws.String(mrap),
	}, func(o *s3control.Options) { o.Region = region })
	if err != nil {
		return nil, err
	}
	return out.Routes, nil
}

func submitRoutes(ctx context.Context, store *awsfake.CloudControl, region string, accountID string, mrap string, routes ...types.MultiRegionAccessPointRoute) error {
	_, err := newFakeAPI(store).SubmitMultiRegionAccessPointRoutes(ctx, &s3control.SubmitMultiRegionAccessPointRoutesInput{
		AccountId:    aws.String(accountID),
		Mrap:         aws.String(mrap),
		RouteUpdates: routes,
	}, func(o *s3control.Options) { o.Region = region })
	return err
}

// submit sends routes for the test Multi-Region Access Point from us-west-2.
func submit(ctx context.Context, store *awsfake.CloudControl, routes ...types.MultiRegionAccessPointRoute) error {
	return submitRoutes(ctx, store, "us-west-2", account, mrapARN(testMrap), routes...)
}

func route(bucket string, region string, dial int32) types.MultiRegionAccessPointRoute {
	r := types.MultiRegionAccessPointRoute{TrafficDialPercentage: aws.Int32(dial)}
	if bucket != "" {
		r.Bucket = aws.String(bucket)
	}
	if region != "" {
		r.Region = aws.String(region)
	}
	return r
}

// dials returns the TrafficDialPercentage of each route, by bucket.
func dials(routes []types.MultiRegionAccessPointRoute) map[string]int32 {
	out := map[string]int32{}
	for _, r := range routes {
		out[aws.ToString(r.Bucket)] = aws.ToInt32(r.TrafficDialPercentage)
	}
	return out
}

// routeCallRules are the rules of both route calls. call makes the call with region, AccountId, and Mrap.
func routeCallRules(call func(ctx context.Context, store *awsfake.CloudControl, region string, accountID string, mrap string) error) []nativetest.Rule {
	return []nativetest.Rule{
		{
			Rule: "route calls go to the endpoints in us-east-1, us-west-2, ap-southeast-2, ap-northeast-1, and eu-west-1 only",
			Check: func(ctx context.Context, store *awsfake.CloudControl) error {
				for _, region := range controlRegions {
					if err := call(ctx, store, region, account, mrapARN(testMrap)); err != nil {
						return fmt.Errorf("a call in %s failed: %w", region, err)
					}
				}
				if call(ctx, store, "eu-north-1", account, mrapARN(testMrap)) == nil {
					return fmt.Errorf("a call in eu-north-1 succeeded")
				}
				return nil
			},
		},
		{
			Rule: "AccountId matches ^\\d{12}$",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				return call(ctx, store, "us-west-2", "1234", mrapARN(testMrap))
			},
			Code: invalidRequestCode,
		},
		{
			Rule: "Mrap is the ARN of a Multi-Region Access Point, arn:aws:s3::account-id:accesspoint/alias, and a random string fails with 400 Bad Request",
			Doc:  failoverErrors,
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				return call(ctx, store, "us-west-2", account, "random")
			},
			Code: invalidRequestCode,
		},
		{
			Rule: "Mrap is at most 200 characters",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				return call(ctx, store, "us-west-2", account, mrapARN(strings.Repeat("a", 200)))
			},
			Code: invalidRequestCode,
		},
		{
			Rule: "the Multi-Region Access Point exists",
			Doc:  errorCodes,
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				return call(ctx, store, "us-west-2", account, mrapARN(missingPrefix+".mrap"))
			},
			Code: noSuchMrapCode,
		},
	}
}

// routeCallsUnclear is what the docs leave unclear about both route calls.
const routeCallsUnclear = "The page gives Mrap the pattern ^[a-zA-Z0-9\\:.-]{3,200}$, which leaves out the \"/\" of every " +
	"ARN, while the failover errors page says to use the ARN; the fake takes the ARN. Neither page names the error " +
	"code for a call in another Region, an AccountId or Mrap that doesn't match its pattern, or an AccountId that " +
	"isn't the Multi-Region Access Point's owner; the fake returns InvalidRequest, from S3's list of error codes, for " +
	"the first three, and NoSuchMultiRegionAccessPoint for the last."

// routesAPI records the S3 Control calls MultiRegionAccessPointRoutes makes.
var routesAPI = map[string]nativetest.Call{
	"GetMultiRegionAccessPointRoutes": {
		Doc: apiReference + "API_control_GetMultiRegionAccessPointRoutes.html",
		Rules: append(routeCallRules(func(ctx context.Context, store *awsfake.CloudControl, region string, accountID string, mrap string) error {
			_, err := getRoutes(ctx, store, region, accountID, mrap)
			return err
		}), nativetest.Rule{
			Rule: "the routes give each bucket's Region and TrafficDialPercentage, 100 for active and 0 for passive",
			Check: func(ctx context.Context, store *awsfake.CloudControl) error {
				routes, err := getRoutes(ctx, store, "us-west-2", account, mrapARN(testMrap))
				if err != nil {
					return err
				}
				for _, r := range routes {
					if aws.ToString(r.Bucket) == "" || aws.ToString(r.Region) == "" || r.TrafficDialPercentage == nil {
						return fmt.Errorf("a route has no bucket, Region, or TrafficDialPercentage: %+v", r)
					}
				}
				return nil
			},
		}),
		Unclear: routeCallsUnclear + " The API reference doesn't say how a new Multi-Region Access Point routes; AWS's " +
			"launch post for failover controls says it routes to every bucket, so the fake makes every route active.",
	},
	"SubmitMultiRegionAccessPointRoutes": {
		Doc: apiReference + "API_control_SubmitMultiRegionAccessPointRoutes.html",
		Rules: append(routeCallRules(func(ctx context.Context, store *awsfake.CloudControl, region string, accountID string, mrap string) error {
			return submitRoutes(ctx, store, region, accountID, mrap, route("bucket-east", "", 100))
		}),
			nativetest.Rule{
				Rule: "the routes of Regions the call doesn't name keep their configuration",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := submit(ctx, store, route("bucket-west", "", 0)); err != nil {
						return err
					}
					if err := submit(ctx, store, route("bucket-eu", "", 0)); err != nil {
						return err
					}
					routes, err := getRoutes(ctx, store, "us-west-2", account, mrapARN(testMrap))
					if err != nil {
						return err
					}
					if got := dials(routes); got["bucket-west"] != 0 || got["bucket-eu"] != 0 || got["bucket-east"] != 100 {
						return fmt.Errorf("the routes are %v", got)
					}
					return nil
				},
			},
			nativetest.Rule{
				Rule: "a route sets Bucket, Region, or both",
				Doc:  routeType,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return submit(ctx, store, route("", "", 0))
				},
				Code: invalidRequestCode,
			},
			nativetest.Rule{
				Rule: "a route that sets both names the bucket's Region",
				Doc:  routeType,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return submit(ctx, store, route("bucket-east", "eu-west-1", 0))
				},
				Code: invalidRequestCode,
			},
			nativetest.Rule{
				Rule: "a route can name its bucket by Region only",
				Doc:  routeType,
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := submit(ctx, store, route("", "eu-west-1", 0)); err != nil {
						return err
					}
					routes, err := getRoutes(ctx, store, "us-west-2", account, mrapARN(testMrap))
					if err != nil {
						return err
					}
					if got := dials(routes)["bucket-eu"]; got != 0 {
						return fmt.Errorf("the route in eu-west-1 is %d", got)
					}
					return nil
				},
			},
			nativetest.Rule{
				Rule: "TrafficDialPercentage is from 0 to 100",
				Doc:  routeType,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return submit(ctx, store, route("bucket-east", "", 101))
				},
				Code: invalidRequestCode,
			},
			nativetest.Rule{
				Rule: "at least one Region stays active, and a configuration with every Region passive fails with InvalidRequest",
				Doc:  routeType,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return submit(ctx, store, route("bucket-east", "", 0), route("bucket-west", "", 0), route("bucket-eu", "", 0))
				},
				Code: invalidRequestCode,
			},
			nativetest.Rule{
				Rule: "a failover needs a Multi-Region Access Point with at least two Regions, and fails with 405 Method Not Allowed otherwise",
				Doc:  failoverErrors,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return submitRoutes(ctx, store, "us-west-2", account, mrapARN(singlePrefix+".mrap"), route("bucket-east", "", 100))
				},
				Code: methodNotAllowedCode,
			},
		),
		Unclear: routeCallsUnclear + " The route data type names no error for a route without Bucket or Region, a " +
			"bucket outside the Region given, a bucket the Multi-Region Access Point doesn't have, or a " +
			"TrafficDialPercentage outside 0 to 100; the fake returns InvalidRequest, the error the page gives for every " +
			"Region passive. The docs describe only 0 (passive) and 100 (active); the fake counts any other value as " +
			"active. The failover errors page gives 405 for a Multi-Region Access Point with one Region without naming " +
			"the code; the fake returns MethodNotAllowed, S3's code for 405, for any route update of one. It gives 409 " +
			"for two updates of one Multi-Region Access Point at once without naming the code; the fake handles one call " +
			"at a time, and the handler sends one call per change. AWS can take up to 2 minutes to route by a new " +
			"configuration; the fake reads it back at once.",
	},
}
