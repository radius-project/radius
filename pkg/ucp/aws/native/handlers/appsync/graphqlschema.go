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

// Package appsync has the native handler for AppSync GraphQL schemas, which Cloud Control can't create in any region.
package appsync

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/appsync"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const schemaType = "AWS::AppSync::GraphQLSchema"

// API is the part of the AppSync client the handler uses.
type API interface {
	StartSchemaCreation(ctx context.Context, params *appsync.StartSchemaCreationInput, optFns ...func(*appsync.Options)) (*appsync.StartSchemaCreationOutput, error)
	GetSchemaCreationStatus(ctx context.Context, params *appsync.GetSchemaCreationStatusInput, optFns ...func(*appsync.Options)) (*appsync.GetSchemaCreationStatusOutput, error)
	GetGraphqlApi(ctx context.Context, params *appsync.GetGraphqlApiInput, optFns ...func(*appsync.Options)) (*appsync.GetGraphqlApiOutput, error)
}

// ObjectAPI is the part of the S3 client the handler uses, to read a definition from DefinitionS3Location.
type ObjectAPI interface {
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

var (
	_ native.Handler           = (*GraphQLSchema)(nil)
	_ native.IdentifierEncoder = (*GraphQLSchema)(nil)
)

// schemaStates are the states of a schema's creation. NOT_APPLICABLE means no creation is running, such as for an API
// that never had a schema.
var schemaStates = awsapi.States{Done: []string{"SUCCESS", "ACTIVE", "NOT_APPLICABLE"}, Failed: []string{"FAILED"}}

// GraphQLSchema serves AWS::AppSync::GraphQLSchema. An API has one schema, so the identifier, Id, is the API's ID.
// AppSync can't return a definition as it was sent, so Definition and DefinitionS3Location are write-only, and has no
// call that removes a schema, so a delete leaves it (onDelete nothing).
type GraphQLSchema struct {
	api     API
	objects ObjectAPI
}

// NewGraphQLSchema returns the handler for AppSync GraphQL schemas. It reads a definition in S3 with objects.
func NewGraphQLSchema(api API, objects ObjectAPI) *GraphQLSchema {
	return &GraphQLSchema{api: api, objects: objects}
}

// TypeName returns AWS::AppSync::GraphQLSchema.
func (h *GraphQLSchema) TypeName() string {
	return schemaType
}

// Options returns how UCP uses the handler.
func (h *GraphQLSchema) Options() native.Options {
	return native.Options{
		List:     native.ListNone,
		Recovery: native.RecoverByIdentifier,
	}
}

// Identifier returns ApiId.
func (h *GraphQLSchema) Identifier(properties map[string]any) (string, error) {
	return awsapi.String(properties, "ApiId"), nil
}

// Read returns the schema's API ID, or ErrNotFound when AWS has no such API.
func (h *GraphQLSchema) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	_, err := h.api.GetGraphqlApi(ctx, &appsync.GetGraphqlApiInput{ApiId: aws.String(identifier)}, func(o *appsync.Options) { o.Region = region })
	if err != nil {
		return nil, awsapi.NotFound(err, "NotFoundException")
	}
	return map[string]any{"ApiId": identifier, "Id": identifier}, nil
}

// Create starts creating the schema of the API ApiId names.
func (h *GraphQLSchema) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	identifier, _ := h.Identifier(request.Properties)
	if err := h.start(ctx, request.Region, identifier, request.Properties); err != nil {
		return "", err
	}
	return identifier, nil
}

// Update starts creating the schema again when the definition changed. A template that no longer sets a definition
// leaves the schema AWS has.
func (h *GraphQLSchema) Update(ctx context.Context, request native.UpdateRequest) error {
	changed := awsapi.Changed(request)
	if !slices.Contains(changed, "Definition") && !slices.Contains(changed, "DefinitionS3Location") {
		return nil
	}
	if awsapi.String(request.Desired, "Definition") == "" && awsapi.String(request.Desired, "DefinitionS3Location") == "" {
		return nil
	}
	return h.start(ctx, request.Region, request.Identifier, request.Desired)
}

// start sends the definition the properties give with StartSchemaCreation.
func (h *GraphQLSchema) start(ctx context.Context, region string, apiID string, properties map[string]any) error {
	definition, err := h.definition(ctx, region, properties)
	if err != nil {
		return err
	}
	_, err = h.api.StartSchemaCreation(ctx, &appsync.StartSchemaCreationInput{ApiId: aws.String(apiID), Definition: definition}, func(o *appsync.Options) { o.Region = region })
	return err
}

// definition returns Definition, or reads the object DefinitionS3Location names, such as s3://bucket/schema.graphql.
func (h *GraphQLSchema) definition(ctx context.Context, region string, properties map[string]any) ([]byte, error) {
	if definition := awsapi.String(properties, "Definition"); definition != "" {
		return []byte(definition), nil
	}
	location := awsapi.String(properties, "DefinitionS3Location")
	bucket, key, ok := strings.Cut(strings.TrimPrefix(location, "s3://"), "/")
	if location == "" || !strings.HasPrefix(location, "s3://") || !ok || bucket == "" || key == "" {
		return nil, &native.Failure{
			Message:     "An AppSync GraphQL schema needs Definition, or DefinitionS3Location in the form s3://bucket/key",
			ClientError: true,
			Outcome:     native.OutcomeRejected,
		}
	}
	out, err := h.objects.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)}, func(o *s3.Options) { o.Region = region })
	if err != nil {
		return nil, fmt.Errorf("failed to read DefinitionS3Location: %w", err)
	}
	defer out.Body.Close()
	b, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read DefinitionS3Location: %w", err)
	}
	return b, nil
}

// Delete does nothing: AppSync has no call that removes a schema. UCP never calls it, because the type's metadata sets
// onDelete to nothing.
func (h *GraphQLSchema) Delete(ctx context.Context, region string, identifier string) error {
	return nil
}

// List returns nothing: UCP can't list the type.
func (h *GraphQLSchema) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, nil
}

// Progress reads the state of the API's last schema creation. A delete changes nothing, so it is done.
func (h *GraphQLSchema) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	if op == native.OperationDelete {
		return native.Progress{Done: true}, nil
	}
	out, err := h.api.GetSchemaCreationStatus(ctx, &appsync.GetSchemaCreationStatusInput{ApiId: aws.String(identifier)}, func(o *appsync.Options) { o.Region = region })
	if err != nil {
		return native.Progress{}, awsapi.NotFound(err, "NotFoundException")
	}
	progress := schemaStates.Progress(string(out.Status))
	if progress.Failure != nil {
		// A schema AppSync failed to create changes nothing, so the next deploy sends the definition again.
		progress.Failure.ClientError, progress.Failure.Outcome = true, native.OutcomeRejected
		if details := aws.ToString(out.Details); details != "" {
			progress.Failure.Message = "AppSync failed to create the schema: " + details
		}
	}
	return progress, nil
}
