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
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/fsx"
	"github.com/aws/aws-sdk-go-v2/service/fsx/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const arnPrefix = "arn:aws:fsx:us-west-2:123456789012:file-system/"

// backup is a final backup the fake took.
type backup struct {
	fileSystem string
	tags       []types.Tag
}

// request is a create or delete the fake accepted with an idempotency token.
type request struct {
	fileSystem string
	params     string
}

// fakeAPI is an in-memory FSx API. It keeps file systems in an awsfake store in the type's property format, so the
// suite's Cloud Control sees the same resources, and builds FSx's responses from them. A new file system is CREATING
// for one read. After an update, an administrative action is IN_PROGRESS for one read. A deleted file system is
// DELETING for two reads and then gone, or, when failBackup is set, back to AVAILABLE because its final backup failed.
// DescribeFileSystems returns one file system per page. The fake checks each call's input as FSx's API reference
// documents (see validate_test.go and api_test.go), and fills in the defaults FSx documents for what a create leaves
// out. Its calls all happen within 6 hours of each other.
type fakeAPI struct {
	store  *awsfake.CloudControl
	failer nativetest.Failer

	mu        sync.Mutex
	next      int
	lifecycle map[string]types.FileSystemLifecycle
	actions   map[string][]types.AdministrativeAction
	creates   map[string]request
	deletes   map[string]request // by token
	backups   []backup

	// tokens maps a file system to the value of its operation tag, which the store leaves out, as Read does.
	tokens map[string]string

	// options maps a file system to the value of its delete options tag, which the store leaves out, as Read does.
	// deleteOptions maps a file system to the OpenZFS options its delete sent.
	options       map[string]string
	deleteOptions map[string][]types.DeleteFileSystemOpenZFSOption

	// associations maps a file system to how many data repository associations link it to S3.
	associations map[string]int

	// deleting maps a file system FSx is deleting to how many more reads report it as DELETING.
	deleting map[string]int

	// failBackup makes the final backup of the next delete fail. failures maps a file system to the reason FSx
	// gives for its state.
	failBackup bool
	failures   map[string]string

	// failUpdate, when set, makes the administrative action of the next update fail with this message. FSx then keeps
	// the file system's values from before the update, which undo holds.
	failUpdate string
	undo       map[string]map[string]any

	// sources maps a backup ID to the properties of the file system it is a backup of, which a restore starts from.
	sources map[string]map[string]any

	// increased holds the file systems whose storage capacity increased. changing maps a file system to the
	// properties its running FILE_SYSTEM_UPDATE action changes.
	increased map[string]bool
	changing  map[string][]string

	// tasks holds the Lustre file systems with a data repository task that is PENDING or EXECUTING.
	tasks map[string]bool

	// errs maps a method name to an error the call returns instead of acting.
	errs  map[string]error
	calls []string
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{
		store:         store,
		lifecycle:     map[string]types.FileSystemLifecycle{},
		actions:       map[string][]types.AdministrativeAction{},
		creates:       map[string]request{},
		deletes:       map[string]request{},
		tokens:        map[string]string{},
		undo:          map[string]map[string]any{},
		options:       map[string]string{},
		deleteOptions: map[string][]types.DeleteFileSystemOpenZFSOption{},
		associations:  map[string]int{},
		failures:      map[string]string{},
		deleting:      map[string]int{},
		sources:       map[string]map[string]any{},
		increased:     map[string]bool{},
		changing:      map[string][]string{},
		tasks:         map[string]bool{},
		errs:          map[string]error{},
	}
}

// addBackup makes a backup with the ID of a file system with the properties, for a restore to start from.
func (f *fakeAPI) addBackup(id string, properties map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	props := copyProps(properties)
	applyDefaults(props)
	f.sources[id] = props
}

func fault(code string, message string) error {
	return &smithy.GenericAPIError{Code: code, Message: message, Fault: smithy.FaultClient}
}

func (f *fakeAPI) call(name string) error {
	f.calls = append(f.calls, name)
	return f.errs[name]
}

