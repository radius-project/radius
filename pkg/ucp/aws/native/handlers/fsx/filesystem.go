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

// Package fsx has the native handler for Amazon FSx file systems, which Cloud Control can't create in any region.
// AWS assigns a file system's ID, and its create and delete calls take an idempotency token, so a resent create
// returns the first file system and a retried delete takes no second final backup.
package fsx

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/fsx"
	"github.com/aws/aws-sdk-go-v2/service/fsx/types"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const fileSystemType = "AWS::FSx::FileSystem"

// API is the part of the FSx client the handler uses.
type API interface {
	CreateFileSystem(ctx context.Context, params *fsx.CreateFileSystemInput, optFns ...func(*fsx.Options)) (*fsx.CreateFileSystemOutput, error)
	CreateFileSystemFromBackup(ctx context.Context, params *fsx.CreateFileSystemFromBackupInput, optFns ...func(*fsx.Options)) (*fsx.CreateFileSystemFromBackupOutput, error)
	DescribeFileSystems(ctx context.Context, params *fsx.DescribeFileSystemsInput, optFns ...func(*fsx.Options)) (*fsx.DescribeFileSystemsOutput, error)
	DescribeDataRepositoryAssociations(ctx context.Context, params *fsx.DescribeDataRepositoryAssociationsInput, optFns ...func(*fsx.Options)) (*fsx.DescribeDataRepositoryAssociationsOutput, error)
	UpdateFileSystem(ctx context.Context, params *fsx.UpdateFileSystemInput, optFns ...func(*fsx.Options)) (*fsx.UpdateFileSystemOutput, error)
	DeleteFileSystem(ctx context.Context, params *fsx.DeleteFileSystemInput, optFns ...func(*fsx.Options)) (*fsx.DeleteFileSystemOutput, error)
	TagResource(ctx context.Context, params *fsx.TagResourceInput, optFns ...func(*fsx.Options)) (*fsx.TagResourceOutput, error)
	UntagResource(ctx context.Context, params *fsx.UntagResourceInput, optFns ...func(*fsx.Options)) (*fsx.UntagResourceOutput, error)
}

var inRegion = awsapi.InRegion[fsx.Options]

var (
	_ native.Handler        = (*FileSystem)(nil)
	_ native.DeletePreparer = (*FileSystem)(nil)
)

// failedStates are the states of a file system that failed, or that FSx can't reach, such as a Windows file system
// whose Active Directory is misconfigured.
var failedStates = []string{
	string(types.FileSystemLifecycleFailed), string(types.FileSystemLifecycleMisconfigured), string(types.FileSystemLifecycleMisconfiguredUnavailable),
}

// optimizations are administrative actions FSx runs in the background after an update took effect, such as moving
// data after a storage capacity increase. They can take hours or days, and the file system is usable, so an update
// doesn't wait for them.
var optimizations = []types.AdministrativeActionType{
	types.AdministrativeActionTypeStorageOptimization, types.AdministrativeActionTypeThroughputOptimization,
	types.AdministrativeActionTypeIopsOptimization, types.AdministrativeActionTypeStorageTypeOptimization,
}

// FileSystem serves AWS::FSx::FileSystem. Its identifier, Id, is the file system ID AWS assigns, such as
// fs-0123456789abcdef0. Deleting a file system takes a final backup where FSx supports one, which Radius never deletes.
type FileSystem struct {
	api API

	// newToken returns a new delete token.
	newToken func() string
}

// NewFileSystem returns the handler for FSx file systems.
func NewFileSystem(api API) *FileSystem {
	return &FileSystem{api: api, newToken: func() string { return uuid.New().String() }}
}

// TypeName returns AWS::FSx::FileSystem.
func (h *FileSystem) TypeName() string {
	return fileSystemType
}

// Options returns how UCP uses the handler. A final backup of a large file system can take hours, so a delete waits
// longer than the default.
func (h *FileSystem) Options() native.Options {
	return native.Options{
		List:               native.ListRegion,
		Recovery:           native.RecoverByResend,
		Equivalent:         equivalent,
		RecordedEquivalent: sameAlias,
		DeleteTimeout:      6 * time.Hour,
	}
}

// equivalent treats a KMS key ARN FSx returns as the key ID or ARN the template sets.
func equivalent(path string, template any, aws any) bool {
	t, okT := template.(string)
	a, okA := aws.(string)
	return path == "KmsKeyId" && okT && okA && (t == a || strings.HasSuffix(a, ":key/"+t))
}

