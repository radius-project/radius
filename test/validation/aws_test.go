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
package validation

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/smithy-go"
	awsclient "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/stretchr/testify/require"
)

func Test_isAWSResourceMissing(t *testing.T) {
	t.Parallel()

	// The CloudControl handler wraps the message from the downstream service, so reproduce that shape
	// rather than matching on the bare sentence.
	logGroupCannotBeFound := "AWS::Logs::LogGroup Handler returned status FAILED: Log group cannot be found. " +
		"(Service: CloudWatchLogs, Status Code: 400, Request ID: f57fca9e-6a8c-4adf-a7d0-65a25d8de2af) " +
		"(SDK Attempt Count: 1) (HandlerErrorCode: InvalidRequest)"
	logGroupDoesNotExist := "AWS::Logs::LogGroup Handler returned status FAILED: The specified log group does not exist. " +
		"(Service: CloudWatchLogs, Status Code: 400, Request ID: 5141a66f-fad4-4275-8120-a55d7486abac) " +
		"(SDK Attempt Count: 1) (HandlerErrorCode: InvalidRequest)"

	tests := []struct {
		name             string
		resourceTypeName string
		err              error
		expected         bool
	}{
		{
			name:             "nil error is not missing",
			resourceTypeName: awsLogGroupTypeName,
			err:              nil,
			expected:         false,
		},
		{
			name:             "typed ResourceNotFoundException is missing",
			resourceTypeName: awsLogGroupTypeName,
			err:              &types.ResourceNotFoundException{Message: aws.String("resource not found")},
			expected:         true,
		},
		{
			name:             "typed ResourceNotFoundException is missing for any resource type",
			resourceTypeName: "AWS::MemoryDB::Cluster",
			err:              &types.ResourceNotFoundException{Message: aws.String("resource not found")},
			expected:         true,
		},
		{
			name:             "log group cannot be found is missing",
			resourceTypeName: awsLogGroupTypeName,
			err:              &types.InvalidRequestException{Message: aws.String(logGroupCannotBeFound)},
			expected:         true,
		},
		{
			name:             "specified log group does not exist is missing",
			resourceTypeName: awsLogGroupTypeName,
			err:              &types.InvalidRequestException{Message: aws.String(logGroupDoesNotExist)},
			expected:         true,
		},
		{
			name:             "wrapped log group InvalidRequestException is missing",
			resourceTypeName: awsLogGroupTypeName,
			err: fmt.Errorf("operation error CloudControl: GetResource: %w",
				&types.InvalidRequestException{Message: aws.String(logGroupCannotBeFound)}),
			expected: true,
		},
		{
			name:             "unrelated InvalidRequestException is not missing",
			resourceTypeName: awsLogGroupTypeName,
			err:              &types.InvalidRequestException{Message: aws.String("Invalid patch document")},
			expected:         false,
		},
		{
			name:             "throttling error is not missing",
			resourceTypeName: awsLogGroupTypeName,
			err:              &types.ThrottlingException{Message: aws.String("rate exceeded")},
			expected:         false,
		},
		{
			name:             "log group message on a different resource type is not missing",
			resourceTypeName: "AWS::MemoryDB::Cluster",
			err:              &types.InvalidRequestException{Message: aws.String(logGroupCannotBeFound)},
			expected:         false,
		},
		{
			name:             "generic API error with matching message is not missing",
			resourceTypeName: awsLogGroupTypeName,
			err:              &smithy.GenericAPIError{Code: "InvalidRequestException", Message: logGroupCannotBeFound},
			expected:         false,
		},
		{
			name:             "arbitrary error is not missing",
			resourceTypeName: awsLogGroupTypeName,
			err:              errors.New("connection reset by peer"),
			expected:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.expected, isAWSResourceMissing(tt.resourceTypeName, tt.err))
		})
	}
}

