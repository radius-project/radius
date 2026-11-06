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

// Package docdb has the native handlers for Amazon DocumentDB clusters and instances, which Cloud Control can't
// create in any region. The DocumentDB API also serves Amazon RDS and Neptune, so every handler checks that a cluster
// or instance runs the docdb engine before it reads, changes, or deletes it.
package docdb

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/docdb"
	"github.com/aws/aws-sdk-go-v2/service/docdb/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	// Engine is the engine of every DocumentDB cluster and instance. Others belong to RDS or Neptune.
	Engine = "docdb"

	clusterType = "AWS::DocDB::DBCluster"
)

// API is the part of the DocumentDB client the handlers use.
type API interface {
	CreateDBCluster(ctx context.Context, params *docdb.CreateDBClusterInput, optFns ...func(*docdb.Options)) (*docdb.CreateDBClusterOutput, error)
	RestoreDBClusterFromSnapshot(ctx context.Context, params *docdb.RestoreDBClusterFromSnapshotInput, optFns ...func(*docdb.Options)) (*docdb.RestoreDBClusterFromSnapshotOutput, error)
	RestoreDBClusterToPointInTime(ctx context.Context, params *docdb.RestoreDBClusterToPointInTimeInput, optFns ...func(*docdb.Options)) (*docdb.RestoreDBClusterToPointInTimeOutput, error)
	DescribeDBClusters(ctx context.Context, params *docdb.DescribeDBClustersInput, optFns ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error)
	ModifyDBCluster(ctx context.Context, params *docdb.ModifyDBClusterInput, optFns ...func(*docdb.Options)) (*docdb.ModifyDBClusterOutput, error)
	DeleteDBCluster(ctx context.Context, params *docdb.DeleteDBClusterInput, optFns ...func(*docdb.Options)) (*docdb.DeleteDBClusterOutput, error)
	CreateDBInstance(ctx context.Context, params *docdb.CreateDBInstanceInput, optFns ...func(*docdb.Options)) (*docdb.CreateDBInstanceOutput, error)
	DescribeDBInstances(ctx context.Context, params *docdb.DescribeDBInstancesInput, optFns ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error)
	ModifyDBInstance(ctx context.Context, params *docdb.ModifyDBInstanceInput, optFns ...func(*docdb.Options)) (*docdb.ModifyDBInstanceOutput, error)
	DeleteDBInstance(ctx context.Context, params *docdb.DeleteDBInstanceInput, optFns ...func(*docdb.Options)) (*docdb.DeleteDBInstanceOutput, error)
	ListTagsForResource(ctx context.Context, params *docdb.ListTagsForResourceInput, optFns ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error)
	AddTagsToResource(ctx context.Context, params *docdb.AddTagsToResourceInput, optFns ...func(*docdb.Options)) (*docdb.AddTagsToResourceOutput, error)
	RemoveTagsFromResource(ctx context.Context, params *docdb.RemoveTagsFromResourceInput, optFns ...func(*docdb.Options)) (*docdb.RemoveTagsFromResourceOutput, error)
}

var inRegion = awsapi.InRegion[docdb.Options]

var (
	_ native.Handler           = (*Cluster)(nil)
	_ native.IdentifierEncoder = (*Cluster)(nil)
	_ native.DeletePreparer    = (*Cluster)(nil)
)

// clusterStates are the states a cluster's create and update wait for.
var clusterStates = awsapi.States{Done: []string{"available"}, Failed: []string{"failed", "inaccessible-encryption-credentials", "incompatible-network", "incompatible-parameters", "incompatible-restore"}}

// Cluster serves AWS::DocDB::DBCluster. Its identifier, Id, is DBClusterIdentifier, which UCP generates when the
// template leaves it out. Deleting a cluster takes a final snapshot named after the cluster and the time of the
// delete, which Radius never deletes.
type Cluster struct {
	api API

	// now returns the current time, for the name of the final snapshot.
	now func() time.Time
}

