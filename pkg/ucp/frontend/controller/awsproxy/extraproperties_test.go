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
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	cctypes "github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/kafka"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/retry"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
)

const (
	mskType        = "AWS::MSK::Cluster"
	serverlessType = "AWS::MSK::ServerlessCluster"
	mskPath        = providers + "AWS.MSK/Cluster/app-kafka"
	serverlessPath = providers + "AWS.MSK/ServerlessCluster/app-kafka"
	mskARN         = "arn:aws:kafka:us-west-2:1234567:cluster/orders/abcd-1"
)

// mskSchemas are the parts of the MSK schemas the tests need, from CloudFormation in us-west-2. Cloud Control returns
// only Arn and CurrentVersion as read-only properties.
var mskSchemas = map[string]string{
	mskType: `{
		"primaryIdentifier": ["/properties/Arn"],
		"readOnlyProperties": ["/properties/Arn", "/properties/CurrentVersion"],
		"createOnlyProperties": ["/properties/BrokerNodeGroupInfo/BrokerAZDistribution", "/properties/BrokerNodeGroupInfo/ClientSubnets", "/properties/BrokerNodeGroupInfo/SecurityGroups", "/properties/EncryptionInfo/EncryptionAtRest", "/properties/EncryptionInfo/EncryptionInTransit/InCluster", "/properties/ClusterName"]
	}`,
	serverlessType: `{
		"primaryIdentifier": ["/properties/Arn"],
		"readOnlyProperties": ["/properties/Arn"],
		"createOnlyProperties": ["/properties/ClusterName", "/properties/VpcConfigs", "/properties/ClientAuthentication", "/properties/Tags"]
	}`,
}

// mskProvisioningTypes are the provisioning types DescribeType returns: AWS can only create and delete a serverless
// cluster.
var mskProvisioningTypes = map[string]cftypes.ProvisioningType{serverlessType: cftypes.ProvisioningTypeImmutable}

// clusterTemplate returns the properties a recipe sets for an MSK cluster of typeName, with no security groups.
func clusterTemplate(typeName string) map[string]any {
	if typeName == serverlessType {
		return map[string]any{
			"ClusterName":          "orders",
			"VpcConfigs":           []any{map[string]any{"SubnetIds": []any{"subnet-a", "subnet-b"}}},
			"ClientAuthentication": map[string]any{"Sasl": map[string]any{"Iam": map[string]any{"Enabled": true}}},
		}
	}
	return map[string]any{
		"ClusterName":         "orders",
		"KafkaVersion":        "3.6.0",
		"NumberOfBrokerNodes": 2.0,
		"BrokerNodeGroupInfo": map[string]any{"ClientSubnets": []any{"subnet-a", "subnet-b"}, "InstanceType": "kafka.m5.large"},
	}
}

// clusterInAWS returns what Cloud Control's GetResource returns for clusterTemplate(typeName). MSK uses the VPC's
// default security group when the template sets none. For a serverless cluster, this is the shape of a live read in
// us-west-2: the default security group inside the VpcConfigs item, and empty Tags.
func clusterInAWS(typeName string) map[string]any {
	out := clusterTemplate(typeName)
	out["Arn"] = mskARN
	if typeName == serverlessType {
		out["VpcConfigs"].([]any)[0].(map[string]any)["SecurityGroups"] = []any{"sg-default"}
		out["Tags"] = map[string]any{}
		return out
	}
	out["CurrentVersion"] = "K1"
	out["BrokerNodeGroupInfo"].(map[string]any)["SecurityGroups"] = []any{"sg-default"}
	return out
}

// succeeded returns properties with the provisioning state of a PUT that finished.
func succeeded(properties map[string]any) map[string]any {
	properties["provisioningState"] = "Succeeded"
	return properties
}

// withAddresses returns properties with the addresses brokers() returns for typeName.
func withAddresses(typeName string, properties map[string]any) map[string]any {
	properties["BootstrapBrokerStringSaslIam"] = "b-1.orders.kafka.us-west-2.amazonaws.com:9098"
	properties["BootstrapBrokerStringSaslIamIpv6"] = "b-1.orders.ipv6.kafka.us-west-2.amazonaws.com:9098"
	if typeName == mskType {
		properties["BootstrapBrokerStringTls"] = "b-1.orders.kafka.us-west-2.amazonaws.com:9094"
	}
	return properties
}

// fakeKafka returns out or err from GetBootstrapBrokers, and records each cluster ARN and region it was called with.
type fakeKafka struct {
	out     *kafka.GetBootstrapBrokersOutput
	err     error
	arns    []string
	regions []string
}

