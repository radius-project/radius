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

package operations

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"
)

// The transforms and array definitions below are copied from the CloudFormation schemas of the types they name.

// rdsDBInstanceSchema is the part of the AWS::RDS::DBInstance schema these tests need.
const rdsDBInstanceSchema = `{
	"typeName": "AWS::RDS::DBInstance",
	"properties": {
		"DBInstanceIdentifier": {"type": "string"},
		"DBName": {"type": "string"},
		"Engine": {"type": "string"},
		"EngineVersion": {"type": "string"},
		"KmsKeyId": {"type": "string"},
		"MasterUserSecret": {"$ref": "#/definitions/MasterUserSecret"},
		"AllocatedStorage": {"type": "string"},
		"Tags": {"type": "array", "insertionOrder": false, "items": {"$ref": "#/definitions/Tag"}}
	},
	"definitions": {
		"MasterUserSecret": {"type": "object", "properties": {"SecretArn": {"type": "string"}, "KmsKeyId": {"type": "string"}}},
		"Tag": {"type": "object", "properties": {"Key": {"type": "string"}, "Value": {"type": "string"}}}
	},
	"createOnlyProperties": ["/properties/DBInstanceIdentifier", "/properties/KmsKeyId", "/properties/DBName"],
	"propertyTransform": {
		"/properties/DBInstanceIdentifier": "$lowercase(DBInstanceIdentifier)",
		"/properties/DBName": "$lowercase(DBName) $OR $uppercase(DBName)",
		"/properties/Engine": "$lowercase(Engine)",
		"/properties/EngineVersion": "$join([$string(EngineVersion), \".*\"])",
		"/properties/KmsKeyId": "$join([\"arn:.+?:kms:.+?:.+?:key\\/\", KmsKeyId])",
		"/properties/MasterUserSecret/KmsKeyId": "$join([\"arn:.+?:kms:.+?:.+?:key\\/\", MasterUserSecret.KmsKeyId])"
	},
	"primaryIdentifier": ["/properties/DBInstanceIdentifier"]
}`

// rdsDBSubnetGroupSchema is the part of the AWS::RDS::DBSubnetGroup schema these tests need.
const rdsDBSubnetGroupSchema = `{
	"typeName": "AWS::RDS::DBSubnetGroup",
	"properties": {
		"DBSubnetGroupName": {"type": "string"},
		"DBSubnetGroupDescription": {"type": "string"},
		"SubnetIds": {"type": "array", "uniqueItems": false, "insertionOrder": false, "items": {"type": "string"}},
		"Tags": {"type": "array", "insertionOrder": false, "items": {"$ref": "#/definitions/Tag"}}
	},
	"definitions": {
		"Tag": {"type": "object", "properties": {"Key": {"type": "string"}, "Value": {"type": "string"}}}
	},
	"createOnlyProperties": ["/properties/DBSubnetGroupName"],
	"propertyTransform": {"/properties/DBSubnetGroupName": "$lowercase(DBSubnetGroupName)"},
	"primaryIdentifier": ["/properties/DBSubnetGroupName"]
}`

func patchFor(t *testing.T, schema string, current map[string]any, desired map[string]any) jsondiff.Patch {
	t.Helper()
	c, err := json.Marshal(current)
	require.NoError(t, err)
	d, err := json.Marshal(desired)
	require.NoError(t, err)
	patch, err := GeneratePatch(c, d, []byte(schema))
	require.NoError(t, err)
	return patch
}

// op is a patch operation without the fields jsondiff fills in for its own use.
type op struct {
	Type  string
	Path  string
	Value any
}

func ops(patch jsondiff.Patch) []op {
	out := []op{}
	for _, o := range patch {
		out = append(out, op{Type: o.Type, Path: o.Path, Value: o.Value})
	}
	return out
}