// sameAlias treats a KMS key ARN FSx returns as the alias the template sets. FSx returns the key's ARN only, so an
// alias can't be told from another key: UCP uses it only where the last deploy set the same alias (see
// native.Options.RecordedEquivalent), and KmsKeyId can't change after create.
func sameAlias(path string, template any, aws any) bool {
	t, okT := template.(string)
	_, okA := aws.(string)
	return path == "KmsKeyId" && okT && okA && (strings.HasPrefix(t, "alias/") || strings.Contains(t, ":alias/"))
}

// describe returns the file system with the identifier, or ErrNotFound.
func (h *FileSystem) describe(ctx context.Context, region string, identifier string) (*types.FileSystem, error) {
	out, err := h.api.DescribeFileSystems(ctx, &fsx.DescribeFileSystemsInput{FileSystemIds: []string{identifier}}, inRegion(region))
	if err != nil {
		return nil, awsapi.NotFound(err, "FileSystemNotFound")
	}
	if len(out.FileSystems) == 0 {
		return nil, native.ErrNotFound
	}
	return &out.FileSystems[0], nil
}

// Read returns the file system's properties.
func (h *FileSystem) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	fs, err := h.describe(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	return properties(fs)
}

// properties returns the properties of the file system. It never returns the ONTAP admin password or the Active
// Directory password, which FSx returns redacted, if at all.
func properties(fs *types.FileSystem) (map[string]any, error) {
	p := map[string]any{
		"Id":                    fs.FileSystemId,
		"DNSName":               fs.DNSName,
		"FileSystemType":        fs.FileSystemType,
		"FileSystemTypeVersion": fs.FileSystemTypeVersion,
		"KmsKeyId":              fs.KmsKeyId,
		"NetworkType":           fs.NetworkType,
		"ResourceARN":           fs.ResourceARN,
		"StorageCapacity":       fs.StorageCapacity,
		"StorageType":           fs.StorageType,
		"SubnetIds":             fs.SubnetIds,
		"Tags":                  awsapi.TagListOf(fs.Tags, deleteOptionsTag, updateBaselineTag),
	}
	if c := fs.LustreConfiguration; c != nil {
		p["LustreMountName"] = c.MountName
		lustre := map[string]any{
			"AutomaticBackupRetentionDays":  c.AutomaticBackupRetentionDays,
			"CopyTagsToBackups":             c.CopyTagsToBackups,
			"DailyAutomaticBackupStartTime": c.DailyAutomaticBackupStartTime,
			"DataCompressionType":           c.DataCompressionType,
			"DataReadCacheConfiguration":    c.DataReadCacheConfiguration,
			"DeploymentType":                c.DeploymentType,
			"DriveCacheType":                c.DriveCacheType,
			"EfaEnabled":                    c.EfaEnabled,
			"LogConfiguration":              c.LogConfiguration,
			"MetadataConfiguration":         c.MetadataConfiguration,
			"PerUnitStorageThroughput":      c.PerUnitStorageThroughput,
			"ThroughputCapacity":            c.ThroughputCapacity,
			"WeeklyMaintenanceStartTime":    c.WeeklyMaintenanceStartTime,
		}
		if r := c.DataRepositoryConfiguration; r != nil {
			lustre["AutoImportPolicy"] = r.AutoImportPolicy
			lustre["ExportPath"] = r.ExportPath
			lustre["ImportPath"] = r.ImportPath
			lustre["ImportedFileChunkSize"] = r.ImportedFileChunkSize
		}
		p["LustreConfiguration"] = lustre
	}
	if c := fs.OntapConfiguration; c != nil {
		p["OntapConfiguration"] = map[string]any{
			"AutomaticBackupRetentionDays":  c.AutomaticBackupRetentionDays,
			"DailyAutomaticBackupStartTime": c.DailyAutomaticBackupStartTime,
			"DeploymentType":                c.DeploymentType,
			"DiskIopsConfiguration":         c.DiskIopsConfiguration,
			"EndpointIpAddressRange":        c.EndpointIpAddressRange,
			"EndpointIpv6AddressRange":      c.EndpointIpv6AddressRange,
			"HAPairs":                       c.HAPairs,
			"PreferredSubnetId":             c.PreferredSubnetId,
			"RouteTableIds":                 c.RouteTableIds,
			"ThroughputCapacity":            c.ThroughputCapacity,
			"ThroughputCapacityPerHAPair":   c.ThroughputCapacityPerHAPair,
			"WeeklyMaintenanceStartTime":    c.WeeklyMaintenanceStartTime,
		}
	}
	if c := fs.OpenZFSConfiguration; c != nil {
		p["RootVolumeId"] = c.RootVolumeId
		p["OpenZFSConfiguration"] = map[string]any{
			"AutomaticBackupRetentionDays":  c.AutomaticBackupRetentionDays,
			"CopyTagsToBackups":             c.CopyTagsToBackups,
			"CopyTagsToVolumes":             c.CopyTagsToVolumes,
			"DailyAutomaticBackupStartTime": c.DailyAutomaticBackupStartTime,
			"DeploymentType":                c.DeploymentType,
			"DiskIopsConfiguration":         c.DiskIopsConfiguration,
			"EndpointIpAddressRange":        c.EndpointIpAddressRange,
			"EndpointIpv6AddressRange":      c.EndpointIpv6AddressRange,
			"PreferredSubnetId":             c.PreferredSubnetId,
			"ReadCacheConfiguration":        c.ReadCacheConfiguration,
			"RouteTableIds":                 c.RouteTableIds,
			"ThroughputCapacity":            c.ThroughputCapacity,
			"WeeklyMaintenanceStartTime":    c.WeeklyMaintenanceStartTime,
		}
	}
	if c := fs.WindowsConfiguration; c != nil {
		aliases := []string{}
		for _, a := range c.Aliases {
			aliases = append(aliases, aws.ToString(a.Name))
		}
		p["WindowsConfiguration"] = map[string]any{
			"ActiveDirectoryId":                       c.ActiveDirectoryId,
			"Aliases":                                 aliases,
			"AuditLogConfiguration":                   c.AuditLogConfiguration,
			"AutomaticBackupRetentionDays":            c.AutomaticBackupRetentionDays,
			"CopyTagsToBackups":                       c.CopyTagsToBackups,
			"DailyAutomaticBackupStartTime":           c.DailyAutomaticBackupStartTime,
			"DeploymentType":                          c.DeploymentType,
			"DiskIopsConfiguration":                   c.DiskIopsConfiguration,
			"FsrmConfiguration":                       c.FsrmConfiguration,
			"PreferredSubnetId":                       c.PreferredSubnetId,
			"SelfManagedActiveDirectoryConfiguration": c.SelfManagedActiveDirectoryConfiguration,
			"ThroughputCapacity":                      c.ThroughputCapacity,
			"WeeklyMaintenanceStartTime":              c.WeeklyMaintenanceStartTime,
		}
	}
	return awsapi.EncodeObject(p)
}

