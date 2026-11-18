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

package rds

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	arnPrefix = "arn:aws:rds:us-west-2:123456789012:"

	// clusterType and instanceType are the store types of the fake's DB clusters and DB instances, which the
	// handlers only read.
	clusterType  = "AWS::RDS::DBCluster"
	instanceType = "AWS::RDS::DBInstance"

	// clusterPageSize is how many clusters DescribeDBClusters returns when it doesn't set MaxRecords, as AWS
	// documents. tenantPageSize is how many tenant databases DescribeTenantDatabases returns, which AWS doesn't
	// document, small so the handler's paging runs.
	clusterPageSize = 100
	tenantPageSize  = 2
)

// fakeAPI is an in-memory RDS API for activity streams and tenant databases. It keeps them in an awsfake store, in
// their types' property format, so the suite's Cloud Control sees the same resources: a stream under its cluster's ARN
// in lowercase while it is starting, started, or stopping, and a tenant database under its identifier. A stream or a
// tenant database reports a starting, creating, modifying, stopping, or deleting status for one read before it
// finishes. It follows the rules in activityStreamAPI and tenantDatabaseAPI.
type fakeAPI struct {
	store  *awsfake.CloudControl
	failer nativetest.Failer

	mu sync.Mutex

	// status maps a tenant database's identifier to its status while it changes.
	status map[string]string

	// pendingPassword holds the tenant databases whose new password is pending for one read.
	pendingPassword map[string]bool

	// waiting maps a cluster's ARN, in lowercase, to the stream call a call without ApplyImmediately left for the
	// cluster's next maintenance window.
	waiting map[string]string

	snapshots []string

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{store: store, status: map[string]string{}, pendingPassword: map[string]bool{}, waiting: map[string]string{}, errs: map[string]error{}}
}

func fault(code string) error {
	return &smithy.GenericAPIError{Code: code, Message: code, Fault: smithy.FaultClient}
}

// change makes a call that changes AWS, failing it where the suite's failer says.
func (f *fakeAPI) change(apply func() error) error {
	if err := f.failer.Before(); err != nil {
		return err
	}
	if err := apply(); err != nil {
		return err
	}
	return f.failer.After()
}

// rules collects the first rule a call breaks, as the client fault AWS returns for it.
type rules struct {
	err error
}

func (r *rules) check(ok bool, code string) {
	if r.err == nil && !ok {
		r.err = fault(code)
	}
}

func (r *rules) required(values ...*string) {
	for _, v := range values {
		r.check(aws.ToString(v) != "", "MissingParameter")
	}
}

var (
	tagPattern        = regexp.MustCompile(`^[\p{L}\p{Z}\p{N}_.:/=+\-@]*$`)
	usernamePattern   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,15}$`)
	snapshotIDPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]{0,254}$`)
)

// tags checks the rules of RDS's Tag data type: a key is 1 to 128 characters and a value at most 256, both of
// letters, digits, white space, and _.:/=+-@, and neither starts with aws: or rds:.
func (r *rules) tags(tags []types.Tag) {
	for _, t := range tags {
		key, value := aws.ToString(t.Key), aws.ToString(t.Value)
		r.check(utf8.RuneCountInString(key) >= 1 && utf8.RuneCountInString(key) <= 128 && utf8.RuneCountInString(value) <= 256, "InvalidParameterValue")
		r.check(tagPattern.MatchString(key) && tagPattern.MatchString(value), "InvalidParameterValue")
		for _, prefix := range []string{"aws:", "rds:"} {
			r.check(!strings.HasPrefix(key, prefix) && !strings.HasPrefix(value, prefix), "InvalidParameterValue")
		}
	}
}

// password checks the rules of a tenant database's master user password: 8 to 30 printable ASCII characters other
// than /, ", @, &, and '.
func (r *rules) password(password *string) {
	if password == nil {
		return
	}
	ok := len(*password) >= 8 && len(*password) <= 30
	for _, c := range *password {
		ok = ok && c >= ' ' && c <= '~' && !strings.ContainsRune(`/"@&'`, c)
	}
	r.check(ok, "InvalidParameterValue")
}

