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
	"maps"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lakeformation"
	"github.com/aws/aws-sdk-go-v2/service/lakeformation/types"
	"github.com/wI2L/jsondiff"

	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	settingsType = "AWS::LakeFormation::DataLakeSettings"

	// settingsIdentifier is the identifier of the data lake settings, because an account has one set in each region.
	settingsIdentifier = "DataLakeSettings"

	// mutationAppend is the MutationType that adds the template's Admins to the ones AWS has, instead of replacing them.
	mutationAppend = "APPEND"

	// iamAllowedPrincipals is the group Lake Formation gives ALL on new databases and tables by default, so access
	// to them is managed by IAM permissions only.
	iamAllowedPrincipals = "IAM_ALLOWED_PRINCIPALS"
)

var (
	_ native.Handler            = (*DataLakeSettings)(nil)
	_ native.IdentifierEncoder  = (*DataLakeSettings)(nil)
	_ native.ScopedReader       = (*DataLakeSettings)(nil)
	_ native.ScopedDeleter      = (*DataLakeSettings)(nil)
	_ native.IdentifierResolver = (*DataLakeSettings)(nil)
)

// DataLakeSettings serves AWS::LakeFormation::DataLakeSettings, the one set of data lake settings of an account in a
// region. Its identifier is always DataLakeSettings. PutDataLakeSettings replaces every setting, so create and update
// read the settings first and change only the ones the template sets or changes. Its metadata makes a delete change
// nothing in AWS, because the settings hold the data lake administrators.
//
// MutationType is an input only. APPEND adds the template's Admins to the ones AWS has. With APPEND, a read during a
// deploy returns only the admins the alias manages, and an update removes only managed admins the template no longer
// lists, so admins added outside Radius stay and are never seen as a change.
type DataLakeSettings struct {
	api API
}

// NewDataLakeSettings returns the handler for Lake Formation data lake settings.
func NewDataLakeSettings(api API) *DataLakeSettings {
	return &DataLakeSettings{api: api}
}

// TypeName returns AWS::LakeFormation::DataLakeSettings.
func (h *DataLakeSettings) TypeName() string {
	return settingsType
}

// Options returns how UCP uses the handler. The settings can't be listed.
func (h *DataLakeSettings) Options() native.Options {
	return native.Options{List: native.ListNone, Recovery: native.RecoverByIdentifier}
}

// Identifier returns DataLakeSettings, whatever the properties.
func (h *DataLakeSettings) Identifier(properties map[string]any) (string, error) {
	return settingsIdentifier, nil
}

// ResolveIdentifier returns the settings' one identifier for any identifier, because an account has one set of
// settings in a region.
func (h *DataLakeSettings) ResolveIdentifier(ctx context.Context, region string, identifier string) (string, error) {
	return settingsIdentifier, nil
}

func (h *DataLakeSettings) get(ctx context.Context, region string) (*types.DataLakeSettings, error) {
	out, err := h.api.GetDataLakeSettings(ctx, &lakeformation.GetDataLakeSettingsInput{}, inRegion(region))
	if err != nil {
		return nil, awsapi.NotFound(err, notFoundCode)
	}
	if out.DataLakeSettings == nil {
		return &types.DataLakeSettings{}, nil
	}
	return out.DataLakeSettings, nil
}

// Read returns the settings with every admin.
func (h *DataLakeSettings) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	return h.ReadScoped(ctx, region, identifier, nil)
}

// ReadScoped returns the settings. When one of managed sets MutationType to APPEND, Admins has only the admins that
// one of managed lists, so admins added outside Radius don't count as a change. The settings always exist, so a
// read never returns ErrNotFound because no managed admin is left.
func (h *DataLakeSettings) ReadScoped(ctx context.Context, region string, identifier string, managed []map[string]any) (map[string]any, error) {
	settings, err := h.get(ctx, region)
	if err != nil {
		return nil, err
	}
	properties, err := awsapi.EncodeObject(settings)
	if err != nil {
		return nil, err
	}
	delete(properties, "DataLakeAdmins")
	properties["Id"] = settingsIdentifier
	admins := sdkPrincipalIDs(settings.DataLakeAdmins)
	if slices.ContainsFunc(managed, appends) {
		listed := []string{}
		for _, m := range managed {
			listed = union(listed, principalIDs(m, "Admins"))
		}
		admins = only(admins, listed)
	}
	if len(admins) > 0 {
		list := []any{}
		for _, id := range admins {
			list = append(list, map[string]any{"DataLakePrincipalIdentifier": id})
		}
		properties["Admins"] = list
	}
	return properties, nil
}

