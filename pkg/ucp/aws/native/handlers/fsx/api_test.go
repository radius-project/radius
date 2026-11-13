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

package fsx

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/fsx"
	"github.com/aws/aws-sdk-go-v2/service/fsx/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	apiReference  = "https://docs.aws.amazon.com/fsx/latest/APIReference/"
	commonErrors  = apiReference + "CommonErrors.html"
	fileSystemDoc = apiReference + "API_FileSystem.html"
	tagDoc        = apiReference + "API_Tag.html"
	filterDoc     = apiReference + "API_Filter.html"
	deleteDoc     = apiReference + "API_DeleteFileSystem.html"

	createLustreDoc  = apiReference + "API_CreateFileSystemLustreConfiguration.html"
	createWindowsDoc = apiReference + "API_CreateFileSystemWindowsConfiguration.html"
	createOntapDoc   = apiReference + "API_CreateFileSystemOntapConfiguration.html"
	createOpenZFSDoc = apiReference + "API_CreateFileSystemOpenZFSConfiguration.html"
	directoryDoc     = apiReference + "API_SelfManagedActiveDirectoryConfiguration.html"

	updateLustreDoc  = apiReference + "API_UpdateFileSystemLustreConfiguration.html"
	updateWindowsDoc = apiReference + "API_UpdateFileSystemWindowsConfiguration.html"
	updateOntapDoc   = apiReference + "API_UpdateFileSystemOntapConfiguration.html"
	updateOpenZFSDoc = apiReference + "API_UpdateFileSystemOpenZFSConfiguration.html"

	deleteLustreDoc  = apiReference + "API_DeleteFileSystemLustreConfiguration.html"
	deleteWindowsDoc = apiReference + "API_DeleteFileSystemWindowsConfiguration.html"
	deleteOpenZFSDoc = apiReference + "API_DeleteFileSystemOpenZFSConfiguration.html"

	guides              = "https://docs.aws.amazon.com/fsx/latest/"
	ontapCreateGuide    = guides + "ONTAPGuide/creating-file-systems.html"
	windowsPerformance  = guides + "WindowsGuide/performance.html"
	windowsStorageGuide = guides + "WindowsGuide/managing-storage-capacity.html"
	lustreStorageGuide  = guides + "LustreGuide/managing-storage-capacity.html"
	openZFSStorageGuide = guides + "OpenZFSGuide/managing-storage-capacity.html"
	ontapStorageGuide   = guides + "ONTAPGuide/storage-capacity-and-IOPS.html"
	tagGuide            = guides + "LustreGuide/tag-resources.html"
)

const (
	subnetA = "subnet-0a1b2c3d4"
	subnetB = "subnet-0b2c3d4e5"

	// unknownFileSystem is a file system ID in FSx's format that no fake has.
	unknownFileSystem = "fs-00000000deadbeef"
)

func lustreInput() *fsx.CreateFileSystemInput {
	return &fsx.CreateFileSystemInput{
		ClientRequestToken: aws.String("token-a"), FileSystemType: types.FileSystemTypeLustre, StorageCapacity: aws.Int32(1200), SubnetIds: []string{subnetA},
		LustreConfiguration: &types.CreateFileSystemLustreConfiguration{DeploymentType: types.LustreDeploymentTypePersistent2, PerUnitStorageThroughput: aws.Int32(125)},
	}
}

func windowsInput() *fsx.CreateFileSystemInput {
	return &fsx.CreateFileSystemInput{
		ClientRequestToken: aws.String("token-a"), FileSystemType: types.FileSystemTypeWindows, StorageCapacity: aws.Int32(32), SubnetIds: []string{subnetA},
		WindowsConfiguration: &types.CreateFileSystemWindowsConfiguration{DeploymentType: types.WindowsDeploymentTypeSingleAz2, ThroughputCapacity: aws.Int32(32)},
	}
}

func ontapInput() *fsx.CreateFileSystemInput {
	return &fsx.CreateFileSystemInput{
		ClientRequestToken: aws.String("token-a"), FileSystemType: types.FileSystemTypeOntap, StorageCapacity: aws.Int32(1024), SubnetIds: []string{subnetA},
		OntapConfiguration: &types.CreateFileSystemOntapConfiguration{DeploymentType: types.OntapDeploymentTypeSingleAz1, ThroughputCapacity: aws.Int32(128)},
	}
}

func openZFSInput() *fsx.CreateFileSystemInput {
	return &fsx.CreateFileSystemInput{
		ClientRequestToken: aws.String("token-a"), FileSystemType: types.FileSystemTypeOpenzfs, StorageCapacity: aws.Int32(64), SubnetIds: []string{subnetA},
		OpenZFSConfiguration: &types.CreateFileSystemOpenZFSConfiguration{DeploymentType: types.OpenZFSDeploymentTypeSingleAz1, ThroughputCapacity: aws.Int32(64)},
	}
}

// available creates a file system from in with the fake FSx API f, reads it once so it is AVAILABLE, and returns
// its ID.
func available(ctx context.Context, f *fakeAPI, in *fsx.CreateFileSystemInput) (string, error) {
	out, err := f.CreateFileSystem(ctx, in)
	if err != nil {
		return "", err
	}
	id := aws.ToString(out.FileSystem.FileSystemId)
	_, err = f.DescribeFileSystems(ctx, &fsx.DescribeFileSystemsInput{FileSystemIds: []string{id}})
	return id, err
}

// describe returns the file system with the ID, as the fake FSx API f describes it.
func describe(ctx context.Context, f *fakeAPI, id string) (*types.FileSystem, error) {
	out, err := f.DescribeFileSystems(ctx, &fsx.DescribeFileSystemsInput{FileSystemIds: []string{id}})
	if err != nil {
		return nil, err
	}
	return &out.FileSystems[0], nil
}

// createBreak returns a Break that creates a file system from base, changed by change.
func createBreak(base func() *fsx.CreateFileSystemInput, change func(in *fsx.CreateFileSystemInput)) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		in := base()
		change(in)
		_, err := newFakeAPI(store).CreateFileSystem(ctx, in)
		return err
	}
}

// createdCheck returns a Check that creates a file system from in and requires check of the file system FSx
// describes to pass.
func createdCheck(in *fsx.CreateFileSystemInput, check func(fs *types.FileSystem) error) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		f := newFakeAPI(store)
		id, err := available(ctx, f, in)
		if err != nil {
			return err
		}
		fs, err := describe(ctx, f, id)
		if err != nil {
			return err
		}
		return check(fs)
	}
}

// updateBreak returns a Break that creates a file system from base, then makes the updates of the file system with
// the ID, reading it after each but the last so FSx finishes it.
func updateBreak(base func() *fsx.CreateFileSystemInput, updates ...func(in *fsx.UpdateFileSystemInput)) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		f := newFakeAPI(store)
		id, err := available(ctx, f, base())
		if err != nil {
			return err
		}
		for i, update := range updates {
			in := &fsx.UpdateFileSystemInput{FileSystemId: aws.String(id)}
			update(in)
			if _, err := f.UpdateFileSystem(ctx, in); err != nil {
				return err
			}
			if i < len(updates)-1 {
				if _, err := describe(ctx, f, id); err != nil {
					return err
				}
			}
		}
		return nil
	}
}

// deleteBreak returns a Break that creates a file system from base, calls setup when it is set, then deletes the file
// system with template, whose FileSystemId it sets.
func deleteBreak(base func() *fsx.CreateFileSystemInput, setup func(f *fakeAPI, id string), template fsx.DeleteFileSystemInput) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		in := template
		f := newFakeAPI(store)
		id, err := available(ctx, f, base())
		if err != nil {
			return err
		}
		if setup != nil {
			setup(f, id)
		}
		in.FileSystemId = aws.String(id)
		_, err = f.DeleteFileSystem(ctx, &in)
		return err
	}
}

