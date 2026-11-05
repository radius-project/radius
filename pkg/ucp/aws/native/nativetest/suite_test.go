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
	"net/url"
	"testing"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativefake"
)

// Test_Suite_NativeType runs the suite for a fake handler of a type that has a CloudFormation type.
func Test_Suite_NativeType(t *testing.T) {
	Run(t, Suite{
		TypeName: "AWS::Test::Cluster",
		Schema: `{
			"typeName": "AWS::Test::Cluster",
			"primaryIdentifier": ["/properties/Name"],
			"createOnlyProperties": ["/properties/Name", "/properties/Engine"],
			"writeOnlyProperties": ["/properties/MasterUserPassword"]
		}`,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			h := nativefake.New("AWS::Test::Cluster", store)
			h.Opts.List = native.ListRegion
			h.Waits = 2
			h.CreateOnly = []string{"Name", "Engine"}
			return h
		},
		IdentifierProperty: "Name",
		WriteOnly:          []string{"MasterUserPassword"},
		Properties:         map[string]any{"Name": "app-cluster", "Engine": "docdb", "Size": 1, "Tier": "standard", "MasterUserPassword": "p@ss"},
		Identifier:         "app-cluster",
		Changed:            map[string]any{"Name": "app-cluster", "Engine": "docdb", "Size": 2, "Tier": "standard", "MasterUserPassword": "p@ss"},
		Removable:          "Tier",
		CreateOnlyChange:   map[string]any{"Name": "app-cluster", "Engine": "mongo", "Size": 1, "Tier": "standard", "MasterUserPassword": "p@ss"},
	})
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
		Removable:        "ContentType",
		CreateOnlyChange: nil,
	})
}
