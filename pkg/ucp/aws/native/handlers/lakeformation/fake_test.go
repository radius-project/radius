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
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lakeformation"
	"github.com/aws/aws-sdk-go-v2/service/lakeformation/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativefake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	account = "123456789012"

	// outsideType is the pseudo type the fake keeps permissions granted outside Radius under, keyed by the parent.
	outsideType = "outside::" + permissionsType

	slrArn = "arn:aws:iam::" + account + ":role/aws-service-role/lakeformation.amazonaws.com/AWSServiceRoleForLakeFormationDataAccess"
)

// fakeAPI is an in-memory Lake Formation API. It keeps data lake settings, permissions, and registered resources in an
// awsfake store, in their types' property format, so the suite's Cloud Control sees the same resources.
//
// The settings are the one entry of their type, under whatever identifier created it. Permissions are one entry per
// identifier, with the permissions that alias granted, and one outside entry per parent with permissions granted
// outside Radius. A grant adds to the current entry, which the test decorator sets, and a revoke takes the
// permissions out of every entry of the parent, as AWS does. List calls return one item per page.
type fakeAPI struct {
	store  *awsfake.CloudControl
	failer nativetest.Failer

	mu      sync.Mutex
	current string
	calls   []string

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error

	// empty makes GetDataLakeSettings and DescribeResource return no settings or resource, and no error.
	empty bool
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{store: store, errs: map[string]error{}}
}

func clientError(code string) error {
	return nativefake.ClientError(code, code)
}

func (f *fakeAPI) call(name string) error {
	f.calls = append(f.calls, name)
	return f.errs[name]
}

// use makes identifier the entry the next grants add permissions to.
func (f *fakeAPI) use(identifier string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.current = identifier
}

// change runs a call that changes AWS, failing it before or after it takes effect as the failer says.
func (f *fakeAPI) change(name string, apply func() error) error {
	if err := f.call(name); err != nil {
		return err
	}
	if err := f.failer.Before(); err != nil {
		return err
	}
	if err := apply(); err != nil {
		return err
	}
	return f.failer.After()
}

func page(token *string, count int) (int, *string) {
	i := 0
	if token != nil {
		i, _ = strconv.Atoi(*token)
	}
	if i+1 < count {
		return i, aws.String(strconv.Itoa(i + 1))
	}
	return i, nil
}

func (f *fakeAPI) GetDataLakeSettings(ctx context.Context, in *lakeformation.GetDataLakeSettingsInput, _ ...func(*lakeformation.Options)) (*lakeformation.GetDataLakeSettingsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("GetDataLakeSettings"); err != nil {
		return nil, err
	}
	if f.empty {
		return &lakeformation.GetDataLakeSettingsOutput{}, nil
	}
	keys := f.store.List(settingsType)
	if len(keys) == 0 {
		return nil, clientError(notFoundCode)
	}
	settings, err := decodeSettings(f.store.Resource(settingsType, keys[0]))
	if err != nil {
		return nil, err
	}
	return &lakeformation.GetDataLakeSettingsOutput{DataLakeSettings: settings}, nil
}

func (f *fakeAPI) PutDataLakeSettings(ctx context.Context, in *lakeformation.PutDataLakeSettingsInput, _ ...func(*lakeformation.Options)) (*lakeformation.PutDataLakeSettingsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &lakeformation.PutDataLakeSettingsOutput{}, f.change("PutDataLakeSettings", func() error {
		if in.DataLakeSettings == nil {
			return clientError("InvalidInputException")
		}
		props, err := awsapi.EncodeObject(in.DataLakeSettings)
		if err != nil {
			return err
		}
		if admins, ok := props["DataLakeAdmins"]; ok {
			props["Admins"] = admins
			delete(props, "DataLakeAdmins")
		}
		key := settingsIdentifier
		if keys := f.store.List(settingsType); len(keys) > 0 {
			key = keys[0]
		}
		f.store.Put(settingsType, key, props)
		return nil
	})
}

