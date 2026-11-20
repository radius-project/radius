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

package redshiftserverless

import (
	"maps"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/redshiftserverless"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const region = "us-west-2"

func readFile(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/handwritten/" + file)
	require.NoError(t, err)
	return string(b)
}

// fakes maps each handler the suite makes to its fake API.
type fakes struct {
	mu  sync.Mutex
	api map[native.Handler]*fakeAPI
}

func (f *fakes) add(h native.Handler, api *fakeAPI) native.Handler {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.api == nil {
		f.api = map[native.Handler]*fakeAPI{}
	}
	f.api[h] = api
	return h
}

func with(properties map[string]any, changes map[string]any) map[string]any {
	out := maps.Clone(properties)
	for k, v := range changes {
		if v == nil {
			delete(out, k)
		} else {
			out[k] = v
		}
	}
	return out
}

// newAPI returns a fake API backed by a new store that has the fake's workgroups.
func newAPI() *fakeAPI {
	store := awsfake.NewCloudControl()
	addWorkgroups(store)
	return newFakeAPI(store)
}

var limit = map[string]any{
	"ResourceArn":  workgroupArn("analytics"),
	"UsageType":    "serverless-compute",
	"Amount":       float64(10),
	"Period":       "daily",
	"BreachAction": "emit-metric",
}

func Test_Suite_UsageLimit(t *testing.T) {
	var fs fakes
	nativetest.Run(t, nativetest.Suite{
		TypeName:    usageLimitType,
		Schema:      readFile(t, "aws-redshiftserverless-usagelimit.json"),
		Metadata:    readFile(t, "aws-redshiftserverless-usagelimit.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			addWorkgroups(store)
			api := newFakeAPI(store)
			return fs.add(NewUsageLimit(api), api)
		},
		IdentifierProperty: "UsageLimitId",
		Properties:         limit,
		Changed:            with(limit, map[string]any{"Amount": float64(20)}),
		CreateOnlyChange:   with(limit, map[string]any{"Period": "weekly"}),
		Duplicates:         true,
		Invalid: []map[string]any{
			with(limit, map[string]any{"Amount": float64(0)}),
			with(limit, map[string]any{"Amount": 1.5}),
			with(limit, map[string]any{"Amount": "10"}),
			with(limit, map[string]any{"UsageType": "compute"}),
			with(limit, map[string]any{"Period": "yearly"}),
			with(limit, map[string]any{"BreachAction": 5}),
		},
		API: usageLimitAPI,
	})
}

var endpoint = map[string]any{
	"EndpointName":        "reads",
	"WorkgroupName":       sharedWorkgroup,
	"SubnetIds":           []any{"subnet-1", "subnet-2"},
	"VpcSecurityGroupIds": []any{"sg-1"},
	"OwnerAccount":        otherAccount,
}

func Test_Suite_EndpointAccess(t *testing.T) {
	var fs fakes
	nativetest.Run(t, nativetest.Suite{
		TypeName:    endpointAccessType,
		Schema:      readFile(t, "aws-redshiftserverless-endpointaccess.json"),
		Metadata:    readFile(t, "aws-redshiftserverless-endpointaccess.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			addWorkgroups(store)
			api := newFakeAPI(store)
			return fs.add(NewEndpointAccess(api), api)
		},
		IdentifierProperty: "EndpointName",
		WriteOnly:          []string{"OwnerAccount"},
		Properties:         endpoint,
		Identifier:         "reads",
		Changed:            with(endpoint, map[string]any{"VpcSecurityGroupIds": []any{"sg-2", "sg-3"}}),
		CreateOnlyChange:   with(endpoint, map[string]any{"SubnetIds": []any{"subnet-3"}}),
		OtherForm:          map[string]any{"SubnetIds": []any{"subnet-2", "subnet-1"}},
		Invalid: []map[string]any{
			with(endpoint, map[string]any{"EndpointName": "Reads"}),
			with(endpoint, map[string]any{"EndpointName": "my--reads"}),
			with(endpoint, map[string]any{"EndpointName": "reads-"}),
			with(endpoint, map[string]any{"EndpointName": strings.Repeat("a", 31)}),
			with(endpoint, map[string]any{"OwnerAccount": "owner"}),
			with(endpoint, map[string]any{"SubnetIds": []any{"subnet-1", 2}}),
			with(endpoint, map[string]any{"WorkgroupName": 5}),
		},
		API: endpointAPI,
	})
}

var domain = map[string]any{
	"WorkgroupName":              "analytics",
	"CustomDomainName":           "db.example.com",
	"CustomDomainCertificateArn": testCertificate,
}

func Test_Suite_CustomDomainAssociation(t *testing.T) {
	var fs fakes
	nativetest.Run(t, nativetest.Suite{
		TypeName:    customDomainType,
		Schema:      readFile(t, "aws-redshiftserverless-customdomainassociation.json"),
		Metadata:    readFile(t, "aws-redshiftserverless-customdomainassociation.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			addWorkgroups(store)
			api := newFakeAPI(store)
			return fs.add(NewCustomDomainAssociation(api), api)
		},
		Properties:       domain,
		Identifier:       "analytics|db.example.com",
		Changed:          with(domain, map[string]any{"CustomDomainCertificateArn": certificates[1]}),
		CreateOnlyChange: with(domain, map[string]any{"WorkgroupName": "reports"}),
		IdentifierProperties: func(identifier string) map[string]any {
			workgroup, name, _ := strings.Cut(identifier, "|")
			return map[string]any{"WorkgroupName": workgroup, "CustomDomainName": name}
		},
		Invalid: []map[string]any{
			with(domain, map[string]any{"CustomDomainName": "DB.example.com"}),
			with(domain, map[string]any{"CustomDomainName": "-db.example.com"}),
			with(domain, map[string]any{"WorkgroupName": "Analytics"}),
			with(domain, map[string]any{"CustomDomainCertificateArn": 5}),
		},
		API: customDomainAPI,
	})
}

func Test_InRegion(t *testing.T) {
	opts := redshiftserverless.Options{}
	inRegion("eu-west-2")(&opts)
	require.Equal(t, "eu-west-2", opts.Region)
}
