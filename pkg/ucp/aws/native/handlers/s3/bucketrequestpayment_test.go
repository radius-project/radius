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
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/s3control"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

// requireRejected requires err to be a client failure AWS applied none of, whose message contains text.
func requireRejected(t *testing.T, err error, text string) {
	t.Helper()
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.Contains(t, failure.Message, text)
}

func Test_BucketRequestPayment_CreateAndUpdate(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewBucketRequestPayment(api)
	ctx := t.Context()

	bucket, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: payment("app-bucket", "Requester")})
	require.NoError(t, err)
	require.Equal(t, "app-bucket", bucket)
	props, err := h.Read(ctx, region, bucket)
	require.NoError(t, err)
	require.Equal(t, payment("app-bucket", "Requester"), props)

	t.Run("a template without a payer is rejected before any call", func(t *testing.T) {
		api.calls = nil
		_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: map[string]any{"Bucket": "app-bucket"}})
		requireRejected(t, err, "RequestPaymentConfiguration.Payer")
		err = h.Update(ctx, native.UpdateRequest{Region: region, Identifier: "app-bucket", Desired: map[string]any{"Bucket": "app-bucket"}})
		requireRejected(t, err, "RequestPaymentConfiguration.Payer")
		require.Empty(t, api.calls)
	})

	t.Run("a bucket S3 doesn't have", func(t *testing.T) {
		_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: payment(missingPrefix+"-bucket", "Requester")})
		require.ErrorContains(t, err, noSuchBucketCode)
		_, err = h.Read(ctx, region, missingPrefix+"-bucket")
		require.ErrorIs(t, err, native.ErrNotFound)
		require.ErrorIs(t, h.Reset(ctx, region, missingPrefix+"-bucket"), native.ErrNotFound)
	})

	t.Run("a bucket another account owns is not taken as missing", func(t *testing.T) {
		_, err := h.Read(ctx, region, otherOwnerPrefix+"-bucket")
		require.ErrorContains(t, err, accessDeniedCode)
		require.NotErrorIs(t, err, native.ErrNotFound)
		err = h.Reset(ctx, region, otherOwnerPrefix+"-bucket")
		require.NotErrorIs(t, err, native.ErrNotFound, "a delete never drops the record of a setting it couldn't reset")
	})
}

func Test_BucketRequestPayment_DeleteResets(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewBucketRequestPayment(api)
	ctx := t.Context()
	_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: payment("app-bucket", "Requester")})
	require.NoError(t, err)

	require.NoError(t, h.Delete(ctx, region, "app-bucket"))
	props, err := h.Read(ctx, region, "app-bucket")
	require.NoError(t, err)
	require.Equal(t, payment("app-bucket", "BucketOwner"), props, "the bucket is back to S3's default")
}

func Test_BucketRequestPayment_ReadWithoutPayer(t *testing.T) {
	h := NewBucketRequestPayment(emptyPayerAPI{})
	props, err := h.Read(t.Context(), region, "app-bucket")
	require.NoError(t, err)
	require.Equal(t, map[string]any{"Bucket": "app-bucket"}, props)
}

// emptyPayerAPI returns no payer.
type emptyPayerAPI struct {
	API
}

func (emptyPayerAPI) GetBucketRequestPayment(ctx context.Context, in *s3.GetBucketRequestPaymentInput, _ ...func(*s3.Options)) (*s3.GetBucketRequestPaymentOutput, error) {
	return &s3.GetBucketRequestPaymentOutput{}, nil
}

func Test_BucketRequestPayment_Identifier(t *testing.T) {
	h := NewBucketRequestPayment(nil)
	id, err := h.Identifier(map[string]any{"Bucket": "app-bucket"})
	require.NoError(t, err, "existing gives only the bucket")
	require.Equal(t, "app-bucket", id)
	_, err = h.Identifier(payment("app-bucket", "requester"))
	requireRejected(t, err, "Requester or BucketOwner")
}

func Test_ListAndProgress(t *testing.T) {
	for _, h := range []native.Handler{NewBucketRequestPayment(nil), NewMultiRegionAccessPointRoutes(nil)} {
		_, err := h.List(t.Context(), region)
		require.ErrorContains(t, err, "can't be listed")
		progress, err := h.Progress(t.Context(), region, "id", native.OperationDelete)
		require.NoError(t, err)
		require.True(t, progress.Done)
	}
}

func Test_Fake_RequiredParameters(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	var params smithy.InvalidParamsError
	_, err := api.GetBucketRequestPayment(t.Context(), &s3.GetBucketRequestPaymentInput{})
	require.ErrorAs(t, err, &params)
	_, err = api.PutBucketRequestPayment(t.Context(), &s3.PutBucketRequestPaymentInput{RequestPaymentConfiguration: &s3types.RequestPaymentConfiguration{Payer: s3types.PayerRequester}})
	require.ErrorAs(t, err, &params)
	_, err = api.SubmitMultiRegionAccessPointRoutes(t.Context(), &s3control.SubmitMultiRegionAccessPointRoutesInput{AccountId: aws.String(account), Mrap: aws.String(mrapARN(testMrap))}, func(o *s3control.Options) { o.Region = region })
	require.ErrorAs(t, err, &params)
}