// backupsOf returns how many final backups the fake took of the file system.
func (f *fakeAPI) backupsOf(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, b := range f.backups {
		if b.fileSystem == id {
			n++
		}
	}
	return n
}

// create stores a file system from the input of a create, or returns the file system the token already made. A token
// sent again with other parameters fails, as FSx does. The fake checks the token, then whether FSx has seen it, and
// then the other parameters with validate. A restore passes the properties of the backed-up file system as base,
// which the input's values override.
func (f *fakeAPI) create(token *string, input any, tags []types.Tag, validate func() error, base map[string]any) (*types.FileSystem, error) {
	if err := validToken(token); err != nil {
		return nil, err
	}
	b, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	if r, ok := f.creates[aws.ToString(token)]; ok {
		if r.params != string(b) {
			return nil, fault("IncompatibleParameterError", "The request has the same client request token as a previous request, with other parameters.")
		}
		fs, _ := f.fileSystem(r.fileSystem)
		return fs, nil
	}
	if err := validate(); err != nil {
		return nil, err
	}
	values, err := awsapi.EncodeObject(input)
	if err != nil {
		return nil, err
	}
	delete(values, "ClientRequestToken")
	delete(values, "Tags")
	props := map[string]any{}
	if base != nil {
		// A restore that leaves StorageType or KmsKeyId out gets SSD and the FSx-managed key, not the backup's.
		props = copyProps(awsapi.Without(base, "Tags", "StorageType", "KmsKeyId"))
	}
	for k, v := range values {
		if config, ok := v.(map[string]any); ok {
			merged := asMap(props[k])
			maps.Copy(merged, config)
			v = merged
		}
		props[k] = v
	}
	applyDefaults(props)
	f.next++
	id := fmt.Sprintf("fs-%017d", f.next)
	m := map[string]string{}
	for _, t := range tags {
		switch aws.ToString(t.Key) {
		case awsapi.OperationTag:
			f.tokens[id] = aws.ToString(t.Value)
		case deleteOptionsTag:
			f.options[id] = aws.ToString(t.Value)
			continue
		}
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	if list := awsapi.TagList(m); list != nil {
		props["Tags"] = list
	}
	f.store.Put(fileSystemType, id, props)
	f.lifecycle[id] = types.FileSystemLifecycleCreating
	f.creates[aws.ToString(token)] = request{fileSystem: id, params: string(b)}
	fs, _ := f.fileSystem(id)
	return fs, nil
}

// managedKey is the ARN of the FSx-managed KMS key of the fake's account, which encrypts a file system whose create
// sets no key.
const managedKey = "arn:aws:kms:us-west-2:123456789012:key/0f1e2d3c-4b5a-4978-8796-a5b4c3d2e1f0"

// configurations maps each file system type to its configuration property.
var configurations = map[string]string{
	string(types.FileSystemTypeLustre):  "LustreConfiguration",
	string(types.FileSystemTypeWindows): "WindowsConfiguration",
	string(types.FileSystemTypeOntap):   "OntapConfiguration",
	string(types.FileSystemTypeOpenzfs): "OpenZFSConfiguration",
}

// applyDefaults fills in the values FSx documents for what a create left out of the file system's properties. A key
// the create sets stays as it was sent: the suite requires AWS to hold the template's values, and its OtherForm and
// RecordedForm cases give the key ARN FSx reports instead.
func applyDefaults(props map[string]any) {
	fsType := awsapi.String(props, "FileSystemType")
	setDefault(props, "StorageType", string(types.StorageTypeSsd))
	name, ok := configurations[fsType]
	if !ok {
		return
	}
	c := asMap(props[name])
	switch fsType {
	case string(types.FileSystemTypeLustre):
		setDefault(c, "DeploymentType", string(types.LustreDeploymentTypeScratch1))
		setDefault(c, "AutomaticBackupRetentionDays", 0.0)
		setDefault(c, "CopyTagsToBackups", false)
		setDefault(c, "DataCompressionType", string(types.DataCompressionTypeNone))
		version := "2.10"
		if c["DeploymentType"] == string(types.LustreDeploymentTypePersistent2) {
			version = "2.12"
			if awsapi.String(asMap(c["MetadataConfiguration"]), "Mode") != "" {
				version = "2.15"
			}
		}
		setDefault(props, "FileSystemTypeVersion", version)
	case string(types.FileSystemTypeWindows):
		setDefault(c, "DeploymentType", string(types.WindowsDeploymentTypeSingleAz1))
		setDefault(c, "AutomaticBackupRetentionDays", 30.0)
		setDefault(c, "CopyTagsToBackups", false)
	case string(types.FileSystemTypeOntap):
		setDefault(c, "AutomaticBackupRetentionDays", 30.0)
		setDefault(c, "HAPairs", 1.0)
		syncOntapThroughput(c, "ThroughputCapacity")
		syncOntapThroughput(c, "ThroughputCapacityPerHAPair")
	case string(types.FileSystemTypeOpenzfs):
		setDefault(c, "AutomaticBackupRetentionDays", 30.0)
		setDefault(c, "CopyTagsToBackups", false)
		setDefault(c, "CopyTagsToVolumes", false)
	}
	props[name] = c
	if fsType != string(types.FileSystemTypeLustre) {
		setDefault(props, "NetworkType", string(types.NetworkTypeIpv4))
	}
	if _, ok := props["KmsKeyId"]; !ok && (fsType != string(types.FileSystemTypeLustre) || strings.HasPrefix(awsapi.String(c, "DeploymentType"), "PERSISTENT")) {
		props["KmsKeyId"] = managedKey
	}
}

func setDefault(props map[string]any, name string, value any) {
	if _, ok := props[name]; !ok {
		props[name] = value
	}
}

// syncOntapThroughput sets the ThroughputCapacity of an ONTAP configuration from its ThroughputCapacityPerHAPair and
// HAPairs, or the reverse. from names the one a call set.
func syncOntapThroughput(c map[string]any, from string) {
	pairs, _ := c["HAPairs"].(float64)
	pairs = max(pairs, 1)
	v, ok := c[from].(float64)
	if !ok {
		return
	}
	if from == "ThroughputCapacity" {
		c["ThroughputCapacityPerHAPair"] = v / pairs
	} else {
		c["ThroughputCapacity"] = v * pairs
	}
}

func (f *fakeAPI) CreateFileSystem(ctx context.Context, in *fsx.CreateFileSystemInput, _ ...func(*fsx.Options)) (*fsx.CreateFileSystemOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("CreateFileSystem"); err != nil {
		return nil, err
	}
	fs, err := f.create(in.ClientRequestToken, in, in.Tags, func() error { return validateCreate(in) }, nil)
	return &fsx.CreateFileSystemOutput{FileSystem: fs}, err
}

// CreateFileSystemFromBackup restores a backup addBackup made. The new file system has the type of the backed-up one,
// and its values unless the call sets others.
func (f *fakeAPI) CreateFileSystemFromBackup(ctx context.Context, in *fsx.CreateFileSystemFromBackupInput, _ ...func(*fsx.Options)) (*fsx.CreateFileSystemFromBackupOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("CreateFileSystemFromBackup"); err != nil {
		return nil, err
	}
	id := aws.ToString(in.BackupId)
	if err := validID("BackupId", id, backupPattern, 12, 128); err != nil {
		return nil, err
	}
	source, ok := f.sources[id]
	validate := func() error {
		if !ok {
			return fault("BackupNotFound", "Backup '"+id+"' does not exist.")
		}
		return validateRestore(in, decodeFileSystem(source))
	}
	fs, err := f.create(in.ClientRequestToken, in, in.Tags, validate, source)
	return &fsx.CreateFileSystemFromBackupOutput{FileSystem: fs}, err
}

// decodeFileSystem returns a file system with the stored properties, in FSx's format.
func decodeFileSystem(props map[string]any) *types.FileSystem {
	fs := &types.FileSystem{}
	lustre := asMap(props["LustreConfiguration"])
	windows := asMap(props["WindowsConfiguration"])
	decodable := awsapi.Without(props, "LustreConfiguration", "WindowsConfiguration")
	decodable["LustreConfiguration"] = awsapi.Without(lustre, "AutoImportPolicy", "ExportPath", "ImportPath", "ImportedFileChunkSize")
	decodable["WindowsConfiguration"] = awsapi.Without(windows, "Aliases")
	if err := awsapi.Decode(decodable, fs); err != nil {
		panic(err)
	}
	if _, ok := props["LustreConfiguration"]; !ok {
		fs.LustreConfiguration = nil
	} else {
		fs.LustreConfiguration.MountName = aws.String("abcdef")
		if path := awsapi.String(lustre, "ImportPath"); path != "" {
			fs.LustreConfiguration.DataRepositoryConfiguration = &types.DataRepositoryConfiguration{ImportPath: aws.String(path)}
		}
	}
	if _, ok := props["WindowsConfiguration"]; !ok {
		fs.WindowsConfiguration = nil
	} else {
		for _, a := range awsapi.Strings(windows, "Aliases") {
			fs.WindowsConfiguration.Aliases = append(fs.WindowsConfiguration.Aliases, types.Alias{Name: aws.String(a), Lifecycle: types.AliasLifecycleAvailable})
		}
	}
	return fs
}

// fileSystem returns the stored file system as DescribeFileSystems does, without moving its state on. The ONTAP
// admin password stays in it, as if AWS returned it.
func (f *fakeAPI) fileSystem(id string) (*types.FileSystem, bool) {
	props := f.store.Resource(fileSystemType, id)
	if props == nil {
		return nil, false
	}
	fs := decodeFileSystem(props)
	if fs.OpenZFSConfiguration != nil {
		fs.OpenZFSConfiguration.RootVolumeId = aws.String("fsvol-0123")
	}
	if token, ok := f.tokens[id]; ok {
		fs.Tags = append(fs.Tags, types.Tag{Key: aws.String(awsapi.OperationTag), Value: aws.String(token)})
	}
	if options, ok := f.options[id]; ok {
		fs.Tags = append(fs.Tags, types.Tag{Key: aws.String(deleteOptionsTag), Value: aws.String(options)})
	}
	fs.FileSystemId = aws.String(id)
	fs.ResourceARN = aws.String(arnPrefix + id)
	fs.DNSName = aws.String(id + ".fsx.us-west-2.amazonaws.com")
	fs.Lifecycle = types.FileSystemLifecycleAvailable
	if l, ok := f.lifecycle[id]; ok {
		fs.Lifecycle = l
	}
	fs.AdministrativeActions = slices.Clone(f.actions[id])
	if message, ok := f.failures[id]; ok {
		fs.FailureDetails = &types.FileSystemFailureDetails{Message: aws.String(message)}
	}
	return fs, true
}

// describe returns the file system and moves a transitional state on.
func (f *fakeAPI) describe(id string) (*types.FileSystem, bool) {
	fs, ok := f.fileSystem(id)
	if !ok {
		return nil, false
	}
	switch fs.Lifecycle {
	case types.FileSystemLifecycleCreating, types.FileSystemLifecycleUpdating:
		delete(f.lifecycle, id)
	case types.FileSystemLifecycleDeleting:
		if f.deleting[id]--; f.deleting[id] > 0 {
			break
		}
		delete(f.deleting, id)
		if f.failBackup {
			f.failBackup = false
			delete(f.lifecycle, id)
			f.backups = slices.DeleteFunc(f.backups, func(b backup) bool { return b.fileSystem == id })
			f.failures[id] = "The final backup failed."
		} else {
			f.store.Remove(fileSystemType, id)
		}
	}
	actions := f.actions[id]
	for i := range actions {
		if actions[i].Status == types.StatusInProgress && !slices.Contains(optimizations, actions[i].AdministrativeActionType) {
			delete(f.changing, id)
			actions[i].Status = types.StatusCompleted
			if f.failUpdate != "" {
				if before, ok := f.undo[id]; ok {
					f.store.Put(fileSystemType, id, before)
					delete(f.undo, id)
				}
				actions[i].Status = types.StatusFailed
				actions[i].FailureDetails = &types.AdministrativeActionFailureDetails{Message: aws.String(f.failUpdate)}
				f.failUpdate = ""
			}
		}
	}
	return fs, true
}

func (f *fakeAPI) DescribeFileSystems(ctx context.Context, in *fsx.DescribeFileSystemsInput, _ ...func(*fsx.Options)) (*fsx.DescribeFileSystemsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DescribeFileSystems"); err != nil {
		return nil, err
	}
	if len(in.FileSystemIds) > 50 {
		return nil, invalid("FileSystemIds can have up to 50 items.")
	}
	for _, id := range in.FileSystemIds {
		if err := validFileSystemID(id); err != nil {
			return nil, err
		}
	}
	if in.MaxResults != nil && *in.MaxResults < 1 {
		return nil, invalid("MaxResults must be greater than 0.")
	}
	if len(in.FileSystemIds) > 0 {
		fs, ok := f.describe(in.FileSystemIds[0])
		if !ok {
			return nil, fault("FileSystemNotFound", "File system '"+in.FileSystemIds[0]+"' does not exist.")
		}
		return &fsx.DescribeFileSystemsOutput{FileSystems: []types.FileSystem{*fs}}, nil
	}
	ids := f.store.List(fileSystemType)
	start, _ := strconv.Atoi(aws.ToString(in.NextToken))
	out := &fsx.DescribeFileSystemsOutput{}
	if start < len(ids) {
		fs, _ := f.fileSystem(ids[start])
		out.FileSystems = []types.FileSystem{*fs}
	}
	if start+1 < len(ids) {
		out.NextToken = aws.String(strconv.Itoa(start + 1))
	}
	return out, nil
}