// cfnResource maps an API resource to the CloudFormation Resource.
func cfnResource(r *types.Resource) map[string]any {
	set := func(m map[string]any, name string, v *string) {
		if v != nil {
			m[name] = *v
		}
	}
	out := map[string]any{}
	switch {
	case r.Database != nil:
		m := map[string]any{}
		set(m, "CatalogId", r.Database.CatalogId)
		set(m, "Name", r.Database.Name)
		out["DatabaseResource"] = m
	case r.Table != nil:
		m := map[string]any{}
		set(m, "CatalogId", r.Table.CatalogId)
		set(m, "DatabaseName", r.Table.DatabaseName)
		set(m, "Name", r.Table.Name)
		if r.Table.TableWildcard != nil {
			m["TableWildcard"] = map[string]any{}
		}
		out["TableResource"] = m
	case r.TableWithColumns != nil:
		t := r.TableWithColumns
		m := map[string]any{}
		set(m, "CatalogId", t.CatalogId)
		set(m, "DatabaseName", t.DatabaseName)
		set(m, "Name", t.Name)
		if len(t.ColumnNames) > 0 {
			m["ColumnNames"] = anyList(t.ColumnNames)
		}
		if t.ColumnWildcard != nil {
			w := map[string]any{}
			if len(t.ColumnWildcard.ExcludedColumnNames) > 0 {
				w["ExcludedColumnNames"] = anyList(t.ColumnWildcard.ExcludedColumnNames)
			}
			m["ColumnWildcard"] = w
		}
		out["TableWithColumnsResource"] = m
	case r.DataLocation != nil:
		m := map[string]any{}
		set(m, "CatalogId", r.DataLocation.CatalogId)
		set(m, "S3Resource", r.DataLocation.ResourceArn)
		out["DataLocationResource"] = m
	}
	return out
}

func normalized(v any) any {
	b, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(b, &out)
	return out
}

// permissionEntry is an entry of the fake's permissions.
type permissionEntry struct {
	typeName string
	key      string
	props    map[string]any
}

// entries returns every entry of permissions, those of aliases first.
func (f *fakeAPI) entries() []permissionEntry {
	out := []permissionEntry{}
	for _, typeName := range []string{permissionsType, outsideType} {
		for _, key := range f.store.List(typeName) {
			out = append(out, permissionEntry{typeName: typeName, key: key, props: f.store.Resource(typeName, key)})
		}
	}
	return out
}

func (e permissionEntry) principal() string {
	p, _ := e.props["DataLakePrincipal"].(map[string]any)
	return awsapi.String(p, "DataLakePrincipalIdentifier")
}

// is reports whether the entry is a grant to principal on resource, in any shape AWS stores as the same grant.
func (e permissionEntry) is(principal *types.DataLakePrincipal, resource *types.Resource) bool {
	return e.principal() == aws.ToString(principal.DataLakePrincipalIdentifier) && canonical(e.resource()) == canonical(resource)
}

func (e permissionEntry) resource() *types.Resource {
	cfn, _ := e.props["Resource"].(map[string]any)
	r, _ := sdkResource(cfn)
	return r
}

// wholeTable returns the database and table name of a grant on a whole table, with ALL_TABLES for every table of the
// database, or false for any other resource. AWS stores a Table and a TableWithColumns with an empty ColumnWildcard as
// the same grant.
func wholeTable(r *types.Resource) (string, string, bool) {
	switch {
	case r.Table != nil && r.Table.TableWildcard != nil:
		return aws.ToString(r.Table.DatabaseName), allTables, true
	case r.Table != nil:
		return aws.ToString(r.Table.DatabaseName), aws.ToString(r.Table.Name), true
	case r.TableWithColumns != nil && r.TableWithColumns.ColumnWildcard != nil && len(r.TableWithColumns.ColumnWildcard.ExcludedColumnNames) == 0:
		return aws.ToString(r.TableWithColumns.DatabaseName), aws.ToString(r.TableWithColumns.Name), true
	}
	return "", "", false
}

// canonical returns the key AWS stores the grant on r under.
func canonical(r *types.Resource) string {
	if db, name, ok := wholeTable(r); ok {
		return "table|" + db + "|" + name
	}
	b, _ := json.Marshal(normalized(cfnResource(r)))
	return string(b)
}

// save stores the entry, without empty lists, or removes it when it has no permission left.
func (f *fakeAPI) save(e permissionEntry) {
	for _, name := range []string{"Permissions", "PermissionsWithGrantOption"} {
		if len(awsapi.Strings(e.props, name)) == 0 {
			delete(e.props, name)
		}
	}
	if _, ok := e.props["Permissions"]; !ok {
		if _, ok := e.props["PermissionsWithGrantOption"]; !ok {
			f.store.Remove(e.typeName, e.key)
			return
		}
	}
	f.store.Put(e.typeName, e.key, e.props)
}

