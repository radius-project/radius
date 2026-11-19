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

package appsync

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/appsync"
	"github.com/aws/aws-sdk-go-v2/service/appsync/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	apiReference = "https://docs.aws.amazon.com/appsync/latest/APIReference/"
	s3Reference  = "https://docs.aws.amazon.com/AmazonS3/latest/API/"
	s3Errors     = s3Reference + "API_Error.html"
)

// startSchema starts creating the schema of the API id with definition, with the fake f.
func startSchema(ctx context.Context, f *fakeAPI, id string, definition []byte) error {
	_, err := f.StartSchemaCreation(ctx, &appsync.StartSchemaCreationInput{ApiId: aws.String(id), Definition: definition})
	return err
}

// schemaStatus returns the status of the last schema creation of the API id, with the fake f.
func schemaStatus(ctx context.Context, f *fakeAPI, id string) (types.SchemaStatus, error) {
	out, err := f.GetSchemaCreationStatus(ctx, &appsync.GetSchemaCreationStatusInput{ApiId: aws.String(id)})
	if err != nil {
		return "", err
	}
	return out.Status, nil
}

// getObject reads the object key of bucket with a fake S3 API that holds objects, which map bucket/key to a body.
func getObject(ctx context.Context, store *awsfake.CloudControl, objects map[string]string, bucket string, key string) error {
	f := newFakeAPI(store)
	f.objects = objects
	_, err := f.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	return err
}

// graphQLSchemaAPI records the AppSync and S3 calls GraphQLSchema makes.
var graphQLSchemaAPI = map[string]nativetest.Call{
	"StartSchemaCreation": {
		Doc: apiReference + "API_StartSchemaCreation.html",
		Rules: []nativetest.Rule{
			{
				Rule: "definition is required",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return startSchema(ctx, newFakeAPI(store, apiID), apiID, nil)
				},
				Code: "BadRequestException",
			},
			{
				Rule: "the API exists",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return startSchema(ctx, newFakeAPI(store, apiID), "unknown", []byte("type Query { a: String }"))
				},
				Code: "NotFoundException",
			},
			{
				Rule: "another modification in progress must complete first, such as a schema creation that is still running",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store, apiID)
					if err := startSchema(ctx, f, apiID, []byte("type Query { a: String }")); err != nil {
						return err
					}
					return startSchema(ctx, f, apiID, []byte("type Query { b: String }"))
				},
				Code: "ConcurrentModificationException",
			},
			{
				Rule: "the call is asynchronous: it returns while the schema is PROCESSING, and GetSchemaCreationStatus " +
					"tells when the creation has completed",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store, apiID)
					out, err := f.StartSchemaCreation(ctx, &appsync.StartSchemaCreationInput{ApiId: aws.String(apiID), Definition: []byte("type Query { a: String }")})
					if err != nil {
						return err
					}
					first, err := schemaStatus(ctx, f, apiID)
					if err != nil {
						return err
					}
					last, err := schemaStatus(ctx, f, apiID)
					if err != nil {
						return err
					}
					if out.Status != types.SchemaStatusProcessing || first != types.SchemaStatusProcessing || last != types.SchemaStatusSuccess {
						return fmt.Errorf("the creation went %s, %s, %s", out.Status, first, last)
					}
					return nil
				},
			},
		},
		Unclear: "The definition is a blob, which the SDK sends base64-encoded in the JSON body; the fake takes the bytes " +
			"as the handler gives them. The page doesn't say whether an empty definition is rejected, or how large one " +
			"can be; the fake accepts any definition, and fails the creation of one that contains \"invalid\" to stand " +
			"for a definition AppSync can't parse. It doesn't say whether a failed creation blocks the next one; the " +
			"fake accepts a new creation after a failure.",
	},
	"GetSchemaCreationStatus": {
		Doc: apiReference + "API_GetSchemaCreationStatus.html",
		Rules: []nativetest.Rule{
			{
				Rule: "the API exists",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := schemaStatus(ctx, newFakeAPI(store, apiID), "unknown")
					return err
				},
				Code: "NotFoundException",
			},
		},
		Unclear: "The page lists the statuses PROCESSING, ACTIVE, DELETING, FAILED, SUCCESS, and NOT_APPLICABLE without " +
			"saying when a schema creation reports ACTIVE, DELETING, or NOT_APPLICABLE. The handler treats ACTIVE and " +
			"NOT_APPLICABLE as done and waits on DELETING; the fake reports only PROCESSING, SUCCESS, and FAILED. The " +
			"page doesn't say what an API that never had a schema reports. The fake keeps an API and its schema as one " +
			"resource, so it returns NotFoundException for such an API.",
	},
	"GetGraphqlApi": {
		Doc: apiReference + "API_GetGraphqlApi.html",
		Rules: []nativetest.Rule{
			{
				Rule: "the API exists",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store, apiID).GetGraphqlApi(ctx, &appsync.GetGraphqlApiInput{ApiId: aws.String("unknown")})
					return err
				},
				Code: "NotFoundException",
			},
		},
		Unclear: "The fake keeps an API and its schema as one resource, so it returns NotFoundException for an API that " +
			"has no schema yet, which AWS returns.",
	},
	"GetObject": {
		Doc: s3Reference + "API_GetObject.html",
		Rules: []nativetest.Rule{
			{
				Rule: "the bucket exists",
				Doc:  s3Errors,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return getObject(ctx, store, map[string]string{}, "schemas", "schema.graphql")
				},
				Code: "NoSuchBucket",
			},
			{
				Rule: "the key exists in the bucket",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return getObject(ctx, store, map[string]string{"schemas/other.graphql": "type Query { a: String }"}, "schemas", "schema.graphql")
				},
				Code: "NoSuchKey",
			},
		},
		Unclear: "For a key that doesn't exist, S3 returns NoSuchKey only to a caller with s3:ListBucket on the bucket, " +
			"and 403 Access Denied otherwise; the fake always returns NoSuchKey. Key has a minimum length of 1, but the " +
			"page doesn't give the error for an empty one; the handler never sends one. The fake doesn't model archived " +
			"objects, which S3 rejects with InvalidObjectState until they are restored.",
	},
}