// addCluster adds an available Aurora cluster to store.
func addCluster(store *awsfake.CloudControl, id string) {
	store.Put(clusterType, id, map[string]any{
		"DBClusterIdentifier": id,
		"DBClusterArn":        arnPrefix + "cluster:" + id,
		"DbClusterResourceId": "cluster-" + resourceID(id),
		"Engine":              "aurora-postgresql",
		"Status":              "available",
	})
}

// addInstance adds an available RDS for Oracle CDB instance in the multi-tenant configuration to store. It has the
// tenant database RDS creates with the instance, which the fake counts but doesn't list.
func addInstance(store *awsfake.CloudControl, id string) {
	store.Put(instanceType, id, map[string]any{
		"DBInstanceIdentifier": id,
		"DbiResourceId":        "db-" + resourceID(id),
		"Status":               "available",
		"InitialTenants":       1,
	})
}

func resourceID(id string) string {
	sum := sha256.Sum256([]byte(id))
	return strings.ToUpper(hex.EncodeToString(sum[:])[:26])
}

func (f *fakeAPI) cluster(identifier string) (map[string]any, bool) {
	for _, id := range f.store.List(clusterType) {
		c := f.store.Resource(clusterType, id)
		if strings.EqualFold(id, identifier) || strings.EqualFold(awsapi.String(c, "DBClusterArn"), identifier) {
			return c, true
		}
	}
	return nil, false
}

// streamKey is the store key of a cluster's stream: its ARN in lowercase.
func streamKey(cluster map[string]any) string {
	return strings.ToLower(awsapi.String(cluster, "DBClusterArn"))
}

// clusterOf returns the cluster as RDS describes it, and moves its stream's transitional status on: a starting
// stream is started at the next read, and a stopping stream is gone.
func (f *fakeAPI) clusterOf(c map[string]any) types.DBCluster {
	out := types.DBCluster{
		DBClusterIdentifier:  aws.String(awsapi.String(c, "DBClusterIdentifier")),
		DBClusterArn:         aws.String(awsapi.String(c, "DBClusterArn")),
		DbClusterResourceId:  aws.String(awsapi.String(c, "DbClusterResourceId")),
		Engine:               aws.String(awsapi.String(c, "Engine")),
		Status:               aws.String(awsapi.String(c, "Status")),
		ActivityStreamStatus: types.ActivityStreamStatusStopped,
	}
	key := streamKey(c)
	stream := f.store.Resource(activityStreamType, key)
	if stream == nil {
		return out
	}
	status := awsapi.String(stream, "Status")
	out.ActivityStreamStatus = types.ActivityStreamStatus(status)
	out.ActivityStreamMode = types.ActivityStreamMode(awsapi.String(stream, "Mode"))
	out.ActivityStreamKmsKeyId = aws.String(awsapi.String(stream, "KmsKeyId"))
	out.ActivityStreamKinesisStreamName = aws.String(awsapi.String(stream, "KinesisStreamName"))
	switch status {
	case string(types.ActivityStreamStatusStarting):
		stream["Status"] = string(types.ActivityStreamStatusStarted)
		f.store.Put(activityStreamType, key, stream)
	case string(types.ActivityStreamStatusStopping):
		f.store.Remove(activityStreamType, key)
	}
	return out
}

func (f *fakeAPI) DescribeDBClusters(ctx context.Context, in *rds.DescribeDBClustersInput, _ ...func(*rds.Options)) (*rds.DescribeDBClustersOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["DescribeDBClusters"]; err != nil {
		return nil, err
	}
	if in.MaxRecords != nil && (*in.MaxRecords < 20 || *in.MaxRecords > 100) {
		return nil, fault("InvalidParameterValue")
	}
	if in.DBClusterIdentifier != nil {
		c, ok := f.cluster(*in.DBClusterIdentifier)
		if !ok {
			return nil, fault("DBClusterNotFoundFault")
		}
		return &rds.DescribeDBClustersOutput{DBClusters: []types.DBCluster{f.clusterOf(c)}}, nil
	}
	ids := f.store.List(clusterType)
	start, _ := strconv.Atoi(aws.ToString(in.Marker))
	size := clusterPageSize
	if in.MaxRecords != nil {
		size = int(*in.MaxRecords)
	}
	out := &rds.DescribeDBClustersOutput{}
	end := min(start+size, len(ids))
	for _, id := range ids[min(start, len(ids)):end] {
		out.DBClusters = append(out.DBClusters, f.clusterOf(f.store.Resource(clusterType, id)))
	}
	if end < len(ids) {
		out.Marker = aws.String(strconv.Itoa(end))
	}
	return out, nil
}