// deleteOptionsTag is the tag key the handler puts on an OpenZFS file system whose template sets
// OpenZFSConfiguration Options, with the options joined by spaces. Options are write-only, so UCP keeps no copy, and a
// delete reads them from this tag. Read leaves the tag out, so it never counts as a change.
const deleteOptionsTag = "radius-delete-options"

// createTags returns the tags a create sends: the template's tags, the operation tag with token, and the delete
// options tag when the template sets OpenZFSConfiguration Options.
func createTags(properties map[string]any, token *string) []types.Tag {
	tags := append(awsapi.SDKTags[types.Tag](awsapi.Tags(properties)), types.Tag{Key: aws.String(awsapi.OperationTag), Value: token})
	if options := awsapi.Strings(asMap(properties["OpenZFSConfiguration"]), "Options"); len(options) > 0 {
		tags = append(tags, types.Tag{Key: aws.String(deleteOptionsTag), Value: aws.String(strings.Join(options, " "))})
	}
	return tags
}

// deleteOptions returns the OpenZFS delete options the file system's tags hold, or nil when there are none.
func deleteOptions(tags []types.Tag) []types.DeleteFileSystemOpenZFSOption {
	var out []types.DeleteFileSystemOpenZFSOption
	for _, t := range tags {
		if aws.ToString(t.Key) == deleteOptionsTag {
			for _, option := range strings.Fields(aws.ToString(t.Value)) {
				out = append(out, types.DeleteFileSystemOpenZFSOption(option))
			}
		}
	}
	return out
}

