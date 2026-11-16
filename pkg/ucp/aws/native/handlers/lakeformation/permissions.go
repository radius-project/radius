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
	"maps"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lakeformation"
	"github.com/aws/aws-sdk-go-v2/service/lakeformation/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const permissionsType = "AWS::LakeFormation::Permissions"

var (
	_ native.Handler            = (*Permissions)(nil)
	_ native.IdentifierEncoder  = (*Permissions)(nil)
	_ native.ScopedReader       = (*Permissions)(nil)
	_ native.ScopedDeleter      = (*Permissions)(nil)
	_ native.IdentifierResolver = (*Permissions)(nil)
)

// Permissions serves AWS::LakeFormation::Permissions: permissions of a principal (DataLakePrincipal) on a resource
// (Resource). Others can grant the same principal more permissions on the resource, so the resource is only the
// permissions in Permissions and PermissionsWithGrantOption, and permissions granted outside Radius stay. Its
// identifier is the JSON of DataLakePrincipal and the canonical form of Resource (see canonicalResource), so every
// template for the same principal and AWS resource gives one identifier, and two aliases can't hold the same grants.
// Granting a permission the principal already has changes nothing, so a lost create is sent again.
type Permissions struct {
	api      API
	accounts AccountAPI
}

// AccountAPI is the part of the STS client the handler uses, to learn the current account.
type AccountAPI interface {
	GetCallerIdentity(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

// NewPermissions returns the handler for Lake Formation permissions. It learns the current account with accounts.
func NewPermissions(api API, accounts AccountAPI) *Permissions {
	return &Permissions{api: api, accounts: accounts}
}

// TypeName returns AWS::LakeFormation::Permissions.
func (h *Permissions) TypeName() string {
	return permissionsType
}

// Options returns how UCP uses the handler. Permissions can't be listed as resources of the type.
func (h *Permissions) Options() native.Options {
	return native.Options{List: native.ListNone, Recovery: native.RecoverByIdentifier}
}

// Identifier returns the JSON of DataLakePrincipal and the canonical form of Resource, with object keys sorted, or ""
// when either is missing. A CatalogId that names the current account stays until ResolveIdentifier drops it.
func (h *Permissions) Identifier(properties map[string]any) (string, error) {
	principal, _ := properties["DataLakePrincipal"].(map[string]any)
	resource, _ := properties["Resource"].(map[string]any)
	if len(principal) == 0 || len(resource) == 0 {
		return "", nil
	}
	b, err := json.Marshal(map[string]any{"DataLakePrincipal": principal, "Resource": canonicalResource(resource)})
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// canonicalResource returns the one form of a Resource that every form of the same AWS resource gives:
//
//   - Glue names, of a database, a table, and columns, in lowercase, because Glue stores them in lowercase.
//   - A TableWithColumnsResource with a ColumnWildcard that excludes no column as the TableResource of the whole table,
//     which AWS treats as the same resource.
//   - ColumnNames and ExcludedColumnNames sorted, without repeats, because they are sets.
//   - No empty strings or lists, which the request leaves out.
//
// It keeps CatalogId, which ResolveIdentifier drops when it names the current account, the account a Resource
// without one means. Values it doesn't know stay as they are.
func canonicalResource(r map[string]any) map[string]any {
	lower := func(m map[string]any, name string) {
		if s, ok := m[name].(string); ok {
			m[name] = strings.ToLower(s)
		}
	}
	names := func(list []string) []any {
		out := []string{}
		for _, n := range list {
			out = append(out, strings.ToLower(n))
		}
		slices.Sort(out)
		return anyList(slices.Compact(out))
	}
	out := map[string]any{}
	for kind, value := range r {
		m, ok := value.(map[string]any)
		if !ok {
			out[kind] = value
			continue
		}
		m = maps.Clone(m)
		dropEmpty(m)
		switch kind {
		case "DatabaseResource":
			lower(m, "Name")
		case "TableResource":
			lower(m, "DatabaseName")
			lower(m, "Name")
		case "TableWithColumnsResource":
			lower(m, "DatabaseName")
			lower(m, "Name")
			if _, ok := m["ColumnNames"]; ok {
				m["ColumnNames"] = names(awsapi.Strings(m, "ColumnNames"))
			}
			if w, ok := m["ColumnWildcard"].(map[string]any); ok {
				excluded := awsapi.Strings(w, "ExcludedColumnNames")
				if len(excluded) == 0 && len(awsapi.Strings(m, "ColumnNames")) == 0 {
					delete(m, "ColumnWildcard")
					out["TableResource"] = m
					continue
				}
				m["ColumnWildcard"] = map[string]any{"ExcludedColumnNames": names(excluded)}
			}
		}
		out[kind] = m
	}
	return out
}

// dropEmpty removes from m the empty strings and lists, which sdkResource leaves out of the request.
func dropEmpty(m map[string]any) {
	for k, v := range m {
		if s, ok := v.(string); ok && s == "" {
			delete(m, k)
		} else if l, ok := v.([]any); ok && len(l) == 0 {
			delete(m, k)
		}
	}
}

// ResolveIdentifier returns the canonical form of identifier, the form Create returns: Resource in its canonical form,
// without a CatalogId that names the current account. It asks STS for the current account only when the identifier
// has a CatalogId. An identifier that isn't the JSON of a principal and a resource, such as one Cloud Control
// assigned, stays as it is.
func (h *Permissions) ResolveIdentifier(ctx context.Context, region string, identifier string) (string, error) {
	var t target
	if err := json.Unmarshal([]byte(identifier), &t); err != nil || len(t.Principal) == 0 || len(t.Resource) == 0 {
		return identifier, nil
	}
	resource := canonicalResource(t.Resource)
	account := ""
	for _, value := range resource {
		m, ok := value.(map[string]any)
		if !ok || awsapi.String(m, "CatalogId") == "" {
			continue
		}
		if account == "" {
			out, err := h.accounts.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{}, func(o *sts.Options) { o.Region = region })
			if err != nil {
				return "", err
			}
			if account = aws.ToString(out.Account); account == "" {
				return "", errors.New("STS returned no account ID for the AWS credentials")
			}
		}
		if awsapi.String(m, "CatalogId") == account {
			delete(m, "CatalogId")
		}
	}
	return h.Identifier(map[string]any{"DataLakePrincipal": t.Principal, "Resource": resource})
}

// target is the principal and resource whose permissions the resource is: Principal and Resource in the template's
// form, and principal and resource as the Lake Formation API takes them.
type target struct {
	Principal map[string]any `json:"DataLakePrincipal"`
	Resource  map[string]any `json:"Resource"`
	principal *types.DataLakePrincipal
	resource  *types.Resource
}

// targetOf returns the principal and resource the identifier names. An identifier Cloud Control assigned names none,
// so they then come from the first of properties that sets DataLakePrincipal and Resource. Both are create-only, so
// every property set of the resource names the same ones.
func targetOf(identifier string, properties ...map[string]any) (target, error) {
	var t target
	if err := json.Unmarshal([]byte(identifier), &t); err != nil || len(t.Principal) == 0 || len(t.Resource) == 0 {
		t = target{}
		for _, m := range properties {
			principal, _ := m["DataLakePrincipal"].(map[string]any)
			resource, _ := m["Resource"].(map[string]any)
			if len(principal) > 0 && len(resource) > 0 {
				t = target{Principal: principal, Resource: resource}
				break
			}
		}
	}
	if t.Principal == nil {
		return target{}, errors.New("the AWS identifier of the Lake Formation permissions names no DataLakePrincipal and Resource")
	}
	id := awsapi.String(t.Principal, "DataLakePrincipalIdentifier")
	if id == "" {
		return target{}, rejected("Set DataLakePrincipal.DataLakePrincipalIdentifier of the Lake Formation permissions.")
	}
	resource, err := sdkResource(t.Resource)
	if err != nil {
		return target{}, err
	}
	t.principal, t.resource = &types.DataLakePrincipal{DataLakePrincipalIdentifier: aws.String(id)}, resource
	return t, nil
}

// rejected returns a failure for a template AWS can't apply, before any call.
func rejected(message string) error {
	return &native.Failure{Message: message, ClientError: true, Outcome: native.OutcomeRejected}
}

// sdkResource maps the CloudFormation Resource to the API's: DatabaseResource to Database, TableResource to Table,
// TableWithColumnsResource to TableWithColumns, and DataLocationResource to DataLocation, whose S3Resource is the
// API's ResourceArn.
func sdkResource(r map[string]any) (*types.Resource, error) {
	out := &types.Resource{}
	if m, ok := r["DatabaseResource"].(map[string]any); ok {
		out.Database = &types.DatabaseResource{CatalogId: optional(m, "CatalogId"), Name: optional(m, "Name")}
	}
	if m, ok := r["TableResource"].(map[string]any); ok {
		out.Table = &types.TableResource{CatalogId: optional(m, "CatalogId"), DatabaseName: optional(m, "DatabaseName"), Name: optional(m, "Name")}
		if _, ok := m["TableWildcard"]; ok {
			out.Table.TableWildcard = &types.TableWildcard{}
		}
	}
	if m, ok := r["TableWithColumnsResource"].(map[string]any); ok {
		out.TableWithColumns = &types.TableWithColumnsResource{
			CatalogId:    optional(m, "CatalogId"),
			DatabaseName: optional(m, "DatabaseName"),
			Name:         optional(m, "Name"),
			ColumnNames:  optionalList(m, "ColumnNames"),
		}
		if w, ok := m["ColumnWildcard"].(map[string]any); ok {
			out.TableWithColumns.ColumnWildcard = &types.ColumnWildcard{ExcludedColumnNames: optionalList(w, "ExcludedColumnNames")}
		}
	}
	if m, ok := r["DataLocationResource"].(map[string]any); ok {
		out.DataLocation = &types.DataLocationResource{CatalogId: optional(m, "CatalogId"), ResourceArn: optional(m, "S3Resource")}
	}
	if out.Database == nil && out.Table == nil && out.TableWithColumns == nil && out.DataLocation == nil {
		return nil, rejected("Set one of DatabaseResource, TableResource, TableWithColumnsResource, and DataLocationResource in Resource of the Lake Formation permissions.")
	}
	return out, nil
}

func optional(m map[string]any, name string) *string {
	if s := awsapi.String(m, name); s != "" {
		return aws.String(s)
	}
	return nil
}

func optionalList(m map[string]any, name string) []string {
	if list := awsapi.Strings(m, name); len(list) > 0 {
		return list
	}
	return nil
}

// allTables is the table name ListPermissions returns for a grant on every table of a database (TableWildcard).
const allTables = "ALL_TABLES"

// tableGrant is a grant on a table, or on columns of it, in one form whatever shape it was granted or listed in.
type tableGrant struct {
	catalog  *string
	database string
	// name is the table's name, or ALL_TABLES for every table of the database.
	name string
	// whole is true for the whole table: a Table, or a TableWithColumns with a ColumnWildcard that excludes nothing.
	whole bool
	// columns are the ColumnNames of a grant on some columns, and excluded the ExcludedColumnNames of a grant on every
	// column but some.
	columns  []string
	excluded []string
}

// tableGrantOf returns the table grant r is, or false when r is no table. ListPermissions returns a grant on a table
// in other shapes than it was granted: SELECT on a Table as a TableWithColumns with an empty ColumnWildcard, other
// permissions on such a TableWithColumns as a Table, and a TableWildcard as the table ALL_TABLES. So all of them are
// the whole table.
func tableGrantOf(r *types.Resource) (tableGrant, bool) {
	switch {
	case r.Table != nil:
		g := tableGrant{catalog: r.Table.CatalogId, database: aws.ToString(r.Table.DatabaseName), name: aws.ToString(r.Table.Name), whole: true}
		if r.Table.TableWildcard != nil {
			g.name = allTables
		}
		return g, true
	case r.TableWithColumns != nil:
		t := r.TableWithColumns
		g := tableGrant{catalog: t.CatalogId, database: aws.ToString(t.DatabaseName), name: aws.ToString(t.Name)}
		if w := t.ColumnWildcard; w != nil {
			g.whole, g.excluded = len(w.ExcludedColumnNames) == 0, w.ExcludedColumnNames
		} else {
			g.columns = t.ColumnNames
		}
		return g, true
	}
	return tableGrant{}, false
}

// matches reports whether got, a resource ListPermissions returns, is want. Glue stores database, table, and column
// names in lowercase, and a template that leaves out CatalogId means the caller's account, which AWS fills in.
func matches(want *types.Resource, got *types.Resource) bool {
	if got == nil {
		return false
	}
	catalog := func(w *string, g *string) bool { return w == nil || aws.ToString(w) == aws.ToString(g) }
	name := func(w *string, g *string) bool { return strings.EqualFold(aws.ToString(w), aws.ToString(g)) }
	if w, ok := tableGrantOf(want); ok {
		g, ok := tableGrantOf(got)
		if !ok || !catalog(w.catalog, g.catalog) || !strings.EqualFold(w.database, g.database) || !strings.EqualFold(w.name, g.name) || w.whole != g.whole {
			return false
		}
		return w.whole || ((w.columns != nil) == (g.columns != nil) && sameNames(w.columns, g.columns) && sameNames(w.excluded, g.excluded))
	}
	switch {
	case want.Database != nil:
		w, g := want.Database, got.Database
		return g != nil && catalog(w.CatalogId, g.CatalogId) && name(w.Name, g.Name)
	case want.DataLocation != nil:
		w, g := want.DataLocation, got.DataLocation
		return g != nil && catalog(w.CatalogId, g.CatalogId) && aws.ToString(w.ResourceArn) == aws.ToString(g.ResourceArn)
	}
	return false
}

// columnPermissions are the permissions a grant on some columns of a table can have.
var columnPermissions = []string{string(types.PermissionSelect)}

// covers reports whether got, a grant on a whole table that ListPermissions returns, covers want, a grant on some
// columns of that table. While the principal has SELECT on the whole table, such as on every table of the database,
// AWS lists only that grant, not the column grant it covers. The column grant still exists, so its SELECT counts as
// held, and a delete revokes it, which leaves the whole-table grant.
func covers(want *types.Resource, got *types.Resource) bool {
	w, ok := tableGrantOf(want)
	if !ok || w.whole {
		return false
	}
	g, ok := tableGrantOf(got)
	return ok && g.whole && (w.catalog == nil || aws.ToString(w.catalog) == aws.ToString(g.catalog)) &&
		strings.EqualFold(w.database, g.database) && strings.EqualFold(w.name, g.name)
}

// sameNames reports whether a and b have the same names in any order, ignoring case.
func sameNames(a []string, b []string) bool {
	lower := func(list []string) []string {
		out := []string{}
		for _, v := range list {
			out = append(out, strings.ToLower(v))
		}
		slices.Sort(out)
		return out
	}
	return slices.Equal(lower(a), lower(b))
}

// list returns the permissions and the permissions with grant option the principal has on the resource.
// ListPermissions can't take a table with columns, so it lists the table's, which include those of its columns.
func (h *Permissions) list(ctx context.Context, region string, t target) ([]string, []string, error) {
	query := t.resource
	if c := t.resource.TableWithColumns; c != nil {
		query = &types.Resource{Table: &types.TableResource{CatalogId: c.CatalogId, DatabaseName: c.DatabaseName, Name: c.Name}}
	}
	perms, grants := []string{}, []string{}
	var token *string
	for {
		out, err := h.api.ListPermissions(ctx, &lakeformation.ListPermissionsInput{Principal: t.principal, Resource: query, NextToken: token}, inRegion(region))
		if err != nil {
			return nil, nil, awsapi.NotFound(err, notFoundCode)
		}
		for _, p := range out.PrincipalResourcePermissions {
			if p.Principal == nil || aws.ToString(p.Principal.DataLakePrincipalIdentifier) != aws.ToString(t.principal.DataLakePrincipalIdentifier) {
				continue
			}
			switch {
			case matches(t.resource, p.Resource):
				perms = union(perms, names(p.Permissions))
				grants = union(grants, names(p.PermissionsWithGrantOption))
			case covers(t.resource, p.Resource):
				perms = union(perms, only(names(p.Permissions), columnPermissions))
				grants = union(grants, only(names(p.PermissionsWithGrantOption), columnPermissions))
			}
		}
		if token = out.NextToken; token == nil {
			return perms, grants, nil
		}
	}
}

// held returns the permissions and the permissions with grant option the principal has on the resource that one of
// managed grants, or every one when managed is nil. It returns ErrNotFound when the principal has none of them.
func (h *Permissions) held(ctx context.Context, region string, t target, managed []map[string]any) ([]string, []string, error) {
	perms, grants, err := h.list(ctx, region, t)
	if err != nil {
		return nil, nil, err
	}
	if managed != nil {
		managedPerms, managedGrants := managedOf(managed)
		perms, grants = only(perms, managedPerms), only(grants, managedGrants)
	}
	if len(perms) == 0 && len(grants) == 0 {
		return nil, nil, native.ErrNotFound
	}
	return perms, grants, nil
}

func names(permissions []types.Permission) []string {
	out := []string{}
	for _, p := range permissions {
		out = append(out, string(p))
	}
	return out
}

// anyList returns list as properties hold a list, once read as JSON.
func anyList(list []string) []any {
	out := []any{}
	for _, v := range list {
		out = append(out, v)
	}
	return out
}

func sdkPermissions(list []string) []types.Permission {
	out := []types.Permission{}
	for _, p := range list {
		out = append(out, types.Permission(p))
	}
	return out
}

// granted returns the permissions properties grant: Permissions and PermissionsWithGrantOption, because a permission
// with grant option is also a permission, and the permissions with grant option.
func granted(properties map[string]any) ([]string, []string) {
	grants := awsapi.Strings(properties, "PermissionsWithGrantOption")
	return union(awsapi.Strings(properties, "Permissions"), grants), grants
}

// managedOf returns the permissions and the permissions with grant option one of managed grants.
func managedOf(managed []map[string]any) ([]string, []string) {
	perms, grants := []string{}, []string{}
	for _, m := range managed {
		p, g := granted(m)
		perms, grants = union(perms, p), union(grants, g)
	}
	return perms, grants
}

// Read returns every permission the principal has on the resource.
func (h *Permissions) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	return h.ReadScoped(ctx, region, identifier, nil)
}

// ReadScoped returns the permissions the principal has on the resource that one of managed grants, or ErrNotFound
// when it has none of them.
func (h *Permissions) ReadScoped(ctx context.Context, region string, identifier string, managed []map[string]any) (map[string]any, error) {
	t, err := targetOf(identifier, managed...)
	if err != nil {
		return nil, err
	}
	perms, grants, err := h.held(ctx, region, t, managed)
	if err != nil {
		return nil, err
	}
	principal, resource, err := h.templateForm(ctx, region, identifier, t, managed)
	if err != nil {
		return nil, err
	}
	properties := map[string]any{"Id": identifier, "DataLakePrincipal": principal, "Resource": resource}
	if len(perms) > 0 {
		properties["Permissions"] = anyList(perms)
	}
	if len(grants) > 0 {
		properties["PermissionsWithGrantOption"] = anyList(grants)
	}
	return properties, nil
}

// templateForm returns DataLakePrincipal and Resource of the first of managed whose identifier resolves to identifier,
// in the template's form, so a template that names the resource in another form, which is create-only, reads back as
// it is. Without one, they are t's, the target the identifier names.
func (h *Permissions) templateForm(ctx context.Context, region string, identifier string, t target, managed []map[string]any) (map[string]any, map[string]any, error) {
	for _, m := range managed {
		id, err := h.Identifier(m)
		if err != nil || id == "" {
			continue
		}
		if id != identifier {
			if id, err = h.ResolveIdentifier(ctx, region, id); err != nil {
				return nil, nil, err
			}
		}
		if id == identifier {
			principal, _ := m["DataLakePrincipal"].(map[string]any)
			resource, _ := m["Resource"].(map[string]any)
			return principal, resource, nil
		}
	}
	return t.Principal, t.Resource, nil
}

// Create grants the template's permissions.
func (h *Permissions) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	identifier, err := h.Identifier(request.Properties)
	if err != nil {
		return "", err
	}
	if identifier == "" {
		return "", rejected("Set DataLakePrincipal and Resource of the Lake Formation permissions.")
	}
	t, err := targetOf(identifier)
	if err != nil {
		return "", err
	}
	perms, grants := granted(request.Properties)
	if len(perms) == 0 {
		return "", rejected("Set Permissions or PermissionsWithGrantOption of the Lake Formation permissions.")
	}
	if identifier, err = h.ResolveIdentifier(ctx, request.Region, identifier); err != nil {
		return "", err
	}
	if err := h.grant(ctx, request.Region, t, perms, grants); err != nil {
		return "", err
	}
	return identifier, nil
}