// NewCluster returns the handler for DocumentDB clusters.
func NewCluster(api API) *Cluster {
	return &Cluster{api: api, now: time.Now}
}

// TypeName returns AWS::DocDB::DBCluster.
func (h *Cluster) TypeName() string {
	return clusterType
}

// Options returns how UCP uses the handler. A restore from a large snapshot can take hours, so UCP waits
// native.DefaultTimeout.
func (h *Cluster) Options() native.Options {
	return native.Options{
		List:         native.ListRegion,
		Recovery:     native.RecoverByIdentifier,
		NameProperty: "DBClusterIdentifier",
		Equivalent:   equivalent,
	}
}

// Identifier returns DBClusterIdentifier in lowercase, as DocumentDB stores it.
func (h *Cluster) Identifier(properties map[string]any) (string, error) {
	return strings.ToLower(awsapi.String(properties, "DBClusterIdentifier")), nil
}

// lowercased are the properties DocumentDB stores in lowercase.
var lowercased = []string{"DBClusterIdentifier", "DBInstanceIdentifier", "DBSubnetGroupName", "DBClusterParameterGroupName"}

// equivalent treats an engine version AWS returns with more parts, such as 5.0.0 for 5.0, and names DocumentDB stores
// in lowercase as the template's values.
func equivalent(path string, template any, aws any) bool {
	t, okT := template.(string)
	a, okA := aws.(string)
	if okT && okA && path == "EngineVersion" {
		return sameVersion(t, a)
	}
	return awsapi.CaseInsensitive(lowercased...)(path, template, aws)
}

// sameVersion reports whether aws is template with zero parts added, such as 5.0.0 for 5.0.
func sameVersion(template string, aws string) bool {
	if !strings.HasPrefix(aws, template+".") {
		return false
	}
	for _, part := range strings.Split(strings.TrimPrefix(aws, template+"."), ".") {
		if strings.Trim(part, "0") != "" {
			return false
		}
	}
	return true
}

// describe returns the cluster with the identifier, ErrNotFound, or an OtherProductError when it doesn't run docdb.
func (h *Cluster) describe(ctx context.Context, region string, identifier string) (*types.DBCluster, error) {
	out, err := h.api.DescribeDBClusters(ctx, &docdb.DescribeDBClustersInput{DBClusterIdentifier: aws.String(identifier)}, inRegion(region))
	if err != nil {
		return nil, awsapi.NotFound(err, "DBClusterNotFoundFault")
	}
	if len(out.DBClusters) == 0 {
		return nil, native.ErrNotFound
	}
	cluster := out.DBClusters[0]
	if engine := aws.ToString(cluster.Engine); engine != Engine {
		return nil, &native.OtherProductError{Identifier: identifier, Product: engine}
	}
	return &cluster, nil
}

// Read returns the cluster's properties.
func (h *Cluster) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	cluster, err := h.describe(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	return h.properties(ctx, region, cluster)
}