func (f *fakeAPI) DescribeDataRepositoryAssociations(ctx context.Context, in *fsx.DescribeDataRepositoryAssociationsInput, _ ...func(*fsx.Options)) (*fsx.DescribeDataRepositoryAssociationsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DescribeDataRepositoryAssociations"); err != nil {
		return nil, err
	}
	if err := validFilters(in.Filters); err != nil {
		return nil, err
	}
	if in.MaxResults != nil && (*in.MaxResults < 1 || *in.MaxResults > 25) {
		return nil, invalid("MaxResults must be 1 to 25.")
	}
	ids := slices.Sorted(maps.Keys(f.associations))
	for _, filter := range in.Filters {
		if filter.Name == types.FilterNameFileSystemId {
			ids = slices.DeleteFunc(ids, func(id string) bool { return !slices.Contains(filter.Values, id) })
		}
	}
	out := &fsx.DescribeDataRepositoryAssociationsOutput{}
	for _, id := range ids {
		for range f.associations[id] {
			out.Associations = append(out.Associations, types.DataRepositoryAssociation{FileSystemId: aws.String(id)})
		}
	}
	return out, nil
}

// change applies a change to the stored file system, as a call that changes AWS. A file system FSx doesn't have
// fails with the code notFound. apply returns an error, without changing props, when FSx rejects the call.
func (f *fakeAPI) change(id string, notFound string, apply func(props map[string]any) error) error {
	if err := f.failer.Before(); err != nil {
		return err
	}
	props := f.store.Resource(fileSystemType, id)
	if props == nil {
		return fault(notFound, "The file system '"+id+"' does not exist.")
	}
	if err := apply(props); err != nil {
		return err
	}
	f.store.Put(fileSystemType, id, props)
	return f.failer.After()
}