func Test_ParseSchemaRules(t *testing.T) {
	rules := parseSchemaRules([]byte(`{
		"properties": {
			"Ordered": {"type": "array"},
			"Explicit": {"type": "array", "insertionOrder": true},
			"Unordered": {"type": "array", "insertionOrder": false},
			"ViaRef": {"$ref": "#/definitions/List"},
			"RefWithOrder": {"$ref": "#/definitions/Plain", "insertionOrder": false},
			"Nested": {"type": "object", "properties": {"Ids": {"type": "array", "insertionOrder": false}}},
			"Items": {"type": "array", "items": {"$ref": "#/definitions/Item"}},
			"Either": {"oneOf": [{"type": "object", "properties": {"Zones": {"type": "array", "insertionOrder": false}}}]},
			"Loop": {"$ref": "#/definitions/Loop"}
		},
		"definitions": {
			"List": {"type": "array", "insertionOrder": false, "items": {"type": "string"}},
			"Plain": {"type": "array", "items": {"type": "string"}},
			"Item": {"type": "object", "properties": {"Tags": {"type": "array", "insertionOrder": false}}},
			"Loop": {"type": "object", "properties": {"Next": {"$ref": "#/definitions/Loop"}, "Ids": {"type": "array", "insertionOrder": false}}}
		},
		"propertyTransform": {
			"/properties/Name": "$lowercase(Name)",
			"/properties/Protocol": "$ipProtocol(Protocol)",
			"/properties/Flag": "Flag and Flag $OR true",
			"/other/Ignored": "$lowercase(Ignored)"
		}
	}`))

	for _, path := range []string{"Unordered", "ViaRef", "RefWithOrder", "Nested/Ids", "Items/*/Tags", "Either/Zones", "Loop/Ids", "Loop/Next/Ids"} {
		require.True(t, rules.unordered[path], path)
	}
	for _, path := range []string{"Ordered", "Explicit", "Items", "Nested", "Missing"} {
		require.False(t, rules.unordered[path], path)
	}
	require.NotNil(t, rules.transforms["Name"])
	require.Nil(t, rules.transforms["Protocol"], "an unsupported function is compared as it is")
	require.Nil(t, rules.transforms["Flag"], "an unsupported operator is compared as it is")
	require.Nil(t, rules.transforms["Ignored"])

	t.Run("a schema in an unexpected shape has no rules for that part", func(t *testing.T) {
		empty := parseSchemaRules([]byte(`not json`))
		require.Empty(t, empty.unordered)
		require.Empty(t, empty.transforms)

		// Tuple items and a non-string transform are skipped; the rest of the schema still applies.
		odd := parseSchemaRules([]byte(`{
			"properties": {"Tuple": {"type": "array", "insertionOrder": false, "items": [{"type": "string"}]}},
			"propertyTransform": {"/properties/Name": "$lowercase(Name)", "/properties/Bad": 3}
		}`))
		require.Empty(t, odd.unordered)
		require.NotNil(t, odd.transforms["Name"])
		require.Nil(t, odd.transforms["Bad"])
	})

	var none *schemaRules
	require.False(t, none.equivalentAt("A", "x", "X", nil))
	require.True(t, none.equivalentAt("A", "x", "x", nil))
}

func Test_ParseTransform(t *testing.T) {
	supported := []string{
		`$lowercase(Name)`,
		`$uppercase(Name)`,
		`$join([$string(EngineVersion), ".*"])`,
		`$join(["arn:.+?:kms:.+?:.+?:key\/", KmsKeyId])`,
		`$join([A, B], "-")`,
		`$join(Items)`,
		`$lowercase(Name) $OR $uppercase(Name)`,
		`KmsKeyIdentifier $OR $join(["arn:aws:kms:.*:key\/", KmsKeyIdentifier])`,
		`$lowercase(Outer.Inner)`,
		`$join(['single \' quoted \u0041', Name])`,
	}
	for _, expression := range supported {
		_, err := parseTransform(expression)
		require.NoError(t, err, expression)
	}

	unsupported := []string{
		`$ipProtocol(IpProtocol)`,
		`$canon(Name)`,
		`$split(Arn, "/")[-1]`,
		`$replace(Name, "prefix", "Prefix") $OR $lowercase(Name)`,
		`Encrypted and Encrypted $OR true`,
		`$contains(Cost, ".") ? Cost : Cost & ".0"`,
		`$uppercase(.)`,
		`$lowercase`,
		`$lowercase(A, B)`,
		`$join()`,
		`"unterminated`,
		`$join([A B])`,
		`A.`,
		`$OR`,
		`"\u12"`,
		`3`,
		`"vpc"`,
		`$join([])`,
		`"vpc" $OR Domain`,
		`"arn:aws[-a-z]*:kms:[a-z0-9-]+:[0-9]{12}:key/[a-zA-Z0-9-]+"`,
	}
	for _, expression := range unsupported {
		_, err := parseTransform(expression)
		require.ErrorIs(t, err, errUnsupported, expression)
	}
}

