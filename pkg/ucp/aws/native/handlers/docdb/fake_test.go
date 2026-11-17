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
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/docdb"
	"github.com/aws/aws-sdk-go-v2/service/docdb/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const arnPrefix = "arn:aws:rds:us-west-2:123456789012:"

// fakeAPI is an in-memory DocumentDB API. It keeps clusters and instances in an awsfake store, in their types'
// property format, so the suite's Cloud Control sees the same resources. A cluster or instance reports a creating,
// modifying, or deleting status for one read before it finishes.
type fakeAPI struct {
	store  *awsfake.CloudControl
	failer nativetest.Failer

	mu        sync.Mutex
	status    map[string]string
	snapshots []string
	calls     []string

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
	return &fakeAPI{store: store, status: map[string]string{}, errs: map[string]error{}}
}

func fault(code string) error {
	return &smithy.GenericAPIError{Code: code, Message: code, Fault: smithy.FaultClient}
}

func (f *fakeAPI) call(name string) error {
	f.calls = append(f.calls, name)
	return f.errs[name]
}

// state returns the status of the resource, and moves a transitional status on: a create or update finishes, and a
// delete removes the resource, which then returns false.
func (f *fakeAPI) state(typeName string, id string) (string, bool) {
	key := typeName + "/" + id
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

func (f *fakeAPI) put(typeName string, id string, input any, status string) error {
	if f.store.Resource(typeName, id) != nil {
		return fault("DBClusterAlreadyExistsFault")
	}
	props, err := awsapi.EncodeObject(input)
	if err != nil {
		return err
	}
	props["Engine"] = Engine
	if typeName == clusterType {
		props["DBClusterIdentifier"] = id
	} else {
		props["DBInstanceIdentifier"] = id
	}
	f.store.Put(typeName, id, props)
	f.status[typeName+"/"+id] = status
	return nil
}

func (f *fakeAPI) CreateDBCluster(ctx context.Context, in *docdb.CreateDBClusterInput, _ ...func(*docdb.Options)) (*docdb.CreateDBClusterOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("CreateDBCluster"); err != nil {
		return nil, err
	}
	return &docdb.CreateDBClusterOutput{}, f.put(clusterType, strings.ToLower(aws.ToString(in.DBClusterIdentifier)), in, "creating")
}

func (f *fakeAPI) RestoreDBClusterFromSnapshot(ctx context.Context, in *docdb.RestoreDBClusterFromSnapshotInput, _ ...func(*docdb.Options)) (*docdb.RestoreDBClusterFromSnapshotOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("RestoreDBClusterFromSnapshot"); err != nil {
		return nil, err
	}
	return &docdb.RestoreDBClusterFromSnapshotOutput{}, f.put(clusterType, strings.ToLower(aws.ToString(in.DBClusterIdentifier)), in, "creating")
}

func (f *fakeAPI) RestoreDBClusterToPointInTime(ctx context.Context, in *docdb.RestoreDBClusterToPointInTimeInput, _ ...func(*docdb.Options)) (*docdb.RestoreDBClusterToPointInTimeOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("RestoreDBClusterToPointInTime"); err != nil {
		return nil, err
	}
	return &docdb.RestoreDBClusterToPointInTimeOutput{}, f.put(clusterType, strings.ToLower(aws.ToString(in.DBClusterIdentifier)), in, "creating")
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
	c.Endpoint = aws.String(id + ".cluster-abc.us-west-2.docdb.amazonaws.com")
	return c, true, nil
}

func stringProp(props map[string]any, name string) *string {
	if s := awsapi.String(props, name); s != "" {
		return aws.String(s)
	}
	return nil
}

func (f *fakeAPI) DescribeDBClusters(ctx context.Context, in *docdb.DescribeDBClustersInput, _ ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DescribeDBClusters"); err != nil {
		return nil, err
	}
	ids := f.store.List(clusterType)
	if in.DBClusterIdentifier != nil {
		ids = []string{strings.ToLower(aws.ToString(in.DBClusterIdentifier))}
	}
	out := &docdb.DescribeDBClustersOutput{}
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
		return fault("DBClusterNotFoundFault")
	}
	apply(props)
	f.store.Put(typeName, id, props)
	return f.failer.After()
}

