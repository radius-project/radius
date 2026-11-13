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
	"maps"
	"os"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/aws/schemas"
	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

func schemaFile(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/native/" + file)
	require.NoError(t, err)
	return string(b)
}

// fakes maps each handler the suite makes to its fake API.
type fakes struct {
	mu  sync.Mutex
	api map[native.Handler]*fakeAPI
}

func (f *fakes) add(h native.Handler, api *fakeAPI) native.Handler {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.api == nil {
		f.api = map[native.Handler]*fakeAPI{}
	}
	f.api[h] = api
	return h
}

func (f *fakes) of(h native.Handler) *fakeAPI {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.api[h]
}

func with(properties map[string]any, changes map[string]any) map[string]any {
	out := maps.Clone(properties)
	for k, v := range changes {
		if v == nil {
			delete(out, k)
		} else {
			out[k] = v
		}
	}
	return out
}

func list(values ...string) []any {
	return anyList(values)
}

func admins(ids ...string) []any {
	out := []any{}
	for _, id := range ids {
		out = append(out, map[string]any{"DataLakePrincipalIdentifier": id})
	}
	return out
}

const (
	adminA  = "arn:aws:iam::123456789012:role/lake-admin-a"
	adminB  = "arn:aws:iam::123456789012:role/lake-admin-b"
	outside = "arn:aws:iam::123456789012:role/outside-admin"
	reader  = "arn:aws:iam::123456789012:role/lake-reader"
	analyst = "arn:aws:iam::123456789012:role/analyst"
	region  = "us-west-2"
)

func settingsSuite(t *testing.T, mutation string) nativetest.Suite {
	settings := map[string]any{
		"Admins":                admins(adminA),
		"ReadOnlyAdmins":        admins(reader),
		"TrustedResourceOwners": list("111122223333", "444455556666"),
		"Parameters":            map[string]any{"CROSS_ACCOUNT_VERSION": "4"},
	}
	if mutation != "" {
		settings["MutationType"] = mutation
	}
	return nativetest.Suite{
		TypeName: settingsType,
		Schema:   schemaFile(t, "aws-lakeformation-datalakesettings.json"),
		Metadata: schemaFile(t, "aws-lakeformation-datalakesettings.metadata.json"),
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return NewDataLakeSettings(newFakeAPI(store))
		},
		IdentifierProperty: "Id",
		WriteOnly:          []string{"MutationType"},
		Properties:         settings,
		Identifier:         settingsIdentifier,
		Changed:            with(settings, map[string]any{"Admins": admins(adminA, adminB), "AllowFullTableExternalDataAccess": true}),
		Removable:          "ReadOnlyAdmins",
		OtherForm:          map[string]any{"TrustedResourceOwners": list("444455556666", "111122223333")},
	}
}

func Test_Suite_DataLakeSettings(t *testing.T) {
	s := settingsSuite(t, "")
	// An account has one set of settings in a region, whatever the name.
	s.SameResourceNames = []string{"other-settings"}
	nativetest.Run(t, s)
}

func Test_Suite_DataLakeSettings_Append(t *testing.T) {
	nativetest.Run(t, settingsSuite(t, mutationAppend))
}

var salesDatabase = map[string]any{"DatabaseResource": map[string]any{"Name": "sales"}}

func permissionsOf(resource map[string]any, perms []any, grants []any) map[string]any {
	out := map[string]any{
		"DataLakePrincipal": map[string]any{"DataLakePrincipalIdentifier": analyst},
		"Resource":          resource,
		"Permissions":       perms,
	}
	if grants != nil {
		out["PermissionsWithGrantOption"] = grants
	}
	return out
}

func identifierOf(t *testing.T, properties map[string]any) string {
	id, err := NewPermissions(nil, nil).Identifier(properties)
	require.NoError(t, err)
	return id
}

