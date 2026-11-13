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

package docdb

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
	"github.com/aws/aws-sdk-go-v2/service/docdb"
	"github.com/aws/aws-sdk-go-v2/service/docdb/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	arnPrefix = "arn:aws:rds:us-west-2:123456789012:"

	// pageSize is how many clusters or instances a describe returns when it doesn't set MaxRecords.
	pageSize = 100
)

// fakeAPI is an in-memory DocumentDB API. It keeps clusters and instances in an awsfake store, in their types'
// property format, so the suite's Cloud Control sees the same resources. A cluster or instance reports a creating,
// modifying, or deleting status for one read before it finishes. It follows the rules in clusterAPI and instanceAPI.
type fakeAPI struct {
	store  *awsfake.CloudControl
	failer nativetest.Failer

	mu        sync.Mutex
	status    map[string]string
	snapshots []string
	calls     []string

	// pending maps a cluster or instance to the changes a modify without ApplyImmediately left for its next
	// maintenance window.
	pending map[string]map[string]any

	// restarts counts the restarts of each instance a CA certificate change caused.
	restarts map[string]int

	// goneAfterDelete makes a DeleteDBCluster call that fails remove the cluster first, as when AWS finishes another
	// delete between the call and the next describe.
	goneAfterDelete bool

	// lastModifyInstance is the input of the last ModifyDBInstance call.
	lastModifyInstance *docdb.ModifyDBInstanceInput

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{store: store, status: map[string]string{}, pending: map[string]map[string]any{}, restarts: map[string]int{}, errs: map[string]error{}}
}

func fault(code string) error {
	return &smithy.GenericAPIError{Code: code, Message: code, Fault: smithy.FaultClient}
}

func (f *fakeAPI) call(name string) error {
	f.calls = append(f.calls, name)
	return f.errs[name]
}

// notFound returns the error DocumentDB returns for a cluster or instance it doesn't have.
func notFound(typeName string) error {
	if typeName == clusterType {
		return fault("DBClusterNotFoundFault")
	}
	return fault("DBInstanceNotFound")
}

func statusKey(typeName string, id string) string {
	return typeName + "/" + id
}

// state returns the status of the resource, and moves a transitional status on: a create or update finishes, and a
// delete removes the resource, which then returns false.
func (f *fakeAPI) state(typeName string, id string) (string, bool) {
	key := statusKey(typeName, id)
	status, ok := f.status[key]
	if !ok {
		return "available", true
	}
	switch status {
	case "creating", "modifying":
		f.status[key] = "available-next"
		return status, true
	case "deleting":
		f.status[key] = "deleted-next"
		return status, true
	case "available-next":
		delete(f.status, key)
		return "available", true
	case "deleted-next":
		delete(f.status, key)
		f.store.Remove(typeName, id)
		return "", false
	}
	return status, true
}

// available reports whether the resource is available: it isn't being created, modified, or deleted.
func (f *fakeAPI) available(typeName string, id string) bool {
	status := f.status[statusKey(typeName, id)]
	return status == "" || status == "available-next"
}

// create stores a new cluster or instance with properties, in the creating status.
func (f *fakeAPI) create(typeName string, id string, props map[string]any) error {
	if f.store.Resource(typeName, id) != nil {
		if typeName == clusterType {
			return fault("DBClusterAlreadyExistsFault")
		}
		return fault("DBInstanceAlreadyExists")
	}
	props["Engine"] = Engine
	if typeName == clusterType {
		props["DBClusterIdentifier"] = id
	} else {
		props["DBInstanceIdentifier"] = id
	}
	f.store.Put(typeName, id, props)
	f.status[statusKey(typeName, id)] = "creating"
	return nil
}

// rules collects the first DocumentDB rule a call breaks, as the client fault AWS returns for it.
type rules struct {
	err error
}

// value fails the call with InvalidParameterValue unless ok.
func (r *rules) value(ok bool) {
	if r.err == nil && !ok {
		r.err = fault("InvalidParameterValue")
	}
}

// combination fails the call with InvalidParameterCombination unless ok.
func (r *rules) combination(ok bool) {
	if r.err == nil && !ok {
		r.err = fault("InvalidParameterCombination")
	}
}

