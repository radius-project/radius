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

package awsproxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	queueinmemory "github.com/radius-project/radius/pkg/components/queue/inmemory"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	awsclient "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/records"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

const (
	AWSKinesisStreamResourceType                    = "AWS.Kinesis/Stream"
	AWSKinesisStreamAWSResourceType                 = "AWS::Kinesis::Stream"
	AWSMemoryDBClusterResourceType                  = "AWS.MemoryDB/Cluster"
	AWSMemoryDBClusterAWSResourceType               = "AWS::MemoryDB::Cluster"
	AWSRedShiftEndpointAuthorizationResourceType    = "AWS.RedShift/EndpointAuthorization"
	AWSRedShiftEndpointAuthorizationAWSResourceType = "AWS::RedShift::EndpointAuthorization"

	testAWSRequestToken = "79B9F0DA-4882-4DC8-A367-6FD3BC122DED" // Random UUID
	testHost            = "localhost:5000"
	testScheme          = "http"

	// testOwner is the owner the tests send on PUT and DELETE.
	testOwner = "deploy;/planes/radius/local/resourceGroups/rg;;/planes/radius/local/resourceGroups/rg/providers/Applications.Core/environments/env"
)

type TestOptions struct {
	AWSCloudControlClient   *awsclient.MockAWSCloudControlClient
	AWSCloudFormationClient *awsclient.MockAWSCloudFormationClient
	DatabaseClient          database.Client
	Queue                   *queueinmemory.Client
	StatusManager           statusmanager.StatusManager
}

// setupTest returns a TestOptions struct with mocked AWS clients, and an in-memory data store and queue.
func setupTest(t *testing.T) TestOptions {
	mockCtrl := gomock.NewController(t)
	db := inmemory.NewClient()
	queue := queueinmemory.New(queueinmemory.NewInMemQueue(time.Minute))
	return TestOptions{
		AWSCloudControlClient:   awsclient.NewMockAWSCloudControlClient(mockCtrl),
		AWSCloudFormationClient: awsclient.NewMockAWSCloudFormationClient(mockCtrl),
		DatabaseClient:          db,
		Queue:                   queue,
		StatusManager:           statusmanager.New(db, queue, "global"),
	}
}

// ControllerOptions returns the controller options for the test's data store and status manager.
func (o TestOptions) ControllerOptions() armrpc_controller.Options {
	return armrpc_controller.Options{DatabaseClient: o.DatabaseClient, StatusManager: o.StatusManager}
}

// Clients returns the test's AWS clients.
func (o TestOptions) Clients() awsclient.Clients {
	return awsclient.Clients{CloudControl: o.AWSCloudControlClient, CloudFormation: o.AWSCloudFormationClient}
}

// testFingerprintKeyBytes is the fingerprint key the tests use, unless a test replaces it.
var testFingerprintKeyBytes = []byte("0123456789abcdef0123456789abcdef")

// testFingerprintKey returns the tests' fingerprint key, like aws.Module.FingerprintKey.
func testFingerprintKey() (*awsclient.FingerprintKey, error) {
	return awsclient.NewFingerprintKey(testFingerprintKeyBytes)
}

// newTestPutController creates the PUT controller with the tests' fingerprint key.
func newTestPutController(opts armrpc_controller.Options, clients awsclient.Clients) (armrpc_controller.Controller, error) {
	return NewCreateOrUpdateAWSResource(opts, clients, testFingerprintKey)
}

// withOwner sets the owner header that PUT and DELETE require.
func withOwner(req *http.Request) *http.Request {
	req.Header.Set(sdkclients.AWSResourceOwnerHeader, testOwner)
	return req
}

type AWSTestResource struct {
	ResourceType          string
	AWSResourceType       string
	ResourceName          string
	ARN                   string
	CollectionPath        string
	SingleResourcePath    string
	OperationResultsPath  string
	OperationStatusesPath string
	LocationHeader        string
	AzureAsyncOpHeader    string
	Schema                string
}