func (f *fakeKafka) GetBootstrapBrokers(ctx context.Context, params *kafka.GetBootstrapBrokersInput, optFns ...func(*kafka.Options)) (*kafka.GetBootstrapBrokersOutput, error) {
	options := kafka.Options{}
	for _, fn := range optFns {
		fn(&options)
	}
	f.arns = append(f.arns, aws.ToString(params.ClusterArn))
	f.regions = append(f.regions, options.Region)
	return f.out, f.err
}

// clashSchemas give AWS::MSK::Cluster a property named BootstrapBrokerStringTls, as if AWS added one with the same
// name as an extra property.
var clashSchemas = map[string]string{
	mskType: `{
		"primaryIdentifier": ["/properties/Arn"],
		"properties": {"Arn": {"type": "string"}, "ClusterName": {"type": "string"}, "BootstrapBrokerStringTls": {"type": "string"}},
		"readOnlyProperties": ["/properties/Arn"],
		"createOnlyProperties": ["/properties/ClusterName"]
	}`,
}

// withMSK runs factory with client as the Kafka client and with the MSK schemas.
func withMSK(client ucp_aws.AWSKafkaClient, factory controllerFactory) controllerFactory {
	return withSchemas(mskSchemas, client, factory)
}

// withSchemas runs factory with client as the Kafka client and with schemas.
func withSchemas(schemas map[string]string, client ucp_aws.AWSKafkaClient, factory controllerFactory) controllerFactory {
	return func(opts armrpc_controller.Options, clients ucp_aws.Clients) (armrpc_controller.Controller, error) {
		clients.Kafka = client
		clients.CloudFormation = &awsfake.CloudFormation{Schemas: schemas, ProvisioningTypes: mskProvisioningTypes}
		return factory(opts, clients)
	}
}

func newGetWithPost(opts armrpc_controller.Options, clients ucp_aws.Clients) (armrpc_controller.Controller, error) {
	return NewGetAWSResourceWithPost(opts, clients, retry.NewNoOpRetryer())
}

// brokers is what GetBootstrapBrokers returns in most tests: three addresses, one empty field, and fields left unset.
func brokers() *kafka.GetBootstrapBrokersOutput {
	return &kafka.GetBootstrapBrokersOutput{
		BootstrapBrokerStringSaslIam:     aws.String("b-1.orders.kafka.us-west-2.amazonaws.com:9098"),
		BootstrapBrokerStringSaslIamIpv6: aws.String("b-1.orders.ipv6.kafka.us-west-2.amazonaws.com:9098"),
		BootstrapBrokerStringTls:         aws.String("b-1.orders.kafka.us-west-2.amazonaws.com:9094"),
		BootstrapBrokerString:            aws.String(""),
	}
}

// seedCluster adds an MSK cluster to AWS, as AWS reports it, and a record of its deploy under path.
func seedCluster(h *harness, typeName string, path string) {
	h.saveRecord(path, func(rec *datamodel.AWSResource) {
		rec.Properties.AWSIdentifier = mskARN
		rec.Properties.LastApplied = clusterTemplate(typeName)
	})
	h.cc.Put(typeName, mskARN, clusterInAWS(typeName))
}