func (f *fakeAPI) ModifyDBCluster(ctx context.Context, in *docdb.ModifyDBClusterInput, _ ...func(*docdb.Options)) (*docdb.ModifyDBClusterOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("ModifyDBCluster"); err != nil {
		return nil, err
	}
	id := aws.ToString(in.DBClusterIdentifier)
	values, err := awsapi.EncodeObject(in)
	if err != nil {
		return nil, err
	}
	err = f.change(clusterType, id, func(props map[string]any) {
		for _, name := range []string{"DBClusterIdentifier", "ApplyImmediately", "AllowMajorVersionUpgrade", "CloudwatchLogsExportConfiguration"} {
			delete(values, name)
		}
		for k, v := range values {
			props[k] = v
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
		f.status[clusterType+"/"+id] = "modifying"
	})
	return &docdb.ModifyDBClusterOutput{}, err
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
	id := aws.ToString(in.DBClusterIdentifier)
	if f.store.Resource(clusterType, id) == nil {
		return nil, fault("DBClusterNotFoundFault")
	}
	if strings.HasPrefix(f.status[clusterType+"/"+id], "delet") {
		return nil, fault("InvalidDBClusterStateFault")
	}
	snapshot := aws.ToString(in.FinalDBSnapshotIdentifier)
	if !validSnapshotName(snapshot) {
		return nil, fault("InvalidParameterValue")
	}
	if slices.Contains(f.snapshots, snapshot) {
		return nil, fault("DBClusterSnapshotAlreadyExistsFault")
	}
	f.snapshots = append(f.snapshots, snapshot)
	f.status[clusterType+"/"+id] = "deleting"
	return &docdb.DeleteDBClusterOutput{}, nil
}

// snapshotPattern is DocumentDB's rule for a cluster snapshot's name: 1 to 63 letters, numbers, or hyphens, starting
// with a letter, with no two hyphens in a row and no hyphen at the end. The DeleteDBCluster reference allows 255
// characters, but the CreateDBClusterSnapshot reference and the guide to deleting a cluster say 63, so the fake
// enforces 63.
var snapshotPattern = regexp.MustCompile(`^[A-Za-z](-?[A-Za-z0-9])*$`)

func validSnapshotName(name string) bool {
	return len(name) <= 63 && snapshotPattern.MatchString(name)
}

func (f *fakeAPI) CreateDBInstance(ctx context.Context, in *docdb.CreateDBInstanceInput, _ ...func(*docdb.Options)) (*docdb.CreateDBInstanceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("CreateDBInstance"); err != nil {
		return nil, err
	}
	return &docdb.CreateDBInstanceOutput{}, f.put(instanceType, strings.ToLower(aws.ToString(in.DBInstanceIdentifier)), in, "creating")
}

func (f *fakeAPI) DescribeDBInstances(ctx context.Context, in *docdb.DescribeDBInstancesInput, _ ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DescribeDBInstances"); err != nil {
		return nil, err
	}
	ids := f.store.List(instanceType)
	if in.DBInstanceIdentifier != nil {
		ids = []string{strings.ToLower(aws.ToString(in.DBInstanceIdentifier))}
	}
	out := &docdb.DescribeDBInstancesOutput{}
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
	id := aws.ToString(in.DBInstanceIdentifier)
	values, err := awsapi.EncodeObject(in)
	if err != nil {
		return nil, err
	}
	err = f.change(instanceType, id, func(props map[string]any) {
		for _, name := range []string{"DBInstanceIdentifier", "ApplyImmediately"} {
			delete(values, name)
		}
		for k, v := range values {
			props[k] = v
		}
		f.status[instanceType+"/"+id] = "modifying"
	})
	return &docdb.ModifyDBInstanceOutput{}, err
}

func (f *fakeAPI) DeleteDBInstance(ctx context.Context, in *docdb.DeleteDBInstanceInput, _ ...func(*docdb.Options)) (*docdb.DeleteDBInstanceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DeleteDBInstance"); err != nil {
		return nil, err
	}
	id := aws.ToString(in.DBInstanceIdentifier)
	if f.store.Resource(instanceType, id) == nil {
		return nil, fault("DBInstanceNotFound")
	}
	if strings.HasPrefix(f.status[instanceType+"/"+id], "delet") {
		return nil, fault("InvalidDBInstanceState")
	}
	f.status[instanceType+"/"+id] = "deleting"
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
	out := &docdb.ListTagsForResourceOutput{}
	for _, t := range awsapi.Tags(f.store.Resource(typeName, id)) {
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