// fakeCloudControl lists pages keyed by the request's NextToken, with "" for the first page, and reads resources from
// get by identifier.
type fakeCloudControl struct {
	awsclient.AWSCloudControlClient
	pages    map[string]*cloudcontrol.ListResourcesOutput
	listErr  error
	get      map[string]fakeGetResult
	getCalls []string
}

type fakeGetResult struct {
	output *cloudcontrol.GetResourceOutput
	err    error
}

func (f *fakeCloudControl) ListResources(_ context.Context, input *cloudcontrol.ListResourcesInput, _ ...func(*cloudcontrol.Options)) (*cloudcontrol.ListResourcesOutput, error) {
	if aws.ToString(input.TypeName) != queryDefinitionTypeName {
		return nil, fmt.Errorf("unexpected type %s", aws.ToString(input.TypeName))
	}
	if f.listErr != nil {
		return nil, f.listErr
	}
	page, ok := f.pages[aws.ToString(input.NextToken)]
	if !ok {
		return nil, fmt.Errorf("unexpected token %q", aws.ToString(input.NextToken))
	}
	return page, nil
}

func (f *fakeCloudControl) GetResource(_ context.Context, input *cloudcontrol.GetResourceInput, _ ...func(*cloudcontrol.Options)) (*cloudcontrol.GetResourceOutput, error) {
	if aws.ToString(input.TypeName) != queryDefinitionTypeName {
		return nil, fmt.Errorf("unexpected type %s", aws.ToString(input.TypeName))
	}
	id := aws.ToString(input.Identifier)
	f.getCalls = append(f.getCalls, id)
	result, ok := f.get[id]
	if !ok {
		return nil, fmt.Errorf("unexpected read of %s", id)
	}
	return result.output, result.err
}

const queryDefinitionTypeName = "AWS::Logs::QueryDefinition"

func listed(id string, properties *string) types.ResourceDescription {
	return types.ResourceDescription{Identifier: aws.String(id), Properties: properties}
}

func onePage(descriptions ...types.ResourceDescription) map[string]*cloudcontrol.ListResourcesOutput {
	return map[string]*cloudcontrol.ListResourcesOutput{"": {ResourceDescriptions: descriptions}}
}

func read(properties string) fakeGetResult {
	return fakeGetResult{output: &cloudcontrol.GetResourceOutput{ResourceDescription: &types.ResourceDescription{Properties: aws.String(properties)}}}
}

