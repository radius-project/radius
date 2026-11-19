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

// Package rds has the native handlers for Amazon RDS resources that have no CloudFormation type, such as the running
// state of a DB instance. Their schemas are hand-written, in pkg/aws/schemas/handwritten.
package rds

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"

	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const dbInstanceStateType = "AWS::RDS::DBInstanceState"

// DBInstanceAPI is the part of the RDS client the DB instance state handler uses.
type DBInstanceAPI interface {
	DescribeDBInstances(ctx context.Context, params *rds.DescribeDBInstancesInput, optFns ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error)
	StartDBInstance(ctx context.Context, params *rds.StartDBInstanceInput, optFns ...func(*rds.Options)) (*rds.StartDBInstanceOutput, error)
	StopDBInstance(ctx context.Context, params *rds.StopDBInstanceInput, optFns ...func(*rds.Options)) (*rds.StopDBInstanceOutput, error)
}

var dbInstanceRegion = awsapi.InRegion[rds.Options]

var (
	_ native.Handler            = (*DBInstanceState)(nil)
	_ native.IdentifierEncoder  = (*DBInstanceState)(nil)
	_ native.IdentifierResolver = (*DBInstanceState)(nil)
	_ native.UpdatePreparer     = (*DBInstanceState)(nil)
)

// The DB instance statuses the handler acts on, besides stopped, which is the State a read gives it too. See "Viewing
// instance status" in the RDS User Guide.
const (
	statusStarting = "starting"
	statusStopping = "stopping"
	statusCreating = "creating"
	statusDeleting = "deleting"
)

// runningStatuses are the statuses of a DB instance that runs, which a read returns as running.
var runningStatuses = []string{
	"available", "backing-up", "configuring-enhanced-monitoring", "configuring-iam-database-auth",
	"configuring-log-exports", "converting-to-vpc", "maintenance", "modifying", "moving-to-vpc", "rebooting", "renaming",
	"resetting-master-credentials", "storage-config-upgrade", "storage-full", "storage-initialization",
	"storage-optimization", "upgrading",
}

// otherProducts are the engines of DB instances the RDS API also returns, which belong to Amazon DocumentDB and
// Amazon Neptune.
var otherProducts = []string{"docdb", "neptune"}

// DBInstanceState serves AWS::RDS::DBInstanceState, which keeps an RDS DB instance Radius doesn't create running or
// stopped. Its identifier is DBInstanceIdentifier, in lowercase as RDS stores it. The DB instance always exists before
// the deploy, so UCP takes it over and updates it, and calls Create only when a read finds no DB instance. Its metadata
// makes a delete change nothing in AWS.
//
// A deploy starts or stops the DB instance only when its state differs from the template's, and then waits for the
// template's state. A DB instance being deleted reads as gone. A DB instance of a cluster is rejected with 400 before
// any change: RDS starts and stops an Aurora DB instance only with its cluster, and can't stop a Multi-AZ DB cluster.
// RDS starts a stopped DB instance on its own after 7 days, so the next deploy finds it running and stops it again.
type DBInstanceState struct {
	api DBInstanceAPI
}

// NewDBInstanceState returns the handler for the running state of RDS DB instances.
func NewDBInstanceState(api DBInstanceAPI) *DBInstanceState {
	return &DBInstanceState{api: api}
}

// TypeName returns AWS::RDS::DBInstanceState.
func (h *DBInstanceState) TypeName() string {
	return dbInstanceStateType
}

// Options returns how UCP uses the handler. The template gives the identifier, and the type can't be listed.
func (h *DBInstanceState) Options() native.Options {
	return native.Options{List: native.ListNone, Recovery: native.RecoverByIdentifier}
}

// Identifier returns DBInstanceIdentifier in lowercase, as RDS stores it. UCP calls it before any AWS call, so a
// template whose DBInstanceIdentifier isn't a string, or whose State is neither running nor stopped, is rejected here.
// State may be left out, as for `existing`.
func (h *DBInstanceState) Identifier(properties map[string]any) (string, error) {
	if value, ok := properties["DBInstanceIdentifier"]; ok {
		if _, ok := value.(string); !ok {
			return "", &native.Failure{Message: "DBInstanceIdentifier of an " + dbInstanceStateType + " must be a string", ClientError: true, Outcome: native.OutcomeRejected}
		}
	}
	if _, ok := properties["State"]; ok {
		if _, err := awsapi.RunningState(properties, dbInstanceStateType); err != nil {
			return "", err
		}
	}
	return strings.ToLower(awsapi.String(properties, "DBInstanceIdentifier")), nil
}

// ResolveIdentifier returns the identifier in lowercase, so a name in another case names the same DB instance.
func (h *DBInstanceState) ResolveIdentifier(ctx context.Context, region string, identifier string) (string, error) {
	return strings.ToLower(identifier), nil
}