func Test_ExtraProperties_Read(t *testing.T) {
	want := withAddresses(mskType, clusterInAWS(mskType))

	t.Run("GET adds the broker addresses AWS returns and leaves out empty ones", func(t *testing.T) {
		h := newHarness(t)
		seedCluster(h, mskType, mskPath)
		k := &fakeKafka{out: brokers()}

		resp := h.do(withMSK(k, NewGetAWSResource), http.MethodGet, mskPath, "", nil)
		require.Equal(t, http.StatusOK, resp.status, resp.raw)
		require.Equal(t, want, resp.body["properties"])
		require.Equal(t, []string{mskARN}, k.arns)
		require.Equal(t, []string{"us-west-2"}, k.regions)
	})

	t.Run("POST /:get adds the broker addresses", func(t *testing.T) {
		h := newHarness(t)
		h.cc.Put(mskType, mskARN, clusterInAWS(mskType))
		k := &fakeKafka{out: brokers()}

		resp := h.do(withMSK(k, newGetWithPost), http.MethodPost, providers+"AWS.MSK/Cluster/:get", "", map[string]any{"name": mskARN})
		require.Equal(t, http.StatusOK, resp.status, resp.raw)
		require.Equal(t, want, resp.body["properties"])
		require.Equal(t, []string{"us-west-2"}, k.regions)
	})

	t.Run("a serverless cluster gets only its own list", func(t *testing.T) {
		h := newHarness(t)
		seedCluster(h, serverlessType, serverlessPath)
		k := &fakeKafka{out: brokers()}

		resp := h.do(withMSK(k, NewGetAWSResource), http.MethodGet, serverlessPath, "", nil)
		require.Equal(t, http.StatusOK, resp.status, resp.raw)
		require.Equal(t, withAddresses(serverlessType, clusterInAWS(serverlessType)), resp.body["properties"])
	})

	t.Run("other types make no extra call", func(t *testing.T) {
		h := newHarness(t)
		h.saveRecord(bktPath, func(rec *datamodel.AWSResource) { rec.Properties.AWSIdentifier = "assets" })
		h.cc.Put(bucketType, "assets", map[string]any{"BucketName": "assets"})
		k := &fakeKafka{err: errors.New("must not be called")}

		resp := h.do(withMSK(k, NewGetAWSResource), http.MethodGet, bktPath, "", nil)
		require.Equal(t, http.StatusOK, resp.status, resp.raw)
		require.Empty(t, k.arns)
	})

	t.Run("a cluster AWS does not have returns 404 without the extra call", func(t *testing.T) {
		h := newHarness(t)
		h.saveRecord(mskPath, func(rec *datamodel.AWSResource) { rec.Properties.AWSIdentifier = mskARN })
		k := &fakeKafka{out: brokers()}

		resp := h.do(withMSK(k, NewGetAWSResource), http.MethodGet, mskPath, "", nil)
		require.Equal(t, http.StatusNotFound, resp.status, resp.raw)
		require.Empty(t, k.arns)
	})
}

func Test_ExtraProperties_ReadFails(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		status  int
		code    string
		message string
	}{
		{
			name:    "missing permission",
			err:     awsError(&smithy.GenericAPIError{Code: "ForbiddenException", Message: "not authorized to perform kafka:GetBootstrapBrokers", Fault: smithy.FaultClient}),
			status:  http.StatusBadRequest,
			code:    "ForbiddenException",
			message: "not authorized to perform kafka:GetBootstrapBrokers",
		},
		{
			name:    "cluster not active yet",
			err:     awsError(&smithy.GenericAPIError{Code: "BadRequestException", Message: "The cluster is in CREATING state.", Fault: smithy.FaultClient}),
			status:  http.StatusBadRequest,
			code:    "BadRequestException",
			message: "The cluster is in CREATING state.",
		},
		{
			name:    "AWS server error",
			err:     awsError(&smithy.GenericAPIError{Code: "InternalServerErrorException", Message: "try again", Fault: smithy.FaultServer}),
			status:  http.StatusInternalServerError,
			code:    "InternalServerErrorException",
			message: "try again",
		},
		{
			name:    "not an AWS API error",
			err:     errors.New("connection reset"),
			status:  http.StatusInternalServerError,
			message: "connection reset",
		},
	}
	for _, tt := range tests {
		for _, read := range []struct {
			name    string
			factory controllerFactory
			method  string
			path    string
			body    any
		}{
			{name: "GET", factory: NewGetAWSResource, method: http.MethodGet, path: mskPath},
			{name: "POST /:get", factory: newGetWithPost, method: http.MethodPost, path: providers + "AWS.MSK/Cluster/:get", body: map[string]any{"name": mskARN}},
		} {
			t.Run(tt.name+", "+read.name, func(t *testing.T) {
				h := newHarness(t)
				seedCluster(h, mskType, mskPath)

				resp := h.do(withMSK(&fakeKafka{err: tt.err}, read.factory), read.method, read.path, "", read.body)
				require.Equal(t, tt.status, resp.status, resp.raw)
				require.Contains(t, resp.message(), "kafka:GetBootstrapBrokers")
				require.Contains(t, resp.message(), mskARN)
				require.Contains(t, resp.message(), tt.message)
				if tt.code != "" {
					require.Equal(t, tt.code, resp.body["error"].(map[string]any)["code"])
				}
				require.NotContains(t, resp.raw, "BootstrapBrokerString")
			})
		}
	}

	t.Run("Cloud Control returns no Arn", func(t *testing.T) {
		h := newHarness(t)
		h.saveRecord(mskPath, func(rec *datamodel.AWSResource) { rec.Properties.AWSIdentifier = mskARN })
		h.cc.Put(mskType, mskARN, map[string]any{"ClusterName": "orders"})
		k := &fakeKafka{out: brokers()}

		resp := h.do(withMSK(k, NewGetAWSResource), http.MethodGet, mskPath, "", nil)
		require.Equal(t, http.StatusInternalServerError, resp.status, resp.raw)
		require.Contains(t, resp.message(), "kafka:GetBootstrapBrokers")
		require.Contains(t, resp.message(), "no Arn")
		require.Empty(t, k.arns)
	})
}

