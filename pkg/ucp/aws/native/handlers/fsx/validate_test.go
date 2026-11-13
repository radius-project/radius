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
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/fsx"
	"github.com/aws/aws-sdk-go-v2/service/fsx/types"
)

// This file holds the checks the fake makes of each call's input, as FSx's API reference documents them. See
// api_test.go for the record of each rule and the page that states it.

var (
	tokenPattern      = regexp.MustCompile(`^[A-za-z0-9_.-]{1,63}$`)
	tagPattern        = regexp.MustCompile(`^[\p{L}\p{Z}\p{N}_.:/=+\-@]*$`)
	fileSystemPattern = regexp.MustCompile(`^fs-[0-9a-f]{8,}$`)
	subnetPattern     = regexp.MustCompile(`^subnet-[0-9a-f]{8,}$`)
	groupPattern      = regexp.MustCompile(`^sg-[0-9a-f]{8,}$`)
	routeTablePattern = regexp.MustCompile(`^rtb-[0-9a-f]{8,}$`)
	backupPattern     = regexp.MustCompile(`^backup-[0-9a-f]{8,}$`)
	dailyPattern      = regexp.MustCompile(`^([01]\d|2[0-3]):?([0-5]\d)$`)
	weeklyPattern     = regexp.MustCompile(`^[1-7]:([01]\d|2[0-3]):?([0-5]\d)$`)
	filterPattern     = regexp.MustCompile(`^[0-9a-zA-Z\*\.\\/\?\-\_]*$`)
)

// maxTags is how many tags FSx lets a resource have, not counting tags with the aws: prefix.
const maxTags = 50

// invalid returns ValidationError, which FSx returns for input that doesn't meet a parameter's format or constraints.
func invalid(format string, args ...any) error {
	return fault("ValidationError", fmt.Sprintf(format, args...))
}

// badRequest returns BadRequest, FSx's error for a client request it rejects for another reason.
func badRequest(format string, args ...any) error {
	return fault("BadRequest", fmt.Sprintf(format, args...))
}

// validToken checks a client request token, which is optional.
func validToken(token *string) error {
	if token != nil && !tokenPattern.MatchString(*token) {
		return invalid("ClientRequestToken must be 1 to 63 characters of [A-za-z0-9_.-].")
	}
	return nil
}

// validTag checks a tag key, and value unless it is nil.
func validTag(key string, value *string) error {
	if n := len([]rune(key)); n < 1 || n > 128 || !tagPattern.MatchString(key) {
		return invalid("Tag key %q must be 1 to 128 letters, numbers, spaces, or _.:/=+-@.", key)
	}
	if value != nil {
		if len([]rune(*value)) > 256 || !tagPattern.MatchString(*value) {
			return invalid("Tag value %q must be up to 256 letters, numbers, spaces, or _.:/=+-@.", *value)
		}
	}
	return nil
}

// validTags checks a list of tags the call sends, which must have 1 to 50 tags.
func validTags(field string, tags []types.Tag) error {
	if tags == nil {
		return nil
	}
	if len(tags) < 1 || len(tags) > maxTags {
		return invalid("%s must have 1 to %d tags.", field, maxTags)
	}
	for _, t := range tags {
		if t.Key == nil || t.Value == nil {
			return invalid("A tag in %s needs a Key and a Value.", field)
		}
		if err := validTag(*t.Key, t.Value); err != nil {
			return err
		}
	}
	return nil
}

// validIDs checks a list of up to 50 IDs of a pattern and length.
func validIDs(field string, ids []string, pattern *regexp.Regexp, minLength int, maxLength int) error {
	if len(ids) > 50 {
		return invalid("%s can have up to 50 items.", field)
	}
	for _, id := range ids {
		if err := validID(field, id, pattern, minLength, maxLength); err != nil {
			return err
		}
	}
	return nil
}

func validID(field string, id string, pattern *regexp.Regexp, minLength int, maxLength int) error {
	if len(id) < minLength || len(id) > maxLength || !pattern.MatchString(id) {
		return invalid("%s %q must match %s.", field, id, pattern)
	}
	return nil
}

