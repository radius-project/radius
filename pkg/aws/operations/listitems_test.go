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
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// Test_GeneratePatchWithOptions_MSKServerlessDefaults uses the CloudFormation schema of AWS::MSK::ServerlessCluster,
// a type AWS can only create and delete. A cluster created with only SubnetIds reads back with the VPC's default
// security group filled into the VpcConfigs item, so an unchanged redeploy must send nothing.
func Test_GeneratePatchWithOptions_MSKServerlessDefaults(t *testing.T) {
	t.Parallel()

	schema, err := os.ReadFile("testdata/aws-msk-serverlesscluster.json")
	require.NoError(t, err)

	vpc := func(securityGroups []any, subnets ...any) map[string]any {
		out := map[string]any{"SubnetIds": subnets}
		if securityGroups != nil {
			out["SecurityGroups"] = securityGroups
		}
		return out
	}
	properties := func(vpcConfigs ...any) map[string]any {
		return map[string]any{
			"ClusterName":          "orders",
			"VpcConfigs":           vpcConfigs,
			"ClientAuthentication": map[string]any{"Sasl": map[string]any{"Iam": map[string]any{"Enabled": true}}},
		}
	}
	aws := properties(vpc([]any{"sg-default"}, "subnet-a", "subnet-b"))
	aws["Arn"] = "arn:aws:kafka:us-west-2:123456789012:cluster/orders/abcd-1"
	aws["Tags"] = map[string]any{}
	template := properties(vpc(nil, "subnet-a", "subnet-b"))

	tests := []struct {
		name        string
		desired     map[string]any
		lastApplied map[string]any
		unresolved  []string
		want        []string
	}{
		{
			name:        "an unchanged redeploy keeps the default security group",
			desired:     template,
			lastApplied: template,
		},
		{
			name:    "a take-over with no record sends the template's list",
			desired: template,
			want:    []string{"remove /VpcConfigs/0/SecurityGroups"},
		},
		{
			name:        "subnets in another order still match",
			desired:     properties(vpc(nil, "subnet-b", "subnet-a")),
			lastApplied: template,
		},
		{
			name:        "a template that now sets the default keeps it",
			desired:     properties(vpc([]any{"sg-default"}, "subnet-a", "subnet-b")),
			lastApplied: template,
		},
		{
			name:        "security groups the last deploy set and the template drops are removed",
			desired:     template,
			lastApplied: properties(vpc([]any{"sg-default"}, "subnet-a", "subnet-b")),
			want:        []string{"remove /VpcConfigs/0/SecurityGroups"},
		},
		{
			name:        "an unresolved list is sent as the template sets it",
			desired:     template,
			lastApplied: template,
			unresolved:  []string{"VpcConfigs"},
			want:        []string{"remove /VpcConfigs/0/SecurityGroups"},
		},
		{
			name:        "a changed item is sent as the template sets it",
			desired:     properties(vpc(nil, "subnet-a", "subnet-c")),
			lastApplied: template,
			want:        []string{"remove /VpcConfigs/0/SecurityGroups", "replace /VpcConfigs/0/SubnetIds/1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.ElementsMatch(t, orEmpty(tt.want), patchOps(t, aws, tt.desired, schema, PatchOptions{
				LastApplied: tt.lastApplied,
				Unresolved:  tt.unresolved,
				Removed:     RemovedPaths(tt.lastApplied, tt.desired),
			}))
		})
	}
}

