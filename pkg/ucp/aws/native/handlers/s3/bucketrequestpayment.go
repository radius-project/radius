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
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	bucketRequestPaymentType = "AWS::S3::BucketRequestPayment"

	// noSuchBucketCode is the error S3 returns for a bucket it doesn't have.
	noSuchBucketCode = "NoSuchBucket"
)

var (
	_ native.Handler           = (*BucketRequestPayment)(nil)
	_ native.IdentifierEncoder = (*BucketRequestPayment)(nil)
	_ native.Resetter          = (*BucketRequestPayment)(nil)
)

// BucketRequestPayment serves AWS::S3::BucketRequestPayment, the Requester Pays setting of a bucket. Its identifier
// is the bucket's name. Every bucket has the setting, so UCP takes it over and updates it, and calls Create only for a
// bucket S3 doesn't have. The type's metadata makes a delete call Reset, which sets the payer back to BucketOwner,
// S3's default.
type BucketRequestPayment struct {
	api API
}

// NewBucketRequestPayment returns the handler for a bucket's Requester Pays setting.
func NewBucketRequestPayment(api API) *BucketRequestPayment {
	return &BucketRequestPayment{api: api}
}

// TypeName returns AWS::S3::BucketRequestPayment.
func (h *BucketRequestPayment) TypeName() string {
	return bucketRequestPaymentType
}

// Options returns how UCP uses the handler. The setting can't be listed.
func (h *BucketRequestPayment) Options() native.Options {
	return native.Options{List: native.ListNone, Recovery: native.RecoverByIdentifier}
}

// payerOf returns the Payer properties set, and whether they set one.
func payerOf(properties map[string]any) (string, bool) {
	configuration, _ := properties["RequestPaymentConfiguration"].(map[string]any)
	payer, ok := configuration["Payer"].(string)
	return payer, ok
}

// Identifier returns Bucket. A Payer S3 doesn't take is rejected before any call.
func (h *BucketRequestPayment) Identifier(properties map[string]any) (string, error) {
	if payer, ok := payerOf(properties); ok && !slices.Contains(types.Payer("").Values(), types.Payer(payer)) {
		return "", rejected("RequestPaymentConfiguration.Payer must be Requester or BucketOwner")
	}
	return awsapi.String(properties, "Bucket"), nil
}

// Read returns the bucket's payer, or ErrNotFound when S3 has no such bucket.
func (h *BucketRequestPayment) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	out, err := h.api.GetBucketRequestPayment(ctx, &s3.GetBucketRequestPaymentInput{Bucket: aws.String(identifier)}, inRegion(region))
	if err != nil {
		return nil, awsapi.NotFound(err, noSuchBucketCode)
	}
	properties := map[string]any{"Bucket": identifier}
	if out.Payer != "" {
		properties["RequestPaymentConfiguration"] = map[string]any{"Payer": string(out.Payer)}
	}
	return properties, nil
}

func (h *BucketRequestPayment) put(ctx context.Context, region string, bucket string, payer types.Payer) error {
	_, err := h.api.PutBucketRequestPayment(ctx, &s3.PutBucketRequestPaymentInput{
		Bucket:                      aws.String(bucket),
		RequestPaymentConfiguration: &types.RequestPaymentConfiguration{Payer: payer},
	}, inRegion(region))
	return err
}

// desiredPayer returns the Payer properties set, or a client failure when they set none.
func desiredPayer(properties map[string]any) (types.Payer, error) {
	payer, _ := payerOf(properties)
	if payer == "" {
		return "", rejected("set RequestPaymentConfiguration.Payer to Requester or BucketOwner")
	}
	return types.Payer(payer), nil
}

// Create sets the payer of the bucket and returns the bucket's name.
func (h *BucketRequestPayment) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	bucket := awsapi.String(request.Properties, "Bucket")
	payer, err := desiredPayer(request.Properties)
	if err != nil {
		return "", err
	}
	if err := h.put(ctx, request.Region, bucket, payer); err != nil {
		return "", err
	}
	return bucket, nil
}

// Update sets the payer of the bucket to the template's.
func (h *BucketRequestPayment) Update(ctx context.Context, request native.UpdateRequest) error {
	payer, err := desiredPayer(request.Desired)
	if err != nil {
		return err
	}
	return h.put(ctx, request.Region, request.Identifier, payer)
}

// Reset sets the payer of the bucket back to BucketOwner, S3's default. It returns ErrNotFound when S3 has no such
// bucket.
func (h *BucketRequestPayment) Reset(ctx context.Context, region string, identifier string) error {
	return awsapi.NotFound(h.put(ctx, region, identifier, types.PayerBucketOwner), noSuchBucketCode)
}

// Delete resets the setting, as Reset does. The type's metadata makes UCP call Reset.
func (h *BucketRequestPayment) Delete(ctx context.Context, region string, identifier string) error {
	return h.Reset(ctx, region, identifier)
}

// List fails, because the setting can't be listed.
func (h *BucketRequestPayment) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, fmt.Errorf("the AWS type %s can't be listed", bucketRequestPaymentType)
}

// Progress reports every operation as done, because PutBucketRequestPayment takes effect at once.
func (h *BucketRequestPayment) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	return native.Progress{Done: true}, nil
}
