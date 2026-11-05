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
	"math"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lakeformation/types"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativefake"
)

func newPermissions() (*trackedPermissions, *fakeAPI, *awsfake.CloudControl) {
	store := awsfake.NewCloudControl()
	api := newFakeAPI(store)
	return &trackedPermissions{Permissions: NewPermissions(api), api: api}, api, store
}

// grantOutside grants perms to the analyst on resource outside Radius.
func grantOutside(t *testing.T, store *awsfake.CloudControl, resource map[string]any, perms ...string) {
	properties := permissionsOf(resource, list(perms...), nil)
	store.Put(outsideType, identifierOf(t, properties), properties)
}

func requireRejected(t *testing.T, err error, message string) {
	t.Helper()
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.Contains(t, failure.Message, message)
}

// Test_Permissions_Resources grants, reads, and revokes permissions on each kind of resource, next to permissions on
// the same table granted outside Radius.
func Test_Permissions_Resources(t *testing.T) {
	table := map[string]any{"TableResource": map[string]any{"DatabaseName": "sales", "Name": "orders"}}
	tests := []struct {
		name     string
		resource map[string]any
	}{
		{"database in a catalog", map[string]any{"DatabaseResource": map[string]any{"CatalogId": account, "Name": "sales"}}},
		{"table", table},
		{"every table", map[string]any{"TableResource": map[string]any{"DatabaseName": "sales", "TableWildcard": map[string]any{}}}},
		{"columns", map[string]any{"TableWithColumnsResource": map[string]any{"DatabaseName": "sales", "Name": "orders", "ColumnNames": list("id", "total")}}},
		{"every column but some", map[string]any{"TableWithColumnsResource": map[string]any{
			"DatabaseName": "sales", "Name": "orders", "ColumnWildcard": map[string]any{"ExcludedColumnNames": list("card")},
		}}},
		{"data location", map[string]any{"DataLocationResource": map[string]any{"S3Resource": "arn:aws:s3:::radius-lake/data"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			h, _, store := newPermissions()
			grantOutside(t, store, table, "ALTER")
			properties := permissionsOf(tt.resource, list("SELECT"), list("SELECT"))
			id, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: properties})
			require.NoError(t, err)
			require.Equal(t, identifierOf(t, properties), id)

			got, err := h.ReadScoped(ctx, region, id, []map[string]any{properties})
			require.NoError(t, err)
			require.Equal(t, list("SELECT"), got["Permissions"])
			require.Equal(t, list("SELECT"), got["PermissionsWithGrantOption"])
			require.Equal(t, tt.resource, got["Resource"])
			require.Equal(t, id, got["Id"])

			require.NoError(t, h.DeleteScoped(ctx, region, id, []map[string]any{properties}))
			_, err = h.Read(ctx, region, id)
			if _, isTable := tt.resource["TableResource"]; isTable && tt.name == "table" {
				require.NoError(t, err, "the permission granted outside Radius stays")
			} else {
				require.ErrorIs(t, err, native.ErrNotFound)
			}
		})
	}
}

// Test_Permissions_GrantOptionOnly deploys a template that sets only PermissionsWithGrantOption. The permissions
// themselves are granted too, and a delete revokes them.
func Test_Permissions_GrantOptionOnly(t *testing.T) {
	ctx := t.Context()
	h, _, store := newPermissions()
	properties := permissionsOf(salesDatabase, nil, list("DESCRIBE"))
	delete(properties, "Permissions")
	id, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)
	got, err := h.ReadScoped(ctx, region, id, []map[string]any{properties})
	require.NoError(t, err)
	require.Equal(t, list("DESCRIBE"), got["Permissions"])

	// Adding a permission keeps the one with grant option.
	desired := with(properties, map[string]any{"Permissions": list("ALTER")})
	require.NoError(t, h.Update(ctx, native.UpdateRequest{Region: region, Identifier: id, Current: got, Desired: desired, Patch: patchOf(t, got, desired)}))
	got, err = h.Read(ctx, region, id)
	require.NoError(t, err)
	require.Equal(t, list("DESCRIBE", "ALTER"), got["Permissions"])
	require.Equal(t, list("DESCRIBE"), got["PermissionsWithGrantOption"])

	require.NoError(t, h.DeleteScoped(ctx, region, id, []map[string]any{properties, desired}))
	require.Zero(t, store.Count(permissionsType))
}

