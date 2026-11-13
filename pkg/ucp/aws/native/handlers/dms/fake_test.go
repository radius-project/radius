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

package dms

import (
	"context"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	dms "github.com/aws/aws-sdk-go-v2/service/databasemigrationservice"
	"github.com/aws/aws-sdk-go-v2/service/databasemigrationservice/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const arnPrefix = "arn:aws:dms:us-west-2:123456789012:rep:"

const (
	// defaultEngineVersion, defaultZone, and defaultWindow are the values the fake gives a create that leaves out
	// EngineVersion, AvailabilityZone, and PreferredMaintenanceWindow, as DMS picks its own.
	defaultEngineVersion = "3.6.1"
	defaultZone          = "us-west-2b"
	defaultWindow        = "mon:03:00-mon:03:30"

	// storageQuota is DMS's default quota of storage, in GB, for all replication instances in a region, and
	// instanceQuota its default quota of replication instances.
	storageQuota  = 30000
	instanceQuota = 60

	// describePageSize is how many instances a page of DescribeReplicationInstances holds when MaxRecords is left out.
	describePageSize = 100
)

// queued are the properties a modify that isn't applied immediately leaves pending until the maintenance window: the
// fields of ReplicationPendingModifiedValues.
var queued = []string{"AllocatedStorage", "EngineVersion", "MultiAZ", "NetworkType", "ReplicationInstanceClass"}

// fakeAPI is an in-memory DMS API. It keeps replication instances in an awsfake store, in the type's property
// format, so the suite's Cloud Control sees the same instances. An instance reports a creating, modifying, or
// deleting status for one read before it finishes. It follows the rules DMS's API reference documents, as recorded
// in api_test.go.
type fakeAPI struct {
	store  *awsfake.CloudControl
	failer nativetest.Failer

	mu     sync.Mutex
	status map[string]string
	calls  []string

	// pending maps an instance to the changes of a modify that wasn't applied immediately.
	pending map[string]map[string]any

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error

	// pageSize, when set, is how many instances one page holds.
	pageSize int

	// holdDeleting keeps a deleting instance deleting, instead of removing it after one read.
	holdDeleting bool

	// pendingReads is how many reads report a modification as pending after the instance is available again.
	pendingReads int

	// lastModify is the input of the last ModifyReplicationInstance call.
	lastModify *dms.ModifyReplicationInstanceInput
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{store: store, status: map[string]string{}, pending: map[string]map[string]any{}, errs: map[string]error{}}
}

// identifierPattern is the pattern of a replication instance identifier: 1 to 63 letters, digits, or hyphens, the
// first a letter. resourceIdentifierPattern is the same for a ResourceIdentifier of up to 31 characters.
var (
	identifierPattern         = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]{0,62}$`)
	resourceIdentifierPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]{0,30}$`)
)

// validName reports whether name matches pattern, and neither ends with a hyphen nor has two hyphens in a row.
func validName(pattern *regexp.Regexp, name string) bool {
	return pattern.MatchString(name) && !strings.HasSuffix(name, "-") && !strings.Contains(name, "--")
}

// windowPattern is the format of a maintenance window, ddd:hh24:mi-ddd:hh24:mi.
var windowPattern = regexp.MustCompile(`^(?i)(mon|tue|wed|thu|fri|sat|sun):([01]\d|2[0-3]):([0-5]\d)-(mon|tue|wed|thu|fri|sat|sun):([01]\d|2[0-3]):([0-5]\d)$`)

var weekDays = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

// validWindow reports whether window has the format of a maintenance window and is at least 30 minutes long.
func validWindow(window string) bool {
	m := windowPattern.FindStringSubmatch(window)
	if m == nil {
		return false
	}
	minute := func(day string, hour string, min string) int {
		h, _ := strconv.Atoi(hour)
		mi, _ := strconv.Atoi(min)
		return slices.Index(weekDays, strings.ToLower(day))*24*60 + h*60 + mi
	}
	const week = 7 * 24 * 60
	length := (minute(m[4], m[5], m[6]) - minute(m[1], m[2], m[3]) + week) % week
	return length >= 30
}