func (f *fakeAPI) GrantPermissions(ctx context.Context, in *lakeformation.GrantPermissionsInput, _ ...func(*lakeformation.Options)) (*lakeformation.GrantPermissionsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &lakeformation.GrantPermissionsOutput{}, f.change("GrantPermissions", func() error {
		perms, grants := names(in.Permissions), names(in.PermissionsWithGrantOption)
		if in.Principal == nil || in.Resource == nil || len(perms) == 0 || len(minus(grants, perms)) > 0 {
			return clientError("InvalidInputException")
		}
		if f.current == "" {
			return errors.New("the test set no current entry")
		}
		props := f.store.Resource(permissionsType, f.current)
		if props == nil {
			props = map[string]any{
				"DataLakePrincipal": map[string]any{"DataLakePrincipalIdentifier": aws.ToString(in.Principal.DataLakePrincipalIdentifier)},
				"Resource":          cfnResource(in.Resource),
			}
		}
		props["Permissions"] = anyList(union(awsapi.Strings(props, "Permissions"), perms))
		props["PermissionsWithGrantOption"] = anyList(union(awsapi.Strings(props, "PermissionsWithGrantOption"), grants))
		f.save(permissionEntry{typeName: permissionsType, key: f.current, props: props})
		return nil
	})
}

func (f *fakeAPI) RevokePermissions(ctx context.Context, in *lakeformation.RevokePermissionsInput, _ ...func(*lakeformation.Options)) (*lakeformation.RevokePermissionsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &lakeformation.RevokePermissionsOutput{}, f.change("RevokePermissions", func() error {
		if in.Principal == nil || in.Resource == nil || in.Permissions == nil {
			return clientError("InvalidInputException")
		}
		perms, grants := names(in.Permissions), names(in.PermissionsWithGrantOption)
		for _, e := range f.entries() {
			if !e.is(in.Principal, in.Resource) {
				continue
			}
			e.props["Permissions"] = anyList(minus(awsapi.Strings(e.props, "Permissions"), perms))
			e.props["PermissionsWithGrantOption"] = anyList(minus(awsapi.Strings(e.props, "PermissionsWithGrantOption"), union(perms, grants)))
			f.save(e)
		}
		return nil
	})
}

// listed reports whether ListPermissions for query returns resource. A query for a table returns its columns too.
func listed(query *types.Resource, resource *types.Resource) bool {
	if qdb, qname, ok := wholeTable(query); ok && query.Table != nil {
		db := ""
		name := ""
		switch {
		case resource.Table != nil:
			db, name = aws.ToString(resource.Table.DatabaseName), aws.ToString(resource.Table.Name)
		case resource.TableWithColumns != nil:
			db, name = aws.ToString(resource.TableWithColumns.DatabaseName), aws.ToString(resource.TableWithColumns.Name)
		}
		return db == qdb && name == qname
	}
	return canonical(query) == canonical(resource)
}

// shapes returns the grants of perms and grants on resource as ListPermissions returns them. A grant on a whole table
// comes back as SELECT on a TableWithColumns with an empty ColumnWildcard and the other permissions on a Table, and a
// TableWildcard as the table ALL_TABLES.
func shapes(resource *types.Resource, perms []string, grants []string) []types.PrincipalResourcePermissions {
	db, name, ok := wholeTable(resource)
	if !ok {
		return []types.PrincipalResourcePermissions{{Resource: resource, Permissions: sdkPermissions(perms), PermissionsWithGrantOption: sdkPermissions(grants)}}
	}
	var catalogID *string
	if resource.Table != nil {
		catalogID = resource.Table.CatalogId
	} else {
		catalogID = resource.TableWithColumns.CatalogId
	}
	out := []types.PrincipalResourcePermissions{}
	selects := []string{"SELECT"}
	if slices.Contains(perms, "SELECT") {
		out = append(out, types.PrincipalResourcePermissions{
			Resource:                   &types.Resource{TableWithColumns: &types.TableWithColumnsResource{CatalogId: catalogID, DatabaseName: aws.String(db), Name: aws.String(name), ColumnWildcard: &types.ColumnWildcard{}}},
			Permissions:                sdkPermissions(selects),
			PermissionsWithGrantOption: sdkPermissions(only(grants, selects)),
		})
	}
	if rest := minus(perms, selects); len(rest) > 0 {
		table := &types.TableResource{CatalogId: catalogID, DatabaseName: aws.String(db), Name: aws.String(name)}
		if name == allTables {
			table.TableWildcard = &types.TableWildcard{}
		}
		out = append(out, types.PrincipalResourcePermissions{
			Resource:                   &types.Resource{Table: table},
			Permissions:                sdkPermissions(rest),
			PermissionsWithGrantOption: sdkPermissions(minus(grants, selects)),
		})
	}
	return out
}

