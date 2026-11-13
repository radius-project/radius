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

package lakeformation

import (
	"context"
	"slices"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lakeformation"
	"github.com/aws/aws-sdk-go-v2/service/lakeformation/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// Test_Replay_DataLakeSettings adds an admin with MutationType APPEND, then sets AuthorizedSessionTagValueList,
// against AWS's recorded responses. A delete changes nothing in AWS, so the test puts back the settings it found.
func Test_Replay_DataLakeSettings(t *testing.T) {
	s := awsreplay.New(t, "testdata/datalakesettings.json")
	api := lakeformation.NewFromConfig(s.Config(t.Context()))
	run := s.Input("RADIUS_REC_RUN")
	admin := s.Input("RADIUS_REC_LAKE_PRINCIPAL_ARN")

	original, err := api.GetDataLakeSettings(t.Context(), &lakeformation.GetDataLakeSettingsInput{})
	require.NoError(t, err)
	t.Cleanup(func() {
		// t.Context() ends before cleanups run.
		_, err := api.PutDataLakeSettings(context.WithoutCancel(t.Context()), &lakeformation.PutDataLakeSettingsInput{DataLakeSettings: original.DataLakeSettings})
		if err != nil {
			t.Errorf("failed to put back the data lake settings: %v", err)
		}
	})

	properties := map[string]any{"MutationType": "APPEND", "Admins": admins(admin)}
	update := with(properties, map[string]any{"AuthorizedSessionTagValueList": list("radius-rec-" + run)})
	nativetest.Lifecycle{
		Handler:     NewDataLakeSettings(api),
		Session:     s,
		Properties:  properties,
		Want:        with(properties, map[string]any{"MutationType": nil, "Id": settingsIdentifier}),
		Update:      update,
		WantUpdated: with(update, map[string]any{"MutationType": nil}),
		SkipDelete:  true,
	}.Run(t)
}

// Test_Replay_Permissions grants a permission on a Glue database, replaces it with another, and revokes it, against
// AWS's recorded responses.
func Test_Replay_Permissions(t *testing.T) {
	s := awsreplay.New(t, "testdata/permissions.json")
	cfg := s.Config(t.Context())
	api := lakeformation.NewFromConfig(cfg)
	database := map[string]any{"DatabaseResource": map[string]any{"Name": s.Input("RADIUS_REC_GLUE_DATABASE")}}
	properties := map[string]any{
		"DataLakePrincipal": map[string]any{"DataLakePrincipalIdentifier": s.Input("RADIUS_REC_LAKE_PRINCIPAL_ARN")},
		"Resource":          database,
		"Permissions":       list("ALTER"),
	}
	nativetest.Lifecycle{
		Handler:    NewPermissions(api, sts.NewFromConfig(cfg)),
		Session:    s,
		Properties: properties,
		Update:     with(properties, map[string]any{"Permissions": list("DESCRIBE")}),
	}.Run(t)
}

// Test_Replay_Resource registers an S3 location with a role, turns on hybrid access, and deregisters it, against AWS's
// recorded responses.
func Test_Replay_Resource(t *testing.T) {
	s := awsreplay.New(t, "testdata/resource.json")
	api := lakeformation.NewFromConfig(s.Config(t.Context()))
	arn := "arn:aws:s3:::" + s.Input("RADIUS_REC_LAKE_BUCKET") + "/radius-rec-" + s.Input("RADIUS_REC_RUN")
	properties := map[string]any{"ResourceArn": arn, "RoleArn": s.Input("RADIUS_REC_LAKE_ROLE_ARN"), "UseServiceLinkedRole": false}
	update := with(properties, map[string]any{"HybridAccessEnabled": true})
	nativetest.Lifecycle{
		Handler:     NewResource(api),
		Session:     s,
		Properties:  properties,
		Want:        with(properties, map[string]any{"UseServiceLinkedRole": nil, "Id": arn}),
		Update:      update,
		WantUpdated: with(update, map[string]any{"UseServiceLinkedRole": nil}),
	}.Run(t)
}

// Test_Replay_PermissionsNextToWildcard grants a principal permissions on a Glue table and on one of its columns while
// the principal also has SELECT on every table of the database (ALL_TABLES), granted outside Radius, against AWS's
// recorded responses. The handler reads, changes, and revokes only the template's permissions: it never adopts or
// revokes the wildcard grant, and a column grant reads back as the template, so a redeploy changes nothing.
func Test_Replay_PermissionsNextToWildcard(t *testing.T) {
	s := awsreplay.New(t, "testdata/permissions-wildcard.json")
	cfg := s.Config(t.Context())
	api := lakeformation.NewFromConfig(cfg)
	h := NewPermissions(api, sts.NewFromConfig(cfg))
	database, table := s.Input("RADIUS_REC_GLUE_DATABASE"), s.Input("RADIUS_REC_GLUE_TABLE")
	principal := map[string]any{"DataLakePrincipalIdentifier": s.Input("RADIUS_REC_LAKE_PRINCIPAL_ARN")}

	// wildcardSelect reports whether the principal still has the wildcard grant made outside Radius.
	wildcardSelect := func() bool {
		out, err := api.ListPermissions(t.Context(), &lakeformation.ListPermissionsInput{
			Principal: &types.DataLakePrincipal{DataLakePrincipalIdentifier: aws.String(principal["DataLakePrincipalIdentifier"].(string))},
			Resource:  &types.Resource{Table: &types.TableResource{DatabaseName: aws.String(database), TableWildcard: &types.TableWildcard{}}},
		})
		require.NoError(t, err)
		for _, p := range out.PrincipalResourcePermissions {
			if slices.Contains(p.Permissions, types.PermissionSelect) {
				return true
			}
		}
		return false
	}
	require.True(t, wildcardSelect(), "the principal has SELECT on ALL_TABLES before the test")

	// readsAsTemplate requires the permissions to read back as the template, so a redeploy has nothing to change.
	readsAsTemplate := func(identifier string, properties map[string]any) {
		got, err := h.ReadScoped(t.Context(), awsreplay.Region, identifier, []map[string]any{properties})
		require.NoError(t, err)
		for _, name := range []string{"DataLakePrincipal", "Resource", "Permissions", "PermissionsWithGrantOption"} {
			require.Equal(t, properties[name], got[name], name)
		}
	}

	tableGrant := map[string]any{
		"DataLakePrincipal": principal,
		"Resource":          map[string]any{"TableResource": map[string]any{"DatabaseName": database, "Name": table}},
		"Permissions":       list("DESCRIBE"),
	}
	tableUpdate := with(tableGrant, map[string]any{"Permissions": list("ALTER", "DESCRIBE")})
	identifier := nativetest.Lifecycle{
		Handler:    h,
		Session:    s,
		Properties: tableGrant,
		Update:     tableUpdate,
		Between: func() {
			readsAsTemplate(identifierOf(t, tableGrant), tableGrant)
			require.True(t, wildcardSelect())
		},
	}.Run(t)
	require.NotEmpty(t, identifier)
	require.True(t, wildcardSelect(), "updating and revoking the table grant leaves the wildcard grant")

	columnGrant := map[string]any{
		"DataLakePrincipal": principal,
		"Resource": map[string]any{"TableWithColumnsResource": map[string]any{
			"DatabaseName": database, "Name": table, "ColumnNames": list("c1"),
		}},
		"Permissions": list("SELECT"),
	}
	nativetest.Lifecycle{
		Handler:    h,
		Session:    s,
		Properties: columnGrant,
		Between: func() {
			readsAsTemplate(identifierOf(t, columnGrant), columnGrant)
		},
	}.Run(t)
	require.True(t, wildcardSelect(), "revoking the column grant leaves the wildcard grant")
}
