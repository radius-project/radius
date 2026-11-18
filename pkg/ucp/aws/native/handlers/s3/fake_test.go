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
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/s3control"
	"github.com/aws/aws-sdk-go-v2/service/s3control/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativefake"
)

const (
	account = "123456789012"

	// missingPrefix starts the names of buckets, and the aliases of Multi-Region Access Points, AWS doesn't have.
	missingPrefix = "missing"

	// otherOwnerPrefix starts the names of buckets another account owns.
	otherOwnerPrefix = "other-owner"

	// singlePrefix starts the aliases of Multi-Region Access Points with one bucket.
	singlePrefix = "single"

	// invalidRequestCode, malformedXMLCode, accessDeniedCode, and methodNotAllowedCode are errors from S3's list of
	// error codes.
	invalidRequestCode   = "InvalidRequest"
	malformedXMLCode     = "MalformedXML"
	accessDeniedCode     = "AccessDenied"
	methodNotAllowedCode = "MethodNotAllowed"
)

// mrapARN returns the ARN of the Multi-Region Access Point with alias, in the fake's account.
func mrapARN(alias string) string {
	return "arn:aws:s3::" + account + ":accesspoint/" + alias
}

var (
	// accountIDPattern is the pattern of AccountId in the route calls.
	accountIDPattern = regexp.MustCompile(`^[0-9]{12}$`)

	// fakeMrapPattern matches the ARN of a Multi-Region Access Point, with its account ID and alias.
	fakeMrapPattern = regexp.MustCompile(`^arn:aws:s3::([0-9]{12}):accesspoint/([a-zA-Z0-9.-]+)$`)
)

// fakeAPI is an in-memory S3 and S3 Control API for Requester Pays settings and Multi-Region Access Point routes. It
// keeps them in an awsfake store, in their types' property format, so the suite's Cloud Control sees the same
// resources.
//
// Every bucket exists, except those named with missingPrefix, and is owned by the caller, except those named with
// otherOwnerPrefix, which is account. A bucket with no entry has S3's default payer, BucketOwner.
//
// Every Multi-Region Access Point in the fake's account exists, except those whose alias starts with missingPrefix.
// One whose alias starts with singlePrefix has one bucket, and the others have three (see buckets). One with no entry
// has every route active, as AWS gives a new one.
type fakeAPI struct {
	store *awsfake.CloudControl

	mu    sync.Mutex
	calls []string

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error
}

var (
	_ API        = (*fakeAPI)(nil)
	_ ControlAPI = (*fakeAPI)(nil)
)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{store: store, errs: map[string]error{}}
}

func fault(code string) error {
	return nativefake.ClientError(code, code)
}

// begin records a call and returns the error errs sets for it.
func (f *fakeAPI) begin(method string) error {
	f.calls = append(f.calls, method)
	return f.errs[method]
}

// missing returns the error the SDK returns, before it sends anything, for a call without a required parameter.
func missing(operation string, parameter string) error {
	err := smithy.InvalidParamsError{Context: operation}
	err.Add(smithy.NewErrParamRequired(parameter))
	return err
}

// bucketError returns the error S3 returns for a call on a bucket the caller doesn't own or AWS doesn't have, or that
// expects another owner.
func bucketError(bucket *string, expectedOwner *string, operation string) error {
	switch {
	case bucket == nil:
		return missing(operation, "Bucket")
	case expectedOwner != nil && *expectedOwner != account:
		return fault(accessDeniedCode)
	}
	switch name := *bucket; {
	case strings.HasPrefix(name, missingPrefix):
		return fault(noSuchBucketCode)
	case strings.HasPrefix(name, otherOwnerPrefix):
		return fault(accessDeniedCode)
	}
	return nil
}

func (f *fakeAPI) GetBucketRequestPayment(ctx context.Context, in *s3.GetBucketRequestPaymentInput, _ ...func(*s3.Options)) (*s3.GetBucketRequestPaymentOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("GetBucketRequestPayment"); err != nil {
		return nil, err
	}
	if err := bucketError(in.Bucket, in.ExpectedBucketOwner, "GetBucketRequestPaymentInput"); err != nil {
		return nil, err
	}
	payer, _ := payerOf(f.store.Resource(bucketRequestPaymentType, *in.Bucket))
	if payer == "" {
		payer = string(s3types.PayerBucketOwner)
	}
	return &s3.GetBucketRequestPaymentOutput{Payer: s3types.Payer(payer)}, nil
}

func (f *fakeAPI) PutBucketRequestPayment(ctx context.Context, in *s3.PutBucketRequestPaymentInput, _ ...func(*s3.Options)) (*s3.PutBucketRequestPaymentOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("PutBucketRequestPayment"); err != nil {
		return nil, err
	}
	if in.Bucket == nil {
		return nil, missing("PutBucketRequestPaymentInput", "Bucket")
	}
	if in.RequestPaymentConfiguration == nil || !slices.Contains(s3types.Payer("").Values(), in.RequestPaymentConfiguration.Payer) {
		return nil, fault(malformedXMLCode)
	}
	if err := bucketError(in.Bucket, in.ExpectedBucketOwner, "PutBucketRequestPaymentInput"); err != nil {
		return nil, err
	}
	bucket := *in.Bucket
	f.store.Put(bucketRequestPaymentType, bucket, map[string]any{
		"Bucket":                      bucket,
		"RequestPaymentConfiguration": map[string]any{"Payer": string(in.RequestPaymentConfiguration.Payer)},
	})
	return &s3.PutBucketRequestPaymentOutput{}, nil
}