func Test_Transform_Matches(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		properties map[string]any
		parentPath []string
		current    any
		want       bool
	}{
		{name: "lowercase", expression: `$lowercase(Name)`, properties: map[string]any{"Name": "MyDB"}, current: "mydb", want: true},
		{name: "lowercase is literal", expression: `$lowercase(Name)`, properties: map[string]any{"Name": "a.c"}, current: "abc", want: false},
		{name: "uppercase", expression: `$uppercase(Name)`, properties: map[string]any{"Name": "abc"}, current: "ABC", want: true},
		{name: "OR second alternative", expression: `$lowercase(Name) $OR $uppercase(Name)`, properties: map[string]any{"Name": "Db"}, current: "DB", want: true},
		{name: "OR no alternative", expression: `$lowercase(Name) $OR $uppercase(Name)`, properties: map[string]any{"Name": "Db"}, current: "Db1", want: false},
		{name: "engine version prefix", expression: `$join([$string(EngineVersion), ".*"])`, properties: map[string]any{"EngineVersion": "8.4"}, current: "8.4.11", want: true},
		{name: "engine version exact", expression: `$join([$string(EngineVersion), ".*"])`, properties: map[string]any{"EngineVersion": "8.4"}, current: "8.4", want: true},
		{name: "engine version changed", expression: `$join([$string(EngineVersion), ".*"])`, properties: map[string]any{"EngineVersion": "8.0"}, current: "8.4.11", want: false},
		{name: "template dot is literal", expression: `$join([$string(EngineVersion), ".*"])`, properties: map[string]any{"EngineVersion": "8.4"}, current: "804.1", want: false},
		{name: "string of a number", expression: `$join([$string(EngineVersion), ".*"])`, properties: map[string]any{"EngineVersion": 8.4}, current: "8.4.11", want: true},
		{name: "string of a bool", expression: `$string(Flag)`, properties: map[string]any{"Flag": true}, current: "true", want: true},
		{name: "kms key id", expression: `$join(["arn:.+?:kms:.+?:.+?:key\/", KmsKeyId])`, properties: map[string]any{"KmsKeyId": "1234abcd"}, current: "arn:aws:kms:us-west-2:817312594854:key/1234abcd", want: true},
		{name: "kms other key", expression: `$join(["arn:.+?:kms:.+?:.+?:key\/", KmsKeyId])`, properties: map[string]any{"KmsKeyId": "1234abcd"}, current: "arn:aws:kms:us-west-2:817312594854:key/9999", want: false},
		{name: "join with separator", expression: `$join([A, B], "-")`, properties: map[string]any{"A": "x", "B": "y"}, current: "x-y", want: true},
		{name: "join an array property", expression: `$join(Items, ",")`, properties: map[string]any{"Items": []any{"a", "b"}}, current: "a,b", want: true},
		{name: "join a single string", expression: `$join(Name)`, properties: map[string]any{"Name": "a"}, current: "a", want: true},
		{name: "dotted reference from the root", expression: `$lowercase(Outer.Name)`, properties: map[string]any{"Outer": map[string]any{"Name": "AB"}}, parentPath: []string{"Outer"}, current: "ab", want: true},
		{name: "leaf reference from the parent", expression: `$lowercase(Name)`, properties: map[string]any{"Outer": map[string]any{"Name": "AB"}}, parentPath: []string{"Outer"}, current: "ab", want: true},
		{name: "missing reference", expression: `$lowercase(Other)`, properties: map[string]any{"Name": "AB"}, current: "ab", want: false},
		{name: "reference to an object", expression: `$lowercase(Name)`, properties: map[string]any{"Name": map[string]any{}}, current: "ab", want: false},
		{name: "lowercase of a number", expression: `$lowercase(Name)`, properties: map[string]any{"Name": 3.0}, current: "3", want: false},
		{name: "string of an object", expression: `$string(Name)`, properties: map[string]any{"Name": []any{map[string]any{}}}, current: "", want: false},
		{name: "join of numbers", expression: `$join(Items)`, properties: map[string]any{"Items": []any{1.0}}, current: "1", want: false},
		{name: "join with a non-string separator", expression: `$join(Items, Sep)`, properties: map[string]any{"Items": []any{"a"}, "Sep": 1.0}, current: "a", want: false},
		{name: "join of a number", expression: `$join(Sep)`, properties: map[string]any{"Sep": 1.0}, current: "1", want: false},
		{name: "current is not a string", expression: `$lowercase(Name)`, properties: map[string]any{"Name": "A"}, current: 1.0, want: false},
		{name: "result is not a string", expression: `[Name]`, properties: map[string]any{"Name": "a"}, current: "a", want: false},
		{name: "invalid regular expression", expression: `$join(["(", Name])`, properties: map[string]any{"Name": "a"}, current: "(a", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr, err := parseTransform(tt.expression)
			require.NoError(t, err)
			parent := tt.properties
			if tt.parentPath != nil {
				v, _ := valueAt(tt.properties, tt.parentPath)
				parent = v.(map[string]any)
			}
			require.Equal(t, tt.want, tr.matches(parent, tt.properties, tt.current))
		})
	}
}