// identifierPattern is the pattern of DocumentDB's cluster, instance, snapshot, and parameter group names: letters,
// numbers, and hyphens, starting with a letter.
var identifierPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*$`)

// validIdentifier reports whether id is 1 to maxLength letters, numbers, or hyphens, starts with a letter, doesn't
// end with a hyphen, and has no two hyphens in a row.
func validIdentifier(id string, maxLength int) bool {
	return len(id) <= maxLength && identifierPattern.MatchString(id) && !strings.HasSuffix(id, "-") && !strings.Contains(id, "--")
}

// optionalIdentifier is validIdentifier for a parameter the call can leave out.
func optionalIdentifier(id *string, maxLength int) bool {
	return id == nil || validIdentifier(*id, maxLength)
}

var (
	usernamePattern       = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{0,62}$`)
	globalClusterPattern  = regexp.MustCompile(`^[A-Za-z][0-9A-Za-z-:._]{0,254}$`)
	tagPattern            = regexp.MustCompile(`^[\p{L}\p{Z}\p{N}_.:/=+\-]*$`)
	backupWindowPattern   = regexp.MustCompile(`^([01]\d|2[0-3]):([0-5]\d)-([01]\d|2[0-3]):([0-5]\d)$`)
	maintenanceDayPattern = `(mon|tue|wed|thu|fri|sat|sun)`
	maintenancePattern    = regexp.MustCompile(`(?i)^` + maintenanceDayPattern + `:([01]\d|2[0-3]):([0-5]\d)-` + maintenanceDayPattern + `:([01]\d|2[0-3]):([0-5]\d)$`)
	days                  = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}
)

// validPassword reports whether password is 8 to 100 printable ASCII characters other than /, ", and @.
func validPassword(password *string) bool {
	if password == nil {
		return true
	}
	if len(*password) < 8 || len(*password) > 100 {
		return false
	}
	for _, r := range *password {
		if r < ' ' || r > '~' || r == '/' || r == '"' || r == '@' {
			return false
		}
	}
	return true
}

func inRange(v *int32, low int32, high int32) bool {
	return v == nil || (*v >= low && *v <= high)
}

func oneOf(v *string, values ...string) bool {
	return v == nil || slices.Contains(values, *v)
}

// tags checks DocumentDB's rules for tags: a key is 1 to 128 characters and a value at most 256, both of letters,
// digits, white space, and _.:/=+-, and neither starts with aws: or rds:.
func (r *rules) tags(tags []types.Tag) {
	for _, t := range tags {
		key, value := aws.ToString(t.Key), aws.ToString(t.Value)
		r.value(utf8.RuneCountInString(key) >= 1 && utf8.RuneCountInString(key) <= 128 && utf8.RuneCountInString(value) <= 256)
		r.value(tagPattern.MatchString(key) && tagPattern.MatchString(value))
		for _, prefix := range []string{"aws:", "rds:"} {
			r.value(!strings.HasPrefix(key, prefix) && !strings.HasPrefix(value, prefix))
		}
	}
}

// window is a time range that repeats every cycle minutes: a day for a backup window, a week for a maintenance
// window. start is in minutes from the start of the cycle.
type window struct {
	start  int
	length int
	cycle  int
}

const (
	day  = 24 * 60
	week = 7 * day
)

func minutes(hours string, mins string) int {
	h, _ := strconv.Atoi(hours)
	m, _ := strconv.Atoi(mins)
	return h*60 + m
}

func newWindow(start int, end int, cycle int) window {
	return window{start: start, length: ((end-start)%cycle + cycle) % cycle, cycle: cycle}
}

// backupWindow reads a backup window, hh24:mi-hh24:mi.
func backupWindow(s string) (window, bool) {
	m := backupWindowPattern.FindStringSubmatch(s)
	if m == nil {
		return window{}, false
	}
	return newWindow(minutes(m[1], m[2]), minutes(m[3], m[4]), day), true
}

// maintenanceWindow reads a maintenance window, ddd:hh24:mi-ddd:hh24:mi.
func maintenanceWindow(s string) (window, bool) {
	m := maintenancePattern.FindStringSubmatch(s)
	if m == nil {
		return window{}, false
	}
	start := slices.Index(days, strings.ToLower(m[1]))*day + minutes(m[2], m[3])
	end := slices.Index(days, strings.ToLower(m[4]))*day + minutes(m[5], m[6])
	return newWindow(start, end, week), true
}

// overlaps reports whether the daily backup window overlaps the weekly maintenance window on any day.
func (backup window) overlaps(maintenance window) bool {
	for d := range 7 {
		start := d*day + backup.start
		if (maintenance.start-start+week)%week < backup.length || (start-maintenance.start+week)%week < maintenance.length {
			return true
		}
	}
	return false
}

