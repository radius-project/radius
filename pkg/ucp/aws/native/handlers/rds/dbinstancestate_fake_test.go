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
	"fmt"
	"maps"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

// recoverableStatus is the status of a DB instance whose KMS key RDS can't reach, which a start can recover.
const recoverableStatus = "inaccessible-encryption-credentials-recoverable"

// fakeDBInstances is an in-memory RDS API for the running state of DB instances. It keeps each DB instance in the
// store under dbInstanceStateType, by its identifier in lowercase, with DBInstanceIdentifier, DBInstanceStatus, and
// State, as the suite sees it: running for available, and the status otherwise. It also keeps what RDS's rules need:
// Engine, postgres unless set, DBClusterIdentifier for a DB instance of a cluster, ReadReplica for a DB instance
// that has a read replica or is one, and MultiAZ for a DB instance in a Multi-AZ deployment. A starting or stopping DB instance, such as one a start or stop just changed,
// reads as such once, and then as available or stopped.
type fakeDBInstances struct {
	store *awsfake.CloudControl

	mu sync.Mutex

	// failStart maps a DB instance to the status a start ends in instead of available, such as
	// incompatible-parameters.
	failStart map[string]string

	// errs maps a method name to an error the call returns instead of acting.
	errs  map[string]error
	calls []string

	// incomplete makes the next DescribeDBInstances leave out DBInstanceStatus, which the API reference marks optional.
	incomplete bool
}

var _ DBInstanceAPI = (*fakeDBInstances)(nil)

func newFakeDBInstances(store *awsfake.CloudControl) *fakeDBInstances {
	return &fakeDBInstances{store: store, failStart: map[string]string{}, errs: map[string]error{}}
}

// putDBInstance puts a DB instance in status in the store, with the extra fields set.
func putDBInstance(store *awsfake.CloudControl, id string, status string, extra map[string]any) {
	props := map[string]any{"DBInstanceIdentifier": id, "Engine": "postgres"}
	setStatus(props, status)
	maps.Copy(props, extra)
	store.Put(dbInstanceStateType, strings.ToLower(id), props)
}

// setStatus sets the status of a stored DB instance, and the State the suite sees.
func setStatus(props map[string]any, status string) {
	props["DBInstanceStatus"], props["State"] = status, status
	if status == "available" {
		props["State"] = "running"
	}
}

func dbFault(code string) error {
	return &smithy.GenericAPIError{Code: code, Message: code, Fault: smithy.FaultClient}
}

// call records a call, and returns the error errs sets for it. A call without a region fails, as with AWS.
func (f *fakeDBInstances) call(name string, opts []func(*rds.Options)) error {
	f.calls = append(f.calls, name)
	options := rds.Options{}
	for _, fn := range opts {
		fn(&options)
	}
	if options.Region == "" {
		return fmt.Errorf("the call %s has no region", name)
	}
	return f.errs[name]
}

// dbInstance returns the stored DB instance with the identifier, ignoring case, or the error RDS returns for a missing
// identifier or one it has no DB instance for.
func (f *fakeDBInstances) dbInstance(id *string) (map[string]any, error) {
	if aws.ToString(id) == "" {
		return nil, dbFault("MissingParameter")
	}
	props := f.store.Resource(dbInstanceStateType, strings.ToLower(*id))
	if props == nil {
		return nil, dbFault("DBInstanceNotFound")
	}
	return props, nil
}

// change sets the status of the stored DB instance, and returns the DB instance as RDS's response has it.
func (f *fakeDBInstances) change(props map[string]any, status string) *types.DBInstance {
	setStatus(props, status)
	f.store.Put(dbInstanceStateType, strings.ToLower(awsapi.String(props, "DBInstanceIdentifier")), props)
	return dbInstanceOf(props)
}

func dbInstanceOf(props map[string]any) *types.DBInstance {
	instance := &types.DBInstance{
		DBInstanceIdentifier: aws.String(awsapi.String(props, "DBInstanceIdentifier")),
		DBInstanceStatus:     aws.String(awsapi.String(props, "DBInstanceStatus")),
		Engine:               aws.String(awsapi.String(props, "Engine")),
	}
	if cluster := awsapi.String(props, "DBClusterIdentifier"); cluster != "" {
		instance.DBClusterIdentifier = aws.String(cluster)
	}
	return instance
}

// settle moves a starting DB instance to available, or to the status in failStart, and a stopping one to stopped,
// after a read.
func (f *fakeDBInstances) settle(props map[string]any) {
	id := strings.ToLower(awsapi.String(props, "DBInstanceIdentifier"))
	switch awsapi.String(props, "DBInstanceStatus") {
	case statusStarting:
		status := "available"
		if failed := f.failStart[id]; failed != "" {
			status = failed
		}
		setStatus(props, status)
	case statusStopping:
		setStatus(props, "stopped")
	default:
		return
	}
	f.store.Put(dbInstanceStateType, id, props)
}

func (f *fakeDBInstances) DescribeDBInstances(ctx context.Context, in *rds.DescribeDBInstancesInput, opts ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DescribeDBInstances", opts); err != nil {
		return nil, err
	}
	ids := f.store.List(dbInstanceStateType)
	if in.DBInstanceIdentifier != nil {
		if _, err := f.dbInstance(in.DBInstanceIdentifier); err != nil {
			return nil, err
		}
		ids = []string{strings.ToLower(*in.DBInstanceIdentifier)}
	}
	out := &rds.DescribeDBInstancesOutput{}
	for _, id := range ids {
		props := f.store.Resource(dbInstanceStateType, id)
		instance := dbInstanceOf(props)
		if f.incomplete {
			instance.DBInstanceStatus = nil
		}
		out.DBInstances = append(out.DBInstances, *instance)
		f.settle(props)
	}
	f.incomplete = false
	return out, nil
}

func (f *fakeDBInstances) StartDBInstance(ctx context.Context, in *rds.StartDBInstanceInput, opts ...func(*rds.Options)) (*rds.StartDBInstanceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("StartDBInstance", opts); err != nil {
		return nil, err
	}
	props, err := f.dbInstance(in.DBInstanceIdentifier)
	if err != nil {
		return nil, err
	}
	status := awsapi.String(props, "DBInstanceStatus")
	if awsapi.String(props, "DBClusterIdentifier") != "" || (status != "stopped" && status != recoverableStatus) {
		return nil, dbFault("InvalidDBInstanceState")
	}
	return &rds.StartDBInstanceOutput{DBInstance: f.change(props, statusStarting)}, nil
}

func (f *fakeDBInstances) StopDBInstance(ctx context.Context, in *rds.StopDBInstanceInput, opts ...func(*rds.Options)) (*rds.StopDBInstanceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("StopDBInstance", opts); err != nil {
		return nil, err
	}
	props, err := f.dbInstance(in.DBInstanceIdentifier)
	if err != nil {
		return nil, err
	}
	sqlServerMultiAZ := strings.HasPrefix(awsapi.String(props, "Engine"), "sqlserver") && props["MultiAZ"] == true
	if awsapi.String(props, "DBClusterIdentifier") != "" || props["ReadReplica"] == true || sqlServerMultiAZ || awsapi.String(props, "DBInstanceStatus") != "available" {
		return nil, dbFault("InvalidDBInstanceState")
	}
	return &rds.StopDBInstanceOutput{DBInstance: f.change(props, statusStopping)}, nil
}