func Test_SchemaRules_EquivalentAt(t *testing.T) {
	rules := parseSchemaRules([]byte(`{
		"properties": {
			"Ids": {"type": "array", "insertionOrder": false},
			"Ordered": {"type": "array"},
			"Rules": {"type": "array", "insertionOrder": false, "items": {"$ref": "#/definitions/Rule"}},
			"Name": {"type": "string"}
		},
		"definitions": {
			"Rule": {"type": "object", "properties": {"Name": {"type": "string"}, "Ports": {"type": "array", "insertionOrder": false}}}
		},
		"propertyTransform": {
			"/properties/Name": "$lowercase(Name)",
			"/properties/Rules/*/Name": "$lowercase(Name)"
		}
	}`))

	eq := func(path string, desired any, current any) bool {
		return rules.equivalentAt(path, desired, current, map[string]any{path: desired})
	}
	require.True(t, eq("Ids", []any{"b", "a"}, []any{"a", "b"}), "reordered")
	require.True(t, eq("Ids", []string{"b", "a", "a"}, []any{"a", "b", "a"}), "duplicates as a multiset")
	require.False(t, eq("Ids", []any{"a", "a", "b"}, []any{"a", "b", "b"}), "different counts")
	require.False(t, eq("Ids", []any{"a", "b"}, []any{"a", "c"}), "different item")
	require.False(t, eq("Ids", []any{"a"}, []any{"a", "b"}), "different length")
	require.False(t, eq("Ids", []any{"a"}, "a"), "not an array")
	require.False(t, eq("Ordered", []any{"b", "a"}, []any{"a", "b"}), "an ordered array keeps its order")
	require.True(t, eq("Ordered", []any{"a", "b"}, []any{"a", "b"}))
	require.True(t, eq("Name", "MyName", "myname"))
	require.False(t, eq("Name", "MyName", "other"))

	rulesValue := []any{
		map[string]any{"Name": "B", "Ports": []any{2.0, 1.0}},
		map[string]any{"Name": "A", "Ports": []any{3.0}},
	}
	aws := []any{
		map[string]any{"Name": "a", "Ports": []any{3.0}},
		map[string]any{"Name": "b", "Ports": []any{1.0, 2.0}},
	}
	require.True(t, eq("Rules", rulesValue, aws), "items match by their own rules, in any order")
	extra := []any{
		map[string]any{"Name": "a", "Ports": []any{3.0}, "Extra": "x"},
		map[string]any{"Name": "b", "Ports": []any{1.0, 2.0}},
	}
	require.False(t, eq("Rules", rulesValue, extra), "a field AWS reports and the template does not set differs")
	require.False(t, eq("Rules", []any{map[string]any{"Name": "a"}}, []any{"a"}), "an item that is not an object")

	// An object compares field by field. A null field AWS reports equals an unset one.
	require.True(t, rules.equivalentAt("Rules/*", map[string]any{"Name": "A"}, map[string]any{"Name": "a", "Ports": nil}, nil))
}