// windows checks the format of the backup and maintenance windows a call sets, that each is at least 30 minutes, and
// that the backup window doesn't overlap the maintenance window. current is the cluster's or instance's properties,
// for the window the call leaves out, or nil.
func (r *rules) windows(backup *string, maintenance *string, current map[string]any) {
	var b, m window
	var okB, okM bool
	if backup != nil {
		b, okB = backupWindow(*backup)
		r.value(okB && b.length >= 30)
	} else {
		b, okB = backupWindow(awsapi.String(current, "PreferredBackupWindow"))
	}
	if maintenance != nil {
		m, okM = maintenanceWindow(*maintenance)
		r.value(okM && m.length >= 30)
	} else {
		m, okM = maintenanceWindow(awsapi.String(current, "PreferredMaintenanceWindow"))
	}
	if (backup != nil || maintenance != nil) && okB && okM {
		r.value(!b.overlaps(m))
	}
}

// defaultWindows sets the windows DocumentDB picks for a cluster or instance that leaves them out. AWS picks them at
// random; the fake picks the first of a few that doesn't overlap the other window.
func defaultWindows(props map[string]any, backup bool) {
	if backup && awsapi.String(props, "PreferredBackupWindow") == "" {
		m, ok := maintenanceWindow(awsapi.String(props, "PreferredMaintenanceWindow"))
		for _, candidate := range []string{"07:00-07:30", "19:00-19:30"} {
			if b, _ := backupWindow(candidate); !ok || !b.overlaps(m) {
				props["PreferredBackupWindow"] = candidate
				break
			}
		}
	}
	if awsapi.String(props, "PreferredMaintenanceWindow") == "" {
		b, ok := backupWindow(awsapi.String(props, "PreferredBackupWindow"))
		for _, candidate := range []string{"sun:10:00-sun:10:30", "wed:22:00-wed:22:30"} {
			if m, _ := maintenanceWindow(candidate); !ok || !b.overlaps(m) {
				props["PreferredMaintenanceWindow"] = candidate
				break
			}
		}
	}
}

// engineVersion returns the version DocumentDB creates a cluster with for version: the newest 8.0 minor version when
// the call leaves it out, which the fake takes to be 8.0.0, and the version the call sets otherwise. The suite's
// OtherForm case covers 5.0 read back as 5.0.0.
func engineVersion(version string) string {
	if version == "" {
		return "8.0.0"
	}
	return version
}

func major(version string) string {
	before, _, _ := strings.Cut(version, ".")
	return before
}

func (f *fakeAPI) CreateDBCluster(ctx context.Context, in *docdb.CreateDBClusterInput, _ ...func(*docdb.Options)) (*docdb.CreateDBClusterOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("CreateDBCluster"); err != nil {
		return nil, err
	}
	r := rules{}
	r.value(validIdentifier(aws.ToString(in.DBClusterIdentifier), 63))
	r.value(aws.ToString(in.Engine) == Engine)
	r.value(inRange(in.BackupRetentionPeriod, 1, 35))
	r.value(in.MasterUsername == nil || usernamePattern.MatchString(*in.MasterUsername))
	r.value(validPassword(in.MasterUserPassword))
	r.combination(!aws.ToBool(in.ManageMasterUserPassword) || in.MasterUserPassword == nil)
	r.combination(in.MasterUserSecretKmsKeyId == nil || aws.ToBool(in.ManageMasterUserPassword))
	r.value(oneOf(in.NetworkType, "IPV4", "DUAL"))
	r.value(oneOf(in.StorageType, "standard", "iopt1"))
	r.windows(in.PreferredBackupWindow, in.PreferredMaintenanceWindow, nil)
	r.value(!strings.EqualFold(aws.ToString(in.DBSubnetGroupName), "default"))
	r.value(in.GlobalClusterIdentifier == nil || globalClusterPattern.MatchString(*in.GlobalClusterIdentifier))
	r.tags(in.Tags)
	if r.err != nil {
		return nil, r.err
	}
	props, err := awsapi.EncodeObject(in)
	if err != nil {
		return nil, err
	}
	props["EngineVersion"] = engineVersion(aws.ToString(in.EngineVersion))
	if in.BackupRetentionPeriod == nil {
		props["BackupRetentionPeriod"] = 1
	}
	if in.CopyTagsToSnapshot == nil {
		props["CopyTagsToSnapshot"] = false
	}
	defaultWindows(props, true)
	return &docdb.CreateDBClusterOutput{}, f.create(clusterType, strings.ToLower(aws.ToString(in.DBClusterIdentifier)), props)
}