// copyProps returns a deep copy of a resource's properties.
func copyProps(props map[string]any) map[string]any {
	b, err := json.Marshal(props)
	if err != nil {
		panic(err)
	}
	out := map[string]any{}
	if err := json.Unmarshal(b, &out); err != nil {
		panic(err)
	}
	return out
}

func (f *fakeAPI) UpdateFileSystem(ctx context.Context, in *fsx.UpdateFileSystemInput, _ ...func(*fsx.Options)) (*fsx.UpdateFileSystemOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("UpdateFileSystem"); err != nil {
		return nil, err
	}
	id := aws.ToString(in.FileSystemId)
	if err := validFileSystemID(id); err != nil {
		return nil, err
	}
	if err := validToken(in.ClientRequestToken); err != nil {
		return nil, err
	}
	values, err := awsapi.EncodeObject(in)
	if err != nil {
		return nil, err
	}
	delete(values, "FileSystemId")
	delete(values, "ClientRequestToken")
	return &fsx.UpdateFileSystemOutput{}, f.change(id, "FileSystemNotFound", func(props map[string]any) error {
		current := decodeFileSystem(props)
		if err := validateUpdate(in, current, f.increased[id], f.changing[id]); err != nil {
			return err
		}
		if f.failUpdate != "" {
			f.undo[id] = copyProps(props)
		}
		changing := []string{}
		for k, v := range values {
			config, ok := v.(map[string]any)
			if !ok {
				props[k] = v
				continue
			}
			current := asMap(props[k])
			routes := awsapi.Strings(current, "RouteTableIds")
			routes = append(routes, awsapi.Strings(config, "AddRouteTableIds")...)
			routes = slices.DeleteFunc(routes, func(r string) bool { return slices.Contains(awsapi.Strings(config, "RemoveRouteTableIds"), r) })
			if _, ok := config["AddRouteTableIds"]; ok {
				current["RouteTableIds"] = routes
			}
			if _, ok := config["RemoveRouteTableIds"]; ok {
				current["RouteTableIds"] = routes
			}
			delete(config, "AddRouteTableIds")
			delete(config, "RemoveRouteTableIds")
			if ad, ok := config["SelfManagedActiveDirectoryConfiguration"].(map[string]any); ok {
				merged := asMap(current["SelfManagedActiveDirectoryConfiguration"])
				maps.Copy(merged, ad)
				config["SelfManagedActiveDirectoryConfiguration"] = merged
			}
			maps.Copy(current, config)
			if k == "OntapConfiguration" {
				for _, from := range []string{"ThroughputCapacity", "ThroughputCapacityPerHAPair", "HAPairs"} {
					if _, ok := config[from]; ok {
						syncOntapThroughput(current, strings.Replace(from, "HAPairs", "ThroughputCapacityPerHAPair", 1))
					}
				}
			}
			props[k] = current
			changing = append(changing, slices.Collect(maps.Keys(config))...)
		}
		if capacity := in.StorageCapacity; capacity != nil && *capacity > aws.ToInt32(current.StorageCapacity) {
			f.increased[id] = true
		}
		f.changing[id] = changing
		f.next++
		requested := aws.Time(time.Unix(1793865600+int64(f.next), 0))
		action := types.AdministrativeAction{AdministrativeActionType: types.AdministrativeActionTypeFileSystemUpdate, Status: types.StatusInProgress, RequestTime: requested}
		f.actions[id] = append(f.actions[id], action)
		if _, ok := values["StorageCapacity"]; ok {
			f.actions[id] = append(f.actions[id], types.AdministrativeAction{AdministrativeActionType: types.AdministrativeActionTypeStorageOptimization, Status: types.StatusInProgress, RequestTime: requested})
		}
		if props["FileSystemType"] == string(types.FileSystemTypeLustre) {
			f.lifecycle[id] = types.FileSystemLifecycleUpdating
		}
		return nil
	})
}

