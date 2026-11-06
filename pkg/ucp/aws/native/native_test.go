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

package native_test

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/aws/schemas"
	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativefake"
)

func Test_NewRegistry(t *testing.T) {
	store := awsfake.NewCloudControl()
	catalog := schemas.Catalog{
		Native:      map[string]string{"AWS::DocDB::DBCluster": `{"writeOnlyProperties": ["/properties/MasterUserPassword"]}`},
		HandWritten: map[string]string{"AWS::S3::Object": `{}`},
	}

	registry, err := native.NewRegistry(catalog, nativefake.New("AWS::DocDB::DBCluster", store), nativefake.New("aws::s3::object", store))
	require.NoError(t, err)

	docdb := registry.Lookup("aws::docdb::dbcluster")
	require.NotNil(t, docdb)
	require.Equal(t, catalog.Native["AWS::DocDB::DBCluster"], docdb.Schema)
	require.False(t, docdb.HandWritten)
	require.Equal(t, []string{"MasterUserPassword"}, docdb.WriteOnly)
	require.Nil(t, docdb.NameRules)

	object := registry.Lookup("AWS::S3::Object")
	require.NotNil(t, object)
	require.Equal(t, `{}`, object.Schema)
	require.True(t, object.HandWritten)
	require.Empty(t, object.WriteOnly)

	require.Nil(t, registry.Lookup("AWS::EC2::VPC"))
	require.Nil(t, (*native.Registry)(nil).Lookup("AWS::S3::Object"))

	_, err = native.NewRegistry(catalog, nativefake.New("AWS::EC2::VPC", store))
	require.EqualError(t, err, "the native handler for the AWS type AWS::EC2::VPC has no schema in pkg/aws/schemas")

	_, err = native.NewRegistry(catalog, nativefake.New("AWS::S3::Object", store), nativefake.New("AWS::S3::OBJECT", store))
	require.EqualError(t, err, "two native handlers serve the AWS type AWS::S3::OBJECT")

	_, err = native.NewRegistry(schemas.Catalog{Native: map[string]string{"AWS::DocDB::DBCluster": "not json"}}, nativefake.New("AWS::DocDB::DBCluster", store))
	require.ErrorContains(t, err, "the schema of the AWS type AWS::DocDB::DBCluster can't be read")
}

func Test_NewRegistry_Metadata(t *testing.T) {
	store := awsfake.NewCloudControl()
	catalog := schemas.Catalog{
		HandWritten: map[string]string{"AWS::IAM::AccountPasswordPolicy": `{}`, "AWS::S3::Object": `{}`},
		Metadata: map[string]schemas.Metadata{
			"AWS::IAM::AccountPasswordPolicy": {TypeName: "AWS::IAM::AccountPasswordPolicy", IdentityScope: schemas.IdentityScopeAccount, OnDelete: schemas.OnDeleteReset},
		},
	}
	registry, err := native.NewRegistry(catalog, nativefake.New("AWS::IAM::AccountPasswordPolicy", store), nativefake.New("AWS::S3::Object", store))
	require.NoError(t, err)
	require.Equal(t, catalog.Metadata["AWS::IAM::AccountPasswordPolicy"], registry.Lookup("AWS::IAM::AccountPasswordPolicy").Metadata)
	require.Equal(t, schemas.Metadata{TypeName: "AWS::S3::Object", IdentityScope: schemas.IdentityScopeRegion, OnDelete: schemas.OnDeleteDelete}, registry.Lookup("AWS::S3::Object").Metadata, "a type with no metadata gets the defaults")

	_, err = native.NewRegistry(catalog, noReset{nativefake.New("AWS::IAM::AccountPasswordPolicy", store)})
	require.EqualError(t, err, "the AWS type AWS::IAM::AccountPasswordPolicy resets on delete, but its native handler has no Reset method")
}

// noReset is a handler without a Reset method.
type noReset struct {
	native.Handler
}

// noFind is a handler without a Find method.
type noFind struct {
	native.Handler
}

func Test_NewRegistry_Recovery(t *testing.T) {
	store := awsfake.NewCloudControl()
	catalog := schemas.Catalog{Native: map[string]string{"AWS::EMR::Cluster": `{}`}}
	h := nativefake.New("AWS::EMR::Cluster", store)
	h.Opts.Recovery = native.RecoverBySearch

	_, err := native.NewRegistry(catalog, h)
	require.NoError(t, err)

	_, err = native.NewRegistry(catalog, noFind{h})
	require.EqualError(t, err, "the native handler for the AWS type AWS::EMR::Cluster finds a lost create by search, but has no Find method")
}