// tagPattern is the pattern of a tag's key and value.
var tagPattern = regexp.MustCompile(`^[\p{L}\p{Z}\p{N}_.:/=+\-]*$`)

// validTags returns ValidationError when a tag's key isn't 1 to 128 characters, its value is more than 256, or
// either doesn't match the tags' pattern or starts with aws: or dms:.
func validTags(tags []types.Tag) error {
	for _, t := range tags {
		key, value := aws.ToString(t.Key), aws.ToString(t.Value)
		if n := utf8.RuneCountInString(key); n < 1 || n > 128 || utf8.RuneCountInString(value) > 256 {
			return fault("ValidationError")
		}
		for _, s := range []string{key, value} {
			if !tagPattern.MatchString(s) || strings.HasPrefix(s, "aws:") || strings.HasPrefix(s, "dms:") {
				return fault("ValidationError")
			}
		}
	}
	return nil
}

// validCreate returns ValidationError when a create breaks a limit of its parameters.
func validCreate(in *dms.CreateReplicationInstanceInput) error {
	valid := validName(identifierPattern, aws.ToString(in.ReplicationInstanceIdentifier)) &&
		utf8.RuneCountInString(aws.ToString(in.ReplicationInstanceClass)) <= 30 &&
		(in.ResourceIdentifier == nil || validName(resourceIdentifierPattern, *in.ResourceIdentifier)) &&
		(in.DnsNameServers == nil || len(strings.Split(*in.DnsNameServers, ",")) <= 4) &&
		(in.PreferredMaintenanceWindow == nil || validWindow(*in.PreferredMaintenanceWindow)) &&
		!(aws.ToBool(in.MultiAZ) && in.AvailabilityZone != nil)
	if !valid {
		return fault("ValidationError")
	}
	return validTags(in.Tags)
}

// createDefaults sets the values DMS gives the parameters a create leaves out. With AutoMinorVersionUpgrade, which is
// true by default, DMS uses the default engine version in place of an older one.
func createDefaults(props map[string]any, in *dms.CreateReplicationInstanceInput) {
	autoUpgrade := in.AutoMinorVersionUpgrade == nil || *in.AutoMinorVersionUpgrade
	props["AutoMinorVersionUpgrade"] = autoUpgrade
	if in.PubliclyAccessible == nil {
		props["PubliclyAccessible"] = true
	}
	if in.AvailabilityZone == nil {
		props["AvailabilityZone"] = defaultZone
	}
	if in.PreferredMaintenanceWindow == nil {
		props["PreferredMaintenanceWindow"] = defaultWindow
	}
	if version := aws.ToString(in.EngineVersion); version == "" || (autoUpgrade && olderVersion(version, defaultEngineVersion)) {
		props["EngineVersion"] = defaultEngineVersion
	}
}

// olderVersion reports whether engine version a is older than b, comparing their dot-separated numbers in order.
func olderVersion(a string, b string) bool {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := range max(len(as), len(bs)) {
		x, y := versionPart(as, i), versionPart(bs, i)
		if x != y {
			return x < y
		}
	}
	return false
}

func versionPart(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	n, _ := strconv.Atoi(parts[i])
	return n
}

// majorVersion returns the major version of an engine version, its first number, such as 3 for 3.5.3.
func majorVersion(version string) string {
	major, _, _ := strings.Cut(version, ".")
	return major
}

// storageOf returns the AllocatedStorage of an instance's properties, or 0 when it has none.
func storageOf(props map[string]any) int {
	switch n := props["AllocatedStorage"].(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	}
	return 0
}

// storage returns the total AllocatedStorage of every instance other than except.
func (f *fakeAPI) storage(except string) int {
	total := 0
	for _, id := range f.store.List(instanceType) {
		if id != except {
			total += storageOf(f.store.Resource(instanceType, id))
		}
	}
	return total
}