// appends reports whether properties set MutationType to APPEND.
func appends(properties map[string]any) bool {
	return strings.EqualFold(awsapi.String(properties, "MutationType"), mutationAppend)
}

// decodeSettings returns the SDK settings properties set.
func decodeSettings(properties map[string]any) (*types.DataLakeSettings, error) {
	values := awsapi.Without(properties, "Admins", "Id", "MutationType")
	if _, ok := properties["Admins"]; ok {
		values["DataLakeAdmins"] = properties["Admins"]
	}
	out := &types.DataLakeSettings{}
	if err := awsapi.Decode(values, out); err != nil {
		return nil, err
	}
	return out, nil
}

// defaultSettings returns AWS's default of each setting, which a setting the template removes goes back to. A new
// account gives IAM_ALLOWED_PRINCIPALS ALL on new databases and tables, so IAM permissions alone manage access to them.
func defaultSettings() *types.DataLakeSettings {
	iamOnly := func() []types.PrincipalPermissions {
		return []types.PrincipalPermissions{{
			Principal:   &types.DataLakePrincipal{DataLakePrincipalIdentifier: aws.String(iamAllowedPrincipals)},
			Permissions: []types.Permission{types.PermissionAll},
		}}
	}
	return &types.DataLakeSettings{
		AllowExternalDataFiltering:       aws.Bool(false),
		AllowFullTableExternalDataAccess: aws.Bool(false),
		AuthorizedSessionTagValueList:    []string{},
		CreateDatabaseDefaultPermissions: iamOnly(),
		CreateTableDefaultPermissions:    iamOnly(),
		DataLakeAdmins:                   []types.DataLakePrincipal{},
		ExternalDataFilteringAllowList:   []types.DataLakePrincipal{},
		ReadOnlyAdmins:                   []types.DataLakePrincipal{},
		TrustedResourceOwners:            []string{},
	}
}

// copySetting sets the setting of the property name in settings to its value in from. Admins and Parameters are
// handled by the caller, and other names are ignored.
func copySetting(settings *types.DataLakeSettings, from *types.DataLakeSettings, name string) {
	switch name {
	case "AllowExternalDataFiltering":
		settings.AllowExternalDataFiltering = from.AllowExternalDataFiltering
	case "AllowFullTableExternalDataAccess":
		settings.AllowFullTableExternalDataAccess = from.AllowFullTableExternalDataAccess
	case "AuthorizedSessionTagValueList":
		settings.AuthorizedSessionTagValueList = from.AuthorizedSessionTagValueList
	case "CreateDatabaseDefaultPermissions":
		settings.CreateDatabaseDefaultPermissions = from.CreateDatabaseDefaultPermissions
	case "CreateTableDefaultPermissions":
		settings.CreateTableDefaultPermissions = from.CreateTableDefaultPermissions
	case "ExternalDataFilteringAllowList":
		settings.ExternalDataFilteringAllowList = from.ExternalDataFilteringAllowList
	case "ReadOnlyAdmins":
		settings.ReadOnlyAdmins = from.ReadOnlyAdmins
	case "TrustedResourceOwners":
		settings.TrustedResourceOwners = from.TrustedResourceOwners
	}
}

// mergeAdmins returns the admins to put. REPLACE puts the template's admins. APPEND keeps the admins AWS has, except
// the managed ones (current) the template no longer lists, and adds the template's.
func mergeAdmins(appendMode bool, have []string, current []string, desired []string) []types.DataLakePrincipal {
	if !appendMode {
		return principals(desired)
	}
	return principals(union(minus(have, minus(current, desired)), desired))
}

// Create sets the settings the template sets and leaves the others as AWS has them. AWS always has settings, so UCP
// takes them over with Update, and calls Create only when AWS reports none.
func (h *DataLakeSettings) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	settings, err := h.get(ctx, request.Region)
	if errors.Is(err, native.ErrNotFound) {
		settings = &types.DataLakeSettings{}
	} else if err != nil {
		return "", err
	}
	desired, err := decodeSettings(request.Properties)
	if err != nil {
		return "", err
	}
	for name := range request.Properties {
		copySetting(settings, desired, name)
	}
	if _, ok := request.Properties["Admins"]; ok {
		settings.DataLakeAdmins = mergeAdmins(appends(request.Properties), sdkPrincipalIDs(settings.DataLakeAdmins), nil, sdkPrincipalIDs(desired.DataLakeAdmins))
	}
	if desired.Parameters != nil {
		settings.Parameters = mergeParameters(settings.Parameters, desired.Parameters)
	}
	if err := h.put(ctx, request.Region, settings); err != nil {
		return "", err
	}
	return settingsIdentifier, nil
}