// permissionsSuite returns the suite for permissions on resource. perms and grants are the template's permissions,
// changed the permissions of an update, and multi and multiGrants those of an update that revokes and then grants.
// outsider is a permission granted outside Radius. The lists are in the order the fake lists them.
func permissionsSuite(t *testing.T, resource map[string]any, other map[string]any, perms []any, grants []any, changed []any, multi []any, multiGrants []any, outsider string) nativetest.Suite {
	var fs fakes
	properties := permissionsOf(resource, perms, grants)
	reversed := slices.Clone(perms)
	slices.Reverse(reversed)
	return nativetest.Suite{
		TypeName: permissionsType,
		Schema:   schemaFile(t, "aws-lakeformation-permissions.json"),
		Metadata: schemaFile(t, "aws-lakeformation-permissions.metadata.json"),
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			api := newFakeAPI(store)
			return fs.add(&trackedPermissions{Permissions: NewPermissions(api, api), api: api}, api)
		},
		IdentifierProperty: "Id",
		Properties:         properties,
		Identifier:         identifierOf(t, properties),
		Changed:            with(properties, map[string]any{"Permissions": changed}),
		Removable:          "PermissionsWithGrantOption",
		CreateOnlyChange:   with(properties, map[string]any{"Resource": other}),
		OtherForm:          map[string]any{"Permissions": reversed},
		MultiCall:          with(properties, map[string]any{"Permissions": multi, "PermissionsWithGrantOption": multiGrants}),
		Calls:              2,
		RejectedCalls:      1,
		FailCall: nativetest.FailCall(2, func(h native.Handler) *nativetest.Failer {
			return &fs.of(h).failer
		}),
		Outside: func(store *awsfake.CloudControl, properties map[string]any) func() bool {
			id := identifierOf(t, properties)
			entry := store.Resource(outsideType, id)
			if entry == nil {
				entry = with(properties, map[string]any{"Permissions": list(), "PermissionsWithGrantOption": nil})
			}
			entry["Permissions"] = append(entry["Permissions"].([]any), outsider)
			store.Put(outsideType, id, entry)
			return func() bool {
				e := store.Resource(outsideType, id)
				return e != nil && slices.Contains(e["Permissions"].([]any), any(outsider))
			}
		},
	}
}

func Test_Suite_Permissions(t *testing.T) {
	s := permissionsSuite(t, salesDatabase, map[string]any{"DatabaseResource": map[string]any{"Name": "marketing"}},
		list("ALTER", "DESCRIBE"), list("DESCRIBE"), list("DESCRIBE", "DROP"), list("DESCRIBE", "DROP"), list("DROP"), "CREATE_TABLE")
	// The same database: in the current account's catalog, named, and in other case, which Glue ignores.
	s.SameResource = sameTargets(s.Properties,
		map[string]any{"DatabaseResource": map[string]any{"CatalogId": account, "Name": "sales"}},
		map[string]any{"DatabaseResource": map[string]any{"Name": "SALES"}},
	)
	// The same database in other forms.
	s.SameResourceNames = []string{
		identifierOf(t, with(s.Properties, map[string]any{"Resource": map[string]any{"DatabaseResource": map[string]any{"CatalogId": account, "Name": "sales"}}})),
		identifierOf(t, with(s.Properties, map[string]any{"Resource": map[string]any{"DatabaseResource": map[string]any{"Name": "SALES"}}})),
	}
	nativetest.Run(t, s)
}

// sameTargets returns properties with each of resources, every one the same Lake Formation resource as properties's.
func sameTargets(properties map[string]any, resources ...map[string]any) []map[string]any {
	out := []map[string]any{}
	for _, r := range resources {
		out = append(out, with(properties, map[string]any{"Resource": r}))
	}
	return out
}

// Test_Suite_Permissions_Table runs the suite for permissions on a table, which ListPermissions returns in other
// shapes than they were granted: SELECT on the table's columns, and the others on the table.
func Test_Suite_Permissions_Table(t *testing.T) {
	orders := map[string]any{"TableResource": map[string]any{"DatabaseName": "sales", "Name": "orders"}}
	returns := map[string]any{"TableResource": map[string]any{"DatabaseName": "sales", "Name": "returns"}}
	s := permissionsSuite(t, orders, returns,
		list("SELECT", "ALTER"), list("SELECT"), list("SELECT", "DROP"), list("SELECT", "DROP"), list("DROP"), "INSERT")
	// The same table: as every column of it, in the current account's catalog, and in other case, which Glue ignores.
	s.SameResource = sameTargets(s.Properties,
		map[string]any{"TableWithColumnsResource": map[string]any{"DatabaseName": "sales", "Name": "orders", "ColumnWildcard": map[string]any{}}},
		map[string]any{"TableResource": map[string]any{"CatalogId": account, "DatabaseName": "sales", "Name": "orders"}},
		map[string]any{"TableResource": map[string]any{"DatabaseName": "Sales", "Name": "ORDERS"}},
		// Empty values the request leaves out.
		map[string]any{"TableWithColumnsResource": map[string]any{"CatalogId": "", "DatabaseName": "sales", "Name": "orders", "ColumnNames": list(), "ColumnWildcard": map[string]any{"ExcludedColumnNames": list()}}},
	)
	s.SameResourceNames = []string{identifierOf(t, with(s.Properties, map[string]any{"Resource": map[string]any{
		"TableWithColumnsResource": map[string]any{"DatabaseName": "sales", "Name": "orders", "ColumnWildcard": map[string]any{}},
	}}))}
	nativetest.Run(t, s)
}

