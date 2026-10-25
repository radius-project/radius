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

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/require"
)

// Test_GeneratedFileIsCurrent fails when zz_generated_arnproperties.go does not match what the generator renders from
// the checked-in inputs. Run `go generate ./pkg/aws/arnproperties` to fix it.
func Test_GeneratedFileIsCurrent(t *testing.T) {
	want, properties, err := generate(filepath.Join("..", defaultSnapshot), filepath.Join("..", defaultReviewed))
	require.NoError(t, err)
	require.NotEmpty(t, properties)

	got, err := os.ReadFile(filepath.Join("..", defaultOut))
	require.NoError(t, err)
	require.Equal(t, string(want), string(got), "run `go generate ./pkg/aws/arnproperties`")
}

func Test_ParseSnapshot(t *testing.T) {
	entries, err := parseSnapshot(strings.NewReader("# comment\n\nAWS::SNS::Topic TopicArn TopicArn\nAWS::EC2::VPC VpcId none\nAWS::A::B Id,Name Arn,OtherArn\n"))
	require.NoError(t, err)
	require.Equal(t, []entry{
		{TypeName: "AWS::SNS::Topic", PrimaryIdentifier: []string{"TopicArn"}, ReadOnlyARNs: []string{"TopicArn"}},
		{TypeName: "AWS::EC2::VPC", PrimaryIdentifier: []string{"VpcId"}},
		{TypeName: "AWS::A::B", PrimaryIdentifier: []string{"Id", "Name"}, ReadOnlyARNs: []string{"Arn", "OtherArn"}},
	}, entries)

	_, err = parseSnapshot(strings.NewReader("AWS::A::Missing Id\n"))
	require.ErrorContains(t, err, "line 1")
}

func Test_ParseReviewed(t *testing.T) {
	reviewed, err := parseReviewed(strings.NewReader("# comment\n\nAWS::A::B FleetArn # the fleet\nAWS::A::C none # a parent\n"))
	require.NoError(t, err)
	require.Equal(t, map[string]string{"AWS::A::B": "FleetArn", "AWS::A::C": ""}, reviewed)

	_, err = parseReviewed(strings.NewReader("AWS::A::B FleetArn\n"))
	require.ErrorContains(t, err, "line 1", "a decision needs a reason")

	_, err = parseReviewed(strings.NewReader("AWS::A::B FleetArn # one\nAWS::A::B none # two\n"))
	require.ErrorContains(t, err, "duplicate decision")
}

func Test_ARNProperties(t *testing.T) {
	entries := []entry{
		// Arn and <ResourceName>Arn, compared case-insensitively.
		{TypeName: "AWS::S3::Bucket", PrimaryIdentifier: []string{"BucketName"}, ReadOnlyARNs: []string{"Arn"}},
		{TypeName: "AWS::SNS::Topic", PrimaryIdentifier: []string{"TopicArn"}, ReadOnlyARNs: []string{"TopicArn"}},
		{TypeName: "AWS::RDS::DBInstance", PrimaryIdentifier: []string{"DBInstanceIdentifier"}, ReadOnlyARNs: []string{"DBInstanceArn", "DbiResourceArn"}},
		// A read-only primary identifier ending in Arn.
		{TypeName: "AWS::IAM::ManagedPolicy", PrimaryIdentifier: []string{"PolicyArn"}, ReadOnlyARNs: []string{"PolicyArn"}},
		// A primary identifier that isn't read-only is another resource's ARN.
		{TypeName: "AWS::Lambda::ResourcePolicy", PrimaryIdentifier: []string{"ResourceArn"}},
		// No ARN.
		{TypeName: "AWS::EC2::VPC", PrimaryIdentifier: []string{"VpcId"}},
		// Reviewed.
		{TypeName: "AWS::GameLift::ContainerFleet", PrimaryIdentifier: []string{"FleetId"}, ReadOnlyARNs: []string{"FleetArn", "GameServerContainerGroupDefinitionArn"}},
		{TypeName: "AWS::Redshift::Cluster", PrimaryIdentifier: []string{"ClusterIdentifier"}, ReadOnlyARNs: []string{"ClusterNamespaceArn", "MasterPasswordSecretArn"}},
	}
	reviewed := map[string]string{"AWS::GameLift::ContainerFleet": "FleetArn", "AWS::Redshift::Cluster": ""}

	properties, err := arnProperties(entries, reviewed)
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"AWS::S3::Bucket":               "Arn",
		"AWS::SNS::Topic":               "TopicArn",
		"AWS::RDS::DBInstance":          "DBInstanceArn",
		"AWS::IAM::ManagedPolicy":       "PolicyArn",
		"AWS::Lambda::ResourcePolicy":   "",
		"AWS::EC2::VPC":                 "",
		"AWS::GameLift::ContainerFleet": "FleetArn",
		"AWS::Redshift::Cluster":        "",
	}, properties)
}