// Test_Permissions_IdentifierFromCloudControl reads and deletes permissions whose identifier Cloud Control assigned,
// so the principal and resource come from the properties UCP has.
func Test_Permissions_IdentifierFromCloudControl(t *testing.T) {
	ctx := t.Context()
	h, _, store := newPermissions()
	properties := permissionsOf(salesDatabase, list("ALTER"), nil)
	store.Put(permissionsType, "id-1", properties)

	_, err := h.Read(ctx, region, "id-1")
	require.ErrorContains(t, err, "names no DataLakePrincipal and Resource")
	got, err := h.ReadScoped(ctx, region, "id-1", []map[string]any{{"Permissions": list("ALTER")}, properties})
	require.NoError(t, err)
	require.Equal(t, list("ALTER"), got["Permissions"])
	require.NoError(t, h.DeleteScoped(ctx, region, "id-1", []map[string]any{properties}))
	require.Zero(t, store.Count(permissionsType))
}

// Test_Permissions_Delete checks that Delete fails and that DeleteScoped without managed permissions revokes nothing,
// so a delete never revokes permissions granted outside Radius.
func Test_Permissions_Delete(t *testing.T) {
	ctx := t.Context()
	h, api, store := newPermissions()
	grantOutside(t, store, salesDatabase, "CREATE_TABLE")
	properties := permissionsOf(salesDatabase, list("ALTER"), nil)
	id, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)
	api.takeCalls()

	require.ErrorContains(t, h.Delete(ctx, region, id), "DeleteScoped")
	require.ErrorIs(t, h.DeleteScoped(ctx, region, id, nil), native.ErrNotFound)
	require.Empty(t, api.takeCalls(), "nothing is revoked")
	require.Equal(t, 1, store.Count(permissionsType))
	require.Equal(t, 1, store.Count(outsideType))

	require.NoError(t, h.DeleteScoped(ctx, region, id, []map[string]any{properties}))
	require.Zero(t, store.Count(permissionsType))
	require.Equal(t, 1, store.Count(outsideType), "a delete keeps a permission granted outside Radius")
}

// Test_Permissions_TableShapes grants permissions on tables that ListPermissions returns in other shapes than they were
// granted, and checks that a read finds them, a redeploy changes nothing, and a delete revokes them.
func Test_Permissions_TableShapes(t *testing.T) {
	tests := []struct {
		name     string
		resource map[string]any
		perms    []any
	}{
		{"SELECT only on a table", map[string]any{"TableResource": map[string]any{"DatabaseName": "sales", "Name": "orders"}}, list("SELECT")},
		{"SELECT and others on a table", map[string]any{"TableResource": map[string]any{"DatabaseName": "sales", "Name": "orders"}}, list("SELECT", "ALTER")},
		{"every table", map[string]any{"TableResource": map[string]any{"DatabaseName": "sales", "TableWildcard": map[string]any{}}}, list("SELECT", "DESCRIBE")},
		{"every table by name", map[string]any{"TableResource": map[string]any{"DatabaseName": "sales", "Name": allTables}}, list("SELECT", "DESCRIBE")},
		{"every column", map[string]any{"TableWithColumnsResource": map[string]any{"DatabaseName": "sales", "Name": "orders", "ColumnWildcard": map[string]any{}}}, list("SELECT", "ALTER")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			h, api, store := newPermissions()
			properties := permissionsOf(tt.resource, tt.perms, list("SELECT"))
			id, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: properties})
			require.NoError(t, err)

			got, err := h.ReadScoped(ctx, region, id, []map[string]any{properties})
			require.NoError(t, err)
			require.Equal(t, tt.perms, got["Permissions"])
			require.Equal(t, list("SELECT"), got["PermissionsWithGrantOption"])

			api.takeCalls()
			require.NoError(t, h.Update(ctx, native.UpdateRequest{Region: region, Identifier: id, Current: got, Desired: properties, Patch: patchOf(t, got, properties)}))
			require.Empty(t, api.takeCalls(), "a redeploy grants nothing again")

			require.NoError(t, h.DeleteScoped(ctx, region, id, []map[string]any{properties}))
			require.Zero(t, store.Count(permissionsType))
		})
	}
}