// describe returns the DB instance with the identifier. It returns ErrNotFound when RDS has none or is deleting it,
// an OtherProductError for a DocumentDB or Neptune DB instance, and a rejected client failure for a DB instance of a
// cluster.
func (h *DBInstanceState) describe(ctx context.Context, region string, identifier string) (*types.DBInstance, error) {
	out, err := h.api.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{DBInstanceIdentifier: aws.String(identifier)}, dbInstanceRegion(region))
	if err != nil {
		return nil, awsapi.NotFound(err, "DBInstanceNotFound")
	}
	i := slices.IndexFunc(out.DBInstances, func(d types.DBInstance) bool {
		return strings.EqualFold(aws.ToString(d.DBInstanceIdentifier), identifier)
	})
	if i < 0 || aws.ToString(out.DBInstances[i].DBInstanceStatus) == statusDeleting {
		return nil, native.ErrNotFound
	}
	instance := &out.DBInstances[i]
	if engine := aws.ToString(instance.Engine); slices.Contains(otherProducts, engine) {
		return nil, &native.OtherProductError{Identifier: identifier, Product: engine}
	}
	if cluster := aws.ToString(instance.DBClusterIdentifier); cluster != "" {
		message := fmt.Sprintf("The DB instance %q belongs to the Multi-AZ DB cluster %q, which RDS can't stop or start.", identifier, cluster)
		if strings.HasPrefix(aws.ToString(instance.Engine), "aurora") {
			message = fmt.Sprintf("The DB instance %q belongs to the Aurora DB cluster %q, and RDS starts and stops it only with the whole cluster. Start or stop the cluster instead.", identifier, cluster)
		}
		return nil, &native.Failure{Message: message, ClientError: true, Outcome: native.OutcomeRejected}
	}
	return instance, nil
}

// stateOf returns the State a read gives a DB instance in status: running for a status in which it runs, and status
// itself otherwise, such as stopped or starting.
func stateOf(status string) string {
	if slices.Contains(runningStatuses, status) {
		return awsapi.StateRunning
	}
	return status
}

// dbInstanceTarget returns the state a DB instance in status is in or on its way to: running for a status in which it
// runs, or starting or creating, stopped for stopped or stopping, and status itself otherwise.
func dbInstanceTarget(status string) string {
	switch status {
	case statusStarting, statusCreating:
		return awsapi.StateRunning
	case statusStopping:
		return awsapi.StateStopped
	}
	return stateOf(status)
}

// updateTarget returns the state an update waits for, given the status the DB instance had before it. An update is
// sent only when the template's state differs, so it stops a DB instance that runs or is stopping, and starts any
// other, including one that is starting or being created, and one that only a start can recover, such as
// inaccessible-encryption-credentials-recoverable.
func updateTarget(before string) string {
	if stateOf(before) == awsapi.StateRunning || before == statusStopping {
		return awsapi.StateStopped
	}
	return awsapi.StateRunning
}

// Read returns the DB instance's identifier, its state, and the status RDS reports.
func (h *DBInstanceState) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	instance, err := h.describe(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	status := aws.ToString(instance.DBInstanceStatus)
	return map[string]any{
		"DBInstanceIdentifier": strings.ToLower(aws.ToString(instance.DBInstanceIdentifier)),
		"State":                stateOf(status),
		"DBInstanceStatus":     status,
	}, nil
}

// Create changes nothing and fails with 400. UCP calls it only when a read found no DB instance, and AWS has nothing
// a deploy can create: it says the DB instance doesn't exist, or, when RDS reports it now, to deploy again, so the
// next deploy takes it over and waits for the template's state as an update.
func (h *DBInstanceState) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	id, err := h.Identifier(request.Properties)
	if err != nil {
		return "", err
	}
	if _, err := awsapi.RunningState(request.Properties, dbInstanceStateType); err != nil {
		return "", err
	}
	if _, err := h.describe(ctx, request.Region, id); errors.Is(err, native.ErrNotFound) {
		return "", dbInstanceGone(id)
	} else if err != nil {
		return "", err
	}
	return "", &native.Failure{
		Message:     fmt.Sprintf("RDS reported no DB instance %q when the deploy read it, and reports it now. Deploy again.", id),
		ClientError: true,
		Outcome:     native.OutcomeRejected,
	}
}

// dbInstanceGone is the failure for a DB instance RDS doesn't have, or is deleting.
func dbInstanceGone(id string) error {
	return &native.Failure{
		Message:     fmt.Sprintf("AWS has no RDS DB instance %q that can run: it doesn't exist, or it is being deleted. Set DBInstanceIdentifier to an existing DB instance.", id),
		ClientError: true,
		Outcome:     native.OutcomeRejected,
	}
}