// Test_GeneratePatch_SchemaRules covers values AWS reports in a different form.
func Test_GeneratePatch_SchemaRules(t *testing.T) {
	t.Run("a DB subnet group whose SubnetIds AWS returns in a different order sends nothing", func(t *testing.T) {
		current := map[string]any{"DBSubnetGroupName": "app-db", "DBSubnetGroupDescription": "db", "SubnetIds": []any{"subnet-b", "subnet-c", "subnet-a"}}
		desired := map[string]any{"DBSubnetGroupName": "app-db", "DBSubnetGroupDescription": "db", "SubnetIds": []any{"subnet-a", "subnet-b", "subnet-c"}}
		require.Empty(t, patchFor(t, rdsDBSubnetGroupSchema, current, desired))
	})

	t.Run("a changed subnet is sent", func(t *testing.T) {
		current := map[string]any{"SubnetIds": []any{"subnet-b", "subnet-a"}}
		desired := map[string]any{"SubnetIds": []any{"subnet-a", "subnet-c"}}
		require.Equal(t, []op{
			{Type: jsondiff.OperationReplace, Path: "/SubnetIds/0", Value: "subnet-a"},
			{Type: jsondiff.OperationReplace, Path: "/SubnetIds/1", Value: "subnet-c"},
		}, ops(patchFor(t, rdsDBSubnetGroupSchema, current, desired)))
	})

	t.Run("reordered tags send nothing", func(t *testing.T) {
		current := map[string]any{"Tags": []any{map[string]any{"Key": "b", "Value": "2"}, map[string]any{"Key": "a", "Value": "1"}}}
		desired := map[string]any{"Tags": []any{map[string]any{"Key": "a", "Value": "1"}, map[string]any{"Key": "b", "Value": "2"}}}
		require.Empty(t, patchFor(t, rdsDBInstanceSchema, current, desired))
	})

	t.Run("an RDS instance with EngineVersion 8.4 against a running 8.4.11 sends nothing", func(t *testing.T) {
		current := map[string]any{"DBInstanceIdentifier": "app-db", "Engine": "mysql", "EngineVersion": "8.4.11", "AllocatedStorage": "20"}
		desired := map[string]any{"DBInstanceIdentifier": "app-db", "Engine": "MySQL", "EngineVersion": "8.4", "AllocatedStorage": "20"}
		require.Empty(t, patchFor(t, rdsDBInstanceSchema, current, desired))
	})

	t.Run("changing EngineVersion to 8.0 sends it", func(t *testing.T) {
		current := map[string]any{"DBInstanceIdentifier": "app-db", "EngineVersion": "8.4.11"}
		desired := map[string]any{"DBInstanceIdentifier": "app-db", "EngineVersion": "8.0"}
		require.Equal(t, []op{{Type: jsondiff.OperationReplace, Path: "/EngineVersion", Value: "8.0"}}, ops(patchFor(t, rdsDBInstanceSchema, current, desired)))
	})

	t.Run("a nested property is compared through its transform", func(t *testing.T) {
		current := map[string]any{"MasterUserSecret": map[string]any{"SecretArn": "arn:s", "KmsKeyId": "arn:aws:kms:us-west-2:817312594854:key/k1"}}
		desired := map[string]any{"MasterUserSecret": map[string]any{"KmsKeyId": "k1"}}
		require.Empty(t, patchFor(t, rdsDBInstanceSchema, current, desired))
	})

	t.Run("a property whose transform uses an unsupported function is compared as it is", func(t *testing.T) {
		schema := `{"properties": {"IpProtocol": {"type": "string"}}, "propertyTransform": {"/properties/IpProtocol": "$ipProtocol(IpProtocol)"}}`
		current := map[string]any{"IpProtocol": "6"}
		desired := map[string]any{"IpProtocol": "tcp"}
		require.Equal(t, []op{{Type: jsondiff.OperationReplace, Path: "/IpProtocol", Value: "tcp"}}, ops(patchFor(t, schema, current, desired)))
	})

	t.Run("a transform that reads no property is compared as it is", func(t *testing.T) {
		schema := `{"properties": {"KeyId": {"type": "string"}}, "propertyTransform": {"/properties/KeyId": "\"arn:aws[-a-z]*:kms:[a-z0-9-]+:[0-9]{12}:key/[a-zA-Z0-9-]+\""}}`
		current := map[string]any{"KeyId": "arn:aws:kms:us-west-2:817312594854:key/k1"}
		desired := map[string]any{"KeyId": "arn:aws:kms:us-west-2:817312594854:key/k2"}
		require.Equal(t, []op{{Type: jsondiff.OperationReplace, Path: "/KeyId", Value: "arn:aws:kms:us-west-2:817312594854:key/k2"}}, ops(patchFor(t, schema, current, desired)))
	})

	t.Run("a schema in an unexpected shape still builds a patch", func(t *testing.T) {
		schema := `{"properties": {"Pair": {"type": "array", "items": [{"type": "string"}]}}, "propertyTransform": {"/properties/Name": 3}}`
		current := map[string]any{"Pair": []any{"a"}, "Name": "x"}
		desired := map[string]any{"Pair": []any{"a"}, "Name": "y"}
		require.Equal(t, []op{{Type: jsondiff.OperationReplace, Path: "/Name", Value: "y"}}, ops(patchFor(t, schema, current, desired)))
	})

	t.Run("an ordered array keeps comparing item by item", func(t *testing.T) {
		schema := `{"properties": {"Steps": {"type": "array", "items": {"type": "string"}}}}`
		current := map[string]any{"Steps": []any{"b", "a"}}
		desired := map[string]any{"Steps": []any{"a", "b"}}
		require.Equal(t, []op{
			{Type: jsondiff.OperationReplace, Path: "/Steps/0", Value: "a"},
			{Type: jsondiff.OperationReplace, Path: "/Steps/1", Value: "b"},
		}, ops(patchFor(t, schema, current, desired)))
	})
}

