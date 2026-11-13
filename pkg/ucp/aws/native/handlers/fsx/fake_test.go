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
	"regexp"
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
// DescribeFileSystems returns one file system per page.
type fakeAPI struct {
	store  *awsfake.CloudControl
	failer nativetest.Failer

	mu        sync.Mutex
	next      int
	lifecycle map[string]types.FileSystemLifecycle
	actions   map[string][]types.AdministrativeAction
	creates   map[string]request
	deletes   map[string]request // by token
	updates   map[string]request // by token
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
		updates:       map[string]request{},
		tokens:        map[string]string{},
		undo:          map[string]map[string]any{},
		options:       map[string]string{},
		deleteOptions: map[string][]types.DeleteFileSystemOpenZFSOption{},
		associations:  map[string]int{},
		failures:      map[string]string{},
		deleting:      map[string]int{},
		errs:          map[string]error{},
	}
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
// sent again with other parameters fails, as FSx does.
func (f *fakeAPI) create(token string, input any, tags []types.Tag) (*types.FileSystem, error) {
	b, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	if r, ok := f.creates[token]; ok {
		if r.params != string(b) {
			return nil, fault("IncompatibleParameterError", "The request has the same client request token as a previous request, with other parameters.")
		}
		fs, _ := f.fileSystem(r.fileSystem)
		return fs, nil
	}
	if err := checkTagCount(len(tags)); err != nil {
		return nil, err
	}
	props, err := awsapi.EncodeObject(input)
	if err != nil {
		return nil, err
	}
	delete(props, "ClientRequestToken")
	delete(props, "Tags")
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
	f.creates[token] = request{fileSystem: id, params: string(b)}
	fs, _ := f.fileSystem(id)
	return fs, nil
}

func (f *fakeAPI) CreateFileSystem(ctx context.Context, in *fsx.CreateFileSystemInput, _ ...func(*fsx.Options)) (*fsx.CreateFileSystemOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("CreateFileSystem"); err != nil {
		return nil, err
	}
	fs, err := f.create(aws.ToString(in.ClientRequestToken), in, in.Tags)
	return &fsx.CreateFileSystemOutput{FileSystem: fs}, err
}

func (f *fakeAPI) CreateFileSystemFromBackup(ctx context.Context, in *fsx.CreateFileSystemFromBackupInput, _ ...func(*fsx.Options)) (*fsx.CreateFileSystemFromBackupOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("CreateFileSystemFromBackup"); err != nil {
		return nil, err
	}
	fs, err := f.create(aws.ToString(in.ClientRequestToken), in, in.Tags)
	if err == nil {
		props := f.store.Resource(fileSystemType, aws.ToString(fs.FileSystemId))
		props["FileSystemType"] = string(types.FileSystemTypeLustre)
		f.store.Put(fileSystemType, aws.ToString(fs.FileSystemId), props)
	}
	return &fsx.CreateFileSystemFromBackupOutput{FileSystem: fs}, err
}

// fileSystem returns the stored file system as DescribeFileSystems does, without moving its state on. The ONTAP
// admin password stays in it, as if AWS returned it.
func (f *fakeAPI) fileSystem(id string) (*types.FileSystem, bool) {
	props := f.store.Resource(fileSystemType, id)
	if props == nil {
		return nil, false
	}
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
	out := &fsx.DescribeDataRepositoryAssociationsOutput{}
	for range f.associations[in.Filters[0].Values[0]] {
		out.Associations = append(out.Associations, types.DataRepositoryAssociation{FileSystemId: aws.String(in.Filters[0].Values[0])})
	}
	return out, nil
}