// Test_GeneratePatchWithOptions_ListItemFields covers fields AWS fills in inside list items for any type, with no
// Equivalent rule: a field stays unless the last deploy set a value at it and the template's list changed.
func Test_GeneratePatchWithOptions_ListItemFields(t *testing.T) {
	t.Parallel()

	item := func(fields ...any) map[string]any {
		out := map[string]any{}
		for i := 0; i < len(fields); i += 2 {
			out[fields[i].(string)] = fields[i+1]
		}
		return out
	}
	list := func(items ...any) []any { return items }
	unordered := []byte(`{"properties": {"Rules": {"type": "array", "insertionOrder": false, "items": {"type": "object"}}}}`)
	writeOnly := []byte(`{"properties": {}, "writeOnlyProperties": ["/properties/Users/*/Password"]}`)

	tests := []struct {
		name        string
		schema      []byte
		lastApplied map[string]any
		current     map[string]any
		desired     map[string]any
		want        []string
	}{
		{
			name:        "a map value inside a list item keeps the keys AWS added",
			lastApplied: map[string]any{"Rules": list(item("Name", "a", "Labels", item("team", "x")))},
			current:     map[string]any{"Rules": list(item("Name", "a", "Labels", item("team", "x", "aws:managed", "true")))},
			desired:     map[string]any{"Rules": list(item("Name", "a", "Labels", item("team", "x")))},
		},
		{
			name:        "a nested list keeps the fields AWS filled into its items",
			lastApplied: map[string]any{"Outer": list(item("Inner", list(item("Name", "a"), item("Name", "b"))))},
			current:     map[string]any{"Outer": list(item("Inner", list(item("Name", "a", "Codec", "h264"), item("Name", "b", "Codec", "h264"))))},
			desired:     map[string]any{"Outer": list(item("Inner", list(item("Name", "a"), item("Name", "b"))))},
		},
		{
			name:    "a take-over with no record sends the template's list",
			current: map[string]any{"Rules": list(item("Name", "a", "Codec", "h264"))},
			desired: map[string]any{"Rules": list(item("Name", "a"))},
			want:    []string{"remove /Rules/0/Codec"},
		},
		{
			name:        "a record whose last deploy set nothing keeps AWS's fields",
			lastApplied: map[string]any{},
			current:     map[string]any{"Rules": list(item("Name", "a", "Codec", "h264"))},
			desired:     map[string]any{"Rules": list(item("Name", "a"))},
		},
		{
			name:        "a field the last deploy set is removed while AWS's default stays",
			lastApplied: map[string]any{"Rules": list(item("Name", "a", "Sharpness", 75.0))},
			current:     map[string]any{"Rules": list(item("Name", "a", "Sharpness", 75.0, "Codec", "h264"))},
			desired:     map[string]any{"Rules": list(item("Name", "a"))},
			want:        []string{"remove /Rules/0/Sharpness"},
		},
		{
			name:        "a field the last deploy set in another item counts as Radius's",
			lastApplied: map[string]any{"Rules": list(item("Name", "a", "Sharpness", 1.0), item("Name", "b"))},
			current:     map[string]any{"Rules": list(item("Name", "a", "Sharpness", 1.0), item("Name", "b", "Sharpness", 1.0))},
			desired:     map[string]any{"Rules": list(item("Name", "a"), item("Name", "b", "Sharpness", 1.0))},
			want:        []string{"remove /Rules/0/Sharpness"},
		},
		{
			name:        "unordered items that look alike keep AWS's fields and its order",
			schema:      unordered,
			lastApplied: map[string]any{"Rules": list(item("Name", "a"), item("Name", "a", "Tint", "warm"))},
			current:     map[string]any{"Rules": list(item("Name", "a", "Tint", "warm", "Codec", "h264"), item("Name", "a", "Codec", "h264"))},
			desired:     map[string]any{"Rules": list(item("Name", "a"), item("Name", "a", "Tint", "warm"))},
		},
		{
			name:        "an added item sends the template's list",
			lastApplied: map[string]any{"Rules": list(item("Name", "a"))},
			current:     map[string]any{"Rules": list(item("Name", "a", "Codec", "h264"))},
			desired:     map[string]any{"Rules": list(item("Name", "a"), item("Name", "b"))},
			want:        []string{"remove /Rules/0/Codec", "add /Rules/-"},
		},
		{
			name:        "a field AWS doesn't have is sent",
			lastApplied: map[string]any{"Rules": list(item("Name", "a", "Tint", "warm"))},
			current:     map[string]any{"Rules": list(item("Name", "a", "Codec", "h264"))},
			desired:     map[string]any{"Rules": list(item("Name", "a", "Tint", "warm"))},
			want:        []string{"remove /Rules/0/Codec", "add /Rules/0/Tint"},
		},
		{
			name:        "a field the template sets to null matches one AWS doesn't have",
			lastApplied: map[string]any{"Rules": list(item("Name", "a", "Tint", nil))},
			current:     map[string]any{"Rules": list(item("Name", "a", "Codec", "h264"))},
			desired:     map[string]any{"Rules": list(item("Name", "a", "Tint", nil))},
		},
		{
			name:        "an item AWS reports in another shape is sent",
			lastApplied: map[string]any{"Rules": list(item("Name", "a"))},
			current:     map[string]any{"Rules": list("a")},
			desired:     map[string]any{"Rules": list(item("Name", "a"))},
			want:        []string{"replace /Rules/0"},
		},
		{
			name:        "an unchanged list with write-only fields keeps AWS's default in another item",
			schema:      []byte(`{"properties": {}, "writeOnlyProperties": ["/properties/Users/*/Secret"]}`),
			lastApplied: map[string]any{"Users": list(item("Name", "a", "Role", "admin"), item("Name", "b"))},
			current:     map[string]any{"Users": list(item("Name", "a", "Role", "admin"), item("Name", "b", "Role", "reader"))},
			desired:     map[string]any{"Users": list(item("Name", "a", "Role", "admin", "Secret", "s"), item("Name", "b", "Secret", "s"))},
			want:        []string{"add /Users/0/Secret", "add /Users/1/Secret"},
		},
		{
			name:        "a list where only a write-only value changed keeps AWS's default",
			schema:      []byte(`{"properties": {}, "writeOnlyProperties": ["/properties/Users/*/Secret"]}`),
			lastApplied: map[string]any{"Users": list(item("Name", "a", "Role", "admin"), item("Name", "b"))},
			current:     map[string]any{"Users": list(item("Name", "a", "Role", "admin"), item("Name", "b", "Role", "reader"))},
			desired:     map[string]any{"Users": list(item("Name", "a", "Role", "admin", "Secret", "new"), item("Name", "b", "Secret", "s"))},
			want:        []string{"add /Users/0/Secret", "add /Users/1/Secret"},
		},
		{
			name:        "a write-only field inside a map in a list item",
			schema:      []byte(`{"properties": {}, "writeOnlyProperties": ["/properties/Users/*/Auth/Token"]}`),
			lastApplied: map[string]any{"Users": list(item("Name", "a", "Role", "admin", "Auth", item("Mode", "t")), item("Name", "b", "Auth", item("Mode", "t")))},
			current:     map[string]any{"Users": list(item("Name", "a", "Role", "admin", "Auth", item("Mode", "t")), item("Name", "b", "Role", "reader", "Auth", item("Mode", "t")))},
			desired:     map[string]any{"Users": list(item("Name", "a", "Role", "admin", "Auth", item("Mode", "t", "Token", "s")), item("Name", "b", "Auth", item("Mode", "t", "Token", "s")))},
			want:        []string{"add /Users/0/Auth/Token", "add /Users/1/Auth/Token"},
		},
		{
			name:        "a write-only field in a nested list",
			schema:      []byte(`{"properties": {}, "writeOnlyProperties": ["/properties/Groups/*/Users/*/Secret"]}`),
			lastApplied: map[string]any{"Groups": list(item("Users", list(item("Name", "a", "Role", "admin"), item("Name", "b"))))},
			current:     map[string]any{"Groups": list(item("Users", list(item("Name", "a", "Role", "admin"), item("Name", "b", "Role", "reader"))))},
			desired:     map[string]any{"Groups": list(item("Users", list(item("Name", "a", "Role", "admin", "Secret", "s"), item("Name", "b", "Secret", "s"))))},
			want:        []string{"add /Groups/0/Users/0/Secret", "add /Groups/0/Users/1/Secret"},
		},
		{
			name:        "a field the last deploy set is still removed from a list with write-only fields",
			schema:      []byte(`{"properties": {}, "writeOnlyProperties": ["/properties/Users/*/Secret"]}`),
			lastApplied: map[string]any{"Users": list(item("Name", "a", "Role", "admin"), item("Name", "b"))},
			current:     map[string]any{"Users": list(item("Name", "a", "Role", "admin"), item("Name", "b", "Role", "reader"))},
			desired:     map[string]any{"Users": list(item("Name", "a", "Secret", "s"), item("Name", "b", "Secret", "s"))},
			want:        []string{"remove /Users/0/Role", "remove /Users/1/Role", "add /Users/0/Secret", "add /Users/1/Secret"},
		},
		{
			name:        "a write-only field AWS never returns is kept for the fingerprints to decide",
			schema:      writeOnly,
			lastApplied: map[string]any{"Users": list(item("Name", "a"))},
			current:     map[string]any{"Users": list(item("Name", "a", "Role", "reader"))},
			desired:     map[string]any{"Users": list(item("Name", "a", "Password", "secret"))},
			want:        []string{"add /Users/0/Password"},
		},
		{
			name:        "an object of only write-only fields AWS never returns is kept for the fingerprints to decide",
			schema:      []byte(`{"properties": {}, "writeOnlyProperties": ["/properties/Users/*/Auth/Password"]}`),
			lastApplied: map[string]any{"Users": list(item("Name", "a", "Auth", item()))},
			current:     map[string]any{"Users": list(item("Name", "a", "Role", "reader"))},
			desired:     map[string]any{"Users": list(item("Name", "a", "Auth", item("Password", "secret")))},
			want:        []string{"add /Users/0/Auth"},
		},
		{
			name:        "an object with a field AWS should return is sent",
			schema:      []byte(`{"properties": {}, "writeOnlyProperties": ["/properties/Users/*/Auth/Password"]}`),
			lastApplied: map[string]any{"Users": list(item("Name", "a"))},
			current:     map[string]any{"Users": list(item("Name", "a", "Role", "reader"))},
			desired:     map[string]any{"Users": list(item("Name", "a", "Auth", item("Password", "secret", "Mode", "x")))},
			want:        []string{"remove /Users/0/Role", "add /Users/0/Auth"},
		},
		{
			name:        "a list of only write-only fields AWS never returns is kept for the fingerprints to decide",
			schema:      []byte(`{"properties": {}, "writeOnlyProperties": ["/properties/Users/*/Keys/*/Secret"]}`),
			lastApplied: map[string]any{"Users": list(item("Name", "a"))},
			current:     map[string]any{"Users": list(item("Name", "a", "Role", "reader"))},
			desired:     map[string]any{"Users": list(item("Name", "a", "Keys", list(item("Secret", "s"))))},
			want:        []string{"add /Users/0/Keys"},
		},
		{
			name:        "a list with a field AWS should return is sent",
			schema:      []byte(`{"properties": {}, "writeOnlyProperties": ["/properties/Users/*/Keys/*/Secret"]}`),
			lastApplied: map[string]any{"Users": list(item("Name", "a"))},
			current:     map[string]any{"Users": list(item("Name", "a", "Role", "reader"))},
			desired:     map[string]any{"Users": list(item("Name", "a", "Keys", list(item("Id", "k1"))))},
			want:        []string{"remove /Users/0/Role", "add /Users/0/Keys"},
		},
		{
			name:        "an empty object AWS doesn't have is sent",
			schema:      []byte(`{"properties": {}, "writeOnlyProperties": ["/properties/Users/*/Auth/Password"]}`),
			lastApplied: map[string]any{"Users": list(item("Name", "a"))},
			current:     map[string]any{"Users": list(item("Name", "a", "Role", "reader"))},
			desired:     map[string]any{"Users": list(item("Name", "a", "Auth", item()))},
			want:        []string{"remove /Users/0/Role", "add /Users/0/Auth"},
		},
		{
			name:        "a removed key with a slash in its name is removed",
			lastApplied: map[string]any{"Statements": list(item("Condition", item("aws:PrincipalTag/team", "a", "aws:RequestedRegion", "us-west-2")))},
			current:     map[string]any{"Statements": list(item("Condition", item("aws:PrincipalTag/team", "a", "aws:RequestedRegion", "us-west-2")))},
			desired:     map[string]any{"Statements": list(item("Condition", item("aws:RequestedRegion", "us-west-2")))},
			want:        []string{"remove /Statements/0/Condition/aws:PrincipalTag~1team"},
		},
		{
			name:        "a key named * is not every item of a list",
			lastApplied: map[string]any{"Rules": list(item("Match", item("*", "a")))},
			current:     map[string]any{"Rules": list(item("Match", item("*", "a")))},
			desired:     map[string]any{"Rules": list(item("Match", item()))},
			want:        []string{"remove /Rules/0/Match/*"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			schema := tt.schema
			if schema == nil {
				schema = []byte(`{}`)
			}
			require.ElementsMatch(t, orEmpty(tt.want), patchOps(t, tt.current, tt.desired, schema, PatchOptions{
				LastApplied: tt.lastApplied,
				Removed:     RemovedPaths(tt.lastApplied, tt.desired),
			}))
		})
	}
}