// Update starts or stops the DB instance, when the template's state differs from the one RDS reports now.
// UpdateProgress waits for the state that follows the status the DB instance had before the update (request.State),
// so an update whose template sets another fails with 400 and calls nothing: a starting DB instance can't be stopped,
// and a stopping one can't be started, until it gets there.
func (h *DBInstanceState) Update(ctx context.Context, request native.UpdateRequest) error {
	desired, err := awsapi.RunningState(request.Desired, dbInstanceStateType)
	if err != nil {
		return err
	}
	if before := request.State; before != "" && updateTarget(before) != desired {
		target := dbInstanceTarget(before)
		message := fmt.Sprintf("The DB instance %q is %s, so RDS can't make it %s now.", request.Identifier, before, desired)
		switch {
		case target == desired:
			message = fmt.Sprintf("The DB instance %q changed to %s while the deploy started. Deploy again.", request.Identifier, before)
		case target != before:
			message += fmt.Sprintf(" Deploy again once it is %s.", target)
		}
		return &native.Failure{Message: message, ClientError: true, Outcome: native.OutcomeRejected}
	}
	return h.apply(ctx, request.Region, request.Identifier, desired)
}

// apply starts or stops the DB instance when it isn't in, or on its way to, the desired state. A call RDS rejects,
// such as a stop of a DB instance that has a read replica, is a rejected client failure that names the DB instance
// and its status.
func (h *DBInstanceState) apply(ctx context.Context, region string, id string, desired string) error {
	instance, err := h.describe(ctx, region, id)
	if errors.Is(err, native.ErrNotFound) {
		return dbInstanceGone(id)
	} else if err != nil {
		return err
	}
	status := aws.ToString(instance.DBInstanceStatus)
	if dbInstanceTarget(status) == desired {
		return nil
	}
	verb := "start"
	if desired == awsapi.StateRunning {
		_, err = h.api.StartDBInstance(ctx, &rds.StartDBInstanceInput{DBInstanceIdentifier: instance.DBInstanceIdentifier}, dbInstanceRegion(region))
	} else {
		verb = "stop"
		_, err = h.api.StopDBInstance(ctx, &rds.StopDBInstanceInput{DBInstanceIdentifier: instance.DBInstanceIdentifier}, dbInstanceRegion(region))
	}
	if err == nil || !ucp_aws.IsClientFault(err) {
		return err
	}
	return &native.Failure{
		Message:     fmt.Sprintf("RDS can't %s the DB instance %q, which is %s: %s", verb, id, status, err),
		ClientError: true,
		Outcome:     native.OutcomeRejected,
	}
}

// Delete changes nothing. The type's metadata makes a delete leave the DB instance as it is, so UCP never calls it.
func (h *DBInstanceState) Delete(ctx context.Context, region string, identifier string) error {
	return nil
}

// List is not supported.
func (h *DBInstanceState) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, nil
}

// Progress reports the operation as finished once the DB instance runs or is stopped, and as failed for a status in
// which it does neither, such as incompatible-parameters. UCP waits for an update with UpdateProgress, and Create
// never starts an operation, so UpdateProgress calls it only for an update whose state it can't read.
func (h *DBInstanceState) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	instance, err := h.describe(ctx, region, identifier)
	if err != nil {
		return native.Progress{}, err
	}
	status := aws.ToString(instance.DBInstanceStatus)
	state := stateOf(status)
	if target := dbInstanceTarget(status); target != state {
		return native.Progress{State: status, Waiting: target}, nil
	}
	progress := native.Progress{Done: true, State: status}
	if state != awsapi.StateRunning && state != awsapi.StateStopped {
		progress.Failure = &native.Failure{Message: fmt.Sprintf("the DB instance %q is %s", identifier, status)}
	}
	return progress, nil
}

// UpdateState returns the status of the DB instance before the update. UCP keeps it on the record
// (native.UpdatePreparer), so the update's progress knows which state it waits for, and can tell an update that
// failed from a read that doesn't show the change yet.
func (h *DBInstanceState) UpdateState(ctx context.Context, region string, identifier string) (string, error) {
	instance, err := h.describe(ctx, region, identifier)
	if err != nil {
		return "", err
	}
	return aws.ToString(instance.DBInstanceStatus), nil
}

// UpdateProgress waits for the state the update moves the DB instance to (see updateTarget), given its status before
// the update in state. A read that still shows the state from before, even in another status, such as available for a
// DB instance that was backing up, doesn't show the change yet. Any other status that isn't the update's state means
// the update failed, such as a start that ended in incompatible-parameters.
func (h *DBInstanceState) UpdateProgress(ctx context.Context, region string, identifier string, state string) (native.Progress, error) {
	if state == "" {
		return h.Progress(ctx, region, identifier, native.OperationUpdate)
	}
	instance, err := h.describe(ctx, region, identifier)
	if err != nil {
		return native.Progress{}, err
	}
	target := updateTarget(state)
	status := aws.ToString(instance.DBInstanceStatus)
	switch {
	case stateOf(status) == target:
		return native.Progress{Done: true, State: status}, nil
	case dbInstanceTarget(status) != stateOf(status), stateOf(status) == stateOf(state):
		return native.Progress{State: status, Waiting: target}, nil
	}
	return native.Progress{Done: true, State: status, Failure: &native.Failure{Message: fmt.Sprintf("the DB instance %q is %s, not %s", identifier, status, target)}}, nil
}