func Test_NewRegistry_NameRules(t *testing.T) {
	store := awsfake.NewCloudControl()
	register := func(schema string) (*native.Type, error) {
		h := nativefake.New("AWS::CodeBuild::Project", store)
		h.Opts.NameProperty = "Name"
		registry, err := native.NewRegistry(schemas.Catalog{Native: map[string]string{"AWS::CodeBuild::Project": schema}}, h)
		if err != nil {
			return nil, err
		}
		return registry.Lookup("AWS::CodeBuild::Project"), nil
	}

	t.Run("rules from the schema", func(t *testing.T) {
		project, err := register(`{"properties": {"Name": {"type": "string", "minLength": 2, "maxLength": 255, "pattern": "^[A-Za-z0-9][A-Za-z0-9\\-_]{1,254}$"}}}`)
		require.NoError(t, err)
		require.Equal(t, "Name", project.NameRules.Property)
		require.Equal(t, 2, project.NameRules.MinLength)
		require.Equal(t, 255, project.NameRules.MaxLength)
		require.NotNil(t, project.NameRules.Pattern)
	})

	tests := []struct {
		name   string
		schema string
		err    string
	}{
		{name: "invalid schema", schema: "not json", err: "the schema of the AWS type AWS::CodeBuild::Project can't be read"},
		{name: "rule of the wrong type", schema: `{"properties": {"Name": {"type": "string", "maxLength": "64"}}}`, err: "failed to read the schema"},
		{name: "not in the schema", schema: `{"properties": {}}`, err: "the name property Name is not a string property in the schema"},
		{name: "not a string", schema: `{"properties": {"Name": {"type": "integer"}}}`, err: "the name property Name is not a string property in the schema"},
		{name: "pattern Go can't read", schema: `{"properties": {"Name": {"type": "string", "pattern": "^(?!aws)"}}}`, err: "the pattern of the name property Name can't be read"},
		{name: "pattern no name fits", schema: `{"properties": {"Name": {"type": "string", "pattern": "^[A-Z]+$"}}}`, err: "which doesn't fit the rules of Name"},
		{name: "too short to stay unique", schema: `{"properties": {"Name": {"type": "string", "maxLength": 14}}}`, err: "which doesn't fit the rules of Name"},
		{name: "longer than a generated name", schema: `{"properties": {"Name": {"type": "string", "minLength": 20}}}`, err: "which doesn't fit the rules of Name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := register(tt.schema)
			require.ErrorContains(t, err, tt.err)
		})
	}
}

func Test_NameRules_Generate(t *testing.T) {
	generate := func(rules *native.NameRules, seed string) string {
		name, err := rules.Generate(seed)
		require.NoError(t, err)
		return name
	}
	rules := &native.NameRules{Property: "Name"}
	name := generate(rules, "2f1c6a9e-5b7d-4c3e-9a8f-0d1e2f3a4b5c")
	require.Regexp(t, "^radius-[0-9a-f]{12}$", name)
	require.Equal(t, name, generate(rules, "2f1c6a9e-5b7d-4c3e-9a8f-0d1e2f3a4b5c"), "the same operation always gets the same name")
	require.NotEqual(t, name, generate(rules, "7b0e3d1a-8c2f-4e6d-b5a4-3c2d1e0f9a8b"))

	rules.MaxLength = 16
	require.Equal(t, name[:16], generate(rules, "2f1c6a9e-5b7d-4c3e-9a8f-0d1e2f3a4b5c"), "a name is cut to the maximum length")

	t.Run("a name the pattern rejects", func(t *testing.T) {
		// The registry's sample names end in b and 7, so this pattern passes registration.
		rules := &native.NameRules{Property: "Name", Pattern: regexp.MustCompile("^radius-[0-9a-f]{11}[b7]$")}
		_, err := rules.Generate("2f1c6a9e-5b7d-4c3e-9a8f-0d1e2f3a4b5c")
		require.EqualError(t, err, "UCP generated the name "+name+", which doesn't fit the rules of Name. Set Name in the template.")
		var failure *native.Failure
		require.ErrorAs(t, err, &failure)
		require.True(t, failure.IsClientError(), "the template can set the name instead")
	})
}

func Test_Options_Timeout(t *testing.T) {
	opts := native.Options{CreateTimeout: time.Minute, DeleteTimeout: time.Second}
	require.Equal(t, time.Minute, opts.Timeout(native.OperationCreate))
	require.Equal(t, native.DefaultTimeout, opts.Timeout(native.OperationUpdate))
	require.Equal(t, time.Second, opts.Timeout(native.OperationDelete))
	require.Equal(t, native.DefaultTimeout, opts.Timeout("UNKNOWN"))
}

func Test_Failure(t *testing.T) {
	failure := &native.Failure{Message: "bad value", ClientError: true}
	require.Equal(t, "bad value", failure.Error())
	require.True(t, failure.IsClientError())
	require.Equal(t, native.OutcomeUnknown, failure.Outcome, "a failure that doesn't say keeps the pending values")
}

func Test_OtherProductError(t *testing.T) {
	err := &native.OtherProductError{Identifier: "app-docdb", Product: "aurora-postgresql"}
	require.Equal(t, `the AWS identifier "app-docdb" belongs to another product (aurora-postgresql)`, err.Error())
}

// readsOnlyScoped has ReadScoped but no DeleteScoped.
type readsOnlyScoped struct {
	native.Handler
}

func (readsOnlyScoped) ReadScoped(ctx context.Context, region string, identifier string, managed []map[string]any) (map[string]any, error) {
	return nil, nil
}

func Test_NewRegistry_Scoped(t *testing.T) {
	store := awsfake.NewCloudControl()
	catalog := schemas.Catalog{Native: map[string]string{"AWS::IAM::UserToGroupAddition": `{}`}}
	h := nativefake.New("AWS::IAM::UserToGroupAddition", store)

	_, err := native.NewRegistry(catalog, h.Scoped("Users"))
	require.NoError(t, err)

	_, err = native.NewRegistry(catalog, readsOnlyScoped{h})
	require.EqualError(t, err, "the native handler for the AWS type AWS::IAM::UserToGroupAddition must have both ReadScoped and DeleteScoped, or neither")
}

func Test_GenerateName(t *testing.T) {
	name := native.GenerateName("token")
	require.Regexp(t, `^radius-[0-9a-f]{12}$`, name)
	require.Equal(t, name, native.GenerateName("token"))
	require.NotEqual(t, name, native.GenerateName("other"))
}