// snapshotExists reports whether the fake has the cluster snapshot with the name or ARN.
func (f *fakeAPI) snapshotExists(snapshot string) bool {
	return slices.Contains(f.snapshots, snapshot[strings.LastIndex(snapshot, ":")+1:])
}

func (f *fakeAPI) RestoreDBClusterFromSnapshot(ctx context.Context, in *docdb.RestoreDBClusterFromSnapshotInput, _ ...func(*docdb.Options)) (*docdb.RestoreDBClusterFromSnapshotOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("RestoreDBClusterFromSnapshot"); err != nil {
		return nil, err
	}
	r := rules{}
	r.value(validIdentifier(aws.ToString(in.DBClusterIdentifier), 63))
	r.value(inRange(in.Port, 1150, 65535))
	r.value(optionalIdentifier(in.DBClusterParameterGroupName, 255))
	r.value(oneOf(in.NetworkType, "IPV4", "DUAL"))
	r.value(oneOf(in.StorageType, "standard", "iopt1"))
	r.tags(in.Tags)
	if r.err != nil {
		return nil, r.err
	}
	if !f.snapshotExists(aws.ToString(in.SnapshotIdentifier)) {
		return nil, fault("DBClusterSnapshotNotFoundFault")
	}
	props, err := awsapi.EncodeObject(in)
	if err != nil {
		return nil, err
	}
	return &docdb.RestoreDBClusterFromSnapshotOutput{}, f.create(clusterType, strings.ToLower(aws.ToString(in.DBClusterIdentifier)), props)
}

// notCopied are the properties a cluster restored to a point in time doesn't take from its source: it gets the
// default security group, the engine's default port, and its own name and tags.
var notCopied = []string{"DBClusterIdentifier", "Tags", "VpcSecurityGroupIds", "Port"}

func (f *fakeAPI) RestoreDBClusterToPointInTime(ctx context.Context, in *docdb.RestoreDBClusterToPointInTimeInput, _ ...func(*docdb.Options)) (*docdb.RestoreDBClusterToPointInTimeOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("RestoreDBClusterToPointInTime"); err != nil {
		return nil, err
	}
	latest := aws.ToBool(in.UseLatestRestorableTime)
	r := rules{}
	r.value(validIdentifier(aws.ToString(in.DBClusterIdentifier), 63))
	r.combination(in.RestoreToTime != nil || latest)
	r.combination(in.RestoreToTime == nil || !latest)
	r.combination(in.RestoreToTime == nil || aws.ToString(in.RestoreType) != "copy-on-write")
	r.value(oneOf(in.RestoreType, "full-copy", "copy-on-write"))
	r.value(inRange(in.Port, 1150, 65535))
	r.value(oneOf(in.NetworkType, "IPV4", "DUAL"))
	r.value(oneOf(in.StorageType, "standard", "iopt1"))
	r.tags(in.Tags)
	if r.err != nil {
		return nil, r.err
	}
	source := f.store.Resource(clusterType, strings.ToLower(aws.ToString(in.SourceDBClusterIdentifier)))
	if source == nil {
		return nil, fault("DBClusterNotFoundFault")
	}
	values, err := awsapi.EncodeObject(in)
	if err != nil {
		return nil, err
	}
	props := awsapi.Without(source, notCopied...)
	for k, v := range values {
		props[k] = v
	}
	return &docdb.RestoreDBClusterToPointInTimeOutput{}, f.create(clusterType, strings.ToLower(aws.ToString(in.DBClusterIdentifier)), props)
}

func (f *fakeAPI) cluster(id string) (types.DBCluster, bool, error) {
	props := f.store.Resource(clusterType, id)
	if props == nil {
		return types.DBCluster{}, false, nil
	}
	status, ok := f.state(clusterType, id)
	if !ok {
		return types.DBCluster{}, false, nil
	}
	c := types.DBCluster{}
	if err := awsapi.Decode(props, &c); err != nil {
		return c, false, err
	}
	if c.Engine == nil {
		c.Engine = aws.String(Engine)
	}
	c.DBClusterIdentifier = aws.String(id)
	c.DBClusterArn = aws.String(arnPrefix + "cluster:" + id)
	c.Status = aws.String(status)
	c.DBClusterParameterGroup = stringProp(props, "DBClusterParameterGroupName")
	c.DBSubnetGroup = stringProp(props, "DBSubnetGroupName")
	c.EnabledCloudwatchLogsExports = awsapi.Strings(props, "EnableCloudwatchLogsExports")
	for _, g := range awsapi.Strings(props, "VpcSecurityGroupIds") {
		c.VpcSecurityGroups = append(c.VpcSecurityGroups, types.VpcSecurityGroupMembership{VpcSecurityGroupId: aws.String(g)})
	}
	if managed, _ := props["ManageMasterUserPassword"].(bool); managed {
		c.MasterUserSecret = &types.ClusterMasterUserSecret{KmsKeyId: stringProp(props, "MasterUserSecretKmsKeyId")}
	}
	c.Endpoint = aws.String(id + ".cluster-abc.us-west-2.docdb.amazonaws.com")
	return c, true, nil
}