func Test_Permissions_Errors(t *testing.T) {
	ctx := t.Context()
	h, api, _ := newPermissions()
	create := func(properties map[string]any) error {
		_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: properties})
		return err
	}

	requireRejected(t, create(map[string]any{"Resource": salesDatabase, "Permissions": list("ALTER")}), "Set DataLakePrincipal and Resource")
	noID := permissionsOf(salesDatabase, list("ALTER"), nil)
	noID["DataLakePrincipal"] = map[string]any{"Other": "x"}
	requireRejected(t, create(noID), "DataLakePrincipalIdentifier")
	requireRejected(t, create(permissionsOf(map[string]any{"CatalogResource": map[string]any{}}, list("ALTER"), nil)), "Set one of DatabaseResource")
	requireRejected(t, create(permissionsOf(salesDatabase, list(), nil)), "Set Permissions or PermissionsWithGrantOption")
	_, err := h.Permissions.Create(ctx, native.CreateRequest{Region: region, Properties: permissionsOf(map[string]any{"DatabaseResource": map[string]any{"Name": math.NaN()}}, list("ALTER"), nil)})
	require.ErrorContains(t, err, "unsupported value")

	api.errs["GrantPermissions"] = nativefake.ClientError(notFoundCode, "no such database")
	require.ErrorContains(t, create(permissionsOf(salesDatabase, list("ALTER"), nil)), notFoundCode)
	delete(api.errs, "GrantPermissions")

	properties := permissionsOf(salesDatabase, list("ALTER", "DESCRIBE"), nil)
	id := identifierOf(t, properties)
	require.NoError(t, create(properties))

	api.errs["ListPermissions"] = nativefake.ClientError(notFoundCode, "no such database")
	_, err = h.Read(ctx, region, id)
	require.ErrorIs(t, err, native.ErrNotFound)
	failure := nativefake.ServerError("InternalServiceException", "failed")
	api.errs["ListPermissions"] = failure
	_, err = h.Read(ctx, region, id)
	require.ErrorIs(t, err, failure)
	require.ErrorIs(t, h.DeleteScoped(ctx, region, id, []map[string]any{properties}), failure)
	delete(api.errs, "ListPermissions")

	// A failed revoke is rejected, even when AWS's answer doesn't show whether it took effect.
	desired := with(properties, map[string]any{"Permissions": list("DESCRIBE")})
	for _, tt := range []struct {
		err    error
		client bool
	}{
		{nativefake.ClientError("InvalidInputException", "invalid"), true},
		{nativefake.ServerError("InternalServiceException", "failed"), false},
	} {
		api.errs["RevokePermissions"] = tt.err
		err = h.Update(ctx, native.UpdateRequest{Region: region, Identifier: id, Current: properties, Desired: desired, Patch: patchOf(t, properties, desired)})
		var f *native.Failure
		require.ErrorAs(t, err, &f)
		require.Equal(t, native.OutcomeRejected, f.Outcome)
		require.Equal(t, tt.client, f.ClientError)
	}
	delete(api.errs, "RevokePermissions")

	err = h.Update(ctx, native.UpdateRequest{Region: region, Identifier: "id-1", Current: map[string]any{}, Desired: map[string]any{}})
	require.ErrorContains(t, err, "names no DataLakePrincipal and Resource")
	err = h.DeleteScoped(ctx, region, "id-1", []map[string]any{{}})
	require.ErrorContains(t, err, "names no DataLakePrincipal and Resource")
	_, err = h.ReadScoped(ctx, region, "id-1", []map[string]any{permissionsOf(map[string]any{"Other": map[string]any{}}, nil, nil)})
	requireRejected(t, err, "Set one of DatabaseResource")
}