// validLength reports whether s has min to max characters. The patterns of such parameters take any Unicode
// character, so FSx counts characters, not bytes.
func validLength(s string, min int, max int) bool {
	n := utf8.RuneCountInString(s)
	return n >= min && n <= max
}

// validPassword checks an ONTAP FsxAdminPassword, which is optional.
func validPassword(password *string) error {
	if password != nil && !validLength(*password, 8, 50) {
		return invalid("FsxAdminPassword must be 8 to 50 characters.")
	}
	return nil
}

// validPreferredSubnet checks a PreferredSubnetId, which is optional.
func validPreferredSubnet(id *string) error {
	if id == nil {
		return nil
	}
	return validID("PreferredSubnetId", *id, subnetPattern, 15, 24)
}

func validFileSystemID(id string) error {
	return validID("FileSystemId", id, fileSystemPattern, 11, 21)
}

// validSchedule checks the backup retention and the maintenance and backup times that every configuration takes.
func validSchedule(retention *int32, weekly *string, daily *string) error {
	if retention != nil && (*retention < 0 || *retention > 90) {
		return invalid("AutomaticBackupRetentionDays must be 0 to 90.")
	}
	if weekly != nil && (len(*weekly) != 7 || !weeklyPattern.MatchString(*weekly)) {
		return invalid("WeeklyMaintenanceStartTime must be d:HH:MM.")
	}
	if daily != nil && (len(*daily) != 5 || !dailyPattern.MatchString(*daily)) {
		return invalid("DailyAutomaticBackupStartTime must be HH:MM.")
	}
	return nil
}

// validNetwork checks the subnets and security groups of a create.
func validNetwork(subnets []string, groups []string) error {
	if len(subnets) == 0 {
		return invalid("SubnetIds is required.")
	}
	if err := validIDs("SubnetIds", subnets, subnetPattern, 15, 24); err != nil {
		return err
	}
	return validIDs("SecurityGroupIds", groups, groupPattern, 11, 20)
}

// windowsThroughput are the throughput capacities, in MBps, an FSx for Windows File Server file system can have.
var windowsThroughput = []int32{8, 16, 32, 64, 128, 256, 512, 1024, 2048, 4608, 6144, 9216, 12288}

// openZFSThroughput are the throughput capacities, in MBps, of each OpenZFS deployment type that documents them.
var openZFSThroughput = map[types.OpenZFSDeploymentType][]int32{
	types.OpenZFSDeploymentTypeSingleAz1: {64, 128, 256, 512, 1024, 2048, 3072, 4096},
	types.OpenZFSDeploymentTypeSingleAz2: {160, 320, 640, 1280, 2560, 3840, 5120, 7680, 10240},
	types.OpenZFSDeploymentTypeMultiAz1:  {160, 320, 640, 1280, 2560, 3840, 5120, 7680, 10240},
}

// ontapThroughputPerPair are the throughput capacities per HA pair, in MBps, of each ONTAP deployment type.
var ontapThroughputPerPair = map[string][]int32{
	"SINGLE_AZ_1": {128, 256, 512, 1024, 2048, 4096},
	"MULTI_AZ_1":  {128, 256, 512, 1024, 2048, 4096},
	"SINGLE_AZ_2": {1536, 3072, 6144},
	"MULTI_AZ_2":  {384, 768, 1536, 3072, 6144},
	"SINGLE_AZ_3": {1536, 3072, 6144, 12288, 18432},
	"MULTI_AZ_3":  {384, 768, 1536, 3072, 6144, 12288, 18432},
}

// lustreThroughputPerTiB are the PerUnitStorageThroughput values, in MB/s/TiB, of each persistent Lustre deployment
// and storage type.
var lustreThroughputPerTiB = map[string][]int32{
	"PERSISTENT_1/SSD": {50, 100, 200},
	"PERSISTENT_1/HDD": {12, 40},
	"PERSISTENT_2/SSD": {125, 250, 500, 1000},
}

// ontapSinglePair reports whether an ONTAP deployment type has exactly one HA pair.
func ontapSinglePair(deployment string) bool {
	return deployment == "SINGLE_AZ_1" || strings.HasPrefix(deployment, "MULTI_AZ_")
}