func Test_ListAWSResourcesByProperty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		client       *fakeCloudControl
		expectedIDs  []string
		expectedErr  string
		expectedGets []string
	}{
		{
			name: "full list properties match without a read",
			client: &fakeCloudControl{pages: onePage(
				listed("id-1", aws.String(`{"Name":"wanted"}`)),
				listed("id-2", aws.String(`{"Name":"other"}`)),
			)},
			expectedIDs: []string{"id-1"},
		},
		{
			name: "nil list properties are read",
			client: &fakeCloudControl{
				pages: onePage(listed("id-1", nil)),
				get:   map[string]fakeGetResult{"id-1": read(`{"Name":"wanted"}`)},
			},
			expectedIDs:  []string{"id-1"},
			expectedGets: []string{"id-1"},
		},
		{
			name: "empty list properties are read",
			client: &fakeCloudControl{
				pages: onePage(listed("id-1", aws.String(`{}`))),
				get:   map[string]fakeGetResult{"id-1": read(`{"Name":"wanted"}`)},
			},
			expectedIDs:  []string{"id-1"},
			expectedGets: []string{"id-1"},
		},
		{
			name: "list properties without the name are read",
			client: &fakeCloudControl{
				pages: onePage(
					listed("id-1", aws.String(`{"QueryDefinitionId":"id-1"}`)),
					listed("id-2", aws.String(`{"QueryDefinitionId":"id-2"}`)),
				),
				get: map[string]fakeGetResult{
					"id-1": read(`{"Name":"other"}`),
					"id-2": read(`{"Name":"wanted"}`),
				},
			},
			expectedIDs:  []string{"id-2"},
			expectedGets: []string{"id-1", "id-2"},
		},
		{
			name: "a resource deleted before the read is skipped",
			client: &fakeCloudControl{
				pages: onePage(listed("id-1", nil)),
				get:   map[string]fakeGetResult{"id-1": {err: &types.ResourceNotFoundException{Message: aws.String("not found")}}},
			},
			expectedIDs:  []string{},
			expectedGets: []string{"id-1"},
		},
		{
			name: "a failed read is returned",
			client: &fakeCloudControl{
				pages: onePage(listed("id-1", nil)),
				get:   map[string]fakeGetResult{"id-1": {err: &types.ThrottlingException{Message: aws.String("rate exceeded")}}},
			},
			expectedErr:  "rate exceeded",
			expectedGets: []string{"id-1"},
		},
		{
			name: "a read without the name is an error",
			client: &fakeCloudControl{
				pages: onePage(listed("id-1", nil)),
				get:   map[string]fakeGetResult{"id-1": read(`{}`)},
			},
			expectedErr:  "without Name",
			expectedGets: []string{"id-1"},
		},
		{
			name: "a read without a description is an error",
			client: &fakeCloudControl{
				pages: onePage(listed("id-1", nil)),
				get:   map[string]fakeGetResult{"id-1": {output: &cloudcontrol.GetResourceOutput{}}},
			},
			expectedErr:  "no description",
			expectedGets: []string{"id-1"},
		},
		{
			name: "invalid read properties are an error",
			client: &fakeCloudControl{
				pages: onePage(listed("id-1", nil)),
				get:   map[string]fakeGetResult{"id-1": read(`{`)},
			},
			expectedErr:  "failed to read the properties",
			expectedGets: []string{"id-1"},
		},
		{
			name: "an empty read is an error",
			client: &fakeCloudControl{
				pages: onePage(listed("id-1", nil)),
				get:   map[string]fakeGetResult{"id-1": {}},
			},
			expectedErr:  "no description",
			expectedGets: []string{"id-1"},
		},
		{
			name:        "invalid list properties are an error",
			client:      &fakeCloudControl{pages: onePage(listed("id-1", aws.String(`{`)))},
			expectedErr: "failed to read the properties",
		},
		{
			name:        "a name that is not a string is an error",
			client:      &fakeCloudControl{pages: onePage(listed("id-1", aws.String(`{"Name":1}`)))},
			expectedErr: "not a string",
		},
		{
			name:        "a listed resource without an identifier is an error",
			client:      &fakeCloudControl{pages: onePage(listed("", aws.String(`{"Name":"wanted"}`)))},
			expectedErr: "no identifier",
		},
		{
			name:        "a failed list is returned",
			client:      &fakeCloudControl{listErr: &types.ThrottlingException{Message: aws.String("rate exceeded")}},
			expectedErr: "rate exceeded",
		},
		{
			name: "every page is searched",
			client: &fakeCloudControl{
				pages: map[string]*cloudcontrol.ListResourcesOutput{
					"": {
						ResourceDescriptions: []types.ResourceDescription{listed("id-1", aws.String(`{"Name":"wanted"}`))},
						NextToken:            aws.String("page-2"),
					},
					"page-2": {
						ResourceDescriptions: []types.ResourceDescription{listed("id-2", nil)},
					},
				},
				get: map[string]fakeGetResult{"id-2": read(`{"Name":"wanted"}`)},
			},
			expectedIDs:  []string{"id-1", "id-2"},
			expectedGets: []string{"id-2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ids, err := ListAWSResourcesByProperty(t.Context(), tt.client, queryDefinitionTypeName, "Name", "wanted")
			if tt.expectedErr != "" {
				require.ErrorContains(t, err, tt.expectedErr)
				require.Nil(t, ids)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedIDs, ids)
			}
			require.Equal(t, tt.expectedGets, tt.client.getCalls)
		})
	}
}
