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
	"fmt"
	"net/url"
	"testing"

	"github.com/radius-project/radius/pkg/aws/schemas"
	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativefake"
)

// Test_Suite_NativeType runs the suite for a fake handler of a type that has a CloudFormation type. Its identifier
// is its name, which UCP generates when the template leaves it out, and its read returns the password, as some service
// APIs return secrets. Its API also serves another product, its update makes one call per change, and AWS returns
// EngineVersion and Tier in another form: Tier by the schema's propertyTransform, and EngineVersion by the handler's
// rule.
func Test_Suite_NativeType(t *testing.T) {
	cluster := func(changes map[string]any) map[string]any {
		properties := map[string]any{"Name": "app-cluster", "Engine": "docdb", "EngineVersion": "5.0", "Size": 1, "Tier": "standard", "MasterUserPassword": "p@ss-one"}
		for k, v := range changes {
			if v == nil {
				delete(properties, k)
			} else {
				properties[k] = v
			}
		}
		return properties
	}
	Run(t, Suite{
		TypeName: "AWS::Test::Cluster",
		Schema: `{
			"typeName": "AWS::Test::Cluster",
			"properties": {"Name": {"type": "string", "minLength": 1, "maxLength": 16, "pattern": "^[a-z][a-z0-9-]*$"}},
			"primaryIdentifier": ["/properties/Name"],
			"createOnlyProperties": ["/properties/Name", "/properties/Engine"],
			"writeOnlyProperties": ["/properties/MasterUserPassword"],
			"propertyTransform": {"/properties/Tier": "$uppercase(Tier)"}
		}`,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			h := nativefake.New("AWS::Test::Cluster", store)
			h.Opts.List = native.ListRegion
			h.Opts.NameProperty = "Name"
			h.Opts.Equivalent = func(path string, template any, aws any) bool {
				return path == "EngineVersion" && aws == fmt.Sprint(template)+".0"
			}
			h.Waits = 2
			h.CreateOnly = []string{"Name", "Engine"}
			h.ReturnsWriteOnly = true
			h.ProductProperty, h.Product = "Engine", "docdb"
			return h
		},
		IdentifierProperty: "Name",
		WriteOnly:          []string{"MasterUserPassword"},
		Properties:         cluster(nil),
		Identifier:         "app-cluster",
		Changed:            cluster(map[string]any{"Size": 2}),
		ChangedWriteOnly:   cluster(map[string]any{"MasterUserPassword": "p@ss-two"}),
		Removable:          "Tier",
		CreateOnlyChange:   cluster(map[string]any{"Engine": "mongo"}),
		OtherForm:          map[string]any{"EngineVersion": "5.0.0", "Tier": "STANDARD"},
		OtherProduct: func(store *awsfake.CloudControl, identifier string) {
			store.Put("AWS::Test::Cluster", identifier, map[string]any{"Name": identifier, "Engine": "aurora-postgresql"})
		},
		MultiCall: cluster(map[string]any{"Size": 3, "Description": "three calls", "Tier": nil}),
		Calls:     3,
		FailCall: func(handler native.Handler, call int, err error) {
			handler.(*nativefake.Handler).FailOn(call, err)
		},
	})
}

// settingSuite returns the suite for a fake handler of an account or region setting with no CloudFormation type, whose
// metadata sets onDelete and identityScope. A reset sets the setting to resetTo, or removes it when resetTo is nil.
func settingSuite(onDelete schemas.OnDelete, scope schemas.IdentityScope, resetTo map[string]any) Suite {
	const typeName = "AWS::Test::Setting"
	return Suite{
		TypeName:    typeName,
		HandWritten: true,
		Schema: `{
			"typeName": "AWS::Test::Setting",
			"primaryIdentifier": ["/properties/Id"],
			"createOnlyProperties": ["/properties/Id"]
		}`,
		Metadata: fmt.Sprintf(`{"typeName": %q, "identityScope": %q, "onDelete": %q}`, typeName, scope, onDelete),
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			h := nativefake.New(typeName, store)
			h.CreateOnly = []string{"Id"}
			h.ResetTo = resetTo
			return h
		},
		IdentifierProperty: "Id",
		Properties:         map[string]any{"Id": "default", "MinimumLength": 12, "RequireSymbols": true},
		Identifier:         "default",
		Changed:            map[string]any{"Id": "default", "MinimumLength": 14, "RequireSymbols": true},
		Removable:          "RequireSymbols",
		ResetTo:            resetTo,
	}
}

// Test_Suite_Settings runs the suite for a setting with each onDelete value, with account and region identity scopes.
func Test_Suite_Settings(t *testing.T) {
	tests := []struct {
		name     string
		onDelete schemas.OnDelete
		scope    schemas.IdentityScope
		resetTo  map[string]any
	}{
		{"reset to a default", schemas.OnDeleteReset, schemas.IdentityScopeAccount, map[string]any{"Id": "default", "MinimumLength": 8}},
		{"reset to absent", schemas.OnDeleteReset, schemas.IdentityScopeRegion, nil},
		{"nothing", schemas.OnDeleteNothing, schemas.IdentityScopeAccount, nil},
		{"leave", schemas.OnDeleteLeave, schemas.IdentityScopeRegion, nil},
		{"reject", schemas.OnDeleteReject, schemas.IdentityScopeAccount, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Run(t, settingSuite(tt.onDelete, tt.scope, tt.resetTo))
		})
	}
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
		// Escaping each value keeps "/" and "|" out of the identifier, so it is one resource ID segment that splits
		// back into the values.
		return url.PathEscape(bucket) + "|" + url.PathEscape(key), nil
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