func stringProp(props map[string]any, name string) *string {
	if s := awsapi.String(props, name); s != "" {
		return aws.String(s)
	}
	return nil
}

// page returns the identifiers on the page that starts at marker, at most maxRecords of them or pageSize, and the
// marker of the next page, or nil.
func page(ids []string, marker *string, maxRecords *int32) ([]string, *string) {
	start, _ := strconv.Atoi(aws.ToString(marker))
	start = min(start, len(ids))
	size := pageSize
	if maxRecords != nil {
		size = int(*maxRecords)
	}
	end := min(start+size, len(ids))
	if end < len(ids) {
		return ids[start:end], aws.String(strconv.Itoa(end))
	}
	return ids[start:end], nil
}

// matches reports whether props pass the filters, of which the fake supports engine, db-cluster-id, and
// db-instance-id. A cluster or instance ID filter takes identifiers and ARNs.
func matches(props map[string]any, filters []types.Filter) bool {
	for _, filter := range filters {
		var value string
		switch aws.ToString(filter.Name) {
		case "engine":
			value = awsapi.String(props, "Engine")
		case "db-cluster-id":
			value = awsapi.String(props, "DBClusterIdentifier")
		case "db-instance-id":
			value = awsapi.String(props, "DBInstanceIdentifier")
		default:
			continue
		}
		if !slices.ContainsFunc(filter.Values, func(v string) bool {
			return v == value || strings.HasSuffix(v, ":"+value)
		}) {
			return false
		}
	}
	return true
}

func (f *fakeAPI) DescribeDBClusters(ctx context.Context, in *docdb.DescribeDBClustersInput, _ ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DescribeDBClusters"); err != nil {
		return nil, err
	}
	if !inRange(in.MaxRecords, 20, 100) {
		return nil, fault("InvalidParameterValue")
	}
	ids := f.store.List(clusterType)
	if in.DBClusterIdentifier != nil {
		ids = []string{strings.ToLower(aws.ToString(in.DBClusterIdentifier))}
	}
	ids = slices.DeleteFunc(ids, func(id string) bool {
		props := f.store.Resource(clusterType, id)
		return props != nil && !matches(props, in.Filters)
	})
	ids, marker := page(ids, in.Marker, in.MaxRecords)
	out := &docdb.DescribeDBClustersOutput{Marker: marker}
	for _, id := range ids {
		c, ok, err := f.cluster(id)
		if err != nil {
			return nil, err
		}
		if ok {
			out.DBClusters = append(out.DBClusters, c)
		}
	}
	if in.DBClusterIdentifier != nil && len(out.DBClusters) == 0 {
		return nil, fault("DBClusterNotFoundFault")
	}
	return out, nil
}

// change applies values, in the type's property format, to the stored resource, as a call that changes AWS.
func (f *fakeAPI) change(typeName string, id string, apply func(props map[string]any)) error {
	if err := f.failer.Before(); err != nil {
		return err
	}
	props := f.store.Resource(typeName, id)
	if props == nil {
		return notFound(typeName)
	}
	apply(props)
	f.store.Put(typeName, id, props)
	return f.failer.After()
}

// hold keeps the values a modify without ApplyImmediately leaves for the next maintenance window out of values.
func (f *fakeAPI) hold(typeName string, id string, values map[string]any, waits func(name string) bool) {
	key := statusKey(typeName, id)
	for name, v := range values {
		if waits(name) {
			if f.pending[key] == nil {
				f.pending[key] = map[string]any{}
			}
			f.pending[key][name] = v
			delete(values, name)
		}
	}
}

// release returns the changes earlier modifies left for the next maintenance window that values doesn't set, and
// forgets them, for a modify with ApplyImmediately, which applies them too.
func (f *fakeAPI) release(typeName string, id string, values map[string]any) map[string]any {
	key := statusKey(typeName, id)
	out := map[string]any{}
	for name, v := range f.pending[key] {
		if _, ok := values[name]; !ok {
			out[name] = v
		}
	}
	delete(f.pending, key)
	return out
}