func (h *Permissions) grant(ctx context.Context, region string, t target, perms []string, grants []string) error {
	input := &lakeformation.GrantPermissionsInput{Principal: t.principal, Resource: t.resource, Permissions: sdkPermissions(perms)}
	if len(grants) > 0 {
		input.PermissionsWithGrantOption = sdkPermissions(grants)
	}
	_, err := h.api.GrantPermissions(ctx, input, inRegion(region))
	return err
}

// revoke revokes perms, and the grant option of grants. perms may be empty to revoke only grant options.
func (h *Permissions) revoke(ctx context.Context, region string, t target, perms []string, grants []string) error {
	input := &lakeformation.RevokePermissionsInput{Principal: t.principal, Resource: t.resource, Permissions: sdkPermissions(perms)}
	if len(grants) > 0 {
		input.PermissionsWithGrantOption = sdkPermissions(grants)
	}
	_, err := h.api.RevokePermissions(ctx, input, inRegion(region))
	return err
}

// Update revokes the permissions and grant options the template removed, then grants the ones it added. A failed
// revoke is rejected, even when its response is lost, so the record keeps every permission it may still have to
// revoke. A failed grant after a revoke is partly applied.
func (h *Permissions) Update(ctx context.Context, request native.UpdateRequest) error {
	t, err := targetOf(request.Identifier, request.Current, request.Desired)
	if err != nil {
		return err
	}
	currentPerms, currentGrants := granted(request.Current)
	desiredPerms, desiredGrants := granted(request.Desired)
	revokePerms, revokeGrants := minus(currentPerms, desiredPerms), minus(currentGrants, desiredGrants)
	addGrants := minus(desiredGrants, currentGrants)
	// A grant option can only be granted with its permission. Granting a permission the principal has changes nothing.
	addPerms := union(minus(desiredPerms, currentPerms), addGrants)

	calls := []func() error{}
	if len(revokePerms) > 0 || len(revokeGrants) > 0 {
		calls = append(calls, func() error {
			err := h.revoke(ctx, request.Region, t, revokePerms, revokeGrants)
			if err != nil {
				return &native.Failure{Message: err.Error(), ClientError: ucp_aws.IsClientFault(err), Outcome: native.OutcomeRejected}
			}
			return nil
		})
	}
	if len(addPerms) > 0 {
		calls = append(calls, func() error {
			return h.grant(ctx, request.Region, t, addPerms, addGrants)
		})
	}
	return awsapi.Calls(calls...)
}

// Delete fails, because UCP deletes permissions with DeleteScoped, which revokes only the permissions the alias manages.
func (h *Permissions) Delete(ctx context.Context, region string, identifier string) error {
	return errors.New("Lake Formation permissions are deleted with DeleteScoped, which revokes only the permissions the alias manages")
}

// DeleteScoped revokes the permissions the principal has on the resource that one of managed grants, and leaves the
// others. It returns ErrNotFound when the principal has none of them. A nil managed manages none, so it revokes nothing.
func (h *Permissions) DeleteScoped(ctx context.Context, region string, identifier string, managed []map[string]any) error {
	if len(managed) == 0 {
		return native.ErrNotFound
	}
	t, err := targetOf(identifier, managed...)
	if err != nil {
		return err
	}
	perms, grants, err := h.held(ctx, region, t, managed)
	if err != nil {
		return err
	}
	return h.revoke(ctx, region, t, perms, grants)
}

// List is not supported.
func (h *Permissions) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, nil
}

// Progress reports every operation as finished, because grants and revokes take effect at once.
func (h *Permissions) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	return native.Progress{Done: true}, nil
}