// Test_Suite_Permissions_Columns runs the suite for permissions on some columns of a table, which AWS keeps whatever
// the order or case of the column names.
func Test_Suite_Permissions_Columns(t *testing.T) {
	columns := func(names ...string) map[string]any {
		return map[string]any{"TableWithColumnsResource": map[string]any{"DatabaseName": "sales", "Name": "orders", "ColumnNames": list(names...)}}
	}
	s := permissionsSuite(t, columns("id", "total"), columns("id"),
		list("SELECT"), nil, list("SELECT"), list("SELECT"), nil, "INSERT")
	s.SameResource = sameTargets(s.Properties, columns("total", "id"), columns("ID", "Total"))
	// Lake Formation grants only SELECT on columns, so no update both revokes and grants a permission.
	s.MultiCall, s.RejectedCalls = nil, 0
	nativetest.Run(t, s)
}

const (
	dataArn   = "arn:aws:s3:::radius-lake/data"
	roleArn   = "arn:aws:iam::123456789012:role/lake-access"
	otherRole = "arn:aws:iam::123456789012:role/lake-access-2"
)

func Test_Suite_Resource(t *testing.T) {
	// An S3 location with a path has "/" in its ARN, which UCP keeps raw as the identifier and escapes only in the
	// resource IDs a list and `existing` return.
	for name, arn := range map[string]string{"bucket": "arn:aws:s3:::radius-lake", "S3 path": dataArn} {
		t.Run(name, func(t *testing.T) { resourceSuite(t, arn) })
	}
}

func resourceSuite(t *testing.T, dataArn string) {
	var fs fakes
	// UseServiceLinkedRole is left out, which means false. The suite looks for each write-only value in every response
	// as text, and Read returns WithFederation false.
	resource := map[string]any{"ResourceArn": dataArn, "RoleArn": roleArn, "HybridAccessEnabled": true}
	nativetest.Run(t, nativetest.Suite{
		TypeName: resourceType,
		Schema:   schemaFile(t, "aws-lakeformation-resource.json"),
		Metadata: schemaFile(t, "aws-lakeformation-resource.metadata.json"),
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			api := newFakeAPI(store)
			return fs.add(NewResource(api), api)
		},
		IdentifierProperty: "ResourceArn",
		WriteOnly:          []string{"UseServiceLinkedRole"},
		Properties:         resource,
		Identifier:         dataArn,
		Changed:            with(resource, map[string]any{"RoleArn": otherRole}),
		CreateOnlyChange:   with(resource, map[string]any{"ResourceArn": "arn:aws:s3:::radius-lake/other"}),
		MultiCall:          with(resource, map[string]any{"UseServiceLinkedRole": true}),
		Calls:              2,
		FailCall: nativetest.FailCall(2, func(h native.Handler) *nativetest.Failer {
			return &fs.of(h).failer
		}),
	})
}

// Test_Registry registers the handlers with the schemas and metadata built into Radius, as UCP does.
func Test_Registry(t *testing.T) {
	catalog, err := schemas.Load()
	require.NoError(t, err)
	registry, err := native.NewRegistry(catalog, NewDataLakeSettings(nil), NewPermissions(nil, nil), NewResource(nil))
	require.NoError(t, err)

	settings := registry.Lookup(settingsType)
	require.Equal(t, schemas.IdentityScopeRegion, settings.Metadata.IdentityScope)
	require.Equal(t, schemas.OnDeleteNothing, settings.Metadata.OnDelete)
	require.Equal(t, []string{"MutationType"}, settings.WriteOnly)
	require.Equal(t, schemas.OnDeleteDelete, registry.Lookup(permissionsType).Metadata.OnDelete)
	resource := registry.Lookup(resourceType)
	require.Equal(t, schemas.OnDeleteDelete, resource.Metadata.OnDelete)
	require.Equal(t, []string{"UseServiceLinkedRole"}, resource.WriteOnly)
}