func (h *Cluster) properties(ctx context.Context, region string, c *types.DBCluster) (map[string]any, error) {
	tags, err := h.api.ListTagsForResource(ctx, &docdb.ListTagsForResourceInput{ResourceName: c.DBClusterArn}, inRegion(region))
	if err != nil {
		return nil, err
	}
	groups := []string{}
	for _, g := range c.VpcSecurityGroups {
		groups = append(groups, aws.ToString(g.VpcSecurityGroupId))
	}
	p := map[string]any{
		"Id":                               c.DBClusterIdentifier,
		"DBClusterIdentifier":              c.DBClusterIdentifier,
		"BackupRetentionPeriod":            c.BackupRetentionPeriod,
		"ClusterResourceId":                c.DbClusterResourceId,
		"CopyTagsToSnapshot":               c.CopyTagsToSnapshot,
		"DBClusterParameterGroupName":      c.DBClusterParameterGroup,
		"DBSubnetGroupName":                c.DBSubnetGroup,
		"DeletionProtection":               c.DeletionProtection,
		"EnableCloudwatchLogsExports":      c.EnabledCloudwatchLogsExports,
		"Endpoint":                         c.Endpoint,
		"EngineVersion":                    c.EngineVersion,
		"MasterUsername":                   c.MasterUsername,
		"NetworkType":                      c.NetworkType,
		"Port":                             c.Port,
		"PreferredBackupWindow":            c.PreferredBackupWindow,
		"PreferredMaintenanceWindow":       c.PreferredMaintenanceWindow,
		"ReadEndpoint":                     c.ReaderEndpoint,
		"ServerlessV2PlatformVersion":      c.ServerlessV2PlatformVersion,
		"ServerlessV2ScalingConfiguration": c.ServerlessV2ScalingConfiguration,
		"StorageEncrypted":                 c.StorageEncrypted,
		"StorageType":                      c.StorageType,
		"VpcSecurityGroupIds":              groups,
		"Tags":                             awsapi.TagListOf(tags.TagList),
	}
	encoded, err := awsapi.EncodeObject(p)
	if err != nil {
		return nil, err
	}
	encoded["ManageMasterUserPassword"] = c.MasterUserSecret != nil
	return encoded, nil
}

// Create creates the cluster, or restores it from a snapshot or another cluster when the template sets
// SnapshotIdentifier or SourceDBClusterIdentifier.
func (h *Cluster) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	p := request.Properties
	identifier, _ := h.Identifier(p)
	opt := inRegion(request.Region)
	var err error
	switch {
	case awsapi.String(p, "SnapshotIdentifier") != "":
		input := &docdb.RestoreDBClusterFromSnapshotInput{}
		if err := restoreInput(p, "SnapshotIdentifier", input); err != nil {
			return "", err
		}
		input.Engine = aws.String(Engine)
		input.Tags = awsapi.SDKTags[types.Tag](awsapi.Tags(p))
		_, err = h.api.RestoreDBClusterFromSnapshot(ctx, input, opt)
	case awsapi.String(p, "SourceDBClusterIdentifier") != "":
		input := &docdb.RestoreDBClusterToPointInTimeInput{}
		if err := restoreInput(p, "SourceDBClusterIdentifier", input); err != nil {
			return "", err
		}
		input.Tags = awsapi.SDKTags[types.Tag](awsapi.Tags(p))
		_, err = h.api.RestoreDBClusterToPointInTime(ctx, input, opt)
	default:
		input := &docdb.CreateDBClusterInput{}
		if err := awsapi.Decode(awsapi.Without(p, "Tags"), input); err != nil {
			return "", err
		}
		input.Engine = aws.String(Engine)
		input.Tags = awsapi.SDKTags[types.Tag](awsapi.Tags(p))
		_, err = h.api.CreateDBCluster(ctx, input, opt)
	}
	if err != nil {
		return "", err
	}
	return identifier, nil
}

// restoreInput sets input, the input of a restore from the property source, from properties. It rejects a property
// the restore call can't take, such as MasterUserPassword, which a restored cluster keeps from its source: the template
// can set it in a later deploy.
func restoreInput(properties map[string]any, source string, input any) error {
	fields := map[string]bool{"tags": true}
	for _, f := range awsapi.FieldNames(input) {
		fields[strings.ToLower(f)] = true
	}
	unsupported := []string{}
	for name := range properties {
		if !fields[strings.ToLower(name)] {
			unsupported = append(unsupported, name)
		}
	}
	if len(unsupported) > 0 {
		slices.Sort(unsupported)
		return &native.Failure{
			Message:     fmt.Sprintf("A DocumentDB cluster restored with %s can't also set %s. Remove them to restore the cluster, then set them in a later deploy.", source, strings.Join(unsupported, ", ")),
			ClientError: true,
			Outcome:     native.OutcomeRejected,
		}
	}
	return awsapi.Decode(awsapi.Without(properties, "Tags"), input)
}

