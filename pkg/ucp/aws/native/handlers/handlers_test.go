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

package handlers

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/aws/awstypes"
	"github.com/radius-project/radius/pkg/aws/schemas"
	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativefake"
)

// Test_NewRegistry checks that every registered handler has a schema built into Radius.
func Test_NewRegistry(t *testing.T) {
	registry, err := NewRegistry(aws.Config{})
	require.NoError(t, err)
	for _, h := range All(aws.Config{}) {
		require.NotNil(t, registry.Lookup(h.TypeName()), h.TypeName())
	}
}

// Test_HandWrittenSchemas checks the hand-written schemas built into Radius. Each has a handler, so bicep-types-aws
// never publishes a type UCP can't serve, and none has the name of a legacy or deprecated CloudFormation type, which
// UCP rejects before it picks a handler. pkg/aws/schemas already fails to load a type with a schema in both native/
// and handwritten/.
func Test_HandWrittenSchemas(t *testing.T) {
	catalog, err := schemas.Load()
	require.NoError(t, err)
	require.Empty(t, withoutHandler(catalog, All(aws.Config{})), "add a handler to All for each hand-written schema")
	require.Empty(t, unsupportedNames(catalog), "a hand-written type must not have the name of a legacy or deprecated CloudFormation type")
}

func Test_withoutHandler(t *testing.T) {
	store := awsfake.NewCloudControl()
	catalog := schemas.Catalog{
		Native:      map[string]string{"AWS::DocDB::DBCluster": `{}`},
		HandWritten: map[string]string{"AWS::S3::Object": `{}`, "AWS::KMS::Grant": `{}`, "AWS::IAM::AccountPasswordPolicy": `{}`},
	}
	handlers := []native.Handler{nativefake.New("aws::s3::object", store)}
	require.Equal(t, []string{"AWS::IAM::AccountPasswordPolicy", "AWS::KMS::Grant"}, withoutHandler(catalog, handlers), "a native schema with no handler isn't hand-written")
	require.Empty(t, withoutHandler(schemas.Catalog{}, handlers))
}

func Test_unsupportedNames(t *testing.T) {
	catalog := schemas.Catalog{HandWritten: map[string]string{
		"AWS::S3::Object":               `{}`,
		"aws::iam::usertogroupaddition": `{}`,
		"AWS::AppMesh::Mesh":            `{}`,
	}}
	require.Equal(t, []string{"AWS::AppMesh::Mesh", "aws::iam::usertogroupaddition"}, unsupportedNames(catalog))
	require.Empty(t, unsupportedNames(schemas.Catalog{HandWritten: map[string]string{"AWS::S3::Object": `{}`}}))
}

// withoutHandler returns the hand-written types in catalog that no handler serves, sorted.
func withoutHandler(catalog schemas.Catalog, handlers []native.Handler) []string {
	served := map[string]bool{}
	for _, h := range handlers {
		served[strings.ToLower(h.TypeName())] = true
	}
	missing := []string{}
	for _, typeName := range slices.Sorted(maps.Keys(catalog.HandWritten)) {
		if !served[strings.ToLower(typeName)] {
			missing = append(missing, typeName)
		}
	}
	return missing
}

// unsupportedNames returns the hand-written types in catalog that have the name of a legacy or deprecated type in
// pkg/aws/awstypes, sorted.
func unsupportedNames(catalog schemas.Catalog) []string {
	clashes := []string{}
	for _, typeName := range slices.Sorted(maps.Keys(catalog.HandWritten)) {
		if _, unsupported := awstypes.UnsupportedMessage(typeName); unsupported {
			clashes = append(clashes, typeName)
		}
	}
	return clashes
}
