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
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lakeformation"
	"github.com/aws/aws-sdk-go-v2/service/lakeformation/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	apiReference   = "https://docs.aws.amazon.com/lake-formation/latest/APIReference/"
	developerGuide = "https://docs.aws.amazon.com/lake-formation/latest/dg/"

	settingsDoc          = apiReference + "API_DataLakeSettings.html"
	principalDoc         = apiReference + "API_DataLakePrincipal.html"
	permissionsReference = developerGuide + "lf-permissions-reference.html"

	// constraintNote is what the pages leave unclear about a value outside a documented limit.
	constraintNote = " The pages don't name the error for a value outside a documented limit, such as a length, a range, " +
		"or the valid values; the fake returns ValidationError, which the Common Errors page gives for input that doesn't " +
		"meet its constraints."
)

// putSettings puts settings with a fake Lake Formation API backed by store.
func putSettings(ctx context.Context, store *awsfake.CloudControl, settings *types.DataLakeSettings) error {
	_, err := newFakeAPI(store).PutDataLakeSettings(ctx, &lakeformation.PutDataLakeSettingsInput{DataLakeSettings: settings})
	return err
}

// getSettings returns the settings of the catalog, or of the caller's account when catalog is nil.
func getSettings(ctx context.Context, store *awsfake.CloudControl, catalog *string) (*types.DataLakeSettings, error) {
	out, err := newFakeAPI(store).GetDataLakeSettings(ctx, &lakeformation.GetDataLakeSettingsInput{CatalogId: catalog})
	if err != nil {
		return nil, err
	}
	return out.DataLakeSettings, nil
}

// withSettings returns a call that puts settings with admin A and changes them with change first.
func withSettings(change func(s *types.DataLakeSettings)) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		s := &types.DataLakeSettings{DataLakeAdmins: principals([]string{adminA})}
		change(s)
		return putSettings(ctx, store, s)
	}
}