func (f *fakeAPI) ModifyDBCluster(ctx context.Context, in *docdb.ModifyDBClusterInput, _ ...func(*docdb.Options)) (*docdb.ModifyDBClusterOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("ModifyDBCluster"); err != nil {
		return nil, err
	}
	id := strings.ToLower(aws.ToString(in.DBClusterIdentifier))
	current := f.store.Resource(clusterType, id)
	managed, _ := current["ManageMasterUserPassword"].(bool)
	r := rules{}
	r.value(inRange(in.BackupRetentionPeriod, 1, 35))
	r.value(inRange(in.Port, 1150, 65535))
	r.value(validPassword(in.MasterUserPassword))
	r.combination(!aws.ToBool(in.ManageMasterUserPassword) || in.MasterUserPassword == nil)
	r.combination(in.ManageMasterUserPassword == nil || *in.ManageMasterUserPassword || !managed || in.MasterUserPassword != nil)
	r.combination(in.MasterUserSecretKmsKeyId == nil || (aws.ToBool(in.ManageMasterUserPassword) && !managed))
	r.combination(!aws.ToBool(in.RotateMasterUserPassword) || (aws.ToBool(in.ApplyImmediately) && managed))
	r.combination(in.EngineVersion == nil || aws.ToBool(in.AllowMajorVersionUpgrade) || current == nil || major(*in.EngineVersion) == major(awsapi.String(current, "EngineVersion")))
	r.value(oneOf(in.NetworkType, "IPV4", "DUAL"))
	r.value(oneOf(in.StorageType, "standard", "iopt1"))
	r.windows(in.PreferredBackupWindow, in.PreferredMaintenanceWindow, current)
	if r.err != nil {
		return nil, r.err
	}
	if current == nil {
		return nil, notFound(clusterType)
	}
	if !f.available(clusterType, id) {
		return nil, fault("InvalidDBClusterStateFault")
	}
	values, err := awsapi.EncodeObject(in)
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"DBClusterIdentifier", "ApplyImmediately", "AllowMajorVersionUpgrade", "CloudwatchLogsExportConfiguration", "RotateMasterUserPassword"} {
		delete(values, name)
	}
	if !aws.ToBool(in.ApplyImmediately) {
		f.hold(clusterType, id, values, func(name string) bool { return name == "MasterUserPassword" || name == "EngineVersion" })
	}
	err = f.change(clusterType, id, func(props map[string]any) {
		if aws.ToBool(in.ApplyImmediately) {
			maps.Copy(props, f.release(clusterType, id, values))
		}
		for k, v := range values {
			props[k] = v
		}
		if manage := in.ManageMasterUserPassword; manage != nil && *manage {
			delete(props, "MasterUserPassword")
		} else if manage != nil {
			delete(props, "MasterUserSecretKmsKeyId")
		}
		if logs := in.CloudwatchLogsExportConfiguration; logs != nil {
			current := awsapi.Strings(props, "EnableCloudwatchLogsExports")
			current = slices.DeleteFunc(current, func(l string) bool { return slices.Contains(logs.DisableLogTypes, l) })
			current = append(current, logs.EnableLogTypes...)
			if len(current) == 0 {
				delete(props, "EnableCloudwatchLogsExports")
			} else {
				props["EnableCloudwatchLogsExports"] = current
			}
		}
		f.status[statusKey(clusterType, id)] = "modifying"
	})
	return &docdb.ModifyDBClusterOutput{}, err
}

// members returns the instances in the cluster.
func (f *fakeAPI) members(cluster string) []string {
	return slices.DeleteFunc(f.store.List(instanceType), func(id string) bool {
		return !strings.EqualFold(awsapi.String(f.store.Resource(instanceType, id), "DBClusterIdentifier"), cluster)
	})
}