// ontapFirstGeneration reports whether an ONTAP deployment type is a first-generation one, whose storage capacity
// can only increase.
func ontapFirstGeneration(deployment string) bool {
	return strings.HasSuffix(deployment, "_1")
}

// validateCreate checks the input of a CreateFileSystem call, other than its token.
func validateCreate(in *fsx.CreateFileSystemInput) error {
	if err := validTags("Tags", in.Tags); err != nil {
		return err
	}
	if err := validNetwork(in.SubnetIds, in.SecurityGroupIds); err != nil {
		return err
	}
	if in.KmsKeyId != nil && !validLength(*in.KmsKeyId, 1, 2048) {
		return invalid("KmsKeyId must be 1 to 2048 characters.")
	}
	storage := in.StorageType
	if storage == "" {
		storage = types.StorageTypeSsd
	}
	switch in.FileSystemType {
	case types.FileSystemTypeLustre:
		return validateLustre(in.StorageCapacity, storage, in.FileSystemTypeVersion, in.LustreConfiguration, in.SubnetIds)
	case types.FileSystemTypeWindows:
		return validateWindows(in.StorageCapacity, storage, in.WindowsConfiguration, in.SubnetIds)
	case types.FileSystemTypeOntap:
		return validateOntap(in.StorageCapacity, in.OntapConfiguration, in.SubnetIds)
	case types.FileSystemTypeOpenzfs:
		return validateOpenZFS(in.StorageCapacity, in.OpenZFSConfiguration)
	}
	return invalid("FileSystemType must be WINDOWS, LUSTRE, ONTAP, or OPENZFS.")
}

// lustreDeployment returns the deployment type of a Lustre configuration, which is SCRATCH_1 when it sets none.
func lustreDeployment(c *types.CreateFileSystemLustreConfiguration) types.LustreDeploymentType {
	if c == nil || c.DeploymentType == "" {
		return types.LustreDeploymentTypeScratch1
	}
	return c.DeploymentType
}

func validateLustre(capacity *int32, storage types.StorageType, version *string, c *types.CreateFileSystemLustreConfiguration, subnets []string) error {
	deployment := lustreDeployment(c)
	if c == nil {
		c = &types.CreateFileSystemLustreConfiguration{}
	}
	if err := validSchedule(c.AutomaticBackupRetentionDays, c.WeeklyMaintenanceStartTime, c.DailyAutomaticBackupStartTime); err != nil {
		return err
	}
	if len(subnets) != 1 {
		return badRequest("A Lustre file system takes exactly one subnet.")
	}
	persistent := strings.HasPrefix(string(deployment), "PERSISTENT")
	switch storage {
	case types.StorageTypeHdd:
		if deployment != types.LustreDeploymentTypePersistent1 {
			return badRequest("HDD storage is supported only on PERSISTENT_1 Lustre file systems.")
		}
		if c.DriveCacheType == "" {
			return badRequest("DriveCacheType is required when StorageType is HDD.")
		}
	case types.StorageTypeIntelligentTiering:
		if deployment != types.LustreDeploymentTypePersistent2 {
			return badRequest("Intelligent-Tiering is supported only on PERSISTENT_2 Lustre file systems.")
		}
	}
	if aws.ToString(version) == "2.10" && deployment == types.LustreDeploymentTypePersistent2 {
		return badRequest("PERSISTENT_2 file systems don't support Lustre 2.10.")
	}
	if persistent && storage != types.StorageTypeIntelligentTiering {
		if c.PerUnitStorageThroughput == nil {
			return badRequest("PerUnitStorageThroughput is required for %s file systems.", deployment)
		}
		if !slices.Contains(lustreThroughputPerTiB[string(deployment)+"/"+string(storage)], *c.PerUnitStorageThroughput) {
			return fault("InvalidPerUnitStorageThroughput", fmt.Sprintf("PerUnitStorageThroughput %d isn't valid for %s %s storage.", *c.PerUnitStorageThroughput, deployment, storage))
		}
	}
	for _, path := range []*string{c.ImportPath, c.ExportPath} {
		if path != nil && !validLength(*path, 3, 4357) {
			return invalid("ImportPath and ExportPath must be 3 to 4357 characters.")
		}
	}
	if c.ExportPath != nil && s3Bucket(*c.ExportPath) != s3Bucket(aws.ToString(c.ImportPath)) {
		return fault("InvalidExportPath", "ExportPath must use the bucket of ImportPath.")
	}
	if capacity != nil && !validLustreCapacity(*capacity, deployment, storage, c.PerUnitStorageThroughput) {
		return badRequest("StorageCapacity %d isn't valid for a %s %s Lustre file system.", *capacity, deployment, storage)
	}
	return nil
}