// validModify returns the error DMS returns for a modify of the instance id that breaks a rule.
func (f *fakeAPI) validModify(id string, in *dms.ModifyReplicationInstanceInput) error {
	props := f.store.Resource(instanceType, id)
	current := awsapi.String(props, "EngineVersion")
	valid := utf8.RuneCountInString(aws.ToString(in.ReplicationInstanceClass)) <= 30 &&
		(in.PreferredMaintenanceWindow == nil || validWindow(*in.PreferredMaintenanceWindow)) &&
		(in.EngineVersion == nil || in.AllowMajorVersionUpgrade || majorVersion(*in.EngineVersion) == majorVersion(current)) &&
		(in.AllocatedStorage == nil || int(*in.AllocatedStorage) >= storageOf(props))
	if !valid {
		return fault("ValidationError")
	}
	if in.AllocatedStorage != nil && f.storage(id)+int(*in.AllocatedStorage) > storageQuota {
		return fault("StorageQuotaExceededFault")
	}
	return nil
}

func fault(code string) error {
	return &smithy.GenericAPIError{Code: code, Message: code, Fault: smithy.FaultClient}
}

func (f *fakeAPI) call(name string) error {
	f.calls = append(f.calls, name)
	return f.errs[name]
}

// state returns the status of the instance, and moves a transitional status on: a create or update finishes, and a
// delete removes the instance, which then returns false.
func (f *fakeAPI) state(id string) (string, bool) {
	status, ok := f.status[id]
	if !ok {
		return "available", true
	}
	switch status {
	case "creating", "modifying":
		f.status[id] = "available-next"
		return status, true
	case "deleting":
		if !f.holdDeleting {
			f.status[id] = "deleted-next"
		}
		return status, true
	case "available-next":
		delete(f.status, id)
		return "available", true
	case "deleted-next":
		delete(f.status, id)
		f.store.Remove(instanceType, id)
		delete(f.pending, id)
		return "", false
	}
	return status, true
}

func (f *fakeAPI) CreateReplicationInstance(ctx context.Context, in *dms.CreateReplicationInstanceInput, _ ...func(*dms.Options)) (*dms.CreateReplicationInstanceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("CreateReplicationInstance"); err != nil {
		return nil, err
	}
	if err := validCreate(in); err != nil {
		return nil, err
	}
	id := strings.ToLower(aws.ToString(in.ReplicationInstanceIdentifier))
	if f.store.Resource(instanceType, id) != nil {
		return nil, fault("ResourceAlreadyExistsFault")
	}
	if len(f.store.List(instanceType)) >= instanceQuota {
		return nil, fault("ResourceQuotaExceededFault")
	}
	if f.storage("")+int(aws.ToInt32(in.AllocatedStorage)) > storageQuota {
		return nil, fault("StorageQuotaExceededFault")
	}
	props, err := awsapi.EncodeObject(in)
	if err != nil {
		return nil, err
	}
	props["ReplicationInstanceIdentifier"] = id
	createDefaults(props, in)
	f.store.Put(instanceType, id, props)
	f.status[id] = "creating"
	return &dms.CreateReplicationInstanceOutput{}, nil
}

// arn returns the ARN of the instance with the identifier and props.
func arn(id string, props map[string]any) string {
	if resource := awsapi.String(props, "ResourceIdentifier"); resource != "" {
		return arnPrefix + resource
	}
	return arnPrefix + "GENERATED" + strings.ToUpper(strings.ReplaceAll(id, "-", ""))
}

// instanceOf returns the identifier of the instance with arn, or "".
func (f *fakeAPI) instanceOf(arnValue *string) string {
	for _, id := range f.store.List(instanceType) {
		if arn(id, f.store.Resource(instanceType, id)) == aws.ToString(arnValue) {
			return id
		}
	}
	return ""
}