// deleteBackups returns a Check that deletes a file system made from base with template, whose FileSystemId it sets,
// and requires the delete to take backups final backups.
func deleteBackups(base func() *fsx.CreateFileSystemInput, template fsx.DeleteFileSystemInput, backups int) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		in := template
		f := newFakeAPI(store)
		id, err := available(ctx, f, base())
		if err != nil {
			return err
		}
		in.FileSystemId = aws.String(id)
		if _, err := f.DeleteFileSystem(ctx, &in); err != nil {
			return err
		}
		if n := f.backupsOf(id); n != backups {
			return fmt.Errorf("the delete took %d final backups", n)
		}
		return nil
	}
}

// tagged creates a file system with n tags with the fake FSx API f and returns its ARN.
func tagged(ctx context.Context, f *fakeAPI, n int) (*string, error) {
	in := openZFSInput()
	for i := range n {
		in.Tags = append(in.Tags, types.Tag{Key: aws.String(fmt.Sprintf("key-%d", i)), Value: aws.String("v")})
	}
	id, err := available(ctx, f, in)
	return aws.String(arnPrefix + id), err
}

func manyTags(n int) []types.Tag {
	out := []types.Tag{}
	for i := range n {
		out = append(out, types.Tag{Key: aws.String(fmt.Sprintf("key-%d", i)), Value: aws.String("v")})
	}
	return out
}

// tokenRules returns the rules FSx documents for the client request token of a create or delete, given a call that
// sends the token with a parameter of value, and returns what it made.
func tokenRules(call func(ctx context.Context, f *fakeAPI, token string, value int32) (string, error)) []nativetest.Rule {
	return []nativetest.Rule{
		{
			Rule: "a call sent again with the same client request token and parameters returns the first call's result and does nothing again",
			Check: func(ctx context.Context, store *awsfake.CloudControl) error {
				f := newFakeAPI(store)
				first, err := call(ctx, f, "token-a", 0)
				if err != nil {
					return err
				}
				again, err := call(ctx, f, "token-a", 0)
				if err != nil {
					return err
				}
				if first != again || store.Count(fileSystemType) > 1 {
					return fmt.Errorf("the resent call returned %s, not %s", again, first)
				}
				return nil
			},
		},
		{
			Rule: "the same client request token with other parameters fails with IncompatibleParameterError",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				f := newFakeAPI(store)
				if _, err := call(ctx, f, "token-a", 0); err != nil {
					return err
				}
				_, err := call(ctx, f, "token-a", 1)
				return err
			},
			Code: "IncompatibleParameterError",
		},
		{
			Rule: "ClientRequestToken is 1 to 63 characters of [A-za-z0-9_.-]",
			Doc:  commonErrors,
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				_, err := call(ctx, newFakeAPI(store), strings.Repeat("a", 64), 0)
				return err
			},
			Code: "ValidationError",
		},
	}
}

// createToken creates a file system with the token, and a storage capacity that grows with value.
func createToken(ctx context.Context, f *fakeAPI, token string, value int32) (string, error) {
	in := openZFSInput()
	in.ClientRequestToken, in.StorageCapacity = aws.String(token), aws.Int32(64+value)
	out, err := f.CreateFileSystem(ctx, in)
	if err != nil {
		return "", err
	}
	return aws.ToString(out.FileSystem.FileSystemId), nil
}

// restoreToken restores a backup of an OpenZFS file system with the token, and a storage capacity that grows with
// value.
func restoreToken(ctx context.Context, f *fakeAPI, token string, value int32) (string, error) {
	f.sources["backup-0a1b2c3d4"] = map[string]any{"FileSystemType": "OPENZFS", "StorageCapacity": 64.0}
	out, err := f.CreateFileSystemFromBackup(ctx, &fsx.CreateFileSystemFromBackupInput{
		ClientRequestToken: aws.String(token), BackupId: aws.String("backup-0a1b2c3d4"), SubnetIds: []string{subnetA}, StorageCapacity: aws.Int32(64 + value),
	})
	if err != nil {
		return "", err
	}
	return aws.ToString(out.FileSystem.FileSystemId), nil
}

// deleteToken deletes an OpenZFS file system, made once per fake, with the token. Its final backup is skipped when
// value is set.
func deleteToken(ctx context.Context, f *fakeAPI, token string, value int32) (string, error) {
	ids := f.store.List(fileSystemType)
	id := ""
	if len(ids) > 0 {
		id = ids[0]
	} else {
		var err error
		if id, err = available(ctx, f, openZFSInput()); err != nil {
			return "", err
		}
	}
	_, err := f.DeleteFileSystem(ctx, &fsx.DeleteFileSystemInput{
		FileSystemId: aws.String(id), ClientRequestToken: aws.String(token),
		OpenZFSConfiguration: &types.DeleteFileSystemOpenZFSConfiguration{SkipFinalBackup: aws.Bool(value != 0)},
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s with %d backups", id, f.backupsOf(id)), nil
}

// restoreWindows returns a Break that restores a backup of a Windows file system of the deployment type backedUp as
// one of the deployment type deployment, in the subnets.
func restoreWindows(backedUp types.WindowsDeploymentType, deployment types.WindowsDeploymentType, subnets []string) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		f := newFakeAPI(store)
		f.sources["backup-0a1b2c3d4"] = map[string]any{
			"FileSystemType": "WINDOWS", "StorageCapacity": 32.0,
			"WindowsConfiguration": map[string]any{"DeploymentType": string(backedUp), "ThroughputCapacity": 32.0},
		}
		_, err := f.CreateFileSystemFromBackup(ctx, &fsx.CreateFileSystemFromBackupInput{
			BackupId: aws.String("backup-0a1b2c3d4"), SubnetIds: subnets,
			WindowsConfiguration: &types.CreateFileSystemWindowsConfiguration{DeploymentType: deployment, ThroughputCapacity: aws.Int32(32)},
		})
		return err
	}
}

// expect returns an error naming what when got isn't want.
func expect[T comparable](what string, got T, want T) error {
	if got != want {
		return fmt.Errorf("%s is %v, not %v", what, got, want)
	}
	return nil
}