func (f *fakeAPI) StartActivityStream(ctx context.Context, in *rds.StartActivityStreamInput, _ ...func(*rds.Options)) (*rds.StartActivityStreamOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["StartActivityStream"]; err != nil {
		return nil, err
	}
	var r rules
	r.required(in.ResourceArn, in.KmsKeyId, (*string)(&in.Mode))
	r.check(in.Mode == "" || slices.Contains(in.Mode.Values(), in.Mode), "InvalidParameterValue")
	if r.err != nil {
		return nil, r.err
	}
	c, ok := f.cluster(aws.ToString(in.ResourceArn))
	if !ok || !strings.EqualFold(awsapi.String(c, "DBClusterArn"), aws.ToString(in.ResourceArn)) {
		return nil, fault("DBClusterNotFoundFault")
	}
	key := streamKey(c)
	if awsapi.String(c, "Status") != "available" || f.store.Resource(activityStreamType, key) != nil || f.waiting[key] != "" {
		return nil, fault("InvalidDBClusterStateFault")
	}
	stream := map[string]any{
		"ResourceArn":       awsapi.String(c, "DBClusterArn"),
		"Mode":              string(in.Mode),
		"KmsKeyId":          aws.ToString(in.KmsKeyId),
		"KinesisStreamName": "aws-rds-das-" + awsapi.String(c, "DbClusterResourceId"),
		"Status":            string(types.ActivityStreamStatusStarting),
	}
	err := f.change(func() error {
		if aws.ToBool(in.ApplyImmediately) {
			f.store.Put(activityStreamType, key, stream)
		} else {
			f.waiting[key] = "start"
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &rds.StartActivityStreamOutput{
		ApplyImmediately:  in.ApplyImmediately,
		KinesisStreamName: aws.String(awsapi.String(stream, "KinesisStreamName")),
		KmsKeyId:          in.KmsKeyId,
		Mode:              in.Mode,
		Status:            types.ActivityStreamStatusStarting,
	}, nil
}

func (f *fakeAPI) StopActivityStream(ctx context.Context, in *rds.StopActivityStreamInput, _ ...func(*rds.Options)) (*rds.StopActivityStreamOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["StopActivityStream"]; err != nil {
		return nil, err
	}
	var r rules
	r.required(in.ResourceArn)
	if r.err != nil {
		return nil, r.err
	}
	c, ok := f.cluster(aws.ToString(in.ResourceArn))
	if !ok || !strings.EqualFold(awsapi.String(c, "DBClusterArn"), aws.ToString(in.ResourceArn)) {
		return nil, fault("DBClusterNotFoundFault")
	}
	key := streamKey(c)
	stream := f.store.Resource(activityStreamType, key)
	if awsapi.String(c, "Status") != "available" || stream == nil || awsapi.String(stream, "Status") != string(types.ActivityStreamStatusStarted) || f.waiting[key] != "" {
		return nil, fault("InvalidDBClusterStateFault")
	}
	err := f.change(func() error {
		if aws.ToBool(in.ApplyImmediately) {
			stream["Status"] = string(types.ActivityStreamStatusStopping)
			f.store.Put(activityStreamType, key, stream)
		} else {
			f.waiting[key] = "stop"
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &rds.StopActivityStreamOutput{
		KinesisStreamName: aws.String(awsapi.String(stream, "KinesisStreamName")),
		KmsKeyId:          aws.String(awsapi.String(stream, "KmsKeyId")),
		Status:            types.ActivityStreamStatusStopping,
	}, nil
}

func (f *fakeAPI) instance(identifier string) (map[string]any, bool) {
	for _, id := range f.store.List(instanceType) {
		if strings.EqualFold(id, identifier) {
			return f.store.Resource(instanceType, id), true
		}
	}
	return nil, false
}

// tenantCount returns how many tenant databases the instance has, including the one RDS created with it.
func (f *fakeAPI) tenantCount(instance map[string]any) int {
	initial, _ := instance["InitialTenants"].(float64)
	n := int(initial)
	for _, id := range f.store.List(tenantDatabaseType) {
		if strings.EqualFold(awsapi.String(f.store.Resource(tenantDatabaseType, id), "DBInstanceIdentifier"), awsapi.String(instance, "DBInstanceIdentifier")) {
			n++
		}
	}
	return n
}

// tenant returns the instance and the tenant database the names give, with the error RDS returns when either is
// missing.
func (f *fakeAPI) tenant(instanceID *string, name *string) (map[string]any, string, map[string]any, error) {
	instance, ok := f.instance(aws.ToString(instanceID))
	if !ok {
		return nil, "", nil, fault("DBInstanceNotFound")
	}
	key := tenantIdentifier(awsapi.String(instance, "DBInstanceIdentifier"), aws.ToString(name))
	tenant := f.store.Resource(tenantDatabaseType, key)
	if tenant == nil {
		return instance, key, nil, fault("TenantDatabaseNotFound")
	}
	return instance, key, tenant, nil
}

// tenantStatus returns the tenant database's status, and moves a transitional status on: a create or update
// finishes, and a delete removes the tenant database, which then returns false.
func (f *fakeAPI) tenantStatus(key string) (string, bool) {
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
		f.store.Remove(tenantDatabaseType, key)
		return "", false
	}
	return status, true
}

func (f *fakeAPI) tenantAvailable(key string) bool {
	status := f.status[key]
	return status == "" || status == "available-next"
}

// tenantOf returns the tenant database as RDS describes it.
func (f *fakeAPI) tenantOf(key string, t map[string]any, status string) types.TenantDatabase {
	out := types.TenantDatabase{
		DBInstanceIdentifier:     aws.String(awsapi.String(t, "DBInstanceIdentifier")),
		TenantDBName:             aws.String(awsapi.String(t, "TenantDBName")),
		MasterUsername:           aws.String(awsapi.String(t, "MasterUsername")),
		CharacterSetName:         aws.String(awsapi.String(t, "CharacterSetName")),
		NcharCharacterSetName:    aws.String(awsapi.String(t, "NcharCharacterSetName")),
		TenantDatabaseARN:        aws.String(awsapi.String(t, "TenantDatabaseARN")),
		TenantDatabaseResourceId: aws.String(awsapi.String(t, "TenantDatabaseResourceId")),
		DbiResourceId:            aws.String(awsapi.String(t, "DbiResourceId")),
		Status:                   aws.String(status),
		TagList:                  awsapi.SDKTags[types.Tag](awsapi.Tags(t)),
	}
	if secret, ok := t["MasterUserSecret"].(map[string]any); ok {
		out.MasterUserSecret = &types.MasterUserSecret{
			SecretArn:    aws.String(awsapi.String(secret, "SecretArn")),
			SecretStatus: aws.String(awsapi.String(secret, "SecretStatus")),
			KmsKeyId:     aws.String(awsapi.String(secret, "KmsKeyId")),
		}
	}
	if f.pendingPassword[key] {
		out.PendingModifiedValues = &types.TenantDatabasePendingModifiedValues{MasterUserPassword: aws.String("****")}
	}
	return out
}

// secret returns the secret RDS makes for a tenant database's master user password, encrypted with key, or the
// aws/secretsmanager key when key is "".
func secret(key string, kmsKey string) map[string]any {
	if kmsKey == "" {
		kmsKey = "arn:aws:kms:us-west-2:123456789012:alias/aws/secretsmanager"
	}
	return map[string]any{
		"SecretArn":    "arn:aws:secretsmanager:us-west-2:123456789012:secret:rds!tdb-" + resourceID(key),
		"SecretStatus": "active",
		"KmsKeyId":     kmsKey,
	}
}

func (f *fakeAPI) CreateTenantDatabase(ctx context.Context, in *rds.CreateTenantDatabaseInput, _ ...func(*rds.Options)) (*rds.CreateTenantDatabaseOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["CreateTenantDatabase"]; err != nil {
		return nil, err
	}
	var r rules
	r.required(in.DBInstanceIdentifier, in.TenantDBName, in.MasterUsername)
	if r.err != nil {
		return nil, r.err
	}
	r.check(usernamePattern.MatchString(aws.ToString(in.MasterUsername)), "InvalidParameterValue")
	name := aws.ToString(in.TenantDBName)
	r.check(len(name) <= 8 && !strings.EqualFold(name, "null"), "InvalidParameterValue")
	r.password(in.MasterUserPassword)
	r.tags(in.Tags)
	r.check(!(aws.ToBool(in.ManageMasterUserPassword) && in.MasterUserPassword != nil), "InvalidParameterCombination")
	r.check(in.MasterUserSecretKmsKeyId == nil || aws.ToBool(in.ManageMasterUserPassword), "InvalidParameterCombination")
	if r.err != nil {
		return nil, r.err
	}
	instance, key, existing, err := f.tenant(in.DBInstanceIdentifier, in.TenantDBName)
	if instance == nil {
		return nil, err
	}
	if existing != nil {
		return nil, fault("TenantDatabaseAlreadyExists")
	}
	if awsapi.String(instance, "Status") != "available" {
		return nil, fault("InvalidDBInstanceState")
	}
	instanceID := awsapi.String(instance, "DBInstanceIdentifier")
	t := map[string]any{
		"DBInstanceIdentifier":     instanceID,
		"TenantDBName":             name,
		"MasterUsername":           aws.ToString(in.MasterUsername),
		"CharacterSetName":         "AL32UTF8",
		"NcharCharacterSetName":    "AL16UTF16",
		"TenantDatabaseARN":        arnPrefix + "db:" + instanceID + ":" + strings.ToLower(name),
		"TenantDatabaseResourceId": "tdb-" + resourceID(key),
		"DbiResourceId":            awsapi.String(instance, "DbiResourceId"),
	}
	if in.CharacterSetName != nil {
		t["CharacterSetName"] = *in.CharacterSetName
	}
	if in.NcharCharacterSetName != nil {
		t["NcharCharacterSetName"] = *in.NcharCharacterSetName
	}
	if in.MasterUserPassword != nil {
		t["MasterUserPassword"] = *in.MasterUserPassword
	}
	if aws.ToBool(in.ManageMasterUserPassword) {
		t["MasterUserSecret"] = secret(key, aws.ToString(in.MasterUserSecretKmsKeyId))
	}
	if len(in.Tags) > 0 {
		t["Tags"] = awsapi.TagListOf(in.Tags)
	}
	err = f.change(func() error {
		f.store.Put(tenantDatabaseType, key, t)
		f.status[key] = "creating"
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := f.tenantOf(key, t, "creating")
	return &rds.CreateTenantDatabaseOutput{TenantDatabase: &out}, nil
}

func (f *fakeAPI) DescribeTenantDatabases(ctx context.Context, in *rds.DescribeTenantDatabasesInput, _ ...func(*rds.Options)) (*rds.DescribeTenantDatabasesOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["DescribeTenantDatabases"]; err != nil {
		return nil, err
	}
	if in.DBInstanceIdentifier != nil {
		if _, ok := f.instance(*in.DBInstanceIdentifier); !ok {
			return nil, fault("DBInstanceNotFound")
		}
	}
	keys := []string{}
	for _, key := range f.store.List(tenantDatabaseType) {
		t := f.store.Resource(tenantDatabaseType, key)
		if in.DBInstanceIdentifier != nil && !strings.EqualFold(awsapi.String(t, "DBInstanceIdentifier"), *in.DBInstanceIdentifier) {
			continue
		}
		if in.TenantDBName != nil && !strings.EqualFold(awsapi.String(t, "TenantDBName"), *in.TenantDBName) {
			continue
		}
		keys = append(keys, key)
	}
	start, _ := strconv.Atoi(aws.ToString(in.Marker))
	size := tenantPageSize
	if in.MaxRecords != nil {
		size = int(*in.MaxRecords)
	}
	out := &rds.DescribeTenantDatabasesOutput{}
	end := min(start+size, len(keys))
	for _, key := range keys[min(start, len(keys)):end] {
		t := f.store.Resource(tenantDatabaseType, key)
		status, ok := f.tenantStatus(key)
		if !ok {
			continue
		}
		out.TenantDatabases = append(out.TenantDatabases, f.tenantOf(key, t, status))
		delete(f.pendingPassword, key)
	}
	if end < len(keys) {
		out.Marker = aws.String(strconv.Itoa(end))
	}
	return out, nil
}

func (f *fakeAPI) ModifyTenantDatabase(ctx context.Context, in *rds.ModifyTenantDatabaseInput, _ ...func(*rds.Options)) (*rds.ModifyTenantDatabaseOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["ModifyTenantDatabase"]; err != nil {
		return nil, err
	}
	var r rules
	r.required(in.DBInstanceIdentifier, in.TenantDBName)
	r.password(in.MasterUserPassword)
	r.check(!(aws.ToBool(in.ManageMasterUserPassword) && in.MasterUserPassword != nil), "InvalidParameterCombination")
	if r.err != nil {
		return nil, r.err
	}
	instance, key, t, err := f.tenant(in.DBInstanceIdentifier, in.TenantDBName)
	if err != nil {
		return nil, err
	}
	if awsapi.String(instance, "Status") != "available" || !f.tenantAvailable(key) {
		return nil, fault("InvalidDBInstanceState")
	}
	_, managed := t["MasterUserSecret"].(map[string]any)
	turningOn := aws.ToBool(in.ManageMasterUserPassword) && !managed
	turningOff := in.ManageMasterUserPassword != nil && !*in.ManageMasterUserPassword && managed
	r.check(!turningOff || in.MasterUserPassword != nil, "MissingParameter")
	r.check(in.MasterUserSecretKmsKeyId == nil || turningOn, "InvalidParameterCombination")
	if r.err != nil {
		return nil, r.err
	}
	changed := maps.Clone(t)
	if in.MasterUserPassword != nil {
		changed["MasterUserPassword"] = *in.MasterUserPassword
	}
	switch {
	case turningOn:
		changed["MasterUserSecret"] = secret(key, aws.ToString(in.MasterUserSecretKmsKeyId))
		delete(changed, "MasterUserPassword")
	case turningOff:
		delete(changed, "MasterUserSecret")
	}
	err = f.change(func() error {
		f.store.Put(tenantDatabaseType, key, changed)
		f.status[key] = "modifying"
		if in.MasterUserPassword != nil {
			f.pendingPassword[key] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := f.tenantOf(key, changed, "modifying")
	return &rds.ModifyTenantDatabaseOutput{TenantDatabase: &out}, nil
}

// validSnapshotID reports whether id is 1 to 255 letters, numbers, or hyphens, starts with a letter, doesn't end with
// a hyphen, and has no two hyphens in a row.
func validSnapshotID(id string) bool {
	return snapshotIDPattern.MatchString(id) && !strings.HasSuffix(id, "-") && !strings.Contains(id, "--")
}

func (f *fakeAPI) DeleteTenantDatabase(ctx context.Context, in *rds.DeleteTenantDatabaseInput, _ ...func(*rds.Options)) (*rds.DeleteTenantDatabaseOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["DeleteTenantDatabase"]; err != nil {
		return nil, err
	}
	var r rules
	r.required(in.DBInstanceIdentifier, in.TenantDBName)
	skip := aws.ToBool(in.SkipFinalSnapshot)
	r.check(!(skip && in.FinalDBSnapshotIdentifier != nil), "InvalidParameterCombination")
	if !skip {
		r.required(in.FinalDBSnapshotIdentifier)
		r.check(in.FinalDBSnapshotIdentifier == nil || validSnapshotID(*in.FinalDBSnapshotIdentifier), "InvalidParameterValue")
	}
	if r.err != nil {
		return nil, r.err
	}
	instance, key, t, err := f.tenant(in.DBInstanceIdentifier, in.TenantDBName)
	if err != nil {
		return nil, err
	}
	snapshot := aws.ToString(in.FinalDBSnapshotIdentifier)
	if !skip && slices.ContainsFunc(f.snapshots, func(s string) bool { return strings.EqualFold(s, snapshot) }) {
		return nil, fault("DBSnapshotAlreadyExists")
	}
	status := f.status[key]
	available := awsapi.String(instance, "Status") == "available" && f.tenantAvailable(key)
	switch {
	case f.tenantCount(instance) <= 1,
		status == "deleting" || status == "deleted-next",
		!skip && !available:
		return nil, fault("InvalidDBInstanceState")
	}
	err = f.change(func() error {
		if !skip {
			f.snapshots = append(f.snapshots, snapshot)
		}
		f.status[key] = "deleting"
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := f.tenantOf(key, t, "deleting")
	return &rds.DeleteTenantDatabaseOutput{TenantDatabase: &out}, nil
}

// tenantByARN returns the key of the tenant database with the ARN, or TenantDatabaseNotFound.
func (f *fakeAPI) tenantByARN(arn *string) (string, error) {
	for _, key := range f.store.List(tenantDatabaseType) {
		if strings.EqualFold(awsapi.String(f.store.Resource(tenantDatabaseType, key), "TenantDatabaseARN"), aws.ToString(arn)) {
			return key, nil
		}
	}
	return "", fault("TenantDatabaseNotFound")
}

func (f *fakeAPI) AddTagsToResource(ctx context.Context, in *rds.AddTagsToResourceInput, _ ...func(*rds.Options)) (*rds.AddTagsToResourceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["AddTagsToResource"]; err != nil {
		return nil, err
	}
	var r rules
	r.required(in.ResourceName)
	r.check(len(in.Tags) > 0, "MissingParameter")
	r.tags(in.Tags)
	if r.err != nil {
		return nil, r.err
	}
	key, err := f.tenantByARN(in.ResourceName)
	if err != nil {
		return nil, err
	}
	return &rds.AddTagsToResourceOutput{}, f.change(func() error {
		t := f.store.Resource(tenantDatabaseType, key)
		tags := map[string]string{}
		for _, tag := range awsapi.Tags(t) {
			tags[tag.Key] = tag.Value
		}
		for _, tag := range in.Tags {
			tags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
		}
		t["Tags"] = awsapi.TagList(tags)
		f.store.Put(tenantDatabaseType, key, t)
		return nil
	})
}

func (f *fakeAPI) RemoveTagsFromResource(ctx context.Context, in *rds.RemoveTagsFromResourceInput, _ ...func(*rds.Options)) (*rds.RemoveTagsFromResourceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["RemoveTagsFromResource"]; err != nil {
		return nil, err
	}
	var r rules
	r.required(in.ResourceName)
	r.check(len(in.TagKeys) > 0, "MissingParameter")
	if r.err != nil {
		return nil, r.err
	}
	key, err := f.tenantByARN(in.ResourceName)
	if err != nil {
		return nil, err
	}
	return &rds.RemoveTagsFromResourceOutput{}, f.change(func() error {
		t := f.store.Resource(tenantDatabaseType, key)
		tags := map[string]string{}
		for _, tag := range awsapi.Tags(t) {
			if !slices.Contains(in.TagKeys, tag.Key) {
				tags[tag.Key] = tag.Value
			}
		}
		if list := awsapi.TagList(tags); list != nil {
			t["Tags"] = list
		} else {
			delete(t, "Tags")
		}
		f.store.Put(tenantDatabaseType, key, t)
		return nil
	})
}