// Create creates the file system, or restores it from a backup when the template sets BackupId. It sends the
// record's client token as the idempotency token, so a resent create returns the first file system. It also tags the
// file system with the token: FSx rejects a resent create whose properties differ, as when UCP looks for a lost create
// before a delete without the write-only values, and then the handler finds the file system by that tag.
func (h *FileSystem) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	if err := awsapi.ReservedTags(request.Properties, reservedTags...); err != nil {
		return "", err
	}
	p := request.Properties
	token := aws.String(request.ClientToken)
	tags := createTags(p, token)
	opt := inRegion(request.Region)
	var fs *types.FileSystem
	var err error
	if awsapi.String(p, "BackupId") != "" {
		input := &fsx.CreateFileSystemFromBackupInput{}
		if err := restoreInput(p, input); err != nil {
			return "", err
		}
		input.ClientRequestToken, input.Tags = token, tags
		var out *fsx.CreateFileSystemFromBackupOutput
		if out, err = h.api.CreateFileSystemFromBackup(ctx, input, opt); err == nil {
			fs = out.FileSystem
		}
	} else {
		input := &fsx.CreateFileSystemInput{}
		if err := awsapi.Decode(awsapi.Without(p, "Tags"), input); err != nil {
			return "", err
		}
		input.ClientRequestToken, input.Tags = token, tags
		var out *fsx.CreateFileSystemOutput
		if out, err = h.api.CreateFileSystem(ctx, input, opt); err == nil {
			fs = out.FileSystem
		}
	}
	if awsapi.HasCode(err, "IncompatibleParameterError") {
		return h.findByToken(ctx, request.Region, request.ClientToken, err)
	}
	if err != nil {
		return "", err
	}
	if fs == nil || fs.FileSystemId == nil {
		return "", errors.New("FSx accepted the create but returned no file system ID")
	}
	return *fs.FileSystemId, nil
}

// restoreInput sets input from properties for a restore from a backup. It rejects a property the restore can't take,
// such as OntapConfiguration: FSx backs up ONTAP volumes, not file systems.
func restoreInput(properties map[string]any, input *fsx.CreateFileSystemFromBackupInput) error {
	fields := map[string]bool{"tags": true, "filesystemtype": true}
	for _, f := range awsapi.FieldNames(input) {
		fields[strings.ToLower(f)] = true
	}
	unsupported := []string{}
	for name := range properties {
		if !fields[strings.ToLower(name)] {
			unsupported = append(unsupported, name)
		}
	}
	if len(unsupported) > 0 {
		slices.Sort(unsupported)
		return &native.Failure{
			Message:     fmt.Sprintf("An FSx file system restored from BackupId can't also set %s.", strings.Join(unsupported, ", ")),
			ClientError: true,
			Outcome:     native.OutcomeRejected,
		}
	}
	return awsapi.Decode(awsapi.Without(properties, "Tags", "FileSystemType"), input)
}

// findByToken returns the file system tagged with the operation token, or rejected, the error FSx returned for the
// resent create, when there is none.
func (h *FileSystem) findByToken(ctx context.Context, region string, token string, rejected error) (string, error) {
	all, err := h.all(ctx, region)
	if err != nil {
		return "", err
	}
	found := []string{}
	for _, fs := range all {
		if slices.ContainsFunc(fs.Tags, func(t types.Tag) bool {
			return aws.ToString(t.Key) == awsapi.OperationTag && aws.ToString(t.Value) == token
		}) {
			found = append(found, aws.ToString(fs.FileSystemId))
		}
	}
	switch len(found) {
	case 0:
		return "", rejected
	case 1:
		return found[0], nil
	}
	return "", awsapi.Ambiguous("FSx file systems", found)
}

// all returns every file system in the region.
func (h *FileSystem) all(ctx context.Context, region string) ([]types.FileSystem, error) {
	out := []types.FileSystem{}
	var next *string
	for {
		page, err := h.api.DescribeFileSystems(ctx, &fsx.DescribeFileSystemsInput{NextToken: next}, inRegion(region))
		if err != nil {
			return nil, err
		}
		out = append(out, page.FileSystems...)
		if next = page.NextToken; next == nil {
			return out, nil
		}
	}
}