// CreateKinesisStreamTestResource creates a test resource of type AWSKinesisStreamResourceType with the given
// resourceName, provider, arn and typeSchema.
func CreateKinesisStreamTestResource(resourceName string) *AWSTestResource {
	resourceType := AWSKinesisStreamResourceType
	awsResourceType := AWSKinesisStreamAWSResourceType
	provider := "AWS.Kinesis"
	arn := fmt.Sprintf("arn:aws:kinesis:us-west-2:123456789012:stream:%s", resourceName)
	typeSchema := getMockKinesisStreamResourceTypeSchema()

	return CreateAWSTestResource(resourceType, awsResourceType, resourceName, provider, arn, typeSchema)
}

// CreateKinesisStreamTestResourceWithInvalidRegion creates a test resource with an invalid region for testing purposes.
func CreateKinesisStreamTestResourceWithInvalidRegion(resourceName string) *AWSTestResource {
	resourceType := AWSKinesisStreamResourceType
	awsResourceType := AWSKinesisStreamAWSResourceType
	provider := "AWS.Kinesis"
	arn := fmt.Sprintf("arn:aws:kinesis:us-west-2:123456789012:stream:%s", resourceName)
	typeSchema := getMockKinesisStreamResourceTypeSchema()

	return CreateAWSTestResourceWithInvalidRegion(resourceType, awsResourceType, resourceName, provider, arn, typeSchema)
}

// CreateMemoryDBClusterTestResource creates a test resource of type AWSMemoryDBClusterResourceType with the given
// resourceName, provider, arn, and typeSchema.
func CreateMemoryDBClusterTestResource(resourceName string) *AWSTestResource {
	resourceType := AWSMemoryDBClusterResourceType
	awsResourceType := AWSMemoryDBClusterAWSResourceType
	provider := "AWS.MemoryDB"
	arn := fmt.Sprintf("arn:aws:memorydb:us-west-2:123456789012:cluster:%s", resourceName)
	typeSchema := getMockMemoryDBClusterResourceTypeSchema()

	return CreateAWSTestResource(resourceType, awsResourceType, resourceName, provider, arn, typeSchema)
}

// CreateRedshiftEndpointAuthorizationTestResource creates a test resource for a Redshift Endpoint Authorization with
// the given resource name and returns a pointer to the AWSTestResource.
func CreateRedshiftEndpointAuthorizationTestResource(resourceName string) *AWSTestResource {
	resourceType := AWSRedShiftEndpointAuthorizationResourceType
	awsResourceType := AWSRedShiftEndpointAuthorizationAWSResourceType
	provider := "AWS.Redshift"
	arn := fmt.Sprintf("arn:aws:redshift:us-west-2:123456789012:endpointauthorization:%s", resourceName)
	typeSchema := getMockRedShiftEndpointAuthorizationResourceTypeSchema()

	return CreateAWSTestResource(resourceType, awsResourceType, resourceName, provider, arn, typeSchema)
}

// CreateAWSTestResource creates an AWSTestResource object with the given parameters and returns it. If an error occurs
// while marshalling the typeSchema, it returns nil.
func CreateAWSTestResource(resourceType, awsResourceType, resourceName, provider, arn string, typeSchema map[string]any) *AWSTestResource {
	serialized, err := json.Marshal(typeSchema)
	if err != nil {
		return nil
	}
	schema := string(serialized)

	return &AWSTestResource{
		ResourceType:          resourceType,
		AWSResourceType:       awsResourceType,
		ResourceName:          resourceName,
		ARN:                   arn,
		CollectionPath:        fmt.Sprintf("/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/%s", resourceType),
		SingleResourcePath:    fmt.Sprintf("/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/%s/%s", resourceType, resourceName),
		OperationResultsPath:  fmt.Sprintf("/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/%s/locations/us-west-2/operationResults/1234567", resourceType),
		OperationStatusesPath: fmt.Sprintf("/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/%s/locations/us-west-2/operationStatuses/2345678", resourceType),
		LocationHeader:        fmt.Sprintf("http://localhost:5000/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/%s/locations/global/operationResults/79b9f0da-4882-4dc8-a367-6fd3bc122ded", provider),
		AzureAsyncOpHeader:    fmt.Sprintf("http://localhost:5000/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/%s/locations/global/operationStatuses/79b9f0da-4882-4dc8-a367-6fd3bc122ded", provider),
		Schema:                schema,
	}
}