func Test_ExtraProperties_NeverSentOrStored(t *testing.T) {
	// The template sets extra properties, as if a caller sent back a body UCP returned, with stale values.
	extras := map[string]any{
		"BootstrapBrokerStringSaslIam": "b-1.stale.kafka.us-west-2.amazonaws.com:9098",
		"BootstrapBrokerString":        "stale",
	}
	withExtras := func(properties map[string]any) map[string]any {
		for k, v := range extras {
			properties[k] = v
		}
		return properties
	}
	put := func(h *harness, k *fakeKafka, properties map[string]any) response {
		return h.do(withMSK(k, h.putController), http.MethodPut, mskPath, testOwner, map[string]any{"properties": properties})
	}
	requireNoExtras := func(t *testing.T, document string, properties map[string]any) {
		require.NotContains(t, document, "BootstrapBrokerString")
		for name := range extras {
			require.NotContains(t, properties, name)
		}
	}

	t.Run("a redeploy with nothing changed calls nothing in AWS but the reads", func(t *testing.T) {
		h := newHarness(t)
		seedCluster(h, mskType, mskPath)
		k := &fakeKafka{out: brokers()}

		resp := put(h, k, withExtras(clusterTemplate(mskType)))
		require.Equal(t, http.StatusOK, resp.status, resp.raw)
		require.Equal(t, []string{"GetResource"}, operations(h.cc.Calls()))
		require.Equal(t, clusterTemplate(mskType), h.record(mskPath).Properties.LastApplied)
		require.Equal(t, succeeded(withAddresses(mskType, clusterInAWS(mskType))), resp.body["properties"], "the response has AWS's addresses, not the template's")
	})

	t.Run("an update sends only the changed property", func(t *testing.T) {
		h := newHarness(t)
		seedCluster(h, mskType, mskPath)
		k := &fakeKafka{out: brokers()}

		template := clusterTemplate(mskType)
		template["KafkaVersion"] = "3.7.0"
		resp := put(h, k, withExtras(template))
		require.Equal(t, http.StatusCreated, resp.status, resp.raw)
		calls := h.cc.Calls()
		require.Equal(t, []string{"GetResource", "UpdateResource"}, operations(calls))
		require.Contains(t, calls[1].Document, "3.7.0")
		rec := h.record(mskPath)
		requireNoExtras(t, calls[1].Document, rec.Properties.PendingLastApplied)
		require.Empty(t, k.arns, "a cluster being updated isn't read; the Deployment Engine reads it with GET when the update finishes")

		h.finish()
		requireNoExtras(t, "", h.record(mskPath).Properties.LastApplied)
	})

	t.Run("a create never sends them", func(t *testing.T) {
		h := newHarness(t)
		k := &fakeKafka{out: brokers()}

		resp := put(h, k, withExtras(clusterTemplate(mskType)))
		require.Equal(t, http.StatusCreated, resp.status, resp.raw)
		calls := h.cc.Calls()
		require.Equal(t, []string{"CreateResource"}, operations(calls))
		requireNoExtras(t, calls[0].Document, h.record(mskPath).Properties.PendingLastApplied)
		require.Empty(t, k.arns, "a cluster being created isn't active yet")
	})
}