// validLustreCapacity reports whether a Lustre file system can be created with the storage capacity.
func validLustreCapacity(capacity int32, deployment types.LustreDeploymentType, storage types.StorageType, perTiB *int32) bool {
	switch {
	case storage == types.StorageTypeHdd:
		step := int32(6000)
		if aws.ToInt32(perTiB) == 40 {
			step = 1800
		}
		return capacity > 0 && capacity%step == 0
	case storage == types.StorageTypeIntelligentTiering:
		return true
	case deployment == types.LustreDeploymentTypeScratch1:
		return capacity == 1200 || capacity == 2400 || (capacity > 0 && capacity%3600 == 0)
	}
	return capacity == 1200 || (capacity > 0 && capacity%2400 == 0)
}

// s3Bucket returns the bucket of an S3 path such as s3://bucket/prefix.
func s3Bucket(path string) string {
	bucket, _, _ := strings.Cut(strings.TrimPrefix(path, "s3://"), "/")
	return bucket
}

// windowsDeployment returns the deployment type of a Windows configuration, which is SINGLE_AZ_1 when it sets none.
func windowsDeployment(deployment types.WindowsDeploymentType) types.WindowsDeploymentType {
	if deployment == "" {
		return types.WindowsDeploymentTypeSingleAz1
	}
	return deployment
}

func validateWindows(capacity *int32, storage types.StorageType, c *types.CreateFileSystemWindowsConfiguration, subnets []string) error {
	if c == nil || c.ThroughputCapacity == nil {
		return invalid("WindowsConfiguration ThroughputCapacity is required.")
	}
	if err := validSchedule(c.AutomaticBackupRetentionDays, c.WeeklyMaintenanceStartTime, c.DailyAutomaticBackupStartTime); err != nil {
		return err
	}
	if len(c.Aliases) > 50 {
		return invalid("Aliases can have up to 50 items.")
	}
	for _, a := range c.Aliases {
		if !validLength(a, 4, 253) {
			return invalid("An alias must be 4 to 253 characters.")
		}
	}
	if ad := c.SelfManagedActiveDirectoryConfiguration; ad != nil {
		if aws.ToString(ad.DomainName) == "" || len(ad.DnsIps) == 0 {
			return invalid("SelfManagedActiveDirectoryConfiguration needs DomainName and DnsIps.")
		}
		if len(ad.DnsIps) > 2 {
			return badRequest("FSx for Windows File Server supports up to two DnsIps.")
		}
	}
	if err := validWindowsThroughput(*c.ThroughputCapacity); err != nil {
		return err
	}
	if err := validPreferredSubnet(c.PreferredSubnetId); err != nil {
		return err
	}
	deployment := windowsDeployment(c.DeploymentType)
	if deployment == types.WindowsDeploymentTypeMultiAz1 {
		if len(subnets) != 2 {
			return badRequest("A MULTI_AZ_1 file system takes exactly two subnets.")
		}
		if c.PreferredSubnetId == nil {
			return badRequest("PreferredSubnetId is required for a MULTI_AZ_1 file system.")
		}
	} else if len(subnets) != 1 {
		return badRequest("A %s file system takes exactly one subnet.", deployment)
	}
	minimum := int32(32)
	if storage == types.StorageTypeHdd {
		if deployment != types.WindowsDeploymentTypeSingleAz2 && deployment != types.WindowsDeploymentTypeMultiAz1 {
			return badRequest("HDD storage is supported only on SINGLE_AZ_2 and MULTI_AZ_1 Windows file systems.")
		}
		minimum = 2000
	}
	if capacity != nil && (*capacity < minimum || *capacity > 65536) {
		return badRequest("StorageCapacity must be %d to 65536 GiB for %s storage.", minimum, storage)
	}
	return nil
}