// supportsBackup reports whether FSx can take a backup of the stored file system.
func supportsBackup(props map[string]any) bool {
	switch props["FileSystemType"] {
	case string(types.FileSystemTypeWindows), string(types.FileSystemTypeOpenzfs):
		return true
	case string(types.FileSystemTypeLustre):
		lustre := asMap(props["LustreConfiguration"])
		return strings.HasPrefix(awsapi.String(lustre, "DeploymentType"), "PERSISTENT") && awsapi.String(lustre, "ImportPath") == ""
	}
	return false
}

// finalBackup returns whether a delete takes a final backup of a file system of the type, and the backup's tags. With
// no SkipFinalBackup, FSx takes one of a Windows or OpenZFS file system, and none of a Lustre or ONTAP one.
func finalBackup(fsType string, skip *bool, tags []types.Tag) (bool, []types.Tag) {
	if skip == nil {
		return fsType == string(types.FileSystemTypeWindows) || fsType == string(types.FileSystemTypeOpenzfs), tags
	}
	return !*skip, tags
}

func (f *fakeAPI) DeleteFileSystem(ctx context.Context, in *fsx.DeleteFileSystemInput, _ ...func(*fsx.Options)) (*fsx.DeleteFileSystemOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DeleteFileSystem"); err != nil {
		return nil, err
	}
	if err := validToken(in.ClientRequestToken); err != nil {
		return nil, err
	}
	if err := validateDelete(in); err != nil {
		return nil, err
	}
	id := aws.ToString(in.FileSystemId)
	props := f.store.Resource(fileSystemType, id)
	if props == nil {
		return nil, fault("FileSystemNotFound", "File system '"+id+"' does not exist.")
	}
	b, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	out := &fsx.DeleteFileSystemOutput{FileSystemId: in.FileSystemId, Lifecycle: types.FileSystemLifecycleDeleting}
	// A token FSx has seen is the same request: FSx returns the first response and does nothing again, even when
	// that delete failed.
	token := aws.ToString(in.ClientRequestToken)
	if r, ok := f.deletes[token]; ok {
		if r.params != string(b) {
			return nil, fault("IncompatibleParameterError", "The request has the same client request token as a previous request, with other parameters.")
		}
		return out, nil
	}
	if f.lifecycle[id] == types.FileSystemLifecycleDeleting {
		return nil, fault("BadRequest", "The file system is already being deleted.")
	}
	fsType := awsapi.String(props, "FileSystemType")
	takeBackup, tags := finalBackup(fsType, nil, nil)
	typed := 0
	if c := in.WindowsConfiguration; c != nil {
		takeBackup, tags = finalBackup(fsType, c.SkipFinalBackup, c.FinalBackupTags)
		typed++
	}
	if c := in.OpenZFSConfiguration; c != nil {
		takeBackup, tags = finalBackup(fsType, c.SkipFinalBackup, c.FinalBackupTags)
		typed++
		f.deleteOptions[id] = c.Options
	}
	if c := in.LustreConfiguration; c != nil {
		takeBackup, tags = finalBackup(fsType, c.SkipFinalBackup, c.FinalBackupTags)
		typed++
	}
	if typed > 1 || (typed == 1 && fsType == string(types.FileSystemTypeOntap)) {
		return nil, fault("BadRequest", "The configuration doesn't match the file system type.")
	}
	if f.tasks[id] {
		return nil, badRequest("A data repository task is PENDING or EXECUTING.")
	}
	if takeBackup {
		if !supportsBackup(props) || f.associations[id] > 0 {
			return nil, fault("BadRequest", "Backups are not supported for this file system.")
		}
		f.backups = append(f.backups, backup{fileSystem: id, tags: tags})
	}
	f.deletes[token] = request{fileSystem: id, params: string(b)}
	f.lifecycle[id] = types.FileSystemLifecycleDeleting
	f.deleting[id] = 2
	return out, nil
}