// instance returns the instance with the identifier as DescribeReplicationInstances does, and false when there is
// none.
func (f *fakeAPI) instance(id string) (types.ReplicationInstance, bool, error) {
	props := f.store.Resource(instanceType, id)
	if props == nil {
		return types.ReplicationInstance{}, false, nil
	}
	status, ok := f.state(id)
	if !ok {
		return types.ReplicationInstance{}, false, nil
	}
	i := types.ReplicationInstance{}
	if err := awsapi.Decode(awsapi.Without(props, "Tags", "VpcSecurityGroupIds"), &i); err != nil {
		return i, false, err
	}
	if i.ReplicationInstanceIdentifier == nil {
		i.ReplicationInstanceIdentifier = aws.String(id)
	}
	i.ReplicationInstanceArn = aws.String(arn(id, props))
	i.ReplicationInstanceStatus = aws.String(status)
	i.ReplicationInstancePrivateIpAddresses = []string{"10.0.0.1"}
	if group := awsapi.String(props, "ReplicationSubnetGroupIdentifier"); group != "" {
		i.ReplicationSubnetGroup = &types.ReplicationSubnetGroup{ReplicationSubnetGroupIdentifier: aws.String(group)}
	}
	for _, g := range awsapi.Strings(props, "VpcSecurityGroupIds") {
		i.VpcSecurityGroups = append(i.VpcSecurityGroups, types.VpcSecurityGroupMembership{VpcSecurityGroupId: aws.String(g), Status: aws.String("active")})
	}
	if values := f.pending[id]; len(values) > 0 {
		i.PendingModifiedValues = &types.ReplicationPendingModifiedValues{}
		if err := awsapi.Decode(values, i.PendingModifiedValues); err != nil {
			return i, false, err
		}
	}
	if status == "available" && f.pendingReads > 0 {
		f.pendingReads--
		i.PendingModifiedValues = &types.ReplicationPendingModifiedValues{ReplicationInstanceClass: aws.String("dms.t3.large")}
	}
	return i, true, nil
}

func (f *fakeAPI) DescribeReplicationInstances(ctx context.Context, in *dms.DescribeReplicationInstancesInput, _ ...func(*dms.Options)) (*dms.DescribeReplicationInstancesOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DescribeReplicationInstances"); err != nil {
		return nil, err
	}
	if in.MaxRecords != nil && (*in.MaxRecords < 20 || *in.MaxRecords > 100) {
		return nil, fault("ValidationError")
	}
	ids := f.store.List(instanceType)
	filtered := false
	for _, filter := range in.Filters {
		switch aws.ToString(filter.Name) {
		case "replication-instance-id":
			ids, filtered = filter.Values, true
		case "replication-instance-arn", "replication-instance-class", "engine-version":
		default:
			return nil, fault("ValidationError")
		}
	}
	size := describePageSize
	if in.MaxRecords != nil {
		size = int(*in.MaxRecords)
	}
	if f.pageSize > 0 {
		size = min(size, f.pageSize)
	}
	out := &dms.DescribeReplicationInstancesOutput{}
	start, _ := strconv.Atoi(aws.ToString(in.Marker))
	for n := start; n < len(ids); n++ {
		if len(out.ReplicationInstances) == size {
			out.Marker = aws.String(strconv.Itoa(n))
			break
		}
		i, ok, err := f.instance(ids[n])
		if err != nil {
			return nil, err
		}
		if ok {
			out.ReplicationInstances = append(out.ReplicationInstances, i)
		}
	}
	if filtered && len(out.ReplicationInstances) == 0 {
		return nil, fault("ResourceNotFoundFault")
	}
	return out, nil
}

// change applies a change to the stored instance arn names, as a call that changes AWS.
func (f *fakeAPI) change(arnValue *string, apply func(id string, props map[string]any)) error {
	if err := f.failer.Before(); err != nil {
		return err
	}
	id := f.instanceOf(arnValue)
	if id == "" {
		return fault("ResourceNotFoundFault")
	}
	props := f.store.Resource(instanceType, id)
	apply(id, props)
	f.store.Put(instanceType, id, props)
	return f.failer.After()
}