// validWindowsThroughput checks a Windows file system's throughput capacity: the range the call takes, then the
// values FSx documents.
func validWindowsThroughput(throughput int32) error {
	if throughput < 8 || throughput > 100000 {
		return invalid("ThroughputCapacity must be 8 to 100000.")
	}
	if !slices.Contains(windowsThroughput, throughput) {
		return badRequest("ThroughputCapacity %d isn't a valid throughput capacity.", throughput)
	}
	return nil
}

func validateOntap(capacity *int32, c *types.CreateFileSystemOntapConfiguration, subnets []string) error {
	if c == nil || c.DeploymentType == "" {
		return invalid("OntapConfiguration DeploymentType is required.")
	}
	if err := validSchedule(c.AutomaticBackupRetentionDays, c.WeeklyMaintenanceStartTime, c.DailyAutomaticBackupStartTime); err != nil {
		return err
	}
	if err := validPassword(c.FsxAdminPassword); err != nil {
		return err
	}
	if err := validPreferredSubnet(c.PreferredSubnetId); err != nil {
		return err
	}
	if err := validIDs("RouteTableIds", c.RouteTableIds, routeTablePattern, 12, 21); err != nil {
		return err
	}
	deployment := string(c.DeploymentType)
	multiAZ := strings.HasPrefix(deployment, "MULTI_AZ_")
	if !multiAZ && (len(c.RouteTableIds) > 0 || c.EndpointIpAddressRange != nil) {
		return badRequest("RouteTableIds and EndpointIpAddressRange are for Multi-AZ file systems only.")
	}
	if multiAZ && c.PreferredSubnetId == nil {
		return badRequest("PreferredSubnetId is required for a %s file system.", deployment)
	}
	if deployment == "MULTI_AZ_1" && len(subnets) != 2 {
		return badRequest("A MULTI_AZ_1 file system takes exactly two subnets.")
	}
	pairs := aws.ToInt32(c.HAPairs)
	if c.HAPairs == nil {
		pairs = 1
	}
	if pairs < 1 || pairs > 12 {
		return badRequest("HAPairs must be 1 to 12.")
	}
	if pairs > 1 && ontapSinglePair(deployment) {
		return badRequest("A %s file system has one HA pair.", deployment)
	}
	if c.ThroughputCapacity != nil && c.ThroughputCapacityPerHAPair != nil {
		return badRequest("A create defines ThroughputCapacity or ThroughputCapacityPerHAPair, not both.")
	}
	if err := validOntapThroughput(deployment, pairs, c.ThroughputCapacity, c.ThroughputCapacityPerHAPair); err != nil {
		return err
	}
	if capacity != nil && (*capacity < 1024*pairs || *capacity > 524288*pairs) {
		return badRequest("StorageCapacity must be 1024 to 524288 GiB per HA pair.")
	}
	return nil
}

// validOntapThroughput checks the throughput capacity of an ONTAP file system with the HA pairs, 1 to 12, given as
// the total or per HA pair.
func validOntapThroughput(deployment string, pairs int32, total *int32, perPair *int32) error {
	per := perPair
	if per == nil && total != nil {
		per = aws.Int32(*total / pairs)
	}
	if per != nil && !slices.Contains(ontapThroughputPerPair[deployment], *per) {
		return badRequest("%d MBps per HA pair isn't valid for a %s file system.", *per, deployment)
	}
	return nil
}

func validateOpenZFS(capacity *int32, c *types.CreateFileSystemOpenZFSConfiguration) error {
	if c == nil || c.DeploymentType == "" || c.ThroughputCapacity == nil {
		return invalid("OpenZFSConfiguration DeploymentType and ThroughputCapacity are required.")
	}
	if err := validSchedule(c.AutomaticBackupRetentionDays, c.WeeklyMaintenanceStartTime, c.DailyAutomaticBackupStartTime); err != nil {
		return err
	}
	if err := validIDs("RouteTableIds", c.RouteTableIds, routeTablePattern, 12, 21); err != nil {
		return err
	}
	if err := validPreferredSubnet(c.PreferredSubnetId); err != nil {
		return err
	}
	multiAZ := c.DeploymentType == types.OpenZFSDeploymentTypeMultiAz1
	if !multiAZ && (len(c.RouteTableIds) > 0 || c.EndpointIpAddressRange != nil) {
		return badRequest("RouteTableIds and EndpointIpAddressRange are for Multi-AZ file systems only.")
	}
	if multiAZ && c.PreferredSubnetId == nil {
		return badRequest("PreferredSubnetId is required for a MULTI_AZ_1 file system.")
	}
	if err := validOpenZFSThroughput(c.DeploymentType, *c.ThroughputCapacity); err != nil {
		return err
	}
	if capacity != nil && (*capacity < 64 || *capacity > 524288) {
		return badRequest("StorageCapacity must be 64 to 524288 GiB.")
	}
	return nil
}