// Test_ExtraProperties_PutResponse checks that a PUT that finds nothing to change returns the extra properties, as a
// GET does, because the Deployment Engine uses that response for the recipe's outputs.
func Test_ExtraProperties_PutResponse(t *testing.T) {
	put := func(h *harness, k *fakeKafka, path string, body map[string]any) response {
		return h.do(withMSK(k, h.putController), http.MethodPut, path, testOwner, body)
	}

	t.Run("a serverless cluster redeployed with nothing changed", func(t *testing.T) {
		h := newHarness(t)
		// Without the fix for #13763, a template must set the default security group AWS fills in, or the redeploy
		// fails because AWS can't change VpcConfigs.
		template := clusterInAWS(serverlessType)
		delete(template, "Arn")
		delete(template, "Tags")
		h.saveRecord(serverlessPath, func(rec *datamodel.AWSResource) {
			rec.Properties.AWSIdentifier = mskARN
			rec.Properties.LastApplied = template
		})
		h.cc.Put(serverlessType, mskARN, clusterInAWS(serverlessType))
		k := &fakeKafka{out: brokers()}

		resp := put(h, k, serverlessPath, map[string]any{"properties": template})
		require.Equal(t, http.StatusOK, resp.status, resp.raw)
		require.Equal(t, []string{"GetResource"}, operations(h.cc.Calls()))
		require.Equal(t, succeeded(withAddresses(serverlessType, clusterInAWS(serverlessType))), resp.body["properties"])
		require.Equal(t, []string{mskARN}, k.arns)
		require.Equal(t, []string{"us-west-2"}, k.regions)
	})

	t.Run("a take-over by name with nothing to change", func(t *testing.T) {
		h := newHarness(t)
		h.cc.Put(mskType, mskARN, clusterInAWS(mskType))
		k := &fakeKafka{out: brokers()}

		resp := put(h, k, mskPath, map[string]any{"name": mskARN, "properties": clusterTemplate(mskType)})
		require.Equal(t, http.StatusOK, resp.status, resp.raw)
		require.Equal(t, []string{"GetResource"}, operations(h.cc.Calls()))
		require.Equal(t, succeeded(withAddresses(mskType, clusterInAWS(mskType))), resp.body["properties"])
		require.Equal(t, mskARN, h.record(mskPath).Properties.AWSIdentifier)
	})

	t.Run("a failed read fails the PUT and leaves the record as it was", func(t *testing.T) {
		h := newHarness(t)
		seedCluster(h, mskType, mskPath)
		// The last deploy set less than this template, so a successful PUT would save the template on the record.
		h.saveRecord(mskPath, func(rec *datamodel.AWSResource) {
			rec.Properties.LastApplied = map[string]any{"ClusterName": "orders"}
		})
		k := &fakeKafka{err: awsError(&smithy.GenericAPIError{Code: "ForbiddenException", Message: "not authorized to perform kafka:GetBootstrapBrokers", Fault: smithy.FaultClient})}

		resp := put(h, k, mskPath, map[string]any{"properties": clusterTemplate(mskType)})
		require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
		require.Contains(t, resp.message(), "kafka:GetBootstrapBrokers")
		require.Contains(t, resp.message(), mskARN)
		require.Equal(t, []string{"GetResource"}, operations(h.cc.Calls()))
		require.Equal(t, map[string]any{"ClusterName": "orders"}, h.record(mskPath).Properties.LastApplied)
	})
}

func Test_ExtraProperties_ListMakesNoExtraCall(t *testing.T) {
	testOptions := setupTest(t)
	// The Kafka mock has no expected calls, so any call fails the test.
	testOptions.AWSCloudControlClient.EXPECT().ListResources(gomock.Any(), gomock.Any(), gomock.Any()).Return(&cloudcontrol.ListResourcesOutput{
		ResourceDescriptions: []cctypes.ResourceDescription{{Identifier: aws.String(mskARN), Properties: aws.String(`{"Arn": "` + mskARN + `"}`)}},
	}, nil)
	controller, err := NewListAWSResources(testOptions.ControllerOptions(), testOptions.Clients())
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodGet, providers+"AWS.MSK/Cluster", nil)
	require.NoError(t, err)
	resp, err := controller.Run(rpctest.NewARMRequestContext(req), nil, req)
	require.NoError(t, err)
	require.NotNil(t, resp)
}

func Test_StringField(t *testing.T) {
	out := &kafka.GetBootstrapBrokersOutput{BootstrapBrokerStringTls: aws.String("b-1:9094")}

	require.Equal(t, "b-1:9094", aws.ToString(stringField(out, "BootstrapBrokerStringTls")))
	require.Nil(t, stringField(out, "BootstrapBrokerString"))
	for _, name := range []string{"NoSuchField", "ResultMetadata", "noSmithyDocumentSerde"} {
		require.Nil(t, stringField(out, name), name)
	}
}