// updatable are the properties of each configuration that UpdateFileSystem changes. FSx can't change the others
// after create, and the schema marks them create-only.
var updatable = map[string][]string{
	"LustreConfiguration": {
		"AutoImportPolicy", "AutomaticBackupRetentionDays", "DailyAutomaticBackupStartTime", "DataCompressionType",
		"DataReadCacheConfiguration", "LogConfiguration", "MetadataConfiguration", "PerUnitStorageThroughput",
		"ThroughputCapacity", "WeeklyMaintenanceStartTime",
	},
	"OntapConfiguration": {
		"AutomaticBackupRetentionDays", "DailyAutomaticBackupStartTime", "DiskIopsConfiguration", "EndpointIpv6AddressRange",
		"FsxAdminPassword", "HAPairs", "RouteTableIds", "ThroughputCapacity", "ThroughputCapacityPerHAPair",
		"WeeklyMaintenanceStartTime",
	},
	"OpenZFSConfiguration": {
		"AutomaticBackupRetentionDays", "CopyTagsToBackups", "CopyTagsToVolumes", "DailyAutomaticBackupStartTime",
		"DiskIopsConfiguration", "EndpointIpv6AddressRange", "ReadCacheConfiguration", "RouteTableIds",
		"ThroughputCapacity", "WeeklyMaintenanceStartTime",
	},
	"WindowsConfiguration": {
		"AuditLogConfiguration", "AutomaticBackupRetentionDays", "DailyAutomaticBackupStartTime", "DiskIopsConfiguration",
		"FsrmConfiguration", "SelfManagedActiveDirectoryConfiguration", "ThroughputCapacity", "WeeklyMaintenanceStartTime",
	},
}

// Update makes the patch's changes with one UpdateFileSystem call, then adds and removes tags. A property the
// template no longer sets keeps AWS's value, because FSx has no call to clear it.
func (h *FileSystem) Update(ctx context.Context, request native.UpdateRequest) error {
	if err := awsapi.ReservedTags(request.Desired, reservedTags...); err != nil {
		return err
	}
	fs, err := h.describe(ctx, request.Region, request.Identifier)
	if err != nil {
		return err
	}
	opt := inRegion(request.Region)
	values := map[string]any{}
	calls := []func() error{}
	for _, name := range awsapi.Changed(request) {
		switch name {
		case "StorageCapacity", "StorageType", "FileSystemTypeVersion", "NetworkType":
			if v, ok := request.Desired[name]; ok {
				values[name] = v
			}
		case "LustreConfiguration", "OntapConfiguration", "OpenZFSConfiguration", "WindowsConfiguration":
			if changes := configChanges(name, request); len(changes) > 0 {
				values[name] = changes
			}
		case "Tags":
			calls = append(calls, h.tagCalls(ctx, fs.ResourceARN, request, opt)...)
		}
	}
	if len(values) > 0 {
		input := &fsx.UpdateFileSystemInput{}
		if err := awsapi.Decode(values, input); err != nil {
			return err
		}
		input.FileSystemId = aws.String(request.Identifier)
		calls = append([]func() error{func() error {
			_, err := h.api.UpdateFileSystem(ctx, input, opt)
			return err
		}}, calls...)
	}
	if baseline := failedBaseline(fs); baseline != "" {
		// An update that makes no administrative action, such as a tag change or none at all, must not take the
		// failed one as its.
		calls = append([]func() error{func() error {
			_, err := h.api.TagResource(ctx, &fsx.TagResourceInput{ResourceARN: fs.ResourceARN, Tags: []types.Tag{{Key: aws.String(updateBaselineTag), Value: aws.String(baseline)}}}, opt)
			return err
		}}, calls...)
	}
	return awsapi.Calls(calls...)
}

// reservedTags are the tag keys the handler keeps its own state in, which a template can't set.
var reservedTags = []string{awsapi.OperationTag, deleteOptionsTag, updateBaselineTag}

// updateBaselineTag is the tag key the handler puts on a file system whose latest administrative action failed, with
// that action's request time, before it starts an update. Progress ignores the actions it names, so an update that
// makes no action of its own, such as a tag change, isn't reported as failed. Read leaves the tag out.
const updateBaselineTag = "radius-update-baseline"

// failedBaseline returns the request time of the file system's latest administrative action, other than optimizations,
// when that action failed and the file system's baseline tag doesn't name it yet, and "" otherwise.
func failedBaseline(fs *types.FileSystem) string {
	latest := latestAction(fs, time.Time{})
	if latest == nil || latest.Status != types.StatusFailed {
		return ""
	}
	value := aws.ToTime(latest.RequestTime).UTC().Format(time.RFC3339Nano)
	if value == tagValue(fs.Tags, updateBaselineTag) {
		return ""
	}
	return value
}