// validOpenZFSThroughput checks an OpenZFS file system's throughput capacity, where FSx documents the values of its
// deployment type.
func validOpenZFSThroughput(deployment types.OpenZFSDeploymentType, throughput int32) error {
	if throughput < 8 || throughput > 100000 {
		return invalid("ThroughputCapacity must be 8 to 100000.")
	}
	if valid, ok := openZFSThroughput[deployment]; ok && !slices.Contains(valid, throughput) {
		return badRequest("ThroughputCapacity %d isn't valid for a %s file system.", throughput, deployment)
	}
	return nil
}

// validateRestore checks the input of a CreateFileSystemFromBackup call, other than its token and backup, given the
// file system the backup is of.
func validateRestore(in *fsx.CreateFileSystemFromBackupInput, source *types.FileSystem) error {
	if err := validTags("Tags", in.Tags); err != nil {
		return err
	}
	if err := validNetwork(in.SubnetIds, in.SecurityGroupIds); err != nil {
		return err
	}
	if c := in.LustreConfiguration; c != nil {
		if err := validSchedule(c.AutomaticBackupRetentionDays, c.WeeklyMaintenanceStartTime, c.DailyAutomaticBackupStartTime); err != nil {
			return err
		}
	}
	if c := in.WindowsConfiguration; c != nil {
		if err := validSchedule(c.AutomaticBackupRetentionDays, c.WeeklyMaintenanceStartTime, c.DailyAutomaticBackupStartTime); err != nil {
			return err
		}
	}
	if c := in.OpenZFSConfiguration; c != nil {
		if err := validSchedule(c.AutomaticBackupRetentionDays, c.WeeklyMaintenanceStartTime, c.DailyAutomaticBackupStartTime); err != nil {
			return err
		}
	}
	backedUp := aws.ToInt32(source.StorageCapacity)
	if capacity := in.StorageCapacity; capacity != nil {
		if source.FileSystemType == types.FileSystemTypeOpenzfs {
			if *capacity < max(64, backedUp) || *capacity > 524288 {
				return badRequest("StorageCapacity must be at least the backup's %d GiB, up to 524288 GiB.", backedUp)
			}
		} else if *capacity != backedUp {
			return badRequest("StorageCapacity must be the backup's %d GiB.", backedUp)
		}
	}
	deployment := types.WindowsDeploymentType("")
	if c := source.WindowsConfiguration; c != nil {
		deployment = c.DeploymentType
	}
	if c := in.WindowsConfiguration; c != nil && c.DeploymentType != "" {
		deployment = c.DeploymentType
	}
	subnets := 1
	if deployment == types.WindowsDeploymentTypeMultiAz1 {
		subnets = 2
	}
	if source.FileSystemType != types.FileSystemTypeOntap && len(in.SubnetIds) != subnets {
		return badRequest("The restored file system takes exactly %d subnets.", subnets)
	}
	return nil
}