// fileSystemOf returns the ID of the file system with the ARN, after checking the ARN.
func fileSystemOf(arn *string) (string, error) {
	if !validLength(aws.ToString(arn), 8, 512) {
		return "", invalid("ResourceARN must be 8 to 512 characters.")
	}
	return strings.TrimPrefix(aws.ToString(arn), arnPrefix), nil
}

func (f *fakeAPI) TagResource(ctx context.Context, in *fsx.TagResourceInput, _ ...func(*fsx.Options)) (*fsx.TagResourceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("TagResource"); err != nil {
		return nil, err
	}
	id, err := fileSystemOf(in.ResourceARN)
	if err != nil {
		return nil, err
	}
	if in.Tags == nil {
		return nil, invalid("Tags is required.")
	}
	if err := validTags("Tags", in.Tags); err != nil {
		return nil, err
	}
	if slices.ContainsFunc(in.Tags, func(t types.Tag) bool { return strings.HasPrefix(aws.ToString(t.Key), "aws:") }) {
		return nil, badRequest("The aws: prefix is reserved for AWS use.")
	}
	return &fsx.TagResourceOutput{}, f.change(id, "ResourceNotFound", func(props map[string]any) error {
		tags := map[string]string{}
		for _, t := range awsapi.Tags(props) {
			tags[t.Key] = t.Value
		}
		for _, t := range in.Tags {
			tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
		}
		if n := f.tagCount(id, tags); n > maxTags {
			return badRequest("A resource can have up to %d tags, and this one would have %d.", maxTags, n)
		}
		props["Tags"] = awsapi.TagList(tags)
		return nil
	})
}