// change applies a change to the stored file system, as a call that changes AWS.
func (f *fakeAPI) change(name string, id string, apply func(props map[string]any)) error {
	if err := f.call(name); err != nil {
		return err
	}
	if err := f.failer.Before(); err != nil {
		return err
	}
	props := f.store.Resource(fileSystemType, id)
	if props == nil {
		return fault("FileSystemNotFound", "File system '"+id+"' does not exist.")
	}
	apply(props)
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

// tokenPattern is the rule for an idempotency token of UpdateFileSystem: 1 to 63 letters, digits, and _.- characters.
var tokenPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,63}$`)

// UpdateFileSystem applies an update. A token FSx has seen with the same parameters is the same request: FSx returns
// the first response and changes nothing again. With other parameters, it fails. The SDK fills in a new token when
// the call leaves it out, so an update without one is always a new request.
func (f *fakeAPI) UpdateFileSystem(ctx context.Context, in *fsx.UpdateFileSystemInput, _ ...func(*fsx.Options)) (*fsx.UpdateFileSystemOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := aws.ToString(in.FileSystemId)
	token := aws.ToString(in.ClientRequestToken)
	if in.ClientRequestToken != nil && !tokenPattern.MatchString(token) {
		f.calls = append(f.calls, "UpdateFileSystem")
		return nil, fault("BadRequest", "ClientRequestToken must be 1 to 63 letters, digits, and _.- characters.")
	}
	b, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	if r, ok := f.updates[token]; ok && token != "" {
		f.calls = append(f.calls, "UpdateFileSystem")
		if r.params != string(b) {
			return nil, fault("IncompatibleParameterError", "The request has the same client request token as a previous request, with other parameters.")
		}
		return &fsx.UpdateFileSystemOutput{}, nil
	}
	values, err := awsapi.EncodeObject(in)
	if err != nil {
		return nil, err
	}
	delete(values, "FileSystemId")
	delete(values, "ClientRequestToken")
	return &fsx.UpdateFileSystemOutput{}, f.change("UpdateFileSystem", id, func(props map[string]any) {
		if token != "" {
			f.updates[token] = request{fileSystem: id, params: string(b)}
		}
		if f.failUpdate != "" {
			f.undo[id] = copyProps(props)
		}
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
			props[k] = current
		}
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

func (f *fakeAPI) DeleteFileSystem(ctx context.Context, in *fsx.DeleteFileSystemInput, _ ...func(*fsx.Options)) (*fsx.DeleteFileSystemOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DeleteFileSystem"); err != nil {
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
	skip, tags, typed := true, []types.Tag(nil), 0
	if c := in.WindowsConfiguration; c != nil {
		skip, tags, typed = aws.ToBool(c.SkipFinalBackup), c.FinalBackupTags, typed+1
	}
	if c := in.OpenZFSConfiguration; c != nil {
		skip, tags, typed = aws.ToBool(c.SkipFinalBackup), c.FinalBackupTags, typed+1
		f.deleteOptions[id] = c.Options
	}
	if c := in.LustreConfiguration; c != nil {
		skip, tags, typed = aws.ToBool(c.SkipFinalBackup), c.FinalBackupTags, typed+1
	}
	if typed > 1 || (typed == 1 && props["FileSystemType"] == string(types.FileSystemTypeOntap)) {
		return nil, fault("BadRequest", "The configuration doesn't match the file system type.")
	}
	if !skip {
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

// checkTagCount fails as FSx does when a file system would have more than 50 tags, not counting those with the aws:
// prefix.
func checkTagCount(n int) error {
	if n > maxTags {
		return fault("BadRequest", "A file system can have at most 50 tags.")
	}
	return nil
}

// tagCount returns how many tags of the file system count toward its limit, with the keys added as well.
func (f *fakeAPI) tagCount(id string, added []types.Tag) int {
	keys := map[string]bool{}
	fs, _ := f.fileSystem(id)
	if fs != nil {
		for _, t := range fs.Tags {
			keys[aws.ToString(t.Key)] = true
		}
	}
	for _, t := range added {
		keys[aws.ToString(t.Key)] = true
	}
	n := 0
	for key := range keys {
		if !strings.HasPrefix(key, "aws:") {
			n++
		}
	}
	return n
}

func (f *fakeAPI) TagResource(ctx context.Context, in *fsx.TagResourceInput, _ ...func(*fsx.Options)) (*fsx.TagResourceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := strings.TrimPrefix(aws.ToString(in.ResourceARN), arnPrefix)
	if err := checkTagCount(f.tagCount(id, in.Tags)); err != nil {
		f.calls = append(f.calls, "TagResource")
		return nil, err
	}
	return &fsx.TagResourceOutput{}, f.change("TagResource", id, func(props map[string]any) {
		tags := map[string]string{}
		for _, t := range awsapi.Tags(props) {
			tags[t.Key] = t.Value
		}
		for _, t := range in.Tags {
			tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
		}
		props["Tags"] = awsapi.TagList(tags)
	})
}

func (f *fakeAPI) UntagResource(ctx context.Context, in *fsx.UntagResourceInput, _ ...func(*fsx.Options)) (*fsx.UntagResourceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := strings.TrimPrefix(aws.ToString(in.ResourceARN), arnPrefix)
	return &fsx.UntagResourceOutput{}, f.change("UntagResource", id, func(props map[string]any) {
		if slices.Contains(in.TagKeys, awsapi.OperationTag) {
			delete(f.tokens, id)
		}
		if slices.Contains(in.TagKeys, deleteOptionsTag) {
			delete(f.options, id)
		}
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