// CreateAWSTestResourceWithInvalidRegion creates an AWSTestResource object with invalid region information and returns it.
// It returns nil if an error occurs while marshalling the typeSchema.
func CreateAWSTestResourceWithInvalidRegion(resourceType, awsResourceType, resourceName, provider, arn string, typeSchema map[string]any) *AWSTestResource {
	serialized, err := json.Marshal(typeSchema)
	if err != nil {
		return nil
	}
	schema := string(serialized)

	return &AWSTestResource{
		ResourceType:          resourceType,
		AWSResourceType:       awsResourceType,
		ResourceName:          resourceName,
		ARN:                   arn,
		CollectionPath:        fmt.Sprintf("/planes/aws/aws/accounts/1234567/providers/%s", resourceType),
		SingleResourcePath:    fmt.Sprintf("/planes/aws/aws/accounts/1234567/providers/%s/%s", resourceType, resourceName),
		OperationResultsPath:  fmt.Sprintf("/planes/aws/aws/accounts/1234567/providers/%s/locations/us-west-2/operationResults/1234567", resourceType),
		OperationStatusesPath: fmt.Sprintf("/planes/aws/aws/accounts/1234567/providers/%s/locations/us-west-2/operationStatuses/2345678", resourceType),
		LocationHeader:        fmt.Sprintf("http://localhost:5000/planes/aws/aws/accounts/1234567/providers/%s/locations/global/operationResults/79b9f0da-4882-4dc8-a367-6fd3bc122ded", provider),
		AzureAsyncOpHeader:    fmt.Sprintf("http://localhost:5000/planes/aws/aws/accounts/1234567/providers/%s/locations/global/operationStatuses/79b9f0da-4882-4dc8-a367-6fd3bc122ded", provider),
		Schema:                schema,
	}
}

func getMockKinesisStreamResourceTypeSchema() map[string]any {
	return map[string]any{
		"primaryIdentifier": []any{
			"/properties/Name",
		},
		"readOnlyProperties": []any{
			"/properties/Arn",
		},
		"createOnlyProperties": []any{
			"/properties/Name",
		},
	}
}

func getMockMemoryDBClusterResourceTypeSchema() map[string]any {
	return map[string]any{
		"primaryIdentifier": []any{
			"/properties/ClusterName",
		},
		"readOnlyProperties": []any{
			"/properties/ClusterEndpoint/Address",
			"/properties/ClusterEndpoint/Port",
			"/properties/ARN",
		},
		"createOnlyProperties": []any{
			"/properties/ClusterName",
			"/properties/Port",
		},
	}
}

func getMockRedShiftEndpointAuthorizationResourceTypeSchema() map[string]any {
	return map[string]any{
		"primaryIdentifier": []any{
			"/properties/ClusterIdentifier",
			"/properties/Account",
		},
	}
}

// saveTestRecord saves a record for the resource id, owned by testOwner, after change edits it.
func saveTestRecord(t *testing.T, options TestOptions, id string, change func(rec *datamodel.AWSResource)) {
	t.Helper()
	parsed, err := resources.ParseResource(id)
	require.NoError(t, err)
	rec := records.NewRecord(parsed, testOwner)
	rec.SetProvisioningState("Succeeded")
	if change != nil {
		change(rec)
	}
	_, err = records.NewStore(options.DatabaseClient).Save(t.Context(), rec, "")
	require.NoError(t, err)
}

// getTestRecord returns the record for the resource id, or nil.
func getTestRecord(t *testing.T, options TestOptions, id string) *datamodel.AWSResource {
	t.Helper()
	parsed, err := resources.ParseResource(id)
	require.NoError(t, err)
	rec, _, err := records.NewStore(options.DatabaseClient).Get(t.Context(), parsed)
	require.NoError(t, err)
	return rec
}

// OperationTypeAWSResourceForTest is the async operation type the frontend queues AWS resource operations under.
const OperationTypeAWSResourceForTest = records.OperationType

func statusmanagerOptions() statusmanager.QueueOperationOptions {
	return statusmanager.QueueOperationOptions{OperationTimeout: time.Minute}
}