// tagCount returns how many tags of the file system count against its limit, given its tags in the store: those
// without the aws: prefix, and the delete options tag, which the store leaves out.
func (f *fakeAPI) tagCount(id string, tags map[string]string) int {
	n := 0
	for key := range tags {
		if !strings.HasPrefix(key, "aws:") {
			n++
		}
	}
	if _, ok := f.options[id]; ok {
		n++
	}
	return n
}

func (f *fakeAPI) UntagResource(ctx context.Context, in *fsx.UntagResourceInput, _ ...func(*fsx.Options)) (*fsx.UntagResourceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("UntagResource"); err != nil {
		return nil, err
	}
	id, err := fileSystemOf(in.ResourceARN)
	if err != nil {
		return nil, err
	}
	if len(in.TagKeys) < 1 || len(in.TagKeys) > maxTags {
		return nil, invalid("TagKeys must have 1 to %d keys.", maxTags)
	}
	for _, key := range in.TagKeys {
		if err := validTag(key, nil); err != nil {
			return nil, err
		}
	}
	if slices.ContainsFunc(in.TagKeys, func(key string) bool { return strings.HasPrefix(key, "aws:") }) {
		return nil, badRequest("The aws: prefix is reserved for AWS use.")
	}
	return &fsx.UntagResourceOutput{}, f.change(id, "ResourceNotFound", func(props map[string]any) error {
		tags := map[string]string{}
		for _, t := range awsapi.Tags(props) {
			if !slices.Contains(in.TagKeys, t.Key) {
				tags[t.Key] = t.Value
			}
		}
		if list := awsapi.TagList(tags); list != nil {
			props["Tags"] = list
		} else {
			delete(props, "Tags")
		}
		return nil
	})
}

// updates is an API that records the input of each UpdateFileSystem call.
type updates struct {
	*fakeAPI
	inputs []*fsx.UpdateFileSystemInput
}

func (u *updates) UpdateFileSystem(ctx context.Context, in *fsx.UpdateFileSystemInput, opts ...func(*fsx.Options)) (*fsx.UpdateFileSystemOutput, error) {
	u.inputs = append(u.inputs, in)
	return u.fakeAPI.UpdateFileSystem(ctx, in, opts...)
}

// noFileSystem is an API that returns no file system from a create or a describe, and no error.
type noFileSystem struct {
	*fakeAPI
}

func (noFileSystem) CreateFileSystem(ctx context.Context, in *fsx.CreateFileSystemInput, _ ...func(*fsx.Options)) (*fsx.CreateFileSystemOutput, error) {
	return &fsx.CreateFileSystemOutput{}, nil
}

func (noFileSystem) DescribeFileSystems(ctx context.Context, in *fsx.DescribeFileSystemsInput, _ ...func(*fsx.Options)) (*fsx.DescribeFileSystemsOutput, error) {
	return &fsx.DescribeFileSystemsOutput{}, nil
}