func (h *DataLakeSettings) put(ctx context.Context, region string, settings *types.DataLakeSettings) error {
	_, err := h.api.PutDataLakeSettings(ctx, &lakeformation.PutDataLakeSettingsInput{DataLakeSettings: settings}, inRegion(region))
	return err
}

// mergeParameters returns current with the keys of desired set.
func mergeParameters(current map[string]string, desired map[string]string) map[string]string {
	out := maps.Clone(current)
	if out == nil {
		out = map[string]string{}
	}
	maps.Copy(out, desired)
	return out
}

// applyParameters makes the patch's changes to Parameters. Removing the whole object keeps AWS's value, because a
// parameter such as CROSS_ACCOUNT_VERSION has no default Radius can go back to, and keys AWS sets stay.
func applyParameters(settings *types.DataLakeSettings, patch jsondiff.Patch, desired map[string]string) {
	for _, op := range patch {
		key, ok := strings.CutPrefix(op.Path, "/Parameters")
		if !ok || (key != "" && !strings.HasPrefix(key, "/")) {
			continue
		}
		key = strings.TrimPrefix(key, "/")
		if key == "" {
			if op.Type != jsondiff.OperationRemove {
				settings.Parameters = mergeParameters(settings.Parameters, desired)
			}
			continue
		}
		key = strings.ReplaceAll(strings.ReplaceAll(key, "~1", "/"), "~0", "~")
		if settings.Parameters == nil {
			settings.Parameters = map[string]string{}
		}
		if op.Type == jsondiff.OperationRemove {
			delete(settings.Parameters, key)
		} else {
			settings.Parameters[key] = fmt.Sprint(op.Value)
		}
	}
}

// Update reads the settings and changes only the ones the patch changes. A setting the template removed goes back to
// AWS's default, except Parameters, which keeps AWS's value. With APPEND, a failure in an update that removes managed
// admins is rejected, even when the response is lost, so the record keeps every admin it may still have to remove.
func (h *DataLakeSettings) Update(ctx context.Context, request native.UpdateRequest) error {
	removes := appends(request.Desired) && len(minus(principalIDs(request.Current, "Admins"), principalIDs(request.Desired, "Admins"))) > 0
	settings, err := h.get(ctx, request.Region)
	if err != nil {
		return removing(removes, err)
	}
	desired, err := decodeSettings(request.Desired)
	if err != nil {
		return err
	}
	defaults := defaultSettings()
	admins := false
	for _, name := range awsapi.Changed(request) {
		switch name {
		case "Admins", "MutationType":
			admins = true
		case "Parameters":
			applyParameters(settings, request.Patch, desired.Parameters)
		default:
			if _, ok := request.Desired[name]; ok {
				copySetting(settings, desired, name)
			} else {
				copySetting(settings, defaults, name)
			}
		}
	}
	if admins {
		settings.DataLakeAdmins = mergeAdmins(appends(request.Desired), sdkPrincipalIDs(settings.DataLakeAdmins), principalIDs(request.Current, "Admins"), sdkPrincipalIDs(desired.DataLakeAdmins))
	}
	return removing(removes, h.put(ctx, request.Region, settings))
}

// removing returns err as a failure AWS applied none of when removes is true. Settings AWS doesn't have stay
// ErrNotFound.
func removing(removes bool, err error) error {
	if err == nil || !removes || errors.Is(err, native.ErrNotFound) {
		return err
	}
	return &native.Failure{Message: err.Error(), ClientError: ucp_aws.IsClientFault(err), Outcome: native.OutcomeRejected}
}

// Delete changes nothing. The type's metadata makes a delete leave the settings as they are, so UCP never calls it.
func (h *DataLakeSettings) Delete(ctx context.Context, region string, identifier string) error {
	return nil
}

// DeleteScoped changes nothing, as Delete.
func (h *DataLakeSettings) DeleteScoped(ctx context.Context, region string, identifier string, managed []map[string]any) error {
	return nil
}

// List is not supported.
func (h *DataLakeSettings) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, nil
}

// Progress reports every operation as finished, because PutDataLakeSettings takes effect at once.
func (h *DataLakeSettings) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	return native.Progress{Done: true}, nil
}