// validateUpdate checks the input of an UpdateFileSystem call for the file system fs, which FSx has. increased is
// true when the file system's storage capacity increased earlier: the fake's calls all happen within 6 hours.
// changing holds what the administrative action FSx is running on the file system changes.
func validateUpdate(in *fsx.UpdateFileSystemInput, fs *types.FileSystem, increased bool, changing []string) error {
	if c := in.LustreConfiguration; c != nil {
		if err := validSchedule(c.AutomaticBackupRetentionDays, c.WeeklyMaintenanceStartTime, c.DailyAutomaticBackupStartTime); err != nil {
			return err
		}
		if lustre := fs.LustreConfiguration; lustre != nil && c.PerUnitStorageThroughput != nil && fs.StorageType == types.StorageTypeSsd {
			if !slices.Contains(lustreThroughputPerTiB[string(lustre.DeploymentType)+"/SSD"], *c.PerUnitStorageThroughput) {
				return badRequest("PerUnitStorageThroughput %d isn't valid for a %s SSD file system.", *c.PerUnitStorageThroughput, lustre.DeploymentType)
			}
		}
	}
	if c := in.WindowsConfiguration; c != nil {
		if err := validSchedule(c.AutomaticBackupRetentionDays, c.WeeklyMaintenanceStartTime, c.DailyAutomaticBackupStartTime); err != nil {
			return err
		}
		if c.ThroughputCapacity != nil {
			if err := validWindowsThroughput(*c.ThroughputCapacity); err != nil {
				return err
			}
			if slices.Contains(changing, "ThroughputCapacity") {
				return badRequest("A throughput capacity update is in progress.")
			}
		}
		if c.SelfManagedActiveDirectoryConfiguration != nil && slices.Contains(changing, "SelfManagedActiveDirectoryConfiguration") {
			return badRequest("A self-managed Active Directory update is in progress.")
		}
	}
	if c := in.OntapConfiguration; c != nil {
		if err := validSchedule(c.AutomaticBackupRetentionDays, c.WeeklyMaintenanceStartTime, c.DailyAutomaticBackupStartTime); err != nil {
			return err
		}
		if err := validIDs("AddRouteTableIds", c.AddRouteTableIds, routeTablePattern, 12, 21); err != nil {
			return err
		}
		if err := validIDs("RemoveRouteTableIds", c.RemoveRouteTableIds, routeTablePattern, 12, 21); err != nil {
			return err
		}
		if err := validPassword(c.FsxAdminPassword); err != nil {
			return err
		}
		if c.HAPairs != nil && (*c.HAPairs < 1 || *c.HAPairs > 12) {
			return invalid("HAPairs must be 1 to 12.")
		}
		if c.ThroughputCapacity != nil && c.ThroughputCapacityPerHAPair != nil {
			return badRequest("ThroughputCapacity and ThroughputCapacityPerHAPair can't be in the same call.")
		}
		if ontap := fs.OntapConfiguration; ontap != nil {
			pairs := aws.ToInt32(c.HAPairs)
			if c.HAPairs == nil {
				pairs = max(aws.ToInt32(ontap.HAPairs), 1)
			}
			if err := validOntapThroughput(string(ontap.DeploymentType), pairs, c.ThroughputCapacity, c.ThroughputCapacityPerHAPair); err != nil {
				return err
			}
		}
	}
	if c := in.OpenZFSConfiguration; c != nil {
		if err := validSchedule(c.AutomaticBackupRetentionDays, c.WeeklyMaintenanceStartTime, c.DailyAutomaticBackupStartTime); err != nil {
			return err
		}
		if err := validIDs("AddRouteTableIds", c.AddRouteTableIds, routeTablePattern, 12, 21); err != nil {
			return err
		}
		if err := validIDs("RemoveRouteTableIds", c.RemoveRouteTableIds, routeTablePattern, 12, 21); err != nil {
			return err
		}
		if zfs := fs.OpenZFSConfiguration; zfs != nil && c.ThroughputCapacity != nil {
			if err := validOpenZFSThroughput(zfs.DeploymentType, *c.ThroughputCapacity); err != nil {
				return err
			}
		}
	}
	if v := in.FileSystemTypeVersion; v != nil {
		if fs.FileSystemType != types.FileSystemTypeLustre || (*v != "2.12" && *v != "2.15") || *v <= aws.ToString(fs.FileSystemTypeVersion) {
			return badRequest("FileSystemTypeVersion must be 2.12 or 2.15, and newer than %s.", aws.ToString(fs.FileSystemTypeVersion))
		}
	}
	if in.StorageCapacity != nil {
		return validCapacityChange(fs, *in.StorageCapacity, increased)
	}
	return nil
}