// fileSystemAPI records the FSx calls FileSystem makes.
var fileSystemAPI = map[string]nativetest.Call{
	"CreateFileSystem": {
		Doc: apiReference + "API_CreateFileSystem.html",
		Rules: append(tokenRules(createToken), []nativetest.Rule{
			{
				Rule:  "Tags has 1 to 50 tags",
				Break: createBreak(openZFSInput, func(in *fsx.CreateFileSystemInput) { in.Tags = manyTags(51) }),
				Code:  "ValidationError",
			},
			{
				Rule: "a tag key is 1 to 128 and a value up to 256 letters, numbers, spaces, or _.:/=+-@",
				Doc:  tagDoc,
				Break: createBreak(openZFSInput, func(in *fsx.CreateFileSystemInput) {
					in.Tags = []types.Tag{{Key: aws.String(strings.Repeat("k", 129)), Value: aws.String("v")}}
				}),
				Code: "ValidationError",
			},
			{
				Rule:  "SubnetIds is required, and has up to 50 IDs of the form subnet-[0-9a-f]{8,}",
				Break: createBreak(openZFSInput, func(in *fsx.CreateFileSystemInput) { in.SubnetIds = []string{"subnet-1"} }),
				Code:  "ValidationError",
			},
			{
				Rule:  "SecurityGroupIds has up to 50 IDs of the form sg-[0-9a-f]{8,}",
				Break: createBreak(openZFSInput, func(in *fsx.CreateFileSystemInput) { in.SecurityGroupIds = []string{"sg-1"} }),
				Code:  "ValidationError",
			},
			{
				Rule: "StorageType defaults to SSD, and NetworkType to IPV4",
				Check: createdCheck(windowsInput(), func(fs *types.FileSystem) error {
					return errors.Join(expect("StorageType", fs.StorageType, types.StorageTypeSsd), expect("NetworkType", fs.NetworkType, types.NetworkTypeIpv4))
				}),
			},
			{
				Rule: "with no KmsKeyId, FSx encrypts with the FSx-managed key and reports the key's ARN",
				Doc:  ontapCreateGuide,
				Check: createdCheck(ontapInput(), func(fs *types.FileSystem) error {
					if !strings.HasPrefix(aws.ToString(fs.KmsKeyId), "arn:aws:kms:") || !strings.Contains(aws.ToString(fs.KmsKeyId), ":key/") {
						return fmt.Errorf("FSx reports the key %q", aws.ToString(fs.KmsKeyId))
					}
					return nil
				}),
			},
			{
				Rule: "AutomaticBackupRetentionDays is 0 to 90, WeeklyMaintenanceStartTime is d:HH:MM, and DailyAutomaticBackupStartTime is HH:MM",
				Doc:  createWindowsDoc,
				Break: createBreak(windowsInput, func(in *fsx.CreateFileSystemInput) {
					in.WindowsConfiguration.AutomaticBackupRetentionDays = aws.Int32(91)
				}),
				Code: "ValidationError",
			},
			{
				Rule: "Lustre: DeploymentType defaults to SCRATCH_1, AutomaticBackupRetentionDays to 0, DataCompressionType to NONE, and CopyTagsToBackups to false",
				Doc:  createLustreDoc,
				Check: createdCheck(&fsx.CreateFileSystemInput{FileSystemType: types.FileSystemTypeLustre, StorageCapacity: aws.Int32(1200), SubnetIds: []string{subnetA}}, func(fs *types.FileSystem) error {
					c := fs.LustreConfiguration
					return errors.Join(expect("DeploymentType", c.DeploymentType, types.LustreDeploymentTypeScratch1), expect("AutomaticBackupRetentionDays", aws.ToInt32(c.AutomaticBackupRetentionDays), 0),
						expect("DataCompressionType", c.DataCompressionType, types.DataCompressionTypeNone), expect("CopyTagsToBackups", aws.ToBool(c.CopyTagsToBackups), false))
				}),
			},
			{
				Rule: "Lustre: FileSystemTypeVersion defaults to 2.10, or to 2.12 for PERSISTENT_2 without a metadata configuration mode",
				Check: createdCheck(lustreInput(), func(fs *types.FileSystem) error {
					return expect("FileSystemTypeVersion", aws.ToString(fs.FileSystemTypeVersion), "2.12")
				}),
			},
			{
				Rule:  "Lustre: a PERSISTENT_2 file system can't have Lustre 2.10",
				Doc:   createLustreDoc,
				Break: createBreak(lustreInput, func(in *fsx.CreateFileSystemInput) { in.FileSystemTypeVersion = aws.String("2.10") }),
				Code:  "BadRequest",
			},
			{
				Rule: "Lustre: PerUnitStorageThroughput is 50, 100, or 200 for PERSISTENT_1 SSD, 12 or 40 for HDD, and 125, 250, 500, or 1000 for PERSISTENT_2",
				Doc:  createLustreDoc,
				Break: createBreak(lustreInput, func(in *fsx.CreateFileSystemInput) {
					in.LustreConfiguration.PerUnitStorageThroughput = aws.Int32(100)
				}),
				Code: "InvalidPerUnitStorageThroughput",
			},
			{
				Rule:  "Lustre: PerUnitStorageThroughput is required for PERSISTENT_1 and PERSISTENT_2 SSD or HDD file systems",
				Doc:   createLustreDoc,
				Break: createBreak(lustreInput, func(in *fsx.CreateFileSystemInput) { in.LustreConfiguration.PerUnitStorageThroughput = nil }),
				Code:  "BadRequest",
			},
			{
				Rule:  "Lustre: SSD StorageCapacity is 1200, 2400, or a multiple of 2400 GiB (of 3600 for SCRATCH_1, and of 6000 or 1800 for HDD)",
				Break: createBreak(lustreInput, func(in *fsx.CreateFileSystemInput) { in.StorageCapacity = aws.Int32(1800) }),
				Code:  "BadRequest",
			},
			{
				Rule: "Lustre: HDD storage is only for PERSISTENT_1 file systems",
				Break: createBreak(lustreInput, func(in *fsx.CreateFileSystemInput) {
					in.StorageType, in.LustreConfiguration.DriveCacheType = types.StorageTypeHdd, types.DriveCacheTypeRead
				}),
				Code: "BadRequest",
			},
			{
				Rule: "Lustre: DriveCacheType is required with HDD storage",
				Doc:  createLustreDoc,
				Break: createBreak(lustreInput, func(in *fsx.CreateFileSystemInput) {
					in.StorageType, in.StorageCapacity = types.StorageTypeHdd, aws.Int32(6000)
					in.LustreConfiguration.DeploymentType, in.LustreConfiguration.PerUnitStorageThroughput = types.LustreDeploymentTypePersistent1, aws.Int32(12)
				}),
				Code: "BadRequest",
			},
			{
				Rule:  "Lustre: a file system takes exactly one subnet",
				Break: createBreak(lustreInput, func(in *fsx.CreateFileSystemInput) { in.SubnetIds = []string{subnetA, subnetB} }),
				Code:  "BadRequest",
			},
			{
				Rule: "Lustre: ExportPath uses the bucket of ImportPath",
				Doc:  createLustreDoc,
				Break: createBreak(lustreInput, func(in *fsx.CreateFileSystemInput) {
					in.LustreConfiguration.ImportPath, in.LustreConfiguration.ExportPath = aws.String("s3://import"), aws.String("s3://other/prefix")
				}),
				Code: "InvalidExportPath",
			},
			{
				Rule:  "Windows: ThroughputCapacity is required",
				Doc:   createWindowsDoc,
				Break: createBreak(windowsInput, func(in *fsx.CreateFileSystemInput) { in.WindowsConfiguration.ThroughputCapacity = nil }),
				Code:  "ValidationError",
			},
			{
				Rule:  "Windows: ThroughputCapacity is 8, 16, 32, 64, 128, 256, 512, 1024, 2048, 4608, 6144, 9216, or 12288 MBps",
				Doc:   windowsPerformance,
				Break: createBreak(windowsInput, func(in *fsx.CreateFileSystemInput) { in.WindowsConfiguration.ThroughputCapacity = aws.Int32(3000) }),
				Code:  "BadRequest",
			},
			{
				Rule: "Windows: DeploymentType defaults to SINGLE_AZ_1, AutomaticBackupRetentionDays to 30, and CopyTagsToBackups to false",
				Doc:  createWindowsDoc,
				Check: createdCheck(&fsx.CreateFileSystemInput{
					FileSystemType: types.FileSystemTypeWindows, StorageCapacity: aws.Int32(32), SubnetIds: []string{subnetA},
					WindowsConfiguration: &types.CreateFileSystemWindowsConfiguration{ThroughputCapacity: aws.Int32(32)},
				}, func(fs *types.FileSystem) error {
					c := fs.WindowsConfiguration
					return errors.Join(expect("DeploymentType", c.DeploymentType, types.WindowsDeploymentTypeSingleAz1), expect("AutomaticBackupRetentionDays", aws.ToInt32(c.AutomaticBackupRetentionDays), 30),
						expect("CopyTagsToBackups", aws.ToBool(c.CopyTagsToBackups), false))
				}),
			},
			{
				Rule: "Windows: a MULTI_AZ_1 file system takes exactly two subnets and a PreferredSubnetId, and a single-AZ one exactly one subnet",
				Break: createBreak(windowsInput, func(in *fsx.CreateFileSystemInput) {
					in.WindowsConfiguration.DeploymentType, in.SubnetIds = types.WindowsDeploymentTypeMultiAz1, []string{subnetA, subnetB}
				}),
				Code: "BadRequest",
			},
			{
				Rule:  "Windows: StorageCapacity is 32 to 65536 GiB for SSD, and 2000 to 65536 GiB for HDD",
				Doc:   fileSystemDoc,
				Break: createBreak(windowsInput, func(in *fsx.CreateFileSystemInput) { in.StorageCapacity = aws.Int32(31) }),
				Code:  "BadRequest",
			},
			{
				Rule: "Windows: HDD storage is only for SINGLE_AZ_2 and MULTI_AZ_1 file systems",
				Break: createBreak(windowsInput, func(in *fsx.CreateFileSystemInput) {
					in.StorageType, in.StorageCapacity, in.WindowsConfiguration.DeploymentType = types.StorageTypeHdd, aws.Int32(2000), types.WindowsDeploymentTypeSingleAz1
				}),
				Code: "BadRequest",
			},
			{
				Rule: "Windows: a self-managed Active Directory has up to two DnsIps",
				Doc:  directoryDoc,
				Break: createBreak(windowsInput, func(in *fsx.CreateFileSystemInput) {
					in.WindowsConfiguration.SelfManagedActiveDirectoryConfiguration = &types.SelfManagedActiveDirectoryConfiguration{
						DomainName: aws.String("corp.example.com"), DnsIps: []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"},
					}
				}),
				Code: "BadRequest",
			},
			{
				Rule:  "ONTAP: DeploymentType is required",
				Doc:   createOntapDoc,
				Break: createBreak(ontapInput, func(in *fsx.CreateFileSystemInput) { in.OntapConfiguration.DeploymentType = "" }),
				Code:  "ValidationError",
			},
			{
				Rule:  "ONTAP: HAPairs is 1 to 12, and only 1 for SINGLE_AZ_1 and Multi-AZ file systems",
				Doc:   createOntapDoc,
				Break: createBreak(ontapInput, func(in *fsx.CreateFileSystemInput) { in.OntapConfiguration.HAPairs = aws.Int32(2) }),
				Code:  "BadRequest",
			},
			{
				Rule: "ONTAP: a create defines ThroughputCapacity or ThroughputCapacityPerHAPair, not both, even when they agree",
				Doc:  createOntapDoc,
				Break: createBreak(ontapInput, func(in *fsx.CreateFileSystemInput) {
					in.OntapConfiguration.ThroughputCapacityPerHAPair = aws.Int32(128)
				}),
				Code: "BadRequest",
			},
			{
				Rule:  "ONTAP: the throughput capacity per HA pair is 128 to 4096 MBps for first-generation file systems, and the values documented for each other deployment type",
				Doc:   createOntapDoc,
				Break: createBreak(ontapInput, func(in *fsx.CreateFileSystemInput) { in.OntapConfiguration.ThroughputCapacity = aws.Int32(300) }),
				Code:  "BadRequest",
			},
			{
				Rule:  "ONTAP: StorageCapacity is 1024 to 524288 GiB per HA pair",
				Break: createBreak(ontapInput, func(in *fsx.CreateFileSystemInput) { in.StorageCapacity = aws.Int32(1000) }),
				Code:  "BadRequest",
			},
			{
				Rule: "ONTAP: a Multi-AZ file system needs a PreferredSubnetId, and a MULTI_AZ_1 one exactly two subnets",
				Doc:  createOntapDoc,
				Break: createBreak(ontapInput, func(in *fsx.CreateFileSystemInput) {
					in.OntapConfiguration.DeploymentType, in.SubnetIds = types.OntapDeploymentTypeMultiAz1, []string{subnetA, subnetB}
				}),
				Code: "BadRequest",
			},
			{
				Rule: "ONTAP: FsxAdminPassword is 8 to 50 characters, not bytes",
				Doc:  createOntapDoc,
				Break: createBreak(ontapInput, func(in *fsx.CreateFileSystemInput) {
					in.OntapConfiguration.FsxAdminPassword = aws.String(strings.Repeat("é", 7))
				}),
				Code: "ValidationError",
			},
			{
				Rule: "ONTAP: an FsxAdminPassword of 50 characters of more than one byte each is valid",
				Doc:  createOntapDoc,
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					in := ontapInput()
					in.OntapConfiguration.FsxAdminPassword = aws.String(strings.Repeat("é", 50))
					_, err := newFakeAPI(store).CreateFileSystem(ctx, in)
					return err
				},
			},
			{
				Rule:  "Windows: an alias is 4 to 253 characters, not bytes",
				Doc:   createWindowsDoc,
				Break: createBreak(windowsInput, func(in *fsx.CreateFileSystemInput) { in.WindowsConfiguration.Aliases = []string{"ééé"} }),
				Code:  "ValidationError",
			},
			{
				Rule:  "KmsKeyId, when the call sets it, is 1 to 2048 characters",
				Doc:   commonErrors,
				Break: createBreak(ontapInput, func(in *fsx.CreateFileSystemInput) { in.KmsKeyId = aws.String("") }),
				Code:  "ValidationError",
			},
			{
				Rule:  "PreferredSubnetId, when the call sets it, is of the form subnet-[0-9a-f]{8,}",
				Doc:   createWindowsDoc,
				Break: createBreak(windowsInput, func(in *fsx.CreateFileSystemInput) { in.WindowsConfiguration.PreferredSubnetId = aws.String("") }),
				Code:  "ValidationError",
			},
			{
				Rule: "Lustre: ImportPath and ExportPath, when the call sets them, are 3 to 4357 characters",
				Doc:  createLustreDoc,
				Break: createBreak(lustreInput, func(in *fsx.CreateFileSystemInput) {
					in.LustreConfiguration.ImportPath, in.LustreConfiguration.ExportPath = aws.String("s3://import"), aws.String("")
				}),
				Code: "ValidationError",
			},
			{
				Rule:  "ONTAP: FsxAdminPassword is 8 to 50 characters",
				Doc:   createOntapDoc,
				Break: createBreak(ontapInput, func(in *fsx.CreateFileSystemInput) { in.OntapConfiguration.FsxAdminPassword = aws.String("short") }),
				Code:  "ValidationError",
			},
			{
				Rule: "ONTAP: AutomaticBackupRetentionDays defaults to 30 and HAPairs to 1, and ThroughputCapacityPerHAPair is ThroughputCapacity with one HA pair",
				Doc:  createOntapDoc,
				Check: createdCheck(ontapInput(), func(fs *types.FileSystem) error {
					c := fs.OntapConfiguration
					return errors.Join(expect("AutomaticBackupRetentionDays", aws.ToInt32(c.AutomaticBackupRetentionDays), 30), expect("HAPairs", aws.ToInt32(c.HAPairs), 1),
						expect("ThroughputCapacityPerHAPair", aws.ToInt32(c.ThroughputCapacityPerHAPair), 128))
				}),
			},
			{
				Rule:  "OpenZFS: DeploymentType and ThroughputCapacity are required",
				Doc:   createOpenZFSDoc,
				Break: createBreak(openZFSInput, func(in *fsx.CreateFileSystemInput) { in.OpenZFSConfiguration.ThroughputCapacity = nil }),
				Code:  "ValidationError",
			},
			{
				Rule:  "OpenZFS: ThroughputCapacity is 64 to 4096 MBps for SINGLE_AZ_1, and 160 to 10240 MBps for SINGLE_AZ_2 and MULTI_AZ_1, at the documented values",
				Doc:   createOpenZFSDoc,
				Break: createBreak(openZFSInput, func(in *fsx.CreateFileSystemInput) { in.OpenZFSConfiguration.ThroughputCapacity = aws.Int32(100) }),
				Code:  "BadRequest",
			},
			{
				Rule:  "OpenZFS: StorageCapacity is 64 to 524288 GiB",
				Break: createBreak(openZFSInput, func(in *fsx.CreateFileSystemInput) { in.StorageCapacity = aws.Int32(32) }),
				Code:  "BadRequest",
			},
			{
				Rule: "OpenZFS: RouteTableIds and EndpointIpAddressRange are for Multi-AZ file systems only, which need a PreferredSubnetId",
				Doc:  createOpenZFSDoc,
				Break: createBreak(openZFSInput, func(in *fsx.CreateFileSystemInput) {
					in.OpenZFSConfiguration.RouteTableIds = []string{"rtb-0a1b2c3d1"}
				}),
				Code: "BadRequest",
			},
			{
				Rule: "OpenZFS: AutomaticBackupRetentionDays defaults to 30, and CopyTagsToBackups and CopyTagsToVolumes to false",
				Doc:  createOpenZFSDoc,
				Check: createdCheck(openZFSInput(), func(fs *types.FileSystem) error {
					c := fs.OpenZFSConfiguration
					return errors.Join(expect("AutomaticBackupRetentionDays", aws.ToInt32(c.AutomaticBackupRetentionDays), 30), expect("CopyTagsToBackups", aws.ToBool(c.CopyTagsToBackups), false),
						expect("CopyTagsToVolumes", aws.ToBool(c.CopyTagsToVolumes), false))
				}),
			},
		}...),
		Unclear: "The page doesn't say how long FSx keeps a client request token, whether it checks the token before the " +
			"other parameters, or whether a token CreateFileSystemFromBackup used counts. The fake keeps tokens for its life, " +
			"checks a used token first, so a resend with other parameters gets IncompatibleParameterError, which the " +
			"handler relies on, and shares tokens between the two creates. The configuration pages state most rules in " +
			"prose without an error code (valid throughput and storage values, subnet counts, PreferredSubnetId, HDD and " +
			"Multi-AZ only options); the fake returns BadRequest, the call's generic client error, as the pages give for " +
			"ONTAP's throughput rules and a storage capacity out of range. The ONTAP page says a create defines " +
			"ThroughputCapacity or ThroughputCapacityPerHAPair, not both, and also lists a 400 for the two not being the " +
			"same; the fake follows the first and rejects both. A create without the configuration of its " +
			"type, or with another type's, isn't described; the fake checks only its type's configuration. The docs show " +
			"the key ARN FSx reports only for the FSx-managed key; the fake keeps a KmsKeyId the create sets as sent, " +
			"because the suite requires AWS to hold the template's values, and the suite's OtherForm and RecordedForm " +
			"cases give the ARN instead. A Lustre ThroughputCapacity is documented both as 1000, 2000, or a multiple of " +
			"4000 and as at least 4000, so the fake doesn't check it. Windows ThroughputCapacity is 8 to 2048 on this page " +
			"but up to 12288 in the performance guide; the fake takes both lists.",
	},
	"CreateFileSystemFromBackup": {
		Doc: apiReference + "API_CreateFileSystemFromBackup.html",
		Rules: append(tokenRules(restoreToken), []nativetest.Rule{
			{
				Rule: "BackupId is the ID of a backup FSx has",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).CreateFileSystemFromBackup(ctx, &fsx.CreateFileSystemFromBackupInput{BackupId: aws.String("backup-0a1b2c3d4"), SubnetIds: []string{subnetA}})
					return err
				},
				Code: "BackupNotFound",
			},
			{
				Rule: "BackupId is 12 to 128 characters of the form backup-[0-9a-f]{8,}",
				Doc:  commonErrors,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).CreateFileSystemFromBackup(ctx, &fsx.CreateFileSystemFromBackupInput{BackupId: aws.String("backup-1"), SubnetIds: []string{subnetA}})
					return err
				},
				Code: "ValidationError",
			},
			{
				Rule: "the file system has the backed-up file system's type and settings, such as its storage capacity and automatic backups, unless the call sets others",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					f.sources["backup-0a1b2c3d4"] = map[string]any{
						"FileSystemType": "WINDOWS", "StorageCapacity": 64.0,
						"WindowsConfiguration": map[string]any{"ThroughputCapacity": 32.0, "AutomaticBackupRetentionDays": 7.0},
					}
					out, err := f.CreateFileSystemFromBackup(ctx, &fsx.CreateFileSystemFromBackupInput{BackupId: aws.String("backup-0a1b2c3d4"), SubnetIds: []string{subnetA}})
					if err != nil {
						return err
					}
					fs := out.FileSystem
					return errors.Join(expect("FileSystemType", fs.FileSystemType, types.FileSystemTypeWindows), expect("StorageCapacity", aws.ToInt32(fs.StorageCapacity), 64),
						expect("AutomaticBackupRetentionDays", aws.ToInt32(fs.WindowsConfiguration.AutomaticBackupRetentionDays), 7))
				},
			},
			{
				Rule: "StorageCapacity of a restored file system other than OpenZFS is the backup's",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					f.sources["backup-0a1b2c3d4"] = map[string]any{"FileSystemType": "LUSTRE", "StorageCapacity": 1200.0}
					_, err := f.CreateFileSystemFromBackup(ctx, &fsx.CreateFileSystemFromBackupInput{BackupId: aws.String("backup-0a1b2c3d4"), SubnetIds: []string{subnetA}, StorageCapacity: aws.Int32(2400)})
					return err
				},
				Code: "BadRequest",
			},
			{
				Rule: "StorageCapacity of a restored OpenZFS file system is at least the backup's, from 64 to 524288 GiB",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := restoreToken(ctx, newFakeAPI(store), "token-a", -1)
					return err
				},
				Code: "BadRequest",
			},
			{
				Rule: "a Windows MULTI_AZ_1 file system takes exactly two subnets, and other Windows, Lustre, and OpenZFS file systems exactly one",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					f.sources["backup-0a1b2c3d4"] = map[string]any{"FileSystemType": "LUSTRE", "StorageCapacity": 1200.0}
					_, err := f.CreateFileSystemFromBackup(ctx, &fsx.CreateFileSystemFromBackupInput{BackupId: aws.String("backup-0a1b2c3d4"), SubnetIds: []string{subnetA, subnetB}})
					return err
				},
				Code: "BadRequest",
			},
			{
				Rule:  "a Windows restore the call makes MULTI_AZ_1 takes two subnets, though the backed-up file system was single-AZ",
				Break: restoreWindows(types.WindowsDeploymentTypeSingleAz2, types.WindowsDeploymentTypeMultiAz1, []string{subnetA}),
				Code:  "BadRequest",
			},
			{
				Rule:  "a Windows restore the call makes SINGLE_AZ_2 takes one subnet, though the backed-up file system was MULTI_AZ_1",
				Break: restoreWindows(types.WindowsDeploymentTypeMultiAz1, types.WindowsDeploymentTypeSingleAz2, []string{subnetA, subnetB}),
				Code:  "BadRequest",
			},
			{
				Rule: "StorageType defaults to SSD, and KmsKeyId to the FSx-managed key, not the backed-up file system's",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					f.sources["backup-0a1b2c3d4"] = map[string]any{
						"FileSystemType": "WINDOWS", "StorageCapacity": 2000.0, "StorageType": "HDD",
						"KmsKeyId":             "arn:aws:kms:us-west-2:123456789012:key/1234abcd-12ab-34cd-56ef-1234567890ab",
						"WindowsConfiguration": map[string]any{"DeploymentType": "SINGLE_AZ_2", "ThroughputCapacity": 32.0},
					}
					out, err := f.CreateFileSystemFromBackup(ctx, &fsx.CreateFileSystemFromBackupInput{BackupId: aws.String("backup-0a1b2c3d4"), SubnetIds: []string{subnetA}})
					if err != nil {
						return err
					}
					return errors.Join(expect("StorageType", out.FileSystem.StorageType, types.StorageTypeSsd), expect("KmsKeyId", aws.ToString(out.FileSystem.KmsKeyId), managedKey))
				},
			},
			{
				Rule: "Tags has 1 to 50 tags",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					f.sources["backup-0a1b2c3d4"] = map[string]any{"FileSystemType": "LUSTRE", "StorageCapacity": 1200.0}
					_, err := f.CreateFileSystemFromBackup(ctx, &fsx.CreateFileSystemFromBackupInput{BackupId: aws.String("backup-0a1b2c3d4"), SubnetIds: []string{subnetA}, Tags: manyTags(51)})
					return err
				},
				Code: "ValidationError",
			},
		}...),
		Unclear: "The page doesn't say how long FSx keeps a client request token, or whether a token CreateFileSystem used " +
			"counts. The fake shares tokens between the two creates, as the handler's restore test expects. It doesn't " +
			"say which settings of the backed-up file system a restore keeps beyond \"the Active Directory, default share " +
			"name, automatic backup, and backup settings\"; the fake keeps all of them unless the call sets others. The " +
			"subnet count has no error code; the fake returns BadRequest.",
	},
	"UpdateFileSystem": {
		Doc: apiReference + "API_UpdateFileSystem.html",
		Rules: []nativetest.Rule{
			{
				Rule: "FileSystemId is 11 to 21 characters of the form fs-[0-9a-f]{8,}",
				Doc:  commonErrors,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).UpdateFileSystem(ctx, &fsx.UpdateFileSystemInput{FileSystemId: aws.String("fs-1")})
					return err
				},
				Code: "ValidationError",
			},
			{
				Rule: "the file system exists",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).UpdateFileSystem(ctx, &fsx.UpdateFileSystemInput{FileSystemId: aws.String(unknownFileSystem)})
					return err
				},
				Code: "FileSystemNotFound",
			},
			{
				Rule: "ClientRequestToken is 1 to 63 characters of [A-za-z0-9_.-]",
				Doc:  commonErrors,
				Break: updateBreak(windowsInput, func(in *fsx.UpdateFileSystemInput) {
					in.ClientRequestToken = aws.String("not a token")
				}),
				Code: "ValidationError",
			},
			{
				Rule:  "storage capacity only increases, except on second- and third-generation ONTAP file systems",
				Break: updateBreak(openZFSInput, func(in *fsx.UpdateFileSystemInput) { in.StorageCapacity = aws.Int32(32) }),
				Code:  "BadRequest",
			},
			{
				Rule:  "Windows and OpenZFS: a storage capacity increase is at least 10 percent",
				Break: updateBreak(windowsInput, func(in *fsx.UpdateFileSystemInput) { in.StorageCapacity = aws.Int32(34) }),
				Code:  "BadRequest",
			},
			{
				Rule: "Windows: increasing storage capacity needs at least 16 MBps of throughput capacity",
				Break: updateBreak(func() *fsx.CreateFileSystemInput {
					in := windowsInput()
					in.WindowsConfiguration.ThroughputCapacity = aws.Int32(8)
					return in
				}, func(in *fsx.UpdateFileSystemInput) { in.StorageCapacity = aws.Int32(64) }),
				Code: "BadRequest",
			},
			{
				Rule:  "Lustre: an SSD file system's storage capacity increases to a multiple of 2400 GiB",
				Break: updateBreak(lustreInput, func(in *fsx.UpdateFileSystemInput) { in.StorageCapacity = aws.Int32(3000) }),
				Code:  "BadRequest",
			},
			{
				Rule: "Lustre: a SCRATCH_1 file system's storage capacity can't increase",
				Break: updateBreak(func() *fsx.CreateFileSystemInput {
					return &fsx.CreateFileSystemInput{FileSystemType: types.FileSystemTypeLustre, StorageCapacity: aws.Int32(1200), SubnetIds: []string{subnetA}}
				}, func(in *fsx.UpdateFileSystemInput) { in.StorageCapacity = aws.Int32(2400) }),
				Code: "BadRequest",
			},
			{
				Rule:  "ONTAP: an SSD storage capacity increase is at least 10 percent, or up to the maximum",
				Doc:   ontapStorageGuide,
				Break: updateBreak(ontapInput, func(in *fsx.UpdateFileSystemInput) { in.StorageCapacity = aws.Int32(1100) }),
				Code:  "BadRequest",
			},
			{
				Rule: "a file system takes no storage capacity increase within 6 hours of the last one, or while one is in progress",
				Doc:  windowsStorageGuide,
				Break: updateBreak(windowsInput,
					func(in *fsx.UpdateFileSystemInput) { in.StorageCapacity = aws.Int32(64) },
					func(in *fsx.UpdateFileSystemInput) { in.StorageCapacity = aws.Int32(128) }),
				Code: "BadRequest",
			},
			{
				Rule:  "Lustre: FileSystemTypeVersion is 2.12 or 2.15, newer than the file system's",
				Break: updateBreak(lustreInput, func(in *fsx.UpdateFileSystemInput) { in.FileSystemTypeVersion = aws.String("2.12") }),
				Code:  "BadRequest",
			},
			{
				Rule: "Windows: ThroughputCapacity is 8, 16, 32, 64, 128, 256, 512, 1024, 2048, 4608, 6144, 9216, or 12288 MBps",
				Doc:  windowsPerformance,
				Break: updateBreak(windowsInput, func(in *fsx.UpdateFileSystemInput) {
					in.WindowsConfiguration = &types.UpdateFileSystemWindowsConfiguration{ThroughputCapacity: aws.Int32(4096)}
				}),
				Code: "BadRequest",
			},
			{
				Rule: "Windows: a throughput capacity update can't start while another is in progress",
				Doc:  updateWindowsDoc,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					id, err := available(ctx, f, windowsInput())
					if err != nil {
						return err
					}
					for _, throughput := range []int32{64, 128} {
						in := &fsx.UpdateFileSystemInput{FileSystemId: aws.String(id), WindowsConfiguration: &types.UpdateFileSystemWindowsConfiguration{ThroughputCapacity: aws.Int32(throughput)}}
						if _, err := f.UpdateFileSystem(ctx, in); err != nil {
							return err
						}
					}
					return nil
				},
				Code: "BadRequest",
			},
			{
				Rule: "Windows: a self-managed Active Directory update can't start while another is in progress",
				Doc:  updateWindowsDoc,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					id, err := available(ctx, f, windowsInput())
					if err != nil {
						return err
					}
					for _, user := range []string{"one", "two"} {
						ad := &types.SelfManagedActiveDirectoryConfigurationUpdates{UserName: aws.String(user), Password: aws.String("secret")}
						in := &fsx.UpdateFileSystemInput{FileSystemId: aws.String(id), WindowsConfiguration: &types.UpdateFileSystemWindowsConfiguration{SelfManagedActiveDirectoryConfiguration: ad}}
						if _, err := f.UpdateFileSystem(ctx, in); err != nil {
							return err
						}
					}
					return nil
				},
				Code: "BadRequest",
			},
			{
				Rule: "OpenZFS: ThroughputCapacity is one of the values documented for the deployment type",
				Doc:  updateOpenZFSDoc,
				Break: updateBreak(openZFSInput, func(in *fsx.UpdateFileSystemInput) {
					in.OpenZFSConfiguration = &types.UpdateFileSystemOpenZFSConfiguration{ThroughputCapacity: aws.Int32(160)}
				}),
				Code: "BadRequest",
			},
			{
				Rule: "ONTAP: FsxAdminPassword is 8 to 50 characters",
				Doc:  updateOntapDoc,
				Break: updateBreak(ontapInput, func(in *fsx.UpdateFileSystemInput) {
					in.OntapConfiguration = &types.UpdateFileSystemOntapConfiguration{FsxAdminPassword: aws.String(strings.Repeat("é", 7))}
				}),
				Code: "ValidationError",
			},
			{
				Rule: "ONTAP: HAPairs is 1 to 12",
				Doc:  commonErrors,
				Break: updateBreak(ontapInput, func(in *fsx.UpdateFileSystemInput) {
					in.OntapConfiguration = &types.UpdateFileSystemOntapConfiguration{HAPairs: aws.Int32(0), ThroughputCapacity: aws.Int32(1536)}
				}),
				Code: "ValidationError",
			},
			{
				Rule: "ONTAP: HAPairs is at most 12",
				Doc:  commonErrors,
				Break: updateBreak(ontapInput, func(in *fsx.UpdateFileSystemInput) {
					in.OntapConfiguration = &types.UpdateFileSystemOntapConfiguration{HAPairs: aws.Int32(13)}
				}),
				Code: "ValidationError",
			},
			{
				Rule: "ONTAP: ThroughputCapacity and ThroughputCapacityPerHAPair can't be in the same call",
				Doc:  updateOntapDoc,
				Break: updateBreak(ontapInput, func(in *fsx.UpdateFileSystemInput) {
					in.OntapConfiguration = &types.UpdateFileSystemOntapConfiguration{ThroughputCapacity: aws.Int32(256), ThroughputCapacityPerHAPair: aws.Int32(256)}
				}),
				Code: "BadRequest",
			},
			{
				Rule: "ONTAP: the throughput capacity per HA pair is one of the values documented for the deployment type",
				Doc:  updateOntapDoc,
				Break: updateBreak(ontapInput, func(in *fsx.UpdateFileSystemInput) {
					in.OntapConfiguration = &types.UpdateFileSystemOntapConfiguration{ThroughputCapacityPerHAPair: aws.Int32(384)}
				}),
				Code: "BadRequest",
			},
			{
				Rule: "ONTAP: ThroughputCapacityPerHAPair is ThroughputCapacity for a file system with one HA pair",
				Doc:  updateOntapDoc,
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					id, err := available(ctx, f, ontapInput())
					if err != nil {
						return err
					}
					in := &fsx.UpdateFileSystemInput{FileSystemId: aws.String(id), OntapConfiguration: &types.UpdateFileSystemOntapConfiguration{ThroughputCapacity: aws.Int32(256)}}
					if _, err := f.UpdateFileSystem(ctx, in); err != nil {
						return err
					}
					fs, err := describe(ctx, f, id)
					if err != nil {
						return err
					}
					return expect("ThroughputCapacityPerHAPair", aws.ToInt32(fs.OntapConfiguration.ThroughputCapacityPerHAPair), 256)
				},
			},
			{
				Rule: "Lustre: PerUnitStorageThroughput is 50, 100, or 200 for PERSISTENT_1 SSD, and 125, 250, 500, or 1000 for PERSISTENT_2 SSD",
				Doc:  updateLustreDoc,
				Break: updateBreak(lustreInput, func(in *fsx.UpdateFileSystemInput) {
					in.LustreConfiguration = &types.UpdateFileSystemLustreConfiguration{PerUnitStorageThroughput: aws.Int32(200)}
				}),
				Code: "BadRequest",
			},
			{
				Rule: "AutomaticBackupRetentionDays is 0 to 90, WeeklyMaintenanceStartTime is d:HH:MM, and DailyAutomaticBackupStartTime is HH:MM",
				Doc:  updateLustreDoc,
				Break: updateBreak(lustreInput, func(in *fsx.UpdateFileSystemInput) {
					in.LustreConfiguration = &types.UpdateFileSystemLustreConfiguration{WeeklyMaintenanceStartTime: aws.String("8:05:00")}
				}),
				Code: "ValidationError",
			},
		},
		Unclear: "The pages don't say which lifecycle states a file system takes an update in; the fake takes one in any " +
			"state. They state the storage, throughput, version, and in-progress rules without an error code; the fake " +
			"returns BadRequest, the call's generic client error. The fake has no clock: it treats its calls as within " +
			"6 hours, so it rejects any second storage capacity increase of a file system. The page says NetworkType " +
			"changes an OpenZFS file system and lists StorageType only for Windows, without saying what FSx does with " +
			"either for another type; the fake applies both. It doesn't say what FSx does with another type's " +
			"configuration, or with ONTAP HAPairs on a first-generation file system; the fake applies them. FSx's SDK " +
			"fills in a new ClientRequestToken for each call the handler makes, so a resent update isn't the same " +
			"request to FSx.",
	},
	"DeleteFileSystem": {
		Doc: deleteDoc,
		Rules: append(tokenRules(deleteToken), []nativetest.Rule{
			{
				Rule: "FileSystemId is 11 to 21 characters of the form fs-[0-9a-f]{8,}",
				Doc:  commonErrors,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).DeleteFileSystem(ctx, &fsx.DeleteFileSystemInput{FileSystemId: aws.String("fs-missing")})
					return err
				},
				Code: "ValidationError",
			},
			{
				Rule: "the file system exists",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).DeleteFileSystem(ctx, &fsx.DeleteFileSystemInput{FileSystemId: aws.String(unknownFileSystem)})
					return err
				},
				Code: "FileSystemNotFound",
			},
			{
				Rule:  "Windows: FSx takes a final backup unless SkipFinalBackup is true",
				Doc:   deleteWindowsDoc,
				Check: deleteBackups(windowsInput, fsx.DeleteFileSystemInput{}, 1),
			},
			{
				Rule:  "OpenZFS: FSx takes a final backup unless SkipFinalBackup is true",
				Doc:   deleteOpenZFSDoc,
				Check: deleteBackups(openZFSInput, fsx.DeleteFileSystemInput{OpenZFSConfiguration: &types.DeleteFileSystemOpenZFSConfiguration{}}, 1),
			},
			{
				Rule:  "Lustre: FSx takes no final backup unless SkipFinalBackup is false",
				Doc:   deleteLustreDoc,
				Check: deleteBackups(lustreInput, fsx.DeleteFileSystemInput{LustreConfiguration: &types.DeleteFileSystemLustreConfiguration{}}, 0),
			},
			{
				Rule: "Lustre: FSx takes no backup of a file system linked to S3",
				Break: deleteBreak(lustreInput, func(f *fakeAPI, id string) { f.associations[id] = 1 }, fsx.DeleteFileSystemInput{
					LustreConfiguration: &types.DeleteFileSystemLustreConfiguration{SkipFinalBackup: aws.Bool(false)},
				}),
				Code: "BadRequest",
			},
			{
				Rule:  "Lustre: a delete fails while a data repository task is PENDING or EXECUTING",
				Break: deleteBreak(lustreInput, func(f *fakeAPI, id string) { f.tasks[id] = true }, fsx.DeleteFileSystemInput{}),
				Code:  "BadRequest",
			},
			{
				Rule: "FinalBackupTags has 1 to 50 tags",
				Doc:  deleteWindowsDoc,
				Break: deleteBreak(windowsInput, nil, fsx.DeleteFileSystemInput{
					WindowsConfiguration: &types.DeleteFileSystemWindowsConfiguration{FinalBackupTags: []types.Tag{}},
				}),
				Code: "ValidationError",
			},
			{
				Rule: "OpenZFS: Options has up to one value, DELETE_CHILD_VOLUMES_AND_SNAPSHOTS",
				Doc:  deleteOpenZFSDoc,
				Break: deleteBreak(openZFSInput, nil, fsx.DeleteFileSystemInput{
					OpenZFSConfiguration: &types.DeleteFileSystemOpenZFSConfiguration{Options: []types.DeleteFileSystemOpenZFSOption{"DELETE_EVERYTHING"}},
				}),
				Code: "ValidationError",
			},
		}...),
		Unclear: "The page doesn't say which lifecycle states a file system can be deleted in, or what FSx does with a " +
			"delete of a file system it is already deleting, or with a configuration of another type. The fake rejects " +
			"both with BadRequest and takes a delete in any other state. It says an ONTAP file system's volumes and " +
			"storage virtual machines, and an OpenZFS file system's S3 access points, must go first, and that an OpenZFS " +
			"file system with child volumes needs DELETE_CHILD_VOLUMES_AND_SNAPSHOTS, without an error code; the fake has " +
			"no volumes, SVMs, or access points. It gives no code for a final backup of an S3-linked Lustre file " +
			"system; the fake returns BadRequest. It doesn't say how long FSx keeps a token.",
	},
	"DescribeFileSystems": {
		Doc: apiReference + "API_DescribeFileSystems.html",
		Rules: []nativetest.Rule{
			{
				Rule: "FileSystemIds has up to 50 IDs of the form fs-[0-9a-f]{8,}",
				Doc:  commonErrors,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).DescribeFileSystems(ctx, &fsx.DescribeFileSystemsInput{FileSystemIds: []string{"fs-1"}})
					return err
				},
				Code: "ValidationError",
			},
			{
				Rule: "MaxResults is greater than 0",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).DescribeFileSystems(ctx, &fsx.DescribeFileSystemsInput{MaxResults: aws.Int32(0)})
					return err
				},
				Code: "ValidationError",
			},
			{
				Rule: "a deleted file system's ID returns FileSystemNotFound",
				Doc:  deleteDoc,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					id, err := available(ctx, f, ontapInput())
					if err != nil {
						return err
					}
					if _, err := f.DeleteFileSystem(ctx, &fsx.DeleteFileSystemInput{FileSystemId: aws.String(id)}); err != nil {
						return err
					}
					for {
						if _, err := describe(ctx, f, id); err != nil {
							return err
						}
					}
				},
				Code: "FileSystemNotFound",
			},
			{
				Rule: "FSx may return fewer file systems than MaxResults with a NextToken",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					for _, token := range []string{"token-a", "token-b"} {
						in := openZFSInput()
						in.ClientRequestToken = aws.String(token)
						if _, err := f.CreateFileSystem(ctx, in); err != nil {
							return err
						}
					}
					out, err := f.DescribeFileSystems(ctx, &fsx.DescribeFileSystemsInput{MaxResults: aws.Int32(10)})
					if err != nil {
						return err
					}
					if len(out.FileSystems) == 2 || out.NextToken == nil {
						return errors.New("the fake returned every file system on one page")
					}
					return nil
				},
			},
		},
	},
	"DescribeDataRepositoryAssociations": {
		Doc: apiReference + "API_DescribeDataRepositoryAssociations.html",
		Rules: []nativetest.Rule{
			{
				Rule: "MaxResults is 1 to 25",
				Doc:  commonErrors,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).DescribeDataRepositoryAssociations(ctx, &fsx.DescribeDataRepositoryAssociationsInput{MaxResults: aws.Int32(26)})
					return err
				},
				Code: "ValidationError",
			},
			{
				Rule: "a filter's name is one of the documented names, and it has up to 20 values of 1 to 128 characters of [0-9a-zA-Z*.\\/?_-]",
				Doc:  filterDoc,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).DescribeDataRepositoryAssociations(ctx, &fsx.DescribeDataRepositoryAssociationsInput{
						Filters: []types.Filter{{Name: "file-system", Values: []string{"fs-0123456789abcdef0"}}},
					})
					return err
				},
				Code: "ValidationError",
			},
			{
				Rule: "the file-system-id filter returns only the associations of that file system",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					f.associations["fs-0000000000000000a"], f.associations["fs-0000000000000000b"] = 1, 2
					out, err := f.DescribeDataRepositoryAssociations(ctx, &fsx.DescribeDataRepositoryAssociationsInput{
						Filters: []types.Filter{{Name: types.FilterNameFileSystemId, Values: []string{"fs-0000000000000000a"}}},
					})
					if err != nil {
						return err
					}
					return expect("the number of associations", len(out.Associations), 1)
				},
			},
		},
		Unclear: "The page says associations are supported on Lustre 2.12 and 2.15 file systems other than SCRATCH_1, " +
			"without saying what FSx returns for another one, or for a file system it doesn't have. The fake returns no " +
			"associations; the handler asks only about PERSISTENT_2 file systems, whose versions are 2.12 or 2.15.",
	},
	"TagResource": {
		Doc: apiReference + "API_TagResource.html",
		Rules: []nativetest.Rule{
			{
				Rule: "Tags has 1 to 50 tags",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					arn, err := tagged(ctx, f, 0)
					if err != nil {
						return err
					}
					_, err = f.TagResource(ctx, &fsx.TagResourceInput{ResourceARN: arn, Tags: []types.Tag{}})
					return err
				},
				Code: "ValidationError",
			},
			{
				Rule: "a tag key is 1 to 128 and a value up to 256 letters, numbers, spaces, or _.:/=+-@",
				Doc:  tagDoc,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					arn, err := tagged(ctx, f, 0)
					if err != nil {
						return err
					}
					_, err = f.TagResource(ctx, &fsx.TagResourceInput{ResourceARN: arn, Tags: []types.Tag{{Key: aws.String("team"), Value: aws.String("a#b")}}})
					return err
				},
				Code: "ValidationError",
			},
			{
				Rule: "ResourceARN is 8 to 512 characters",
				Doc:  commonErrors,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).TagResource(ctx, &fsx.TagResourceInput{ResourceARN: aws.String("arn:aws"), Tags: manyTags(1)})
					return err
				},
				Code: "ValidationError",
			},
			{
				Rule: "the resource exists",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).TagResource(ctx, &fsx.TagResourceInput{ResourceARN: aws.String(arnPrefix + unknownFileSystem), Tags: manyTags(1)})
					return err
				},
				Code: "ResourceNotFound",
			},
			{
				Rule: "a tag with a key the resource has replaces its value",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					arn, err := tagged(ctx, f, 1)
					if err != nil {
						return err
					}
					if _, err := f.TagResource(ctx, &fsx.TagResourceInput{ResourceARN: arn, Tags: []types.Tag{{Key: aws.String("key-0"), Value: aws.String("new")}}}); err != nil {
						return err
					}
					fs, err := describe(ctx, f, strings.TrimPrefix(aws.ToString(arn), arnPrefix))
					if err != nil {
						return err
					}
					return errors.Join(expect("the number of tags", len(fs.Tags), 1), expect("the tag's value", aws.ToString(fs.Tags[0].Value), "new"))
				},
			},
			{
				Rule: "a resource has up to 50 tags, not counting tags with the aws: prefix",
				Doc:  tagGuide,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					arn, err := tagged(ctx, f, 50)
					if err != nil {
						return err
					}
					_, err = f.TagResource(ctx, &fsx.TagResourceInput{ResourceARN: arn, Tags: []types.Tag{{Key: aws.String("one-more"), Value: aws.String("v")}}})
					return err
				},
				Code: "BadRequest",
			},
			{
				Rule: "the aws: prefix is reserved for AWS",
				Doc:  tagGuide,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					arn, err := tagged(ctx, f, 0)
					if err != nil {
						return err
					}
					_, err = f.TagResource(ctx, &fsx.TagResourceInput{ResourceARN: arn, Tags: []types.Tag{{Key: aws.String("aws:team"), Value: aws.String("a")}}})
					return err
				},
				Code: "BadRequest",
			},
		},
		Unclear: "The guide gives the 50-tag limit and the reserved aws: prefix without an error code; the fake returns " +
			"BadRequest, the call's generic client error. The handler's own tags (the operation tag, and the delete " +
			"options and update baseline tags when it adds them) count toward the limit, so a template with 49 or 50 " +
			"tags can fail.",
	},
	"UntagResource": {
		Doc: apiReference + "API_UntagResource.html",
		Rules: []nativetest.Rule{
			{
				Rule: "TagKeys has 1 to 50 keys of 1 to 128 characters",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					arn, err := tagged(ctx, f, 1)
					if err != nil {
						return err
					}
					_, err = f.UntagResource(ctx, &fsx.UntagResourceInput{ResourceARN: arn, TagKeys: []string{}})
					return err
				},
				Code: "ValidationError",
			},
			{
				Rule: "a key the resource doesn't have succeeds, so the call is idempotent",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					arn, err := tagged(ctx, f, 1)
					if err != nil {
						return err
					}
					_, err = f.UntagResource(ctx, &fsx.UntagResourceInput{ResourceARN: arn, TagKeys: []string{"absent"}})
					return err
				},
			},
			{
				Rule: "the resource exists",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).UntagResource(ctx, &fsx.UntagResourceInput{ResourceARN: aws.String(arnPrefix + unknownFileSystem), TagKeys: []string{"team"}})
					return err
				},
				Code: "ResourceNotFound",
			},
			{
				Rule: "a tag with the aws: prefix can't be deleted",
				Doc:  tagGuide,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					arn, err := tagged(ctx, f, 1)
					if err != nil {
						return err
					}
					_, err = f.UntagResource(ctx, &fsx.UntagResourceInput{ResourceARN: arn, TagKeys: []string{"aws:cloudformation:stack-name"}})
					return err
				},
				Code: "BadRequest",
			},
		},
		Unclear: "The guide gives the reserved aws: prefix without an error code; the fake returns BadRequest.",
	},
}
