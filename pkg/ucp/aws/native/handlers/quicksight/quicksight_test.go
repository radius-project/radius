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

package quicksight

import (
	"maps"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/quicksight"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

func readFile(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/handwritten/" + file)
	require.NoError(t, err)
	return string(b)
}

func with(properties map[string]any, changes map[string]any) map[string]any {
	out := maps.Clone(properties)
	maps.Copy(out, changes)
	return out
}

// membershipTemplate returns a template that adds the dashboard dash-1 to the folder sales.
func membershipTemplate() map[string]any {
	return map[string]any{"AwsAccountId": account, "FolderId": "sales", "MemberType": "DASHBOARD", "MemberId": "dash-1"}
}

// aliasTemplate returns a template for the alias live of the template report, pointing to version 2.
func aliasTemplate() map[string]any {
	return map[string]any{"AwsAccountId": account, "TemplateId": "report", "AliasName": "live", "TemplateVersionNumber": float64(2)}
}

func Test_Suite_FolderMembership(t *testing.T) {
	properties := membershipTemplate()
	nativetest.Run(t, nativetest.Suite{
		TypeName:    folderMembershipType,
		Schema:      readFile(t, "aws-quicksight-foldermembership.json"),
		Metadata:    readFile(t, "aws-quicksight-foldermembership.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return NewFolderMembership(newFakeAPI(store))
		},
		Properties: properties,
		Identifier: strings.Join([]string{account, "sales", "DASHBOARD", "dash-1"}, "|"),
		// Every property is create-only, so a deploy of the same template is the only update.
		Changed:          properties,
		CreateOnlyChange: with(properties, map[string]any{"MemberId": "dash-2"}),
		Invalid: []map[string]any{
			with(properties, map[string]any{"MemberType": "REPORT"}),
			with(properties, map[string]any{"FolderId": "sales folder"}),
			with(properties, map[string]any{"AwsAccountId": "1234"}),
			with(properties, map[string]any{"MemberId": 7}),
			with(properties, map[string]any{"MemberId": strings.Repeat("a", maxIDLength+1)}),
		},
		IdentifierProperties: func(identifier string) map[string]any {
			id, err := parseMembershipID(identifier)
			require.NoError(t, err)
			return map[string]any{"AwsAccountId": id.account, "FolderId": id.folder, "MemberType": string(id.kind), "MemberId": id.member}
		},
		API: membershipAPI,
	})
}

func Test_Suite_TemplateAlias(t *testing.T) {
	properties := aliasTemplate()
	nativetest.Run(t, nativetest.Suite{
		TypeName:    templateAliasType,
		Schema:      readFile(t, "aws-quicksight-templatealias.json"),
		Metadata:    readFile(t, "aws-quicksight-templatealias.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return NewTemplateAlias(newFakeAPI(store))
		},
		Properties:       properties,
		Identifier:       strings.Join([]string{account, "report", "live"}, "|"),
		Changed:          with(properties, map[string]any{"TemplateVersionNumber": float64(3)}),
		CreateOnlyChange: with(properties, map[string]any{"AliasName": "staging"}),
		Invalid: []map[string]any{
			with(properties, map[string]any{"AliasName": "$LATEST"}),
			with(properties, map[string]any{"AliasName": "live alias"}),
			with(properties, map[string]any{"TemplateId": strings.Repeat("a", maxTemplateIDLength+1)}),
			with(properties, map[string]any{"TemplateVersionNumber": float64(0)}),
			with(properties, map[string]any{"TemplateVersionNumber": 1.5}),
			with(properties, map[string]any{"TemplateVersionNumber": "2"}),
		},
		IdentifierProperties: func(identifier string) map[string]any {
			id, err := parseAliasID(identifier)
			require.NoError(t, err)
			return map[string]any{"AwsAccountId": id.account, "TemplateId": id.template, "AliasName": id.alias}
		},
		API: aliasAPI,
	})
}

func Test_InRegion(t *testing.T) {
	opts := quicksight.Options{}
	inRegion("eu-west-2")(&opts)
	require.Equal(t, "eu-west-2", opts.Region)
}
