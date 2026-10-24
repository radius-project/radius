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
	"os"
	"path/filepath"
	"strings"
	"testing"

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
}