// latestAction returns the file system's latest administrative action other than optimizations that FSx requested
// after since, or nil when there is none.
func latestAction(fs *types.FileSystem, since time.Time) *types.AdministrativeAction {
	var latest *types.AdministrativeAction
	for i := range fs.AdministrativeActions {
		a := &fs.AdministrativeActions[i]
		if slices.Contains(optimizations, a.AdministrativeActionType) || !aws.ToTime(a.RequestTime).After(since) {
			continue
		}
		if latest == nil || aws.ToTime(a.RequestTime).After(aws.ToTime(latest.RequestTime)) {
			latest = a
		}
	}
	return latest
}

// tagValue returns the value of the tag with the key, or "".
func tagValue(tags []types.Tag, key string) string {
	for _, t := range tags {
		if aws.ToString(t.Key) == key {
			return aws.ToString(t.Value)
		}
	}
	return ""
}

// configChanges returns the changes to the configuration name, such as LustreConfiguration, in the format of
// UpdateFileSystem's configuration.
func configChanges(name string, request native.UpdateRequest) map[string]any {
	current := asMap(request.Current[name])
	desired := asMap(request.Desired[name])
	out := map[string]any{}
	for _, key := range changedKeys(request.Patch, []string{name}, current, desired) {
		value, ok := desired[key]
		if !ok || !slices.Contains(updatable[name], key) {
			continue
		}
		switch key {
		case "RouteTableIds":
			add, remove := differences(awsapi.Strings(current, key), awsapi.Strings(desired, key))
			out["AddRouteTableIds"], out["RemoveRouteTableIds"] = add, remove
		case "SelfManagedActiveDirectoryConfiguration":
			out[key] = directoryChanges(request.Patch, asMap(current[key]), asMap(value))
		default:
			out[key] = value
		}
	}
	if name == "OntapConfiguration" {
		oneThroughput(out, current, desired)
	}
	return out
}

// oneThroughput leaves ThroughputCapacity out of ONTAP values that also set ThroughputCapacityPerHAPair to an agreeing
// value: FSx takes only one of the two in a call, and sets the other from it. Values that disagree are both sent, and
// FSx rejects them.
func oneThroughput(out map[string]any, current map[string]any, desired map[string]any) {
	total, okTotal := out["ThroughputCapacity"].(float64)
	perPair, okPair := out["ThroughputCapacityPerHAPair"].(float64)
	if !okTotal || !okPair {
		return
	}
	pairs, ok := desired["HAPairs"].(float64)
	if !ok {
		pairs, ok = current["HAPairs"].(float64)
	}
	if !ok {
		pairs = 1
	}
	if total == perPair*pairs {
		delete(out, "ThroughputCapacity")
	}
}

// directoryChanges returns the changed values of a Windows file system's Active Directory configuration. FSx takes a
// new user name and password together.
func directoryChanges(patch jsondiff.Patch, current map[string]any, desired map[string]any) map[string]any {
	out := map[string]any{}
	for _, key := range changedKeys(patch, []string{"WindowsConfiguration", "SelfManagedActiveDirectoryConfiguration"}, current, desired) {
		if v, ok := desired[key]; ok {
			out[key] = v
		}
	}
	if _, ok := out["Password"]; ok {
		out["UserName"] = desired["UserName"]
	}
	if _, ok := out["UserName"]; ok {
		out["Password"] = desired["Password"]
	}
	return out
}

// differences returns the values of desired that current doesn't have, and the values of current that desired
// doesn't have.
func differences(current []string, desired []string) (add []string, remove []string) {
	for _, v := range desired {
		if !slices.Contains(current, v) {
			add = append(add, v)
		}
	}
	for _, v := range current {
		if !slices.Contains(desired, v) {
			remove = append(remove, v)
		}
	}
	return add, remove
}

// changedKeys returns the properties of the object at prefix that the patch changes, sorted. A patch that replaces
// the whole object changes every property whose value differs.
func changedKeys(patch jsondiff.Patch, prefix []string, current map[string]any, desired map[string]any) []string {
	unescape := strings.NewReplacer("~1", "/", "~0", "~")
	out := []string{}
	add := func(key string) {
		if !slices.Contains(out, key) {
			out = append(out, key)
		}
	}
	for _, op := range patch {
		segments := strings.Split(strings.TrimPrefix(op.Path, "/"), "/")
		if len(segments) < len(prefix) || !slices.Equal(segments[:len(prefix)], prefix) {
			continue
		}
		if len(segments) > len(prefix) {
			add(unescape.Replace(segments[len(prefix)]))
			continue
		}
		for _, m := range []map[string]any{current, desired} {
			for key := range m {
				if !reflect.DeepEqual(current[key], desired[key]) {
					add(key)
				}
			}
		}
	}
	slices.Sort(out)
	return out
}