// modifiable are the properties ModifyDBCluster changes. Tags change through their own calls.
var modifiable = []string{
	"BackupRetentionPeriod", "CopyTagsToSnapshot", "DBClusterParameterGroupName", "DeletionProtection",
	"EnableCloudwatchLogsExports", "EngineVersion", "ManageMasterUserPassword", "MasterUserPassword",
	"MasterUserSecretKmsKeyId", "NetworkType", "Port", "PreferredBackupWindow", "PreferredMaintenanceWindow",
	"RotateMasterUserPassword", "ServerlessV2ScalingConfiguration", "StorageType", "VpcSecurityGroupIds",
}

// removedDefaults are the values a property AWS can't clear goes back to when the template removes it. AWS keeps the
// value of any other removed property.
var removedDefaults = map[string]any{
	"BackupRetentionPeriod":       1,
	"CopyTagsToSnapshot":          false,
	"DeletionProtection":          false,
	"EnableCloudwatchLogsExports": []any{},
}

// Update makes the patch's changes with ModifyDBCluster, applied at once, then adds and removes tags.
func (h *Cluster) Update(ctx context.Context, request native.UpdateRequest) error {
	cluster, err := h.describe(ctx, request.Region, request.Identifier)
	if err != nil {
		return err
	}
	opt := inRegion(request.Region)
	changed := awsapi.Changed(request)
	values := map[string]any{}
	for _, name := range changed {
		if !slices.Contains(modifiable, name) {
			continue
		}
		value, ok := request.Desired[name]
		if !ok {
			if value, ok = removedDefaults[name]; !ok {
				continue
			}
		}
		values[name] = value
	}
	calls := []func() error{}
	if len(values) > 0 {
		input := &docdb.ModifyDBClusterInput{}
		if err := awsapi.Decode(awsapi.Without(values, "EnableCloudwatchLogsExports"), input); err != nil {
			return err
		}
		input.DBClusterIdentifier = aws.String(request.Identifier)
		input.ApplyImmediately = aws.Bool(true)
		if _, ok := values["EngineVersion"]; ok {
			input.AllowMajorVersionUpgrade = aws.Bool(true)
		}
		if logs, ok := values["EnableCloudwatchLogsExports"]; ok {
			input.CloudwatchLogsExportConfiguration = logChanges(cluster.EnabledCloudwatchLogsExports, logs)
		}
		calls = append(calls, func() error {
			_, err := h.api.ModifyDBCluster(ctx, input, opt)
			return err
		})
	}
	if slices.Contains(changed, "Tags") {
		calls = append(calls, tagCalls(ctx, h.api, cluster.DBClusterArn, request, opt)...)
	}
	return awsapi.Calls(calls...)
}

// tagCalls returns the calls that add and remove tags to go from the request's current tags to the desired ones.
func tagCalls(ctx context.Context, api API, arn *string, request native.UpdateRequest, opt func(*docdb.Options)) []func() error {
	set, remove := awsapi.TagChanges(awsapi.Tags(request.Current), awsapi.Tags(request.Desired))
	calls := []func() error{}
	if len(set) > 0 {
		calls = append(calls, func() error {
			_, err := api.AddTagsToResource(ctx, &docdb.AddTagsToResourceInput{ResourceName: arn, Tags: awsapi.SDKTags[types.Tag](set)}, opt)
			return err
		})
	}
	if len(remove) > 0 {
		calls = append(calls, func() error {
			_, err := api.RemoveTagsFromResource(ctx, &docdb.RemoveTagsFromResourceInput{ResourceName: arn, TagKeys: remove}, opt)
			return err
		})
	}
	return calls
}