func (f *fakeAPI) DeleteDBCluster(ctx context.Context, in *docdb.DeleteDBClusterInput, _ ...func(*docdb.Options)) (*docdb.DeleteDBClusterOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DeleteDBCluster"); err != nil {
		if f.goneAfterDelete {
			f.store.Remove(clusterType, aws.ToString(in.DBClusterIdentifier))
		}
		return nil, err
	}
	skip := aws.ToBool(in.SkipFinalSnapshot)
	r := rules{}
	r.combination(!skip || in.FinalDBSnapshotIdentifier == nil)
	r.combination(skip || in.FinalDBSnapshotIdentifier != nil)
	r.value(optionalIdentifier(in.FinalDBSnapshotIdentifier, 255))
	if r.err != nil {
		return nil, r.err
	}
	id := strings.ToLower(aws.ToString(in.DBClusterIdentifier))
	props := f.store.Resource(clusterType, id)
	if props == nil {
		return nil, notFound(clusterType)
	}
	if !f.available(clusterType, id) || len(f.members(id)) > 0 {
		return nil, fault("InvalidDBClusterStateFault")
	}
	if protected, _ := props["DeletionProtection"].(bool); protected {
		return nil, fault("InvalidParameterCombination")
	}
	if !skip {
		snapshot := aws.ToString(in.FinalDBSnapshotIdentifier)
		if slices.Contains(f.snapshots, snapshot) {
			return nil, fault("DBClusterSnapshotAlreadyExistsFault")
		}
		f.snapshots = append(f.snapshots, snapshot)
	}
	f.status[statusKey(clusterType, id)] = "deleting"
	return &docdb.DeleteDBClusterOutput{}, nil
}

func (f *fakeAPI) CreateDBInstance(ctx context.Context, in *docdb.CreateDBInstanceInput, _ ...func(*docdb.Options)) (*docdb.CreateDBInstanceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("CreateDBInstance"); err != nil {
		return nil, err
	}
	r := rules{}
	r.value(validIdentifier(aws.ToString(in.DBInstanceIdentifier), 63))
	r.value(aws.ToString(in.Engine) == Engine)
	r.value(inRange(in.PromotionTier, 0, 15))
	r.windows(nil, in.PreferredMaintenanceWindow, nil)
	r.value(len(in.Tags) <= 10)
	r.tags(in.Tags)
	if r.err != nil {
		return nil, r.err
	}
	if f.store.Resource(clusterType, strings.ToLower(aws.ToString(in.DBClusterIdentifier))) == nil {
		return nil, fault("DBClusterNotFoundFault")
	}
	props, err := awsapi.EncodeObject(in)
	if err != nil {
		return nil, err
	}
	if in.PromotionTier == nil {
		props["PromotionTier"] = 1
	}
	if in.AvailabilityZone == nil {
		props["AvailabilityZone"] = "us-west-2a"
	}
	if in.CopyTagsToSnapshot == nil {
		props["CopyTagsToSnapshot"] = false
	}
	defaultWindows(props, false)
	return &docdb.CreateDBInstanceOutput{}, f.create(instanceType, strings.ToLower(aws.ToString(in.DBInstanceIdentifier)), props)
}

func (f *fakeAPI) DescribeDBInstances(ctx context.Context, in *docdb.DescribeDBInstancesInput, _ ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DescribeDBInstances"); err != nil {
		return nil, err
	}
	if !inRange(in.MaxRecords, 20, 100) {
		return nil, fault("InvalidParameterValue")
	}
	ids := f.store.List(instanceType)
	if in.DBInstanceIdentifier != nil {
		ids = []string{strings.ToLower(aws.ToString(in.DBInstanceIdentifier))}
	}
	ids = slices.DeleteFunc(ids, func(id string) bool {
		props := f.store.Resource(instanceType, id)
		return props != nil && !matches(props, in.Filters)
	})
	ids, marker := page(ids, in.Marker, in.MaxRecords)
	out := &docdb.DescribeDBInstancesOutput{Marker: marker}
	for _, id := range ids {
		props := f.store.Resource(instanceType, id)
		if props == nil {
			continue
		}
		status, ok := f.state(instanceType, id)
		if !ok {
			continue
		}
		i := types.DBInstance{}
		if err := awsapi.Decode(props, &i); err != nil {
			return nil, err
		}
		if i.Engine == nil {
			i.Engine = aws.String(Engine)
		}
		i.DBInstanceIdentifier = aws.String(id)
		i.DBInstanceArn = aws.String(arnPrefix + "db:" + id)
		i.DBInstanceStatus = aws.String(status)
		if v, ok := props["EnablePerformanceInsights"].(bool); ok {
			i.PerformanceInsightsEnabled = aws.Bool(v)
		}
		i.Endpoint = &types.Endpoint{Address: aws.String(id + ".abc.us-west-2.docdb.amazonaws.com"), Port: aws.Int32(27017)}
		out.DBInstances = append(out.DBInstances, i)
	}
	if in.DBInstanceIdentifier != nil && len(out.DBInstances) == 0 {
		return nil, fault("DBInstanceNotFound")
	}
	return out, nil
}