// buckets returns the routes of the Multi-Region Access Point with alias, each active.
func buckets(alias string) []any {
	route := func(bucket string, region string) any {
		return map[string]any{"Bucket": bucket, "Region": region, "TrafficDialPercentage": activeDial}
	}
	if strings.HasPrefix(alias, singlePrefix) {
		return []any{route("bucket-east", "us-east-1")}
	}
	return []any{route("bucket-east", "us-east-1"), route("bucket-west", "us-west-2"), route("bucket-eu", "eu-west-1")}
}

// routes returns the routes of the Multi-Region Access Point a route call names, or the error AWS returns for the
// call: a Region that doesn't take route calls, an AccountId or Mrap AWS doesn't take, and a Multi-Region Access
// Point AWS doesn't have.
func (f *fakeAPI) routes(accountID *string, mrap *string, optFns []func(*s3control.Options)) ([]any, error) {
	options := s3control.Options{}
	for _, fn := range optFns {
		fn(&options)
	}
	if !slices.Contains(controlRegions, options.Region) {
		return nil, fault(invalidRequestCode)
	}
	if !accountIDPattern.MatchString(aws.ToString(accountID)) {
		return nil, fault(invalidRequestCode)
	}
	arn := aws.ToString(mrap)
	match := fakeMrapPattern.FindStringSubmatch(arn)
	if match == nil || len(arn) > maxMrapLength {
		return nil, fault(invalidRequestCode)
	}
	if match[1] != aws.ToString(accountID) || strings.HasPrefix(match[2], missingPrefix) {
		return nil, fault(noSuchMrapCode)
	}
	if stored := f.store.Resource(routesType, arn); stored != nil {
		return stored["Routes"].([]any), nil
	}
	return buckets(match[2]), nil
}

func (f *fakeAPI) GetMultiRegionAccessPointRoutes(ctx context.Context, in *s3control.GetMultiRegionAccessPointRoutesInput, optFns ...func(*s3control.Options)) (*s3control.GetMultiRegionAccessPointRoutesOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("GetMultiRegionAccessPointRoutes"); err != nil {
		return nil, err
	}
	routes, err := f.routes(in.AccountId, in.Mrap, optFns)
	if err != nil {
		return nil, err
	}
	out := &s3control.GetMultiRegionAccessPointRoutesOutput{Mrap: in.Mrap}
	for _, r := range routes {
		m := r.(map[string]any)
		out.Routes = append(out.Routes, types.MultiRegionAccessPointRoute{
			Bucket:                aws.String(awsapi.String(m, "Bucket")),
			Region:                aws.String(awsapi.String(m, "Region")),
			TrafficDialPercentage: aws.Int32(int32(dial(m))),
		})
	}
	return out, nil
}

// dial returns the TrafficDialPercentage of a stored route.
func dial(route map[string]any) int {
	switch n := route["TrafficDialPercentage"].(type) {
	case int:
		return n
	case float64:
		return int(n)
	}
	return 0
}

// SubmitMultiRegionAccessPointRoutes changes the routes the call names and keeps the others. It fails, changing
// nothing, for a route that names neither a bucket nor a Region, or a bucket in another Region than the one it gives,
// a TrafficDialPercentage outside 0 to 100, a Multi-Region Access Point with one bucket, and a configuration with no
// active route.
func (f *fakeAPI) SubmitMultiRegionAccessPointRoutes(ctx context.Context, in *s3control.SubmitMultiRegionAccessPointRoutesInput, optFns ...func(*s3control.Options)) (*s3control.SubmitMultiRegionAccessPointRoutesOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("SubmitMultiRegionAccessPointRoutes"); err != nil {
		return nil, err
	}
	if in.RouteUpdates == nil {
		return nil, missing("SubmitMultiRegionAccessPointRoutesInput", "RouteUpdates")
	}
	current, err := f.routes(in.AccountId, in.Mrap, optFns)
	if err != nil {
		return nil, err
	}
	if len(current) < 2 {
		return nil, fault(methodNotAllowedCode)
	}
	updated := []any{}
	for _, r := range current {
		m := r.(map[string]any)
		updated = append(updated, map[string]any{"Bucket": m["Bucket"], "Region": m["Region"], "TrafficDialPercentage": dial(m)})
	}
	for _, u := range in.RouteUpdates {
		bucket, region := aws.ToString(u.Bucket), aws.ToString(u.Region)
		if (bucket == "" && region == "") || u.TrafficDialPercentage == nil || *u.TrafficDialPercentage < 0 || *u.TrafficDialPercentage > 100 {
			return nil, fault(invalidRequestCode)
		}
		i := slices.IndexFunc(updated, func(r any) bool {
			m := r.(map[string]any)
			return (bucket == "" || m["Bucket"] == bucket) && (region == "" || m["Region"] == region)
		})
		if i < 0 {
			return nil, fault(invalidRequestCode)
		}
		updated[i].(map[string]any)["TrafficDialPercentage"] = int(*u.TrafficDialPercentage)
	}
	if !slices.ContainsFunc(updated, func(r any) bool { return dial(r.(map[string]any)) > 0 }) {
		return nil, fault(invalidRequestCode)
	}
	arn := aws.ToString(in.Mrap)
	f.store.Put(routesType, arn, map[string]any{"Mrap": arn, "Routes": updated})
	return &s3control.SubmitMultiRegionAccessPointRoutesOutput{}, nil
}