func Test_ARNProperties_NeedsReview(t *testing.T) {
	entries := []entry{
		{TypeName: "AWS::Location::Map", PrimaryIdentifier: []string{"MapName"}, ReadOnlyARNs: []string{"Arn", "MapArn"}},
		{TypeName: "AWS::Redshift::Cluster", PrimaryIdentifier: []string{"ClusterIdentifier"}, ReadOnlyARNs: []string{"ClusterNamespaceArn"}},
		{TypeName: "AWS::A::Typo", PrimaryIdentifier: []string{"Id"}, ReadOnlyARNs: []string{"ThingArn"}},
	}
	reviewed := map[string]string{"AWS::A::Typo": "OtherArn", "AWS::A::Removed": ""}

	_, err := arnProperties(entries, reviewed)
	require.ErrorContains(t, err, "AWS::Location::Map: needs a decision in reviewed.txt, more than one candidate: [Arn MapArn]")
	require.ErrorContains(t, err, "AWS::Redshift::Cluster: needs a decision in reviewed.txt, only other read-only ARN properties: [ClusterNamespaceArn]")
	require.ErrorContains(t, err, `AWS::A::Typo: reviewed property "OtherArn" is not one of its read-only ARN properties [ThingArn]`)
	require.ErrorContains(t, err, "AWS::A::Removed: has a decision in reviewed.txt but is not in the snapshot")
}

func Test_EntryFromSchema(t *testing.T) {
	schema := `{
		"primaryIdentifier": ["/properties/TopicArn"],
		"readOnlyProperties": ["/properties/TopicArn", "/properties/Name", "/properties/Nested/Arn", "/properties/OwnerARN"]
	}`
	e, err := entryFromSchema("AWS::SNS::Topic", []byte(schema))
	require.NoError(t, err)
	require.Equal(t, entry{TypeName: "AWS::SNS::Topic", PrimaryIdentifier: []string{"TopicArn"}, ReadOnlyARNs: []string{"OwnerARN", "TopicArn"}}, e)
	require.Equal(t, "AWS::SNS::Topic TopicArn OwnerARN,TopicArn", formatEntry(e))

	_, err = entryFromSchema("AWS::A::B", []byte(`{}`))
	require.ErrorContains(t, err, "no primary identifier")

	_, err = entryFromSchema("AWS::A::B", []byte(`not json`))
	require.Error(t, err)
}

func Test_Generate_Errors(t *testing.T) {
	dir := t.TempDir()
	snapshot := filepath.Join(dir, "snapshot.txt")
	reviewed := filepath.Join(dir, "reviewed.txt")
	require.NoError(t, os.WriteFile(snapshot, []byte("AWS::Location::Map MapName Arn,MapArn\n"), 0o600))
	require.NoError(t, os.WriteFile(reviewed, []byte("# nothing reviewed\n"), 0o600))

	_, _, err := generate(filepath.Join(dir, "missing.txt"), reviewed)
	require.ErrorIs(t, err, os.ErrNotExist)

	_, _, err = generate(snapshot, filepath.Join(dir, "missing.txt"))
	require.ErrorIs(t, err, os.ErrNotExist)

	_, _, err = generate(snapshot, reviewed)
	require.ErrorContains(t, err, "AWS::Location::Map: needs a decision")

	require.NoError(t, os.WriteFile(reviewed, []byte("AWS::Location::Map Arn\n"), 0o600))
	_, _, err = generate(snapshot, reviewed)
	require.ErrorContains(t, err, "reviewed.txt: line 1")

	require.NoError(t, os.WriteFile(snapshot, []byte("AWS::Location::Map\n"), 0o600))
	_, _, err = generate(snapshot, reviewed)
	require.ErrorContains(t, err, "snapshot.txt: line 1")
}

