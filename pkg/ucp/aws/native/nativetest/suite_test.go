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

package nativetest

import (
	"encoding/json"
	"testing"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativefake"
)

// Test_Suite_NativeType runs the suite for a fake handler of a type that has a CloudFormation type. Its identifier
// is its name, which UCP generates when the template leaves it out, and its read returns the password, as some service
// APIs return secrets.
func Test_Suite_NativeType(t *testing.T) {
	Run(t, Suite{
		TypeName: "AWS::Test::Cluster",
		Schema: `{
			"typeName": "AWS::Test::Cluster",
			"properties": {"Name": {"type": "string", "minLength": 1, "maxLength": 16, "pattern": "^[a-z][a-z0-9-]*$"}},
			"primaryIdentifier": ["/properties/Name"],
			"createOnlyProperties": ["/properties/Name", "/properties/Engine"],
			"writeOnlyProperties": ["/properties/MasterUserPassword"]
		}`,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			h := nativefake.New("AWS::Test::Cluster", store)
			h.Opts.List = native.ListRegion
			h.Opts.NameProperty = "Name"
			h.Waits = 2
			h.CreateOnly = []string{"Name", "Engine"}
			h.ReturnsWriteOnly = true
			return h
		},
		IdentifierProperty: "Name",
		WriteOnly:          []string{"MasterUserPassword"},
		Properties:         map[string]any{"Name": "app-cluster", "Engine": "docdb", "Size": 1, "Tier": "standard", "MasterUserPassword": "p@ss-one"},
		Identifier:         "app-cluster",
		Changed:            map[string]any{"Name": "app-cluster", "Engine": "docdb", "Size": 2, "Tier": "standard", "MasterUserPassword": "p@ss-one"},
		ChangedWriteOnly:   map[string]any{"Name": "app-cluster", "Engine": "docdb", "Size": 1, "Tier": "standard", "MasterUserPassword": "p@ss-two"},
		Removable:          "Tier",
		CreateOnlyChange:   map[string]any{"Name": "app-cluster", "Engine": "mongo", "Size": 1, "Tier": "standard", "MasterUserPassword": "p@ss-one"},
	})
}

// assignedSchema is the schema of a type whose identifier, Id, AWS assigns.
func assignedSchema(typeName string) string {
	return `{
		"typeName": "` + typeName + `",
		"primaryIdentifier": ["/properties/Id"],
		"readOnlyProperties": ["/properties/Id"],
		"writeOnlyProperties": ["/properties/SharedKey"]
	}`
}

// assignedSuite returns the suite for a fake handler of a type whose identifier AWS assigns, which finds a lost create
// as recovery says.
func assignedSuite(typeName string, recovery native.Recovery) Suite {
	return Suite{
		TypeName: typeName,
		Schema:   assignedSchema(typeName),
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			h := nativefake.New(typeName, store)
			h.Opts.Recovery = recovery
			h.Opts.List = native.ListRegion
			h.Waits = 1
			return h
		},
		IdentifierProperty: "Id",
		WriteOnly:          []string{"SharedKey"},
		Properties:         map[string]any{"Description": "vpn", "Size": 1, "SharedKey": "k3y-one"},
		Changed:            map[string]any{"Description": "vpn", "Size": 2, "SharedKey": "k3y-one"},
		ChangedWriteOnly:   map[string]any{"Description": "vpn", "Size": 1, "SharedKey": "k3y-two"},
		Removable:          "Description",
	}
}

// Test_Suite_Resend runs the suite for a type whose API takes an idempotency token: UCP finds a lost create by
// sending it again with the record's client token.
func Test_Suite_Resend(t *testing.T) {
	Run(t, assignedSuite("AWS::Test::Endpoint", native.RecoverByResend))
}

// Test_Suite_Search runs the suite for a type whose API takes no idempotency token: the handler tags the create with
// the record's client token and finds a lost create by that tag.
func Test_Suite_Search(t *testing.T) {
	Run(t, assignedSuite("AWS::Test::Permission", native.RecoverBySearch))
}

// Test_Suite_HandWrittenType runs the suite for a fake handler of a type with no CloudFormation type, whose
// identifier values contain "|" and "/", so it has its own identifier encoding.
func Test_Suite_HandWrittenType(t *testing.T) {
	identifierOf := func(properties map[string]any) (string, error) {
		bucket, _ := properties["Bucket"].(string)
		key, _ := properties["Key"].(string)
		if bucket == "" || key == "" {
			return "", nil
		}
		b, err := json.Marshal([]string{bucket, key})
		return string(b), err
	}
	properties := map[string]any{"Bucket": "app-assets", "Key": "config/a|b.json", "ContentType": "application/json", "Body": "e30="}
	identifier, err := identifierOf(properties)
	if err != nil {
		t.Fatal(err)
	}
	Run(t, Suite{
		TypeName:    "AWS::Test::Object",
		HandWritten: true,
		Schema: `{
			"typeName": "AWS::Test::Object",
			"primaryIdentifier": ["/properties/Bucket", "/properties/Key"],
			"createOnlyProperties": ["/properties/Bucket", "/properties/Key"],
			"writeOnlyProperties": ["/properties/Body"]
		}`,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			h := nativefake.New("AWS::Test::Object", store)
			h.IdentifierOf = identifierOf
			h.CreateOnly = []string{"Bucket", "Key"}
			return h.Encoding()
		},
		WriteOnly:        []string{"Body"},
		Properties:       properties,
		Identifier:       identifier,
		Changed:          map[string]any{"Bucket": "app-assets", "Key": "config/a|b.json", "ContentType": "text/plain", "Body": "e30="},
		ChangedWriteOnly: map[string]any{"Bucket": "app-assets", "Key": "config/a|b.json", "ContentType": "application/json", "Body": "eyJhIjoxfQ=="},
		Removable:        "ContentType",
		CreateOnlyChange: nil,
	})
}