func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

// tagCalls returns the calls that add and remove tags to go from the request's current tags to the desired ones.
// The current tags never include the operation tag, so it stays.
func (h *FileSystem) tagCalls(ctx context.Context, arn *string, request native.UpdateRequest, opt func(*fsx.Options)) []func() error {
	set, remove := awsapi.TagChanges(awsapi.Tags(request.Current), awsapi.Tags(request.Desired))
	calls := []func() error{}
	if len(set) > 0 {
		calls = append(calls, func() error {
			_, err := h.api.TagResource(ctx, &fsx.TagResourceInput{ResourceARN: arn, Tags: awsapi.SDKTags[types.Tag](set)}, opt)
			return err
		})
	}
	if len(remove) > 0 {
		calls = append(calls, func() error {
			_, err := h.api.UntagResource(ctx, &fsx.UntagResourceInput{ResourceARN: arn, TagKeys: remove}, opt)
			return err
		})
	}
	return calls
}

// Delete deletes the file system with a new delete token. UCP calls DeleteWithToken instead, with the token it saved
// on the record.
func (h *FileSystem) Delete(ctx context.Context, region string, identifier string) error {
	return h.DeleteWithToken(ctx, region, identifier, h.newToken())
}

// DeleteToken returns a new UUID, which every attempt to delete the file system sends as FSx's idempotency token.
func (h *FileSystem) DeleteToken(ctx context.Context, region string, identifier string) (string, error) {
	return h.newToken(), nil
}

// DeleteWithToken deletes the file system, sending token as the idempotency token, so a retried delete is the same
// delete and takes no second final backup. It takes a final backup, tagged with the file system's tags, for Windows
// and OpenZFS file systems, and for persistent Lustre file systems not linked to S3. FSx supports none for scratch or
// S3-linked Lustre file systems, or for ONTAP file systems, which FSx backs up by volume. An OpenZFS file system's
// delete also sends the template's OpenZFSConfiguration Options, such as DELETE_CHILD_VOLUMES_AND_SNAPSHOTS, from the
// tag the create put on it (see deleteOptionsTag).
func (h *FileSystem) DeleteWithToken(ctx context.Context, region string, identifier string, token string) error {
	fs, err := h.describe(ctx, region, identifier)
	if err != nil {
		return err
	}
	tags := backupTags(fs.Tags)
	input := &fsx.DeleteFileSystemInput{FileSystemId: aws.String(identifier), ClientRequestToken: aws.String(token)}
	switch fs.FileSystemType {
	case types.FileSystemTypeWindows:
		input.WindowsConfiguration = &types.DeleteFileSystemWindowsConfiguration{SkipFinalBackup: aws.Bool(false), FinalBackupTags: tags}
	case types.FileSystemTypeOpenzfs:
		input.OpenZFSConfiguration = &types.DeleteFileSystemOpenZFSConfiguration{SkipFinalBackup: aws.Bool(false), FinalBackupTags: tags, Options: deleteOptions(fs.Tags)}
	case types.FileSystemTypeLustre:
		backup, err := h.lustreBackup(ctx, region, fs)
		if err != nil {
			return err
		}
		input.LustreConfiguration = &types.DeleteFileSystemLustreConfiguration{SkipFinalBackup: aws.Bool(!backup)}
		if backup {
			input.LustreConfiguration.FinalBackupTags = tags
		}
	}
	_, err = h.api.DeleteFileSystem(ctx, input, inRegion(region))
	return awsapi.NotFound(err, "FileSystemNotFound")
}

// backupTags returns the file system's tags for its final backup, without the tags AWS or UCP added, or nil when none
// are left, because FSx rejects an empty list.
func backupTags(tags []types.Tag) []types.Tag {
	var out []types.Tag
	for _, t := range tags {
		key := aws.ToString(t.Key)
		if !strings.HasPrefix(key, "aws:") && key != awsapi.OperationTag && key != deleteOptionsTag && key != updateBaselineTag {
			out = append(out, t)
		}
	}
	return out
}