func (f *fakeAPI) ModifyReplicationInstance(ctx context.Context, in *dms.ModifyReplicationInstanceInput, _ ...func(*dms.Options)) (*dms.ModifyReplicationInstanceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("ModifyReplicationInstance"); err != nil {
		return nil, err
	}
	f.lastModify = in
	if id := f.instanceOf(in.ReplicationInstanceArn); id != "" {
		if err := f.validModify(id, in); err != nil {
			return nil, err
		}
	}
	values, err := awsapi.EncodeObject(in)
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"ReplicationInstanceArn", "ApplyImmediately", "AllowMajorVersionUpgrade"} {
		delete(values, name)
	}
	if in.EngineVersion != nil {
		values["AllowMajorVersionUpgrade"] = in.AllowMajorVersionUpgrade
	}
	err = f.change(in.ReplicationInstanceArn, func(id string, props map[string]any) {
		// A modify applied immediately also applies the pending changes, and any other leaves its changes of the
		// queued properties pending.
		pending := f.pending[id]
		if pending == nil {
			pending = map[string]any{}
		}
		for _, name := range queued {
			if v, ok := values[name]; ok {
				pending[name] = v
				delete(values, name)
			}
		}
		if in.ApplyImmediately {
			maps.Copy(values, pending)
			delete(f.pending, id)
		} else if len(pending) > 0 {
			f.pending[id] = pending
		}
		maps.Copy(props, values)
		f.status[id] = "modifying"
	})
	return &dms.ModifyReplicationInstanceOutput{}, err
}

func (f *fakeAPI) DeleteReplicationInstance(ctx context.Context, in *dms.DeleteReplicationInstanceInput, _ ...func(*dms.Options)) (*dms.DeleteReplicationInstanceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DeleteReplicationInstance"); err != nil {
		return nil, err
	}
	id := f.instanceOf(in.ReplicationInstanceArn)
	if id == "" {
		return nil, fault("ResourceNotFoundFault")
	}
	if strings.HasPrefix(f.status[id], "delet") {
		return nil, fault("InvalidResourceStateFault")
	}
	f.status[id] = "deleting"
	return &dms.DeleteReplicationInstanceOutput{}, nil
}

func (f *fakeAPI) ListTagsForResource(ctx context.Context, in *dms.ListTagsForResourceInput, _ ...func(*dms.Options)) (*dms.ListTagsForResourceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("ListTagsForResource"); err != nil {
		return nil, err
	}
	id := f.instanceOf(in.ResourceArn)
	if id == "" {
		return nil, fault("ResourceNotFoundFault")
	}
	out := &dms.ListTagsForResourceOutput{}
	for _, t := range awsapi.Tags(f.store.Resource(instanceType, id)) {
		out.TagList = append(out.TagList, types.Tag{Key: aws.String(t.Key), Value: aws.String(t.Value), ResourceArn: in.ResourceArn})
	}
	return out, nil
}

func (f *fakeAPI) AddTagsToResource(ctx context.Context, in *dms.AddTagsToResourceInput, _ ...func(*dms.Options)) (*dms.AddTagsToResourceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("AddTagsToResource"); err != nil {
		return nil, err
	}
	if err := validTags(in.Tags); err != nil {
		return nil, err
	}
	return &dms.AddTagsToResourceOutput{}, f.change(in.ResourceArn, func(_ string, props map[string]any) {
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

func (f *fakeAPI) RemoveTagsFromResource(ctx context.Context, in *dms.RemoveTagsFromResourceInput, _ ...func(*dms.Options)) (*dms.RemoveTagsFromResourceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("RemoveTagsFromResource"); err != nil {
		return nil, err
	}
	return &dms.RemoveTagsFromResourceOutput{}, f.change(in.ResourceArn, func(_ string, props map[string]any) {
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