// dataLakeSettingsAPI records the Lake Formation calls DataLakeSettings makes.
var dataLakeSettingsAPI = map[string]nativetest.Call{
	"GetDataLakeSettings": {
		Doc: apiReference + "API_GetDataLakeSettings.html",
		Rules: []nativetest.Rule{
			{
				Rule: "CatalogId defaults to the caller's account",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := putSettings(ctx, store, &types.DataLakeSettings{DataLakeAdmins: principals([]string{adminA})}); err != nil {
						return err
					}
					s, err := getSettings(ctx, store, aws.String(account))
					if err != nil {
						return err
					}
					if ids := sdkPrincipalIDs(s.DataLakeAdmins); len(ids) != 1 || ids[0] != adminA {
						return fmt.Errorf("the account's catalog has admins %v", ids)
					}
					return nil
				},
			},
		},
		Unclear: "A new account starts with \"Use only IAM access control\" for new databases and tables (" +
			developerGuide + "change-settings.html), and the page doesn't say when AWS returns EntityNotFoundException. " +
			"The fake returns EntityNotFoundException until settings are put, so the handler's create can be tested, and " +
			"returns the same settings for any CatalogId.",
	},
	"PutDataLakeSettings": {
		Doc: apiReference + "API_PutDataLakeSettings.html",
		Rules: []nativetest.Rule{
			{
				Rule: "the call replaces the list of data lake admins, so an admin left out is no longer one",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := putSettings(ctx, store, &types.DataLakeSettings{DataLakeAdmins: principals([]string{adminA, adminB})}); err != nil {
						return err
					}
					if err := putSettings(ctx, store, &types.DataLakeSettings{DataLakeAdmins: principals([]string{adminB})}); err != nil {
						return err
					}
					s, err := getSettings(ctx, store, nil)
					if err != nil {
						return err
					}
					if ids := sdkPrincipalIDs(s.DataLakeAdmins); len(ids) != 1 || ids[0] != adminB {
						return fmt.Errorf("the admins are %v", ids)
					}
					return nil
				},
			},
			{
				Rule: "leaving out CreateDatabaseDefaultPermissions or CreateTableDefaultPermissions is the same as an empty list",
				Doc:  developerGuide + "change-settings.html",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					iamOnly := defaultSettings()
					err := putSettings(ctx, store, &types.DataLakeSettings{
						CreateDatabaseDefaultPermissions: iamOnly.CreateDatabaseDefaultPermissions,
						CreateTableDefaultPermissions:    iamOnly.CreateTableDefaultPermissions,
					})
					if err != nil {
						return err
					}
					if err := putSettings(ctx, store, &types.DataLakeSettings{DataLakeAdmins: principals([]string{adminA})}); err != nil {
						return err
					}
					s, err := getSettings(ctx, store, nil)
					if err != nil {
						return err
					}
					if len(s.CreateDatabaseDefaultPermissions) > 0 || len(s.CreateTableDefaultPermissions) > 0 {
						return errors.New("the default permissions left out are still set")
					}
					return nil
				},
			},
			{
				Rule: "DataLakeAdmins, ReadOnlyAdmins, and ExternalDataFilteringAllowList have at most 30 principals",
				Doc:  settingsDoc,
				Break: withSettings(func(s *types.DataLakeSettings) {
					ids := []string{}
					for i := range 31 {
						ids = append(ids, "arn:aws:iam::123456789012:role/admin-"+strconv.Itoa(i))
					}
					s.DataLakeAdmins = principals(ids)
				}),
				Code: validationCode,
			},
			{
				Rule: "a principal's DataLakePrincipalIdentifier is 1 to 255 characters",
				Doc:  principalDoc,
				Break: withSettings(func(s *types.DataLakeSettings) {
					s.ReadOnlyAdmins = principals([]string{""})
				}),
				Code: validationCode,
			},
			{
				Rule: "CreateDatabaseDefaultPermissions and CreateTableDefaultPermissions are empty, or grant only ALL to IAM_ALLOWED_PRINCIPALS",
				Doc:  settingsDoc,
				Break: withSettings(func(s *types.DataLakeSettings) {
					s.CreateTableDefaultPermissions = []types.PrincipalPermissions{{
						Principal:   &types.DataLakePrincipal{DataLakePrincipalIdentifier: aws.String(analyst)},
						Permissions: []types.Permission{types.PermissionSelect},
					}}
				}),
				Code: invalidInputCode,
			},
			{
				Rule: "the CROSS_ACCOUNT_VERSION parameter is 1, 2, 3, 4, or 5",
				Doc:  settingsDoc,
				Break: withSettings(func(s *types.DataLakeSettings) {
					s.Parameters = map[string]string{"CROSS_ACCOUNT_VERSION": "6"}
				}),
				Code: invalidInputCode,
			},
			{
				Rule: "the SET_SOURCE_IDENTITY parameter is TRUE or FALSE",
				Doc:  settingsDoc,
				Break: withSettings(func(s *types.DataLakeSettings) {
					s.Parameters = map[string]string{"SET_SOURCE_IDENTITY": "YES"}
				}),
				Code: invalidInputCode,
			},
			{
				Rule: "Parameters keys are 1 to 255 characters",
				Doc:  settingsDoc,
				Break: withSettings(func(s *types.DataLakeSettings) {
					s.Parameters = map[string]string{strings.Repeat("k", 256): "v"}
				}),
				Code: validationCode,
			},
			{
				Rule: "Parameters values are at most 512000 characters",
				Doc:  settingsDoc,
				Break: withSettings(func(s *types.DataLakeSettings) {
					s.Parameters = map[string]string{"K": strings.Repeat("v", maxParameterLength+1)}
				}),
				Code: validationCode,
			},
			{
				Rule: "Parameters values are counted in characters, so 512000 characters that aren't ASCII are taken",
				Doc:  settingsDoc,
				Check: withSettings(func(s *types.DataLakeSettings) {
					s.Parameters = map[string]string{"K": strings.Repeat("é", maxParameterLength)}
				}),
			},
			{
				Rule: "AuthorizedSessionTagValueList and TrustedResourceOwners values are 1 to 255 characters",
				Doc:  settingsDoc,
				Break: withSettings(func(s *types.DataLakeSettings) {
					s.TrustedResourceOwners = []string{""}
				}),
				Code: validationCode,
			},
		},
		Unclear: "The page says the call replaces the list of admins, and the guide says the default permissions left out " +
			"become empty, but neither says so for the other settings; the fake replaces them all, as the handler expects by " +
			"reading the settings before each put. The pages don't name the error for a value DataLakeSettings rules out in " +
			"words, such as default permissions other than ALL for IAM_ALLOWED_PRINCIPALS, or a CROSS_ACCOUNT_VERSION " +
			"outside 1 to 5; the fake returns InvalidInputException, the error the call lists for input that isn't valid. " +
			"DataLakeSettings says admins are IAM users or roles, but the permissions reference names other kinds of " +
			"principal, so the fake doesn't check the kind. The fake keeps Parameters keys other than the two the page " +
			"lists, because AWS sets others, such as SET_CONTEXT." + constraintNote,
	},
}