// lustreBackup reports whether FSx can take a final backup of the Lustre file system: it is persistent and not linked
// to S3, by its data repository configuration or by a data repository association.
func (h *FileSystem) lustreBackup(ctx context.Context, region string, fs *types.FileSystem) (bool, error) {
	c := fs.LustreConfiguration
	if c == nil || !strings.HasPrefix(string(c.DeploymentType), "PERSISTENT") {
		return false, nil
	}
	if r := c.DataRepositoryConfiguration; r != nil && aws.ToString(r.ImportPath) != "" {
		return false, nil
	}
	if c.DeploymentType != types.LustreDeploymentTypePersistent2 {
		return true, nil
	}
	out, err := h.api.DescribeDataRepositoryAssociations(ctx, &fsx.DescribeDataRepositoryAssociationsInput{
		Filters: []types.Filter{{Name: types.FilterNameFileSystemId, Values: []string{aws.ToString(fs.FileSystemId)}}},
	}, inRegion(region))
	if err != nil {
		return false, err
	}
	return len(out.Associations) == 0, nil
}

// List returns every file system in the region.
func (h *FileSystem) List(ctx context.Context, region string) ([]native.Resource, error) {
	all, err := h.all(ctx, region)
	if err != nil {
		return nil, err
	}
	out := []native.Resource{}
	for i := range all {
		p, err := properties(&all[i])
		if err != nil {
			return nil, err
		}
		out = append(out, native.Resource{Identifier: aws.ToString(all[i].FileSystemId), Properties: p})
	}
	return out, nil
}

// Progress reads the file system's state. A create waits for it to be available. An update also waits for FSx to
// finish its administrative actions, except optimizations that run in the background, and fails when the latest
// one failed. A delete waits until FSx no
// longer has the file system, and fails if FSx stops deleting it, such as when the final backup fails.
func (h *FileSystem) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	fs, err := h.describe(ctx, region, identifier)
	if err != nil {
		return native.Progress{}, err
	}
	state := string(fs.Lifecycle)
	failure := func(message string) native.Progress {
		if fs.FailureDetails != nil && fs.FailureDetails.Message != nil {
			message += ": " + aws.ToString(fs.FailureDetails.Message)
		}
		return native.Progress{Done: true, State: state, Failure: &native.Failure{Message: message}}
	}
	switch op {
	case native.OperationDelete:
		if fs.Lifecycle == types.FileSystemLifecycleDeleting {
			return native.Progress{State: state, Waiting: "deleted"}, nil
		}
		return failure(fmt.Sprintf("FSx stopped deleting the file system, which is %s", state)), nil
	case native.OperationCreate:
		progress := awsapi.States{Done: []string{string(types.FileSystemLifecycleAvailable)}, Failed: failedStates}.Progress(state)
		if progress.Failure != nil {
			return failure(progress.Failure.Message), nil
		}
		return progress, nil
	}
	if slices.Contains(failedStates, state) {
		return failure(fmt.Sprintf("AWS reports the file system as %q", state)), nil
	}
	if fs.Lifecycle != types.FileSystemLifecycleAvailable {
		return native.Progress{State: state, Waiting: string(types.FileSystemLifecycleAvailable)}, nil
	}
	for _, a := range fs.AdministrativeActions {
		if !slices.Contains(optimizations, a.AdministrativeActionType) && (a.Status == types.StatusPending || a.Status == types.StatusInProgress) {
			return native.Progress{State: fmt.Sprintf("%s %s", a.AdministrativeActionType, a.Status), Waiting: string(types.StatusCompleted)}, nil
		}
	}
	// The latest action after the baseline is the update's. FSx accepts an update and then runs it, so a change it
	// can't make fails there, and the file system stays as it was. An update that made no action has none.
	since, _ := time.Parse(time.RFC3339Nano, tagValue(fs.Tags, updateBaselineTag))
	if latest := latestAction(fs, since); latest != nil && latest.Status == types.StatusFailed {
		message := fmt.Sprintf("FSx failed the %s", latest.AdministrativeActionType)
		if d := latest.FailureDetails; d != nil && d.Message != nil {
			message += ": " + aws.ToString(d.Message)
		}
		return native.Progress{Done: true, State: state, Failure: &native.Failure{Message: message}}, nil
	}
	return native.Progress{Done: true, State: state}, nil
}