// withCatalog returns r with every CatalogId set, as AWS returns it.
func withCatalog(r *types.Resource) *types.Resource {
	fill := func(id **string) {
		if *id == nil {
			*id = aws.String(account)
		}
	}
	switch {
	case r.Database != nil:
		fill(&r.Database.CatalogId)
	case r.Table != nil:
		fill(&r.Table.CatalogId)
	case r.TableWithColumns != nil:
		fill(&r.TableWithColumns.CatalogId)
	case r.DataLocation != nil:
		fill(&r.DataLocation.CatalogId)
	}
	return r
}

func (f *fakeAPI) ListPermissions(ctx context.Context, in *lakeformation.ListPermissionsInput, _ ...func(*lakeformation.Options)) (*lakeformation.ListPermissionsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("ListPermissions"); err != nil {
		return nil, err
	}
	if in.Resource == nil || in.Resource.TableWithColumns != nil {
		return nil, clientError("InvalidInputException")
	}
	found := []types.PrincipalResourcePermissions{}
	for _, e := range f.entries() {
		if in.Principal != nil && e.principal() != aws.ToString(in.Principal.DataLakePrincipalIdentifier) {
			continue
		}
		resource := e.resource()
		if resource == nil {
			continue
		}
		for _, p := range shapes(resource, awsapi.Strings(e.props, "Permissions"), awsapi.Strings(e.props, "PermissionsWithGrantOption")) {
			if !listed(in.Resource, p.Resource) {
				continue
			}
			p.Principal = &types.DataLakePrincipal{DataLakePrincipalIdentifier: aws.String(e.principal())}
			p.Resource = withCatalog(p.Resource)
			found = append(found, p)
		}
	}
	if len(found) == 0 {
		return &lakeformation.ListPermissionsOutput{}, nil
	}
	i, next := page(in.NextToken, len(found))
	return &lakeformation.ListPermissionsOutput{PrincipalResourcePermissions: found[i : i+1], NextToken: next}, nil
}

func (f *fakeAPI) RegisterResource(ctx context.Context, in *lakeformation.RegisterResourceInput, _ ...func(*lakeformation.Options)) (*lakeformation.RegisterResourceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &lakeformation.RegisterResourceOutput{}, f.change("RegisterResource", func() error {
		arn := aws.ToString(in.ResourceArn)
		if arn == "" || (!aws.ToBool(in.UseServiceLinkedRole) && in.RoleArn == nil) {
			return clientError("InvalidInputException")
		}
		if f.store.Resource(resourceType, arn) != nil {
			return clientError("AlreadyExistsException")
		}
		props, err := awsapi.EncodeObject(map[string]any{
			"ResourceArn":          in.ResourceArn,
			"RoleArn":              in.RoleArn,
			"UseServiceLinkedRole": in.UseServiceLinkedRole,
			"HybridAccessEnabled":  in.HybridAccessEnabled,
			"WithFederation":       in.WithFederation,
		})
		if err != nil {
			return err
		}
		f.store.Put(resourceType, arn, props)
		return nil
	})
}

func (f *fakeAPI) info(arn string) *types.ResourceInfo {
	props := f.store.Resource(resourceType, arn)
	if props == nil {
		return nil
	}
	role := awsapi.String(props, "RoleArn")
	if role == "" && props["UseServiceLinkedRole"] == true {
		role = slrArn
	}
	hybrid, _ := props["HybridAccessEnabled"].(bool)
	federation, _ := props["WithFederation"].(bool)
	return &types.ResourceInfo{
		ResourceArn:         aws.String(arn),
		RoleArn:             aws.String(role),
		HybridAccessEnabled: aws.Bool(hybrid),
		WithFederation:      aws.Bool(federation),
	}
}