// onDatabase returns the database name in the caller's catalog.
func onDatabase(name string) *types.Resource {
	return &types.Resource{Database: &types.DatabaseResource{Name: aws.String(name)}}
}

// onTable returns the table name of the database sales.
func onTable(name string) *types.Resource {
	return &types.Resource{Table: &types.TableResource{DatabaseName: aws.String("sales"), Name: aws.String(name)}}
}

// onColumns returns the columns of the table orders of the database sales.
func onColumns(columns ...string) *types.Resource {
	return &types.Resource{TableWithColumns: &types.TableWithColumnsResource{DatabaseName: aws.String("sales"), Name: aws.String("orders"), ColumnNames: columns}}
}

// otherAccount is an account other than the caller's.
const otherAccount = "111122223333"

// inCatalog returns resource, a table or columns of one, in the catalog of the account id.
func inCatalog(id string, resource *types.Resource) *types.Resource {
	if resource.Table != nil {
		resource.Table.CatalogId = aws.String(id)
	} else {
		resource.TableWithColumns.CatalogId = aws.String(id)
	}
	return resource
}

var onLocation = &types.Resource{DataLocation: &types.DataLocationResource{ResourceArn: aws.String(dataArn)}}

// grantTo grants perms, and the grant option of grants, to principal on resource with a fake Lake Formation API
// backed by store.
func grantTo(ctx context.Context, store *awsfake.CloudControl, principal string, resource *types.Resource, perms []string, grants []string) error {
	f := newFakeAPI(store)
	f.use(principal + "|" + canonical(resource))
	in := &lakeformation.GrantPermissionsInput{
		Principal:   &types.DataLakePrincipal{DataLakePrincipalIdentifier: aws.String(principal)},
		Resource:    resource,
		Permissions: sdkPermissions(perms),
	}
	if grants != nil {
		in.PermissionsWithGrantOption = sdkPermissions(grants)
	}
	_, err := f.GrantPermissions(ctx, in)
	return err
}

// grant grants perms to the analyst on resource.
func grant(ctx context.Context, store *awsfake.CloudControl, resource *types.Resource, perms ...string) error {
	return grantTo(ctx, store, analyst, resource, perms, nil)
}

// revokeFrom revokes perms from principal on resource.
func revokeFrom(ctx context.Context, store *awsfake.CloudControl, principal string, resource *types.Resource, perms ...string) error {
	_, err := newFakeAPI(store).RevokePermissions(ctx, &lakeformation.RevokePermissionsInput{
		Principal:   &types.DataLakePrincipal{DataLakePrincipalIdentifier: aws.String(principal)},
		Resource:    resource,
		Permissions: sdkPermissions(perms),
	})
	return err
}

// listOn returns the analyst's grants that a list on resource returns.
func listOn(ctx context.Context, store *awsfake.CloudControl, resource *types.Resource) ([]types.PrincipalResourcePermissions, error) {
	out, err := newFakeAPI(store).ListPermissions(ctx, &lakeformation.ListPermissionsInput{
		Principal: &types.DataLakePrincipal{DataLakePrincipalIdentifier: aws.String(analyst)},
		Resource:  resource,
	})
	if err != nil {
		return nil, err
	}
	return out.PrincipalResourcePermissions, nil
}