func (f *fakeAPI) ModifyDBInstance(ctx context.Context, in *docdb.ModifyDBInstanceInput, _ ...func(*docdb.Options)) (*docdb.ModifyDBInstanceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("ModifyDBInstance"); err != nil {
		return nil, err
	}
	f.lastModifyInstance = in
	r := rules{}
	r.value(inRange(in.PromotionTier, 0, 15))
	r.windows(nil, in.PreferredMaintenanceWindow, nil)
	if r.err != nil {
		return nil, r.err
	}
	id := strings.ToLower(aws.ToString(in.DBInstanceIdentifier))
	current := f.store.Resource(instanceType, id)
	if current == nil {
		return nil, notFound(instanceType)
	}
	if !f.available(instanceType, id) {
		return nil, fault("InvalidDBInstanceState")
	}
	values, err := awsapi.EncodeObject(in)
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"DBInstanceIdentifier", "ApplyImmediately"} {
		delete(values, name)
	}
	if !aws.ToBool(in.ApplyImmediately) {
		f.hold(instanceType, id, values, func(name string) bool { return name != "PreferredMaintenanceWindow" })
	}
	err = f.change(instanceType, id, func(props map[string]any) {
		changes := maps.Clone(values)
		if aws.ToBool(in.ApplyImmediately) {
			queued := f.release(instanceType, id, values)
			if _, ok := values["CACertificateIdentifier"]; ok {
				// The request's CA change goes with the request's CertificateRotationRestart, not a queued one.
				delete(queued, "CertificateRotationRestart")
			}
			maps.Copy(changes, queued)
		}
		maps.Copy(props, changes)
		ca, changed := changes["CACertificateIdentifier"]
		if restart, ok := changes["CertificateRotationRestart"].(bool); changed && ca != current["CACertificateIdentifier"] && (!ok || restart) {
			f.restarts[id]++
		}
		f.status[statusKey(instanceType, id)] = "modifying"
	})
	return &docdb.ModifyDBInstanceOutput{}, err
}

func (f *fakeAPI) DeleteDBInstance(ctx context.Context, in *docdb.DeleteDBInstanceInput, _ ...func(*docdb.Options)) (*docdb.DeleteDBInstanceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DeleteDBInstance"); err != nil {
		return nil, err
	}
	id := strings.ToLower(aws.ToString(in.DBInstanceIdentifier))
	if f.store.Resource(instanceType, id) == nil {
		return nil, notFound(instanceType)
	}
	if !f.available(instanceType, id) {
		return nil, fault("InvalidDBInstanceState")
	}
	f.status[statusKey(instanceType, id)] = "deleting"
	return &docdb.DeleteDBInstanceOutput{}, nil
}

// resource returns the type and identifier of the resource arn names.
func resource(arn string) (string, string) {
	rest := strings.TrimPrefix(arn, arnPrefix)
	if id, ok := strings.CutPrefix(rest, "cluster:"); ok {
		return clusterType, id
	}
	return instanceType, strings.TrimPrefix(rest, "db:")
}

func (f *fakeAPI) ListTagsForResource(ctx context.Context, in *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("ListTagsForResource"); err != nil {
		return nil, err
	}
	typeName, id := resource(aws.ToString(in.ResourceName))
	props := f.store.Resource(typeName, id)
	if props == nil {
		return nil, notFound(typeName)
	}
	out := &docdb.ListTagsForResourceOutput{}
	for _, t := range awsapi.Tags(props) {
		out.TagList = append(out.TagList, types.Tag{Key: aws.String(t.Key), Value: aws.String(t.Value)})
	}
	return out, nil
}

func (f *fakeAPI) AddTagsToResource(ctx context.Context, in *docdb.AddTagsToResourceInput, _ ...func(*docdb.Options)) (*docdb.AddTagsToResourceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("AddTagsToResource"); err != nil {
		return nil, err
	}
	r := rules{}
	r.tags(in.Tags)
	if r.err != nil {
		return nil, r.err
	}
	typeName, id := resource(aws.ToString(in.ResourceName))
	return &docdb.AddTagsToResourceOutput{}, f.change(typeName, id, func(props map[string]any) {
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

func (f *fakeAPI) RemoveTagsFromResource(ctx context.Context, in *docdb.RemoveTagsFromResourceInput, _ ...func(*docdb.Options)) (*docdb.RemoveTagsFromResourceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("RemoveTagsFromResource"); err != nil {
		return nil, err
	}
	typeName, id := resource(aws.ToString(in.ResourceName))
	return &docdb.RemoveTagsFromResourceOutput{}, f.change(typeName, id, func(props map[string]any) {
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
