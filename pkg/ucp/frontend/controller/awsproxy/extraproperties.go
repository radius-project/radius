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
	"encoding/json"
	"fmt"
	"reflect"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kafka"

	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/aws/extraproperties"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
)

// extraPropertyNames returns the extra properties of typeName that its CloudFormation schema does not declare. When
// AWS adds a property with the same name as an extra property, AWS's property wins: UCP neither adds the extra value
// on a read nor drops the property from a PUT.
func extraPropertyNames(typeName string, schema string) ([]string, error) {
	names := extraproperties.Names(typeName)
	if len(names) == 0 {
		return nil, nil
	}
	declared := struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}{}
	if err := json.Unmarshal([]byte(schema), &declared); err != nil {
		return nil, fmt.Errorf("failed to read the CloudFormation schema of %s: %w", typeName, err)
	}
	return slices.DeleteFunc(names, func(name string) bool {
		_, ok := declared.Properties[name]
		return ok
	}), nil
}

// addExtraProperties adds the extra read-only properties of typeName to properties, which Cloud Control returned for
// the resource. Only the MSK cluster types have them today: the broker addresses from kafka:GetBootstrapBrokers, each
// named after AWS's field. An address AWS leaves empty is left out, and so is a name the type's schema declares (see
// extraPropertyNames).
//
// A read whose extra call fails returns that failure, so UCP never leaves the values out silently. Only reads of one
// resource call it, never a list, and the values are never sent to AWS or stored (see extraproperties.Without).
func addExtraProperties(ctx context.Context, client ucp_aws.AWSKafkaClient, typeName string, region string, schema string, properties map[string]any) (armrpc_rest.Response, error) {
	names, err := extraPropertyNames(typeName, schema)
	if err != nil || len(names) == 0 {
		return nil, err
	}

	arn, _ := properties["Arn"].(string)
	if arn == "" {
		return nil, fmt.Errorf("cannot call kafka:GetBootstrapBrokers for the %s resource, because AWS Cloud Control returned no Arn for it", typeName)
	}
	out, err := client.GetBootstrapBrokers(ctx, &kafka.GetBootstrapBrokersInput{ClusterArn: aws.String(arn)}, func(o *kafka.Options) {
		o.Region = region
	})
	if err != nil {
		return getBootstrapBrokersError(ctx, arn, err)
	}

	for _, name := range names {
		if value := aws.ToString(stringField(out, name)); value != "" {
			properties[name] = value
		}
	}
	return nil, nil
}

// stringField returns the *string field name of out, or nil when out has no such field. Each extra property is named
// after the AWS field it comes from, so the list in extraproperties needs no separate mapping. The tests of that list
// check that every name is a field.
func stringField(out *kafka.GetBootstrapBrokersOutput, name string) *string {
	field := reflect.ValueOf(out).Elem().FieldByName(name)
	if !field.IsValid() || !field.CanInterface() {
		return nil
	}
	value, _ := field.Interface().(*string)
	return value
}

// getBootstrapBrokersError returns the response for a failed kafka:GetBootstrapBrokers call. It maps the error as
// HandleAWSError does for every AWS call: 400 for a client fault and 500 otherwise, with AWS's error code. It keeps
// AWS's message, such as the message for a cluster that is not active yet, and names the call, so a missing permission
// is clear.
func getBootstrapBrokersError(ctx context.Context, arn string, err error) (armrpc_rest.Response, error) {
	prefix := fmt.Sprintf("Reading the broker addresses of the MSK cluster %q with kafka:GetBootstrapBrokers failed", arn)
	resp, handleErr := ucp_aws.HandleAWSError(ctx, err)
	switch r := resp.(type) {
	case *armrpc_rest.BadRequestResponse:
		r.Body.Error.Message = prefix + ": " + r.Body.Error.Message
		return r, nil
	case *armrpc_rest.InternalServerErrorResponse:
		r.Body.Error.Message = prefix + ": " + r.Body.Error.Message
		return r, nil
	}
	return nil, fmt.Errorf("%s: %w", prefix, handleErr)
}