// grantedThen returns a call that grants perms on first, and then perms2 on second, to the analyst.
func grantedThen(first *types.Resource, perms []string, second *types.Resource, perms2 ...string) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		if err := grant(ctx, store, first, perms...); err != nil {
			return err
		}
		return grant(ctx, store, second, perms2...)
	}
}

// permissionsAPI records the Lake Formation and STS calls Permissions makes.
var permissionsAPI = map[string]nativetest.Call{
	"GetCallerIdentity": {
		Doc: "https://docs.aws.amazon.com/STS/latest/APIReference/API_GetCallerIdentity.html",
	},
	"GrantPermissions": {
		Doc: apiReference + "API_GrantPermissions.html",
		Rules: []nativetest.Rule{
			{
				Rule: "Permissions and PermissionsWithGrantOption are of the valid values",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return grant(ctx, store, onDatabase("sales"), "READ")
				},
				Code: validationCode,
			},
			{
				Rule: "PermissionsWithGrantOption is a subset of Permissions",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return grantTo(ctx, store, analyst, onDatabase("sales"), []string{"ALTER"}, []string{"DROP"})
				},
				Code: invalidInputCode,
			},
			{
				Rule: "a database takes ALL, ALTER, CREATE_TABLE, DESCRIBE, and DROP, and a table ALL, ALTER, DELETE, DESCRIBE, DROP, INSERT, and SELECT",
				Doc:  permissionsReference,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return grant(ctx, store, onTable("orders"), "CREATE_TABLE")
				},
				Code: invalidInputCode,
			},
			{
				Rule: "an S3 location takes only DATA_LOCATION_ACCESS",
				Doc:  permissionsReference,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return grant(ctx, store, onLocation, "SELECT")
				},
				Code: invalidInputCode,
			},
			{
				Rule: "a table with columns takes only SELECT",
				Doc:  apiReference + "API_TableWithColumnsResource.html",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return grant(ctx, store, onColumns("id"), "ALTER")
				},
				Code: invalidInputCode,
			},
			{
				Rule: "a table with columns sets ColumnNames or ColumnWildcard",
				Doc:  apiReference + "API_TableWithColumnsResource.html",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return grant(ctx, store, onColumns(), "SELECT")
				},
				Code: invalidInputCode,
			},
			{
				Rule: "a table sets Name or TableWildcard",
				Doc:  apiReference + "API_TableResource.html",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return grant(ctx, store, &types.Resource{Table: &types.TableResource{DatabaseName: aws.String("sales")}}, "ALTER")
				},
				Code: invalidInputCode,
			},
			{
				Rule: "database, table, and column names, and CatalogId, are 1 to 255 characters",
				Doc:  apiReference + "API_DatabaseResource.html",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return grant(ctx, store, onDatabase(strings.Repeat("d", 256)), "ALTER")
				},
				Code: validationCode,
			},
			{
				Rule: "the principal's DataLakePrincipalIdentifier is 1 to 255 characters",
				Doc:  principalDoc,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return grantTo(ctx, store, "", onDatabase("sales"), []string{"ALTER"}, nil)
				},
				Code: validationCode,
			},
			{
				Rule: "SELECT on some columns takes no grant option",
				Doc:  permissionsReference + "#perm-select",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return grantTo(ctx, store, analyst, onColumns("id"), []string{"SELECT"}, []string{"SELECT"})
				},
				Code: invalidInputCode,
			},
			{
				Rule:  "a principal with ALTER, DROP, DELETE, or INSERT on a table can't get SELECT on some of its columns",
				Doc:   permissionsReference + "#perm-select",
				Break: grantedThen(onTable("orders"), []string{"ALTER"}, onColumns("id"), "SELECT"),
				Code:  invalidInputCode,
			},
			{
				Rule:  "a principal with SELECT on some columns of a table can't get ALTER, DROP, DELETE, or INSERT on the table",
				Doc:   permissionsReference + "#perm-select",
				Break: grantedThen(onColumns("id"), []string{"SELECT"}, onTable("orders"), "INSERT"),
				Code:  invalidInputCode,
			},
			{
				Rule:  "a principal with SELECT on some columns of a table can't get DESCRIBE on the table",
				Doc:   permissionsReference + "#perm-describe",
				Break: grantedThen(onColumns("id"), []string{"SELECT"}, onTable("orders"), "DESCRIBE"),
				Code:  invalidInputCode,
			},
			{
				Rule:  "a table is identified by its catalog too, so ALTER on a table of another catalog doesn't block SELECT on some columns of the table with the same names in the caller's",
				Doc:   apiReference + "API_TableResource.html",
				Check: grantedThen(inCatalog(otherAccount, onTable("orders")), []string{"ALTER"}, onColumns("id"), "SELECT"),
			},
			{
				Rule: "names are counted in characters, so a database name of 255 characters that aren't ASCII is taken",
				Doc:  apiReference + "API_DatabaseResource.html",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					return grant(ctx, store, onDatabase(strings.Repeat("é", 255)), "ALTER")
				},
			},
			{
				Rule: "a resource without CatalogId is in the caller's account's catalog",
				Doc:  apiReference + "API_DatabaseResource.html",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := grant(ctx, store, onDatabase("sales"), "ALTER"); err != nil {
						return err
					}
					found, err := listOn(ctx, store, &types.Resource{Database: &types.DatabaseResource{CatalogId: aws.String(account), Name: aws.String("sales")}})
					if err != nil {
						return err
					}
					if len(found) != 1 {
						return fmt.Errorf("the account's catalog lists %d grants on the database", len(found))
					}
					return nil
				},
			},
		},
		Unclear: "The page doesn't say what AWS does when the principal already has a permission; the fake changes " +
			"nothing, as the handler's resend of a lost create expects. It doesn't say what AWS does with a Resource that " +
			"names more than one kind, or a table with columns that sets both ColumnNames and ColumnWildcard; the fake " +
			"takes the first kind it knows, and both. The permissions reference names no errors; the fake returns " +
			"InvalidInputException, the error the call lists for input that isn't valid. The fake doesn't know a table's " +
			"partition keys, which column filtering can't restrict, nor require a location to be registered before " +
			"DATA_LOCATION_ACCESS is granted on it, which the reference states without an error." + constraintNote,
	},
	"RevokePermissions": {
		Doc: apiReference + "API_RevokePermissions.html",
		Rules: []nativetest.Rule{
			{
				Rule: "Permissions and PermissionsWithGrantOption are of the valid values",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return revokeFrom(ctx, store, analyst, onDatabase("sales"), "READ")
				},
				Code: validationCode,
			},
			{
				Rule: "each kind of resource takes only its permissions, such as no SELECT on a database",
				Doc:  permissionsReference,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return revokeFrom(ctx, store, analyst, onDatabase("sales"), "SELECT")
				},
				Code: invalidInputCode,
			},
			{
				Rule: "the principal's DataLakePrincipalIdentifier is 1 to 255 characters",
				Doc:  principalDoc,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return revokeFrom(ctx, store, strings.Repeat("p", 256), onDatabase("sales"), "ALTER")
				},
				Code: validationCode,
			},
			{
				Rule: "a table is identified by its catalog too, so a revoke on a table leaves the grant on the table with the same names in another catalog",
				Doc:  apiReference + "API_TableResource.html",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := grant(ctx, store, inCatalog(otherAccount, onTable("orders")), "ALTER"); err != nil {
						return err
					}
					if err := revokeFrom(ctx, store, analyst, onTable("orders"), "ALTER"); err != nil {
						return err
					}
					found, err := listOn(ctx, store, inCatalog(otherAccount, onTable("orders")))
					if err != nil {
						return err
					}
					if len(found) != 1 {
						return errors.New("the revoke on the caller's table removed the grant on another catalog's")
					}
					return nil
				},
			},
		},
		Unclear: "The page doesn't say what AWS does when the principal doesn't have a permission it revokes; the fake " +
			"changes nothing. Permissions is required, but the page doesn't say whether an empty list is taken; the handler " +
			"sends one to revoke only grant options, and the fake takes it." + constraintNote,
	},
	"ListPermissions": {
		Doc: apiReference + "API_ListPermissions.html",
		Rules: []nativetest.Rule{
			{
				Rule: "Resource can't be a table with columns",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := listOn(ctx, store, onColumns("id"))
					return err
				},
				Code: invalidInputCode,
			},
			{
				Rule: "MaxResults is 1 to 1000",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).ListPermissions(ctx, &lakeformation.ListPermissionsInput{Resource: onDatabase("sales"), MaxResults: aws.Int32(0)})
					return err
				},
				Code: validationCode,
			},
			{
				Rule: "a list on a table returns the grants on its columns too",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := grant(ctx, store, onColumns("id"), "SELECT"); err != nil {
						return err
					}
					found, err := listOn(ctx, store, onTable("orders"))
					if err != nil {
						return err
					}
					for _, p := range found {
						if p.Resource != nil && p.Resource.TableWithColumns != nil {
							return nil
						}
					}
					return errors.New("a list on the table didn't return the grant on its columns")
				},
			},
			{
				Rule: "a table is identified by its catalog too, so a list on a table doesn't return grants on the table with the same names in another catalog",
				Doc:  apiReference + "API_TableResource.html",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := grant(ctx, store, inCatalog(otherAccount, onTable("orders")), "ALTER"); err != nil {
						return err
					}
					if err := grant(ctx, store, &types.Resource{Table: &types.TableResource{CatalogId: aws.String(otherAccount), DatabaseName: aws.String("sales"), TableWildcard: &types.TableWildcard{}}}, "DROP"); err != nil {
						return err
					}
					found, err := listOn(ctx, store, onTable("orders"))
					if err != nil {
						return err
					}
					if len(found) > 0 {
						return fmt.Errorf("a list on the caller's table returned %d grants of another catalog", len(found))
					}
					return nil
				},
			},
		},
		Unclear: "The page says that with both Principal and Resource, as the handler sends them, AWS returns effective " +
			"permissions rather than the ones granted, without saying how they differ; the fake returns the grants it " +
			"keeps. The shapes the fake returns a grant on a whole table in, SELECT on a table with every column and the " +
			"table ALL_TABLES for a table wildcard, come from AWS's recorded responses, not the docs. Resource isn't " +
			"required, but the fake rejects a list without one with InvalidInputException, because the handler always " +
			"sends one." + constraintNote,
	},
}