// logChanges returns the log types to enable and disable to go from current to the desired list.
func logChanges(current []string, desired any) *types.CloudwatchLogsExportConfiguration {
	want := []string{}
	if list, ok := desired.([]any); ok {
		for _, v := range list {
			want = append(want, fmt.Sprint(v))
		}
	}
	out := &types.CloudwatchLogsExportConfiguration{}
	for _, l := range want {
		if !slices.Contains(current, l) {
			out.EnableLogTypes = append(out.EnableLogTypes, l)
		}
	}
	for _, l := range current {
		if !slices.Contains(want, l) {
			out.DisableLogTypes = append(out.DisableLogTypes, l)
		}
	}
	return out
}

// Delete deletes the cluster with a final snapshot named by DeleteToken. UCP calls DeleteWithToken instead, with the
// name it saved on the record.
func (h *Cluster) Delete(ctx context.Context, region string, identifier string) error {
	token, err := h.DeleteToken(ctx, region, identifier)
	if err != nil {
		return err
	}
	return h.DeleteWithToken(ctx, region, identifier, token)
}

// DeleteToken returns the name of the final snapshot: the identifier, "-final-", and the UTC time, such as
// app-docdb-final-20261104-212751.
func (h *Cluster) DeleteToken(ctx context.Context, region string, identifier string) (string, error) {
	return fmt.Sprintf("%s-final-%s", identifier, h.now().UTC().Format("20060102-150405")), nil
}

// DeleteWithToken deletes the cluster with a final snapshot named token. A cluster AWS is already deleting had a
// delete reach AWS before, so the delete succeeds. If AWS can't take the snapshot, it rejects the delete, and the
// cluster stays.
func (h *Cluster) DeleteWithToken(ctx context.Context, region string, identifier string, token string) error {
	if _, err := h.describe(ctx, region, identifier); err != nil {
		return err
	}
	_, err := h.api.DeleteDBCluster(ctx, &docdb.DeleteDBClusterInput{
		DBClusterIdentifier:       aws.String(identifier),
		FinalDBSnapshotIdentifier: aws.String(token),
		SkipFinalSnapshot:         aws.Bool(false),
	}, inRegion(region))
	if awsapi.HasCode(err, "InvalidDBClusterStateFault") {
		cluster, describeErr := h.describe(ctx, region, identifier)
		if describeErr != nil {
			return describeErr
		}
		if strings.EqualFold(aws.ToString(cluster.Status), "deleting") {
			return nil
		}
	}
	return awsapi.NotFound(err, "DBClusterNotFoundFault")
}

// List returns every DocumentDB cluster in the region. Clusters of other products are left out.
func (h *Cluster) List(ctx context.Context, region string) ([]native.Resource, error) {
	out := []native.Resource{}
	var marker *string
	for {
		page, err := h.api.DescribeDBClusters(ctx, &docdb.DescribeDBClustersInput{
			Filters: []types.Filter{{Name: aws.String("engine"), Values: []string{Engine}}},
			Marker:  marker,
		}, inRegion(region))
		if err != nil {
			return nil, err
		}
		for i := range page.DBClusters {
			c := &page.DBClusters[i]
			if aws.ToString(c.Engine) != Engine {
				continue
			}
			properties, err := h.properties(ctx, region, c)
			if err != nil {
				return nil, err
			}
			out = append(out, native.Resource{Identifier: aws.ToString(c.DBClusterIdentifier), Properties: properties})
		}
		if marker = page.Marker; marker == nil {
			return out, nil
		}
	}
}

// Progress reads the cluster's status. A create or update waits for available, and a delete for the cluster to be
// gone.
func (h *Cluster) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	cluster, err := h.describe(ctx, region, identifier)
	if err != nil {
		return native.Progress{}, err
	}
	status := aws.ToString(cluster.Status)
	if op == native.OperationDelete {
		return native.Progress{State: status, Waiting: "deleted"}, nil
	}
	return clusterStates.Progress(status), nil
}