func (f *fakeAPI) DescribeResource(ctx context.Context, in *lakeformation.DescribeResourceInput, _ ...func(*lakeformation.Options)) (*lakeformation.DescribeResourceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DescribeResource"); err != nil {
		return nil, err
	}
	if f.empty {
		return &lakeformation.DescribeResourceOutput{}, nil
	}
	info := f.info(aws.ToString(in.ResourceArn))
	if info == nil {
		return nil, clientError(notFoundCode)
	}
	// Like AWS, DescribeResource leaves the ARN out of ResourceInfo.
	described := *info
	described.ResourceArn = nil
	return &lakeformation.DescribeResourceOutput{ResourceInfo: &described}, nil
}

func (f *fakeAPI) UpdateResource(ctx context.Context, in *lakeformation.UpdateResourceInput, _ ...func(*lakeformation.Options)) (*lakeformation.UpdateResourceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &lakeformation.UpdateResourceOutput{}, f.change("UpdateResource", func() error {
		arn := aws.ToString(in.ResourceArn)
		props := f.store.Resource(resourceType, arn)
		if props == nil {
			return clientError(notFoundCode)
		}
		if in.RoleArn == nil {
			return clientError("InvalidInputException")
		}
		props["RoleArn"] = *in.RoleArn
		if in.HybridAccessEnabled != nil {
			props["HybridAccessEnabled"] = *in.HybridAccessEnabled
		}
		if in.WithFederation != nil {
			props["WithFederation"] = *in.WithFederation
		}
		f.store.Put(resourceType, arn, props)
		return nil
	})
}

func (f *fakeAPI) DeregisterResource(ctx context.Context, in *lakeformation.DeregisterResourceInput, _ ...func(*lakeformation.Options)) (*lakeformation.DeregisterResourceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &lakeformation.DeregisterResourceOutput{}, f.change("DeregisterResource", func() error {
		arn := aws.ToString(in.ResourceArn)
		if f.store.Resource(resourceType, arn) == nil {
			return clientError(notFoundCode)
		}
		f.store.Remove(resourceType, arn)
		return nil
	})
}

func (f *fakeAPI) ListResources(ctx context.Context, in *lakeformation.ListResourcesInput, _ ...func(*lakeformation.Options)) (*lakeformation.ListResourcesOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("ListResources"); err != nil {
		return nil, err
	}
	arns := f.store.List(resourceType)
	if len(arns) == 0 {
		return &lakeformation.ListResourcesOutput{}, nil
	}
	i, next := page(in.NextToken, len(arns))
	return &lakeformation.ListResourcesOutput{ResourceInfoList: []types.ResourceInfo{*f.info(arns[i])}, NextToken: next}, nil
}

// trackedPermissions is the Permissions handler with its fake's current entry set to the identifier of each call, so
// a grant adds the permissions to that alias's entry.
type trackedPermissions struct {
	*Permissions
	api *fakeAPI
}

var (
	_ native.ScopedReader  = (*trackedPermissions)(nil)
	_ native.ScopedDeleter = (*trackedPermissions)(nil)
)

func (t *trackedPermissions) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	identifier, err := t.Identifier(request.Properties)
	if err != nil {
		return "", err
	}
	t.api.use(identifier)
	return t.Permissions.Create(ctx, request)
}

func (t *trackedPermissions) Update(ctx context.Context, request native.UpdateRequest) error {
	t.api.use(request.Identifier)
	return t.Permissions.Update(ctx, request)
}

func (t *trackedPermissions) ReadScoped(ctx context.Context, region string, identifier string, managed []map[string]any) (map[string]any, error) {
	t.api.use(identifier)
	return t.Permissions.ReadScoped(ctx, region, identifier, managed)
}

func (t *trackedPermissions) DeleteScoped(ctx context.Context, region string, identifier string, managed []map[string]any) error {
	t.api.use(identifier)
	return t.Permissions.DeleteScoped(ctx, region, identifier, managed)
}

// calls returns the calls the fake received, and clears them.
func (f *fakeAPI) takeCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.calls
	f.calls = nil
	return out
}