// register registers the location dataArn with role, or with the service-linked role when role is "".
func register(ctx context.Context, store *awsfake.CloudControl, role string) error {
	in := &lakeformation.RegisterResourceInput{ResourceArn: aws.String(dataArn)}
	if role == "" {
		in.UseServiceLinkedRole = aws.Bool(true)
	} else {
		in.RoleArn = aws.String(role)
	}
	_, err := newFakeAPI(store).RegisterResource(ctx, in)
	return err
}

// updateRole changes the role of the location dataArn to role.
func updateRole(ctx context.Context, store *awsfake.CloudControl, role string) error {
	_, err := newFakeAPI(store).UpdateResource(ctx, &lakeformation.UpdateResourceInput{ResourceArn: aws.String(dataArn), RoleArn: aws.String(role)})
	return err
}

// resourceAPI records the Lake Formation calls Resource makes.
var resourceAPI = map[string]nativetest.Call{
	"RegisterResource": {
		Doc: apiReference + "API_RegisterResource.html",
		Rules: []nativetest.Rule{
			{
				Rule: "RoleArn is required unless UseServiceLinkedRole is true",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).RegisterResource(ctx, &lakeformation.RegisterResourceInput{ResourceArn: aws.String(dataArn), UseServiceLinkedRole: aws.Bool(false)})
					return err
				},
				Code: invalidInputCode,
			},
			{
				Rule: "RoleArn matches arn:aws:iam::[0-9]*:role/.*",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return register(ctx, store, "arn:aws:iam::123456789012:user/lake-access")
				},
				Code: validationCode,
			},
			{
				Rule: "a location registered already can't be registered again",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := register(ctx, store, roleArn); err != nil {
						return err
					}
					return register(ctx, store, otherRole)
				},
				Code: "AlreadyExistsException",
			},
			{
				Rule: "a location registered with UseServiceLinkedRole has the role AWSServiceRoleForLakeFormationDataAccess",
				Doc:  developerGuide + "service-linked-roles.html",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := register(ctx, store, ""); err != nil {
						return err
					}
					out, err := newFakeAPI(store).DescribeResource(ctx, &lakeformation.DescribeResourceInput{ResourceArn: aws.String(dataArn)})
					if err != nil {
						return err
					}
					if role := aws.ToString(out.ResourceInfo.RoleArn); !strings.HasSuffix(role, "/AWSServiceRoleForLakeFormationDataAccess") {
						return fmt.Errorf("the location has the role %q", role)
					}
					return nil
				},
			},
		},
		Unclear: "The page doesn't say what AWS does with RoleArn next to UseServiceLinkedRole true; the fake keeps the role " +
			"and reports it. It doesn't say what HybridAccessEnabled and WithFederation default to, or rule out any " +
			"combination of them; the fake takes any, and defaults both to false. The page doesn't name the error for a " +
			"missing role; the fake returns InvalidInputException, the error the call lists for input that isn't valid." +
			constraintNote,
	},
	"UpdateResource": {
		Doc: apiReference + "API_UpdateResource.html",
		Rules: []nativetest.Rule{
			{
				Rule: "the location is registered",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return updateRole(ctx, store, roleArn)
				},
				Code: notFoundCode,
			},
			{
				Rule: "a location registered with the service-linked role can't be edited, only deregistered and registered again",
				Doc:  developerGuide + "register-location.html",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := register(ctx, store, ""); err != nil {
						return err
					}
					return updateRole(ctx, store, roleArn)
				},
				Code: invalidInputCode,
			},
			{
				Rule: "RoleArn matches arn:aws:iam::[0-9]*:role/.*",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := register(ctx, store, roleArn); err != nil {
						return err
					}
					return updateRole(ctx, store, "lake-access")
				},
				Code: validationCode,
			},
		},
		Unclear: "The guide doesn't name the error for editing a location registered with the service-linked role; the fake " +
			"returns InvalidInputException, the error the call lists for input that isn't valid. The page doesn't say " +
			"whether HybridAccessEnabled and WithFederation keep their values when left out; the fake keeps them, as the " +
			"handler expects when it sends only the changed ones. RoleArn is required, which the SDK checks before " +
			"sending; the fake returns InvalidInputException without it." + constraintNote,
	},
	"DescribeResource": {
		Doc: apiReference + "API_DescribeResource.html",
		Rules: []nativetest.Rule{
			{
				Rule: "the location is registered",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).DescribeResource(ctx, &lakeformation.DescribeResourceInput{ResourceArn: aws.String(dataArn)})
					return err
				},
				Code: notFoundCode,
			},
		},
		Unclear: "The response syntax has ResourceArn and WithFederation in ResourceInfo, but AWS's recorded responses give " +
			"null for ResourceArn, and for WithFederation when the location was registered without it. The fake leaves " +
			"ResourceArn out, as AWS does, and returns WithFederation false.",
	},
	"DeregisterResource": {
		Doc: apiReference + "API_DeregisterResource.html",
		Rules: []nativetest.Rule{
			{
				Rule: "the location is registered",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).DeregisterResource(ctx, &lakeformation.DeregisterResourceInput{ResourceArn: aws.String(dataArn)})
					return err
				},
				Code: notFoundCode,
			},
		},
	},
	"ListResources": {
		Doc: apiReference + "API_ListResources.html",
		Rules: []nativetest.Rule{
			{
				Rule: "MaxResults is 1 to 1000",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).ListResources(ctx, &lakeformation.ListResourcesInput{MaxResults: aws.Int32(1001)})
					return err
				},
				Code: validationCode,
			},
		},
		Unclear: strings.TrimSpace(constraintNote),
	},
}
