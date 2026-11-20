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

package opensearch

import (
	"maps"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

func readFile(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/handwritten/" + file)
	require.NoError(t, err)
	return string(b)
}

func with(properties map[string]any, changes map[string]any) map[string]any {
	out := maps.Clone(properties)
	maps.Copy(out, changes)
	return out
}

func list(values ...string) []any {
	return anyList(values)
}

// endpointTemplate returns a template for an endpoint to the domain logs in the subnets, with the security groups
// when there are any.
func endpointTemplate(subnets []any, groups []any) map[string]any {
	options := map[string]any{"SubnetIds": subnets}
	if groups != nil {
		options["SecurityGroupIds"] = groups
	}
	return map[string]any{"DomainArn": domainArn("logs"), "VpcOptions": options}
}

func Test_Suite_VpcEndpoint(t *testing.T) {
	properties := endpointTemplate(list("subnet-a", "subnet-b"), list("sg-web", "sg-db"))
	nativetest.Run(t, nativetest.Suite{
		TypeName:    vpcEndpointType,
		Schema:      readFile(t, "aws-opensearchservice-vpcendpoint.json"),
		Metadata:    readFile(t, "aws-opensearchservice-vpcendpoint.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			api := newFakeAPI(store)
			return NewVpcEndpoint(api, api)
		},
		IdentifierProperty: "VpcEndpointId",
		Properties:         properties,
		Changed:            endpointTemplate(list("subnet-b"), list("sg-web")),
		CreateOnlyChange:   with(properties, map[string]any{"DomainArn": domainArn("metrics")}),
		OtherForm: map[string]any{
			"VpcOptions": map[string]any{"SubnetIds": list("subnet-b", "subnet-a"), "SecurityGroupIds": list("sg-db", "sg-web")},
		},
		API: endpointAPI,
	})
}

func Test_Suite_VpcEndpointAccess(t *testing.T) {
	properties := map[string]any{"DomainName": "logs", "Account": otherAccount}
	nativetest.Run(t, nativetest.Suite{
		TypeName:    vpcEndpointAccessType,
		Schema:      readFile(t, "aws-opensearchservice-vpcendpointaccess.json"),
		Metadata:    readFile(t, "aws-opensearchservice-vpcendpointaccess.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return NewVpcEndpointAccess(newFakeAPI(store))
		},
		Properties: properties,
		Identifier: "logs|" + otherAccount,
		// Every property is create-only, so a deploy of the same template is the only update.
		Changed:          properties,
		CreateOnlyChange: with(properties, map[string]any{"Account": "333333333333"}),
		Invalid: []map[string]any{
			with(properties, map[string]any{"Service": "application.opensearchservice.amazonaws.com"}),
			with(properties, map[string]any{"DomainName": "Logs"}),
			with(properties, map[string]any{"Account": "acct-1"}),
			with(properties, map[string]any{"ServiceOptions": map[string]any{"SupportedRegions": list("us-east-1")}}),
			{"DomainName": "logs", "Service": "es.amazonaws.com"},
		},
		IdentifierProperties: func(identifier string) map[string]any {
			id, err := parseAccessID(identifier)
			require.NoError(t, err)
			return map[string]any{"DomainName": id.domain, "Account": id.principal}
		},
		API: accessAPI,
	})
}

// Test_Suite_VpcEndpointAccessService runs the suite for a service principal whose template sets ServiceOptions with
// no Regions, which must redeploy with no change, and takes the resource for `existing` by the schema's primary
// identifier, DomainName and Principal.
func Test_Suite_VpcEndpointAccessService(t *testing.T) {
	properties := map[string]any{"DomainName": "logs", "Service": service, "ServiceOptions": map[string]any{"SupportedRegions": []any{}}}
	nativetest.Run(t, nativetest.Suite{
		TypeName:    vpcEndpointAccessType,
		Schema:      readFile(t, "aws-opensearchservice-vpcendpointaccess.json"),
		Metadata:    readFile(t, "aws-opensearchservice-vpcendpointaccess.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return NewVpcEndpointAccess(newFakeAPI(store))
		},
		Properties:       properties,
		Identifier:       "logs|" + service,
		Changed:          properties,
		CreateOnlyChange: with(properties, map[string]any{"ServiceOptions": map[string]any{"SupportedRegions": list("us-east-1")}}),
		IdentifierProperties: func(identifier string) map[string]any {
			id, err := parseAccessID(identifier)
			require.NoError(t, err)
			return map[string]any{"DomainName": id.domain, "Principal": id.principal}
		},
		API: accessAPI,
	})
}

func Test_InRegion(t *testing.T) {
	opts := opensearch.Options{}
	inRegion("eu-west-2")(&opts)
	require.Equal(t, "eu-west-2", opts.Region)
}