func Test_ExtraProperties_NameClash(t *testing.T) {
	ccProperties := map[string]any{"Arn": mskARN, "ClusterName": "orders", "BootstrapBrokerStringTls": "from-cloud-control"}

	for _, read := range []struct {
		name    string
		factory controllerFactory
		method  string
		path    string
		body    any
	}{
		{name: "GET", factory: NewGetAWSResource, method: http.MethodGet, path: mskPath},
		{name: "POST /:get", factory: newGetWithPost, method: http.MethodPost, path: providers + "AWS.MSK/Cluster/:get", body: map[string]any{"name": mskARN}},
	} {
		t.Run(read.name+" keeps the value Cloud Control returns for a name the schema declares", func(t *testing.T) {
			h := newHarness(t)
			h.saveRecord(mskPath, func(rec *datamodel.AWSResource) { rec.Properties.AWSIdentifier = mskARN })
			h.cc.Put(mskType, mskARN, ccProperties)
			k := &fakeKafka{out: brokers()}

			resp := h.do(withSchemas(clashSchemas, k, read.factory), read.method, read.path, "", read.body)
			require.Equal(t, http.StatusOK, resp.status, resp.raw)
			properties := resp.body["properties"].(map[string]any)
			require.Equal(t, "from-cloud-control", properties["BootstrapBrokerStringTls"])
			require.Equal(t, "b-1.orders.kafka.us-west-2.amazonaws.com:9098", properties["BootstrapBrokerStringSaslIam"])
		})
	}

	t.Run("a read makes no extra call when the schema declares every name", func(t *testing.T) {
		h := newHarness(t)
		seedCluster(h, serverlessType, serverlessPath)
		schemas := map[string]string{serverlessType: `{
			"primaryIdentifier": ["/properties/Arn"],
			"properties": {"BootstrapBrokerStringSaslIam": {"type": "string"}, "BootstrapBrokerStringSaslIamIpv6": {"type": "string"}}
		}`}
		k := &fakeKafka{err: errors.New("must not be called")}

		resp := h.do(withSchemas(schemas, k, NewGetAWSResource), http.MethodGet, serverlessPath, "", nil)
		require.Equal(t, http.StatusOK, resp.status, resp.raw)
		require.Empty(t, k.arns)
	})

	t.Run("PUT sends a declared name to AWS and still drops the others", func(t *testing.T) {
		h := newHarness(t)
		k := &fakeKafka{}

		resp := h.do(withSchemas(clashSchemas, k, h.putController), http.MethodPut, mskPath, testOwner, map[string]any{"properties": map[string]any{
			"ClusterName":                  "orders",
			"BootstrapBrokerStringTls":     "set-by-template",
			"BootstrapBrokerStringSaslIam": "stale",
		}})
		require.Equal(t, http.StatusCreated, resp.status, resp.raw)
		calls := h.cc.Calls()
		require.Equal(t, []string{"CreateResource"}, operations(calls))
		require.Contains(t, calls[0].Document, `"BootstrapBrokerStringTls":"set-by-template"`)
		require.NotContains(t, calls[0].Document, "BootstrapBrokerStringSaslIam")
		require.Equal(t, map[string]any{"ClusterName": "orders", "BootstrapBrokerStringTls": "set-by-template"}, h.record(mskPath).Properties.PendingLastApplied)
	})
}

func Test_ExtraProperties_SchemaFailures(t *testing.T) {
	t.Run("GET fails when DescribeType fails", func(t *testing.T) {
		h := newHarness(t)
		seedCluster(h, mskType, mskPath)
		k := &fakeKafka{out: brokers()}

		// The fake has no schema for the type, so DescribeType fails.
		resp := h.do(withSchemas(map[string]string{}, k, NewGetAWSResource), http.MethodGet, mskPath, "", nil)
		require.Equal(t, http.StatusInternalServerError, resp.status, resp.raw)
		require.Empty(t, k.arns)
	})

	t.Run("PUT fails, before calling AWS, when the schema is not JSON", func(t *testing.T) {
		h := newHarness(t)
		resp := h.do(withSchemas(map[string]string{mskType: "not json"}, &fakeKafka{}, h.putController), http.MethodPut, mskPath, testOwner, map[string]any{"properties": map[string]any{"ClusterName": "orders"}})
		require.Equal(t, http.StatusInternalServerError, resp.status, resp.raw)
		require.Contains(t, resp.message(), mskType)
		require.Empty(t, h.cc.Calls())
		require.Nil(t, h.record(mskPath))
	})

	t.Run("a schema that is not JSON", func(t *testing.T) {
		_, err := extraPropertyNames(mskType, "not json")
		require.ErrorContains(t, err, mskType)

		names, err := extraPropertyNames(bucketType, "not json")
		require.NoError(t, err)
		require.Nil(t, names)
	})
}