func Test_GeneratePatch_NormalizedValues(t *testing.T) {
	t.Run("an RDS instance whose template sets DBInstanceIdentifier in mixed case sends nothing", func(t *testing.T) {
		current := map[string]any{"DBInstanceIdentifier": "app-db", "EngineVersion": "8.4.11"}
		desired := map[string]any{"DBInstanceIdentifier": "App-DB", "EngineVersion": "8.4"}
		require.Empty(t, patchFor(t, rdsDBInstanceSchema, current, desired))
	})

	t.Run("a KMS key ID against the ARN AWS reports sends nothing", func(t *testing.T) {
		require.Empty(t, patchFor(t, rdsDBInstanceSchema, map[string]any{"KmsKeyId": "arn:aws:kms:us-west-2:817312594854:key/k1"}, map[string]any{"KmsKeyId": "k1"}))
	})

	t.Run("DBName in either case sends nothing", func(t *testing.T) {
		require.Empty(t, patchFor(t, rdsDBInstanceSchema, map[string]any{"DBName": "APPDB"}, map[string]any{"DBName": "AppDb"}))
	})

	t.Run("a different identifier is still sent", func(t *testing.T) {
		current := map[string]any{"DBInstanceIdentifier": "app-db"}
		desired := map[string]any{"DBInstanceIdentifier": "Other-DB"}
		require.Equal(t, []op{{Type: jsondiff.OperationReplace, Path: "/DBInstanceIdentifier", Value: "Other-DB"}}, ops(patchFor(t, rdsDBInstanceSchema, current, desired)))
	})

	t.Run("a schema that is not JSON is an error", func(t *testing.T) {
		_, err := GeneratePatch([]byte(`{}`), []byte(`{}`), []byte(`not json`))
		require.Error(t, err)
	})
}

// Test_GeneratePatch_EmptyObjects covers an object AWS reports as {}, such as DataProtectionPolicy on a log group.
func Test_GeneratePatch_EmptyObjects(t *testing.T) {
	const schema = `{"properties": {"LogGroupName": {"type": "string"}, "DataProtectionPolicy": {"type": "object"}, "Outer": {"type": "object"}}}`

	t.Run("an unchanged redeploy sends nothing", func(t *testing.T) {
		current := map[string]any{"LogGroupName": "lg", "DataProtectionPolicy": map[string]any{}, "Outer": map[string]any{"Inner": map[string]any{}}}
		desired := map[string]any{"LogGroupName": "lg"}
		require.Empty(t, patchFor(t, schema, current, desired))
	})

	t.Run("a value the template sets inside an empty object is sent", func(t *testing.T) {
		for range 20 {
			current := map[string]any{"LogGroupName": "lg", "DataProtectionPolicy": map[string]any{}}
			desired := map[string]any{"LogGroupName": "lg", "DataProtectionPolicy": map[string]any{"Name": "p"}}
			require.Equal(t, []op{{Type: jsondiff.OperationAdd, Path: "/DataProtectionPolicy/Name", Value: "p"}}, ops(patchFor(t, schema, current, desired)))
		}
	})
}
