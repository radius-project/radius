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
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

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

// Test_Suite_DeleteToken runs the suite for a type whose delete takes a copy, like a final snapshot, named by a delete
// token UCP saves on the record before the first delete call. A retried delete sends the same token and takes no
// second copy.
func Test_Suite_DeleteToken(t *testing.T) {
	var handler *nativefake.Handler
	suite := assignedSuite("AWS::Test::Snapshotted", native.RecoverByResend)
	suite.NewHandler = func(store *awsfake.CloudControl) native.Handler {
		handler = nativefake.New("AWS::Test::Snapshotted", store)
		handler.Opts.Recovery = native.RecoverByResend
		handler.Waits = 1
		return handler.DeleteTokens()
	}
	Run(t, suite)

	t.Run("one copy for a retried delete", func(t *testing.T) {
		suite.deleteTokenRetry(t)
		require.Equal(t, 1, handler.Copies())
	})
}

// Test_Suite_Scoped runs the suite for a type whose resource is some members of a parent, like users a template adds
// to an IAM group. Members added outside Radius stay through a take-over, an update, and a delete, and an update that
// fails while it removes members keeps every member it may still have to remove.
func Test_Suite_Scoped(t *testing.T) {
	const typeName = "AWS::Test::Membership"
	group := func(users ...string) map[string]any {
		list := []any{}
		for _, u := range users {
			list = append(list, u)
		}
		return map[string]any{"Group": "admins", "Users": list}
	}
	Run(t, Suite{
		TypeName: typeName,
		Schema: `{
			"typeName": "AWS::Test::Membership",
			"primaryIdentifier": ["/properties/Group"],
			"createOnlyProperties": ["/properties/Group"]
		}`,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			h := nativefake.New(typeName, store)
			h.Opts.Recovery = native.RecoverByResend
			h.CreateOnly = []string{"Group"}
			return h.Scoped("Users")
		},
		IdentifierProperty: "Group",
		Properties:         group("ana", "bo"),
		Identifier:         "admins",
		Changed:            group("bo", "cy"),
		CreateOnlyChange:   map[string]any{"Group": "readers", "Users": []any{"ana", "bo"}},
		MultiCall:          group("bo", "cy", "di"),
		Calls:              3,
		RejectedCalls:      1,
		FailCall: func(handler native.Handler, call int, err error) {
			handler.(interface{ FailOn(int, error) }).FailOn(call, err)
		},
		Outside: func(store *awsfake.CloudControl, properties map[string]any) func() bool {
			id := fmt.Sprint(properties["Group"])
			current := store.Resource(typeName, id)
			if current == nil {
				current = map[string]any{"Group": id, "Users": []any{}}
			}
			current["Users"] = append(current["Users"].([]any), "outsider")
			store.Put(typeName, id, current)
			return func() bool {
				props := store.Resource(typeName, id)
				return props != nil && slices.Contains(props["Users"].([]any), any("outsider"))
			}
		},
	})
}