// patchOps returns the patch from current to desired as "op path" strings.
func patchOps(t *testing.T, current map[string]any, desired map[string]any, schema []byte, opts PatchOptions) []string {
	t.Helper()
	currentState, err := json.Marshal(current)
	require.NoError(t, err)
	desiredState, err := json.Marshal(desired)
	require.NoError(t, err)
	patch, err := GeneratePatchWithOptions(currentState, desiredState, schema, opts)
	require.NoError(t, err)
	got := []string{}
	for _, op := range patch {
		got = append(got, string(op.Type)+" "+op.Path)
	}
	return got
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// Test_FillAt_SlashInKeyAbove checks a list under a map key that holds a "/". Its flattened path doesn't lead back to
// the list, so fillAt doesn't match, and the template's list is sent.
func Test_FillAt_SlashInKeyAbove(t *testing.T) {
	t.Parallel()

	list := []any{map[string]any{"Name": "a"}}
	root := map[string]any{"Map": map[string]any{"a/b": list}}
	fields := listFields{awsOwned: func([]string) bool { return true }}
	_, ok := parseSchemaRules([]byte(`{}`)).fillAt("Map/a/b", list, []any{map[string]any{"Name": "a", "Codec": "h264"}}, root, fields)
	require.False(t, ok)
}

func Test_RemovePaths(t *testing.T) {
	t.Parallel()

	properties := map[string]any{
		"Password": "p",
		"Auth":     map[string]any{"Token": "t", "Mode": "m"},
		"Users":    []any{map[string]any{"Name": "a", "Secret": "s"}, "not an object"},
		"Keys":     []any{"k1", "k2"},
		"Name":     "n",
	}
	RemovePaths(properties, []string{"Password", "Auth/Token", "Users/*/Secret", "Keys/*", "Missing/Child", ""})
	require.Equal(t, map[string]any{
		"Auth":  map[string]any{"Mode": "m"},
		"Users": []any{map[string]any{"Name": "a"}, "not an object"},
		"Name":  "n",
	}, properties)
	require.NotPanics(t, func() { RemovePaths(nil, []string{"A"}) })
}