// fakeRegistry serves ListTypes in pages and DescribeType from a map of schemas.
type fakeRegistry struct {
	pages   [][]string
	schemas map[string]string
	listErr error
}

func (f *fakeRegistry) ListTypes(_ context.Context, params *cloudformation.ListTypesInput, _ ...func(*cloudformation.Options)) (*cloudformation.ListTypesOutput, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	if params.Type != types.RegistryTypeResource || params.Visibility != types.VisibilityPublic ||
		params.ProvisioningType != types.ProvisioningTypeFullyMutable || params.Filters.Category != types.CategoryAwsTypes {
		return nil, errors.New("unexpected ListTypes filter")
	}
	page := 0
	if params.NextToken != nil {
		page = len(*params.NextToken)
	}
	out := &cloudformation.ListTypesOutput{}
	for _, name := range f.pages[page] {
		out.TypeSummaries = append(out.TypeSummaries, types.TypeSummary{TypeName: aws.String(name)})
	}
	if page+1 < len(f.pages) {
		out.NextToken = aws.String(strings.Repeat("x", page+1))
	}
	return out, nil
}

func (f *fakeRegistry) DescribeType(_ context.Context, params *cloudformation.DescribeTypeInput, _ ...func(*cloudformation.Options)) (*cloudformation.DescribeTypeOutput, error) {
	schema, ok := f.schemas[aws.ToString(params.TypeName)]
	if !ok {
		return nil, errors.New("type not found")
	}
	return &cloudformation.DescribeTypeOutput{Schema: aws.String(schema)}, nil
}

func Test_RefreshSnapshot(t *testing.T) {
	registry := &fakeRegistry{
		pages: [][]string{{"AWS::SNS::Topic"}, {"AWS::EC2::VPC"}},
		schemas: map[string]string{
			"AWS::SNS::Topic": `{"primaryIdentifier": ["/properties/TopicArn"], "readOnlyProperties": ["/properties/TopicArn"]}`,
			"AWS::EC2::VPC":   `{"primaryIdentifier": ["/properties/VpcId"], "readOnlyProperties": ["/properties/VpcId", "/properties/CidrBlockAssociations"]}`,
		},
	}
	path := filepath.Join(t.TempDir(), defaultSnapshot)

	err := refreshSnapshot(t.Context(), registry, "us-west-2", time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC), path)
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, `# Public FULLY_MUTABLE AWS resource types in the CloudFormation registry (us-west-2, October 2026): the type name, its primary
# identifier properties, and its top-level read-only properties whose names end in "arn".
# Refresh with `+"`go run ./gen -refresh`"+` from pkg/aws/arnproperties.
AWS::EC2::VPC VpcId none
AWS::SNS::Topic TopicArn TopicArn
`, string(got))

	entries, err := readFile(path, parseSnapshot)
	require.NoError(t, err)
	require.Len(t, entries, 2)
}

func Test_RefreshSnapshot_Errors(t *testing.T) {
	path := filepath.Join(t.TempDir(), defaultSnapshot)
	now := time.Now()

	err := refreshSnapshot(t.Context(), &fakeRegistry{listErr: errors.New("access denied")}, "us-west-2", now, path)
	require.ErrorContains(t, err, "access denied")

	registry := &fakeRegistry{
		pages:   [][]string{{"AWS::A::Missing", "AWS::A::NoIdentifier"}},
		schemas: map[string]string{"AWS::A::NoIdentifier": `{}`},
	}
	err = refreshSnapshot(t.Context(), registry, "us-west-2", now, path)
	require.ErrorContains(t, err, "AWS::A::Missing: type not found")
	require.ErrorContains(t, err, "AWS::A::NoIdentifier: schema has no primary identifier")
	require.NoFileExists(t, path)
}