// validCapacityChange checks a change of the file system's storage capacity to target.
func validCapacityChange(fs *types.FileSystem, target int32, increased bool) error {
	current := aws.ToInt32(fs.StorageCapacity)
	if increased && target > current {
		return badRequest("The storage capacity increased less than 6 hours ago.")
	}
	atLeast10Percent := int64(target)*10 >= int64(current)*11
	switch fs.FileSystemType {
	case types.FileSystemTypeLustre:
		c := fs.LustreConfiguration
		if c == nil || c.DeploymentType == types.LustreDeploymentTypeScratch1 {
			return badRequest("A SCRATCH_1 file system's storage capacity can't increase.")
		}
		step := int32(2400)
		if fs.StorageType == types.StorageTypeHdd {
			step = 6000
			if aws.ToInt32(c.PerUnitStorageThroughput) == 40 {
				step = 1800
			}
		}
		if target <= current || target%step != 0 {
			return badRequest("StorageCapacity must be a multiple of %d GiB greater than %d.", step, current)
		}
	case types.FileSystemTypeWindows:
		if !atLeast10Percent || target > 65536 {
			return badRequest("StorageCapacity must be at least 10 percent greater than %d, up to 65536 GiB.", current)
		}
		if c := fs.WindowsConfiguration; c == nil || aws.ToInt32(c.ThroughputCapacity) < 16 {
			return badRequest("Increasing storage capacity needs at least 16 MBps of throughput capacity.")
		}
	case types.FileSystemTypeOpenzfs:
		if !atLeast10Percent || target > 524288 {
			return badRequest("StorageCapacity must be at least 10 percent greater than %d, up to 524288 GiB.", current)
		}
	case types.FileSystemTypeOntap:
		c := fs.OntapConfiguration
		pairs := int32(1)
		deployment := ""
		if c != nil {
			pairs, deployment = max(aws.ToInt32(c.HAPairs), 1), string(c.DeploymentType)
		}
		maximum := 524288 * pairs
		switch {
		case target < current && !ontapFirstGeneration(deployment):
			if int64(target)*100 > int64(current)*91 {
				return badRequest("A decrease must be at least 9 percent of %d.", current)
			}
		case target <= current:
			return badRequest("A %s file system's storage capacity can only increase.", deployment)
		case target > maximum || (!atLeast10Percent && target != maximum):
			return badRequest("StorageCapacity must be at least 10 percent greater than %d, or the maximum.", current)
		}
	}
	return nil
}

// validateDelete checks the input of a DeleteFileSystem call, other than its token.
func validateDelete(in *fsx.DeleteFileSystemInput) error {
	if err := validFileSystemID(aws.ToString(in.FileSystemId)); err != nil {
		return err
	}
	if c := in.LustreConfiguration; c != nil {
		if err := validTags("FinalBackupTags", c.FinalBackupTags); err != nil {
			return err
		}
	}
	if c := in.WindowsConfiguration; c != nil {
		if err := validTags("FinalBackupTags", c.FinalBackupTags); err != nil {
			return err
		}
	}
	if c := in.OpenZFSConfiguration; c != nil {
		if err := validTags("FinalBackupTags", c.FinalBackupTags); err != nil {
			return err
		}
		if len(c.Options) > 1 {
			return invalid("Options can have up to 1 item.")
		}
		for _, o := range c.Options {
			if o != types.DeleteFileSystemOpenZFSOptionDeleteChildVolumesAndSnapshots {
				return invalid("Options must be DELETE_CHILD_VOLUMES_AND_SNAPSHOTS.")
			}
		}
	}
	return nil
}

// validFilters checks the filters of a describe call.
func validFilters(filters []types.Filter) error {
	if len(filters) > 10 {
		return invalid("Filters can have up to 10 items.")
	}
	for _, f := range filters {
		if f.Name != "" && !slices.Contains(types.FilterName("").Values(), f.Name) {
			return invalid("Filter name %q isn't valid.", f.Name)
		}
		if len(f.Values) > 20 {
			return invalid("A filter can have up to 20 values.")
		}
		for _, v := range f.Values {
			if len(v) < 1 || len(v) > 128 || !filterPattern.MatchString(v) {
				return invalid("Filter value %q isn't valid.", v)
			}
		}
	}
	return nil
}