func Test_Matches(t *testing.T) {
	db := func(catalog *string, name string) *types.Resource {
		return &types.Resource{Database: &types.DatabaseResource{CatalogId: catalog, Name: aws.String(name)}}
	}
	table := func(name string, wildcard bool) *types.Resource {
		r := &types.Resource{Table: &types.TableResource{DatabaseName: aws.String("sales"), Name: aws.String(name)}}
		if wildcard {
			r.Table.TableWildcard = &types.TableWildcard{}
		}
		return r
	}
	columns := func(names []string, excluded []string) *types.Resource {
		r := &types.Resource{TableWithColumns: &types.TableWithColumnsResource{DatabaseName: aws.String("Sales"), Name: aws.String("Orders"), ColumnNames: names}}
		if excluded != nil {
			r.TableWithColumns.ColumnWildcard = &types.ColumnWildcard{ExcludedColumnNames: excluded}
		}
		return r
	}
	allColumns := func(name string) *types.Resource {
		return &types.Resource{TableWithColumns: &types.TableWithColumnsResource{DatabaseName: aws.String("sales"), Name: aws.String(name), ColumnWildcard: &types.ColumnWildcard{}}}
	}
	location := func(arn string) *types.Resource {
		return &types.Resource{DataLocation: &types.DataLocationResource{ResourceArn: aws.String(arn)}}
	}
	tests := []struct {
		name string
		want *types.Resource
		got  *types.Resource
		same bool
	}{
		{"database in any catalog", db(nil, "Sales"), db(aws.String(account), "sales"), true},
		{"database in another catalog", db(aws.String("111122223333"), "sales"), db(aws.String(account), "sales"), false},
		{"another database", db(nil, "sales"), db(nil, "marketing"), false},
		{"table", table("orders", false), table("ORDERS", false), true},
		{"every table for a table", table("", true), table("", false), false},
		{"columns in another order", columns([]string{"id", "Total"}, nil), columns([]string{"total", "id"}, nil), true},
		{"other columns", columns([]string{"id"}, nil), columns([]string{"total"}, nil), false},
		{"every column but some", columns(nil, []string{"card"}), columns(nil, []string{"Card"}), true},
		{"every column but others", columns(nil, []string{"card"}), columns(nil, []string{"ssn"}), false},
		{"columns for every column", columns([]string{"id"}, nil), columns(nil, []string{}), false},
		{"columns for a table", columns([]string{"id"}, nil), table("orders", false), false},
		{"data location", location("arn:aws:s3:::a"), location("arn:aws:s3:::a"), true},
		{"another data location", location("arn:aws:s3:::a"), location("arn:aws:s3:::b"), false},
		{"table listed as every column", table("orders", false), columns(nil, []string{}), true},
		{"every column listed as a table", columns(nil, []string{}), table("orders", false), true},
		{"every column but some for a table", columns(nil, []string{"card"}), table("orders", false), false},
		{"table for some columns", table("orders", false), columns([]string{"id"}, nil), false},
		{"every table listed by name", table("", true), table(allTables, false), true},
		{"every table listed with the wildcard", table("", true), table(allTables, true), true},
		{"every table listed as every column", table("", true), columns(nil, []string{}), false},
		{"every table listed as every column of ALL_TABLES", table("", true), allColumns(allTables), true},
		{"every table for one table", table("", true), table("orders", false), false},
		{"nothing", db(nil, "sales"), nil, false},
		{"no kind", &types.Resource{}, db(nil, "sales"), false},
	}
	for _, tt := range tests {
		require.Equal(t, tt.same, matches(tt.want, tt.got), tt.name)
	}
}

func Test_Permissions_Identifier(t *testing.T) {
	h := NewPermissions(nil)
	id, err := h.Identifier(map[string]any{
		"Resource":          map[string]any{"TableResource": map[string]any{"Name": "t", "DatabaseName": "d"}},
		"DataLakePrincipal": map[string]any{"DataLakePrincipalIdentifier": analyst},
		"Permissions":       list("SELECT"),
	})
	require.NoError(t, err)
	require.Equal(t, `{"DataLakePrincipal":{"DataLakePrincipalIdentifier":"`+analyst+`"},"Resource":{"TableResource":{"DatabaseName":"d","Name":"t"}}}`, id)

	id, err = h.Identifier(map[string]any{"Resource": salesDatabase})
	require.NoError(t, err)
	require.Empty(t, id)
}
