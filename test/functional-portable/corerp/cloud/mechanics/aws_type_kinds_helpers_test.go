/*
Copyright 2026 The Radius Authors.

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

package mechanics_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/docdb"
	docdbtypes "github.com/aws/aws-sdk-go-v2/service/docdb/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/kafka"
	kafkatypes "github.com/aws/aws-sdk-go-v2/service/kafka/types"
	"github.com/aws/smithy-go"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/test"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UCP logs one of these messages, with the alias, right before each call that changes a resource in AWS.
const (
	msgCreating = "Creating AWS resource"
	msgUpdating = "Updating AWS resource"
	msgDeleting = "Deleting AWS resource"
)

const (
	// cleanupTimeout bounds each cleanup. Deleting a DocumentDB cluster or an MSK cluster takes several minutes.
	cleanupTimeout = 30 * time.Minute

	// cleanupReserve is the time the slow tests keep for cleanup before the go test deadline. When the test binary
	// times out, Go runs no cleanup, so the clusters and their network would stay in AWS.
	cleanupReserve = 10 * time.Minute
)

func loadAWSConfig(ctx context.Context, t *testing.T) aws.Config {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	require.NoError(t, err)
	return cfg
}

// cleanupContext returns a context for cleanup that ends 30 seconds before the go test deadline. t.Context is canceled
// before cleanup runs.
func cleanupContext(t *testing.T) (context.Context, context.CancelFunc) {
	return cleanupContextBefore(t, 30*time.Second)
}

// cleanupContextBefore returns a context for cleanup that ends margin before the go test deadline.
func cleanupContextBefore(t *testing.T, margin time.Duration) (context.Context, context.CancelFunc) {
	deadline := time.Now().Add(cleanupTimeout)
	if testDeadline, ok := t.Deadline(); ok && testDeadline.Add(-margin).Before(deadline) {
		deadline = testDeadline.Add(-margin)
	}
	return context.WithDeadline(context.Background(), deadline) //nolint:usetesting
}

// cleanupRadiusResources deletes the Radius resources a test deploys when it ends, in place of the framework's
// teardown, which waits on each delete without a deadline when RADIUS_TEST_FAST_CLEANUP isn't set. A delete that hangs
// there would use up the time the AWS cleanups need. This one ends cleanupReserve/2 before the go test deadline, so
// the AWS cleanups registered before it still run. Register it last, so it runs first.
func cleanupRadiusResources(t *testing.T, client clients.ApplicationsManagementClient, resources ...validation.RPResource) {
	t.Cleanup(func() {
		ctx, cancel := cleanupContextBefore(t, cleanupReserve/2)
		defer cancel()

		for _, resource := range resources {
			t.Logf("deleting %s %s", resource.Type, resource.Name)
			var err error
			switch resource.Type {
			case validation.ApplicationsResource:
				_, err = client.DeleteApplication(ctx, resource.Name, false)
			case validation.EnvironmentsResource:
				_, err = client.DeleteEnvironment(ctx, resource.Name)
			default:
				// force deletes a resource left in a non-terminal state by a failed deploy.
				_, err = client.DeleteResource(ctx, resource.Type, resource.Name, true)
			}
			if err != nil && !clients.Is404Error(err) {
				t.Errorf("failed to delete %s %s: %v", resource.Type, resource.Name, err)
			}
		}
	})
}

// beforeCleanup returns executor with a context that ends cleanupReserve before the go test deadline, so a slow
// deploy or delete leaves time for cleanup.
func beforeCleanup(executor step.Executor) step.FuncExecutor {
	wrapped := step.NewFuncExecutor(func(ctx context.Context, t *testing.T, options test.TestOptions) {
		if deadline, ok := t.Deadline(); ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, deadline.Add(-cleanupReserve))
			defer cancel()
		}
		executor.Execute(ctx, t, options)
	})
	wrapped.Description = executor.GetDescription()
	return wrapped
}

// awsWrites returns the messages UCP logged since the time since for calls that change the AWS resource of alias.
// The tests deploy through the Deployment Engine, so UCP's log is the only place that shows which calls it made.
func awsWrites(ctx context.Context, t *testing.T, ct rp.RPTest, since time.Time, alias string) []string {
	client := ct.Options.K8sClient
	pods, err := client.CoreV1().Pods("radius-system").List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=ucp"})
	require.NoError(t, err)
	require.NotEmpty(t, pods.Items, "found no UCP pod")

	writes := []string{}
	for _, pod := range pods.Items {
		options := &corev1.PodLogOptions{Container: "ucp"}
		if !since.IsZero() {
			options.SinceTime = &metav1.Time{Time: since}
		}
		stream, err := client.CoreV1().Pods("radius-system").GetLogs(pod.Name, options).Stream(ctx)
		require.NoError(t, err, "failed to read the logs of %s", pod.Name)
		data, err := io.ReadAll(stream)
		_ = stream.Close()
		require.NoError(t, err)

		for line := range strings.SplitSeq(string(data), "\n") {
			start := strings.IndexByte(line, '{')
			if start < 0 {
				continue
			}
			entry := map[string]any{}
			if json.Unmarshal([]byte(line[start:]), &entry) != nil {
				continue
			}
			message, _ := entry["message"].(string)
			if entry["alias"] == alias && slices.Contains([]string{msgCreating, msgUpdating, msgDeleting}, message) {
				writes = append(writes, message)
			}
		}
	}
	return writes
}

// requireAWSWrite requires that UCP logged message for alias. It shows that requireNoAWSWrites reads the right log.
func requireAWSWrite(ctx context.Context, t *testing.T, ct rp.RPTest, alias string, message string) {
	require.Contains(t, awsWrites(ctx, t, ct, time.Time{}, alias), message, "UCP did not log %q for the alias %s", message, alias)
}

// requireNoAWSWrites requires that UCP sent nothing to AWS for alias since the time since.
func requireNoAWSWrites(ctx context.Context, t *testing.T, ct rp.RPTest, since time.Time, alias string) {
	require.Empty(t, awsWrites(ctx, t, ct, since, alias), "UCP changed the AWS resource of the alias %s", alias)
}

// errorCode returns the error code of an AWS API error, or "".
func errorCode(err error) string {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode()
	}
	return ""
}

// poll calls done every interval until it returns true or an error, or ctx ends.
func poll(ctx context.Context, interval time.Duration, done func() (bool, error)) error {
	for {
		ok, err := done()
		if err != nil || ok {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

// createTestNetwork creates a VPC with two subnets in different Availability Zones and returns the subnet IDs. The
// VPC is deleted when the test ends, after the cleanups registered later.
func createTestNetwork(ctx context.Context, t *testing.T, client *ec2.Client, name string, creationTimestamp string) []string {
	tags := func(resourceType ec2types.ResourceType) []ec2types.TagSpecification {
		return []ec2types.TagSpecification{{
			ResourceType: resourceType,
			Tags: []ec2types.Tag{
				{Key: aws.String("Name"), Value: aws.String(name)},
				{Key: aws.String("RadiusCreationTimestamp"), Value: aws.String(creationTimestamp)},
			},
		}}
	}

	vpc, err := client.CreateVpc(ctx, &ec2.CreateVpcInput{CidrBlock: aws.String("10.0.0.0/16"), TagSpecifications: tags(ec2types.ResourceTypeVpc)})
	require.NoError(t, err)
	vpcID := aws.ToString(vpc.Vpc.VpcId)
	subnetIDs := []string{}
	t.Cleanup(func() { deleteTestNetwork(t, client, vpcID, &subnetIDs) })

	require.NoError(t, ec2.NewVpcAvailableWaiter(client).Wait(ctx, &ec2.DescribeVpcsInput{VpcIds: []string{vpcID}}, 2*time.Minute))
	// MSK requires DNS hostnames.
	_, err = client.ModifyVpcAttribute(ctx, &ec2.ModifyVpcAttributeInput{VpcId: aws.String(vpcID), EnableDnsHostnames: &ec2types.AttributeBooleanValue{Value: aws.Bool(true)}})
	require.NoError(t, err)

	zones, err := client.DescribeAvailabilityZones(ctx, &ec2.DescribeAvailabilityZonesInput{Filters: []ec2types.Filter{
		{Name: aws.String("state"), Values: []string{"available"}},
		{Name: aws.String("zone-type"), Values: []string{"availability-zone"}},
	}})
	require.NoError(t, err)
	names := []string{}
	for _, zone := range zones.AvailabilityZones {
		// MSK doesn't support subnets in use1-az3.
		if aws.ToString(zone.ZoneId) != "use1-az3" {
			names = append(names, aws.ToString(zone.ZoneName))
		}
	}
	slices.Sort(names)
	require.GreaterOrEqual(t, len(names), 2, "the Region needs two Availability Zones")

	for i, zone := range names[:2] {
		subnet, err := client.CreateSubnet(ctx, &ec2.CreateSubnetInput{
			VpcId:             aws.String(vpcID),
			CidrBlock:         aws.String([]string{"10.0.0.0/24", "10.0.1.0/24"}[i]),
			AvailabilityZone:  aws.String(zone),
			TagSpecifications: tags(ec2types.ResourceTypeSubnet),
		})
		require.NoError(t, err)
		subnetIDs = append(subnetIDs, aws.ToString(subnet.Subnet.SubnetId))
	}
	return subnetIDs
}

// deleteTestNetwork deletes the subnets and the VPC. A deleted cluster's network interfaces can stay for a few minutes,
// so it retries while AWS reports a dependency.
func deleteTestNetwork(t *testing.T, client *ec2.Client, vpcID string, subnetIDs *[]string) {
	ctx, cancel := cleanupContext(t)
	defer cancel()

	deleteRetrying := func(what string, del func() error) {
		err := poll(ctx, 15*time.Second, func() (bool, error) {
			err := del()
			switch {
			case err == nil, strings.HasSuffix(errorCode(err), ".NotFound"):
				return true, nil
			case errorCode(err) == "DependencyViolation":
				return false, nil
			default:
				return false, err
			}
		})
		if err != nil {
			t.Errorf("failed to delete %s: %v", what, err)
		}
	}
	for _, subnetID := range *subnetIDs {
		t.Logf("deleting subnet %s", subnetID)
		deleteRetrying("subnet "+subnetID, func() error {
			_, err := client.DeleteSubnet(ctx, &ec2.DeleteSubnetInput{SubnetId: aws.String(subnetID)})
			return err
		})
	}
	t.Logf("deleting VPC %s", vpcID)
	deleteRetrying("VPC "+vpcID, func() error {
		_, err := client.DeleteVpc(ctx, &ec2.DeleteVpcInput{VpcId: aws.String(vpcID)})
		return err
	})
}

// placementGroups returns the placement groups with the Name tag name.
func placementGroups(ctx context.Context, client *ec2.Client, name string) ([]ec2types.PlacementGroup, error) {
	out, err := client.DescribePlacementGroups(ctx, &ec2.DescribePlacementGroupsInput{
		Filters: []ec2types.Filter{{Name: aws.String("tag:Name"), Values: []string{name}}},
	})
	if err != nil {
		return nil, err
	}
	return out.PlacementGroups, nil
}

// requireOnePlacementGroup requires that AWS has exactly one available placement group with the Name tag name and
// the strategy, and returns its ID. AWS generates the group's name, so the test finds it by its tag.
func requireOnePlacementGroup(ctx context.Context, t *testing.T, client *ec2.Client, name string, strategy string) string {
	var groups []ec2types.PlacementGroup
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		groups, err = placementGroups(ctx, client, name)
		require.NoError(c, err)
		require.NotEmpty(c, groups, "AWS has no placement group tagged %s", name)
	}, time.Minute, 5*time.Second)
	require.Len(t, groups, 1, "AWS has more than one placement group tagged %s", name)
	require.Equal(t, ec2types.PlacementGroupStateAvailable, groups[0].State)
	require.Equal(t, strategy, string(groups[0].Strategy))
	return aws.ToString(groups[0].GroupId)
}

// cleanupPlacementGroups deletes the placement groups with the Name tag name when the test ends.
func cleanupPlacementGroups(t *testing.T, client *ec2.Client, name string) {
	t.Cleanup(func() {
		ctx, cancel := cleanupContext(t)
		defer cancel()

		groups, err := placementGroups(ctx, client, name)
		if err != nil {
			t.Errorf("failed to list the placement groups tagged %s: %v", name, err)
			return
		}
		for _, group := range groups {
			t.Logf("deleting placement group %s", aws.ToString(group.GroupName))
			if _, err := client.DeletePlacementGroup(ctx, &ec2.DeletePlacementGroupInput{GroupName: group.GroupName}); err != nil {
				t.Errorf("failed to delete placement group %s: %v", aws.ToString(group.GroupName), err)
			}
		}
	})
}

// newestAmazonLinuxImage returns the ID of the newest public Amazon Linux 2023 AMI in the Region.
func newestAmazonLinuxImage(ctx context.Context, t *testing.T, client *ec2.Client) string {
	pages := ec2.NewDescribeImagesPaginator(client, &ec2.DescribeImagesInput{
		Owners: []string{"amazon"},
		Filters: []ec2types.Filter{
			{Name: aws.String("name"), Values: []string{"al2023-ami-2023.*-x86_64"}},
			{Name: aws.String("state"), Values: []string{"available"}},
			{Name: aws.String("is-public"), Values: []string{"true"}},
		},
	})
	var newest ec2types.Image
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		require.NoError(t, err)
		for _, image := range page.Images {
			if aws.ToString(image.CreationDate) > aws.ToString(newest.CreationDate) {
				newest = image
			}
		}
	}
	require.NotEmpty(t, aws.ToString(newest.ImageId), "found no public Amazon Linux 2023 AMI")
	return aws.ToString(newest.ImageId)
}

// createDocDBSubnetGroup creates a DB subnet group for the subnets, and deletes it when the test ends.
func createDocDBSubnetGroup(ctx context.Context, t *testing.T, client *docdb.Client, name string, subnetIDs []string, creationTimestamp string) {
	_, err := client.CreateDBSubnetGroup(ctx, &docdb.CreateDBSubnetGroupInput{
		DBSubnetGroupName:        aws.String(name),
		DBSubnetGroupDescription: aws.String("Radius functional test"),
		SubnetIds:                subnetIDs,
		Tags:                     []docdbtypes.Tag{{Key: aws.String("RadiusCreationTimestamp"), Value: aws.String(creationTimestamp)}},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := cleanupContext(t)
		defer cancel()

		t.Logf("deleting DB subnet group %s", name)
		err := poll(ctx, 15*time.Second, func() (bool, error) {
			_, err := client.DeleteDBSubnetGroup(ctx, &docdb.DeleteDBSubnetGroupInput{DBSubnetGroupName: aws.String(name)})
			switch {
			case err == nil, errorCode(err) == "DBSubnetGroupNotFoundFault":
				return true, nil
			case errorCode(err) == "InvalidDBSubnetGroupStateFault":
				return false, nil
			default:
				return false, err
			}
		})
		if err != nil {
			t.Errorf("failed to delete DB subnet group %s: %v", name, err)
		}
	})
}

// docDBCluster returns the DocumentDB cluster with the identifier, or nil when AWS has none.
func docDBCluster(ctx context.Context, client *docdb.Client, identifier string) (*docdbtypes.DBCluster, error) {
	out, err := client.DescribeDBClusters(ctx, &docdb.DescribeDBClustersInput{DBClusterIdentifier: aws.String(identifier)})
	if errorCode(err) == "DBClusterNotFoundFault" {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	if len(out.DBClusters) == 0 {
		return nil, nil
	}
	return &out.DBClusters[0], nil
}

func docDBClusterExists(ctx context.Context, client *docdb.Client, identifier string) (bool, error) {
	cluster, err := docDBCluster(ctx, client, identifier)
	return cluster != nil, err
}

// requireDocDBCluster requires that AWS has an available DocumentDB cluster with the identifier, and returns it.
func requireDocDBCluster(ctx context.Context, t *testing.T, client *docdb.Client, identifier string) *docdbtypes.DBCluster {
	cluster, err := docDBCluster(ctx, client, identifier)
	require.NoError(t, err)
	require.NotNil(t, cluster, "AWS has no DocumentDB cluster %s", identifier)
	require.Equal(t, "docdb", aws.ToString(cluster.Engine))
	require.Equal(t, "available", aws.ToString(cluster.Status))
	return cluster
}

// docDBSnapshots returns the manual snapshots of the cluster with the identifier. The identifier is unique to the test,
// so these are the final snapshots UCP took when it deleted the cluster.
func docDBSnapshots(ctx context.Context, client *docdb.Client, identifier string) ([]docdbtypes.DBClusterSnapshot, error) {
	out, err := client.DescribeDBClusterSnapshots(ctx, &docdb.DescribeDBClusterSnapshotsInput{
		DBClusterIdentifier: aws.String(identifier),
		SnapshotType:        aws.String("manual"),
	})
	if err != nil {
		return nil, err
	}
	return out.DBClusterSnapshots, nil
}

// cleanupDocDBCluster deletes the cluster with the identifier, if the test left it, and then every snapshot of it, when
// the test ends. Radius never deletes a final snapshot, so the test must.
func cleanupDocDBCluster(t *testing.T, client *docdb.Client, identifier string) {
	t.Cleanup(func() {
		ctx, cancel := cleanupContext(t)
		defer cancel()

		err := poll(ctx, 15*time.Second, func() (bool, error) {
			cluster, err := docDBCluster(ctx, client, identifier)
			if err != nil || cluster == nil {
				return cluster == nil, err
			}
			if aws.ToString(cluster.Status) != "available" {
				return false, nil
			}
			t.Logf("deleting DocumentDB cluster %s", identifier)
			_, err = client.DeleteDBCluster(ctx, &docdb.DeleteDBClusterInput{DBClusterIdentifier: aws.String(identifier), SkipFinalSnapshot: aws.Bool(true)})
			if errorCode(err) == "InvalidDBClusterStateFault" {
				return false, nil
			}
			return false, err
		})
		if err != nil {
			t.Errorf("failed to delete DocumentDB cluster %s: %v", identifier, err)
			return
		}

		err = poll(ctx, 15*time.Second, func() (bool, error) {
			snapshots, err := docDBSnapshots(ctx, client, identifier)
			if err != nil {
				return false, err
			}
			for _, snapshot := range snapshots {
				if aws.ToString(snapshot.Status) != "available" {
					continue
				}
				t.Logf("deleting DocumentDB snapshot %s", aws.ToString(snapshot.DBClusterSnapshotIdentifier))
				_, err := client.DeleteDBClusterSnapshot(ctx, &docdb.DeleteDBClusterSnapshotInput{DBClusterSnapshotIdentifier: snapshot.DBClusterSnapshotIdentifier})
				if err != nil && errorCode(err) != "DBClusterSnapshotNotFoundFault" {
					return false, err
				}
			}
			return len(snapshots) == 0, nil
		})
		if err != nil {
			t.Errorf("failed to delete the final snapshots of DocumentDB cluster %s: %v", identifier, err)
		}
	})
}

// listMSKClusters returns the ARNs of the MSK clusters named name.
func listMSKClusters(ctx context.Context, client *kafka.Client, name string) ([]string, error) {
	arns := []string{}
	pages := kafka.NewListClustersV2Paginator(client, &kafka.ListClustersV2Input{ClusterNameFilter: aws.String(name)})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, cluster := range page.ClusterInfoList {
			if aws.ToString(cluster.ClusterName) == name {
				arns = append(arns, aws.ToString(cluster.ClusterArn))
			}
		}
	}
	return arns, nil
}

// requireOneMSKCluster requires that AWS has exactly one active MSK cluster named name, with the RadiusCreationTimestamp
// tag set to creationTimestamp, and returns its ARN. AWS assigns the ARN, so the test finds the cluster by its name.
func requireOneMSKCluster(ctx context.Context, t *testing.T, client *kafka.Client, name string, creationTimestamp string) string {
	arns, err := listMSKClusters(ctx, client, name)
	require.NoError(t, err)
	require.Len(t, arns, 1, "want one MSK cluster named %s", name)
	out, err := client.DescribeClusterV2(ctx, &kafka.DescribeClusterV2Input{ClusterArn: aws.String(arns[0])})
	require.NoError(t, err)
	require.Equal(t, kafkatypes.ClusterStateActive, out.ClusterInfo.State)
	require.Equal(t, creationTimestamp, out.ClusterInfo.Tags["RadiusCreationTimestamp"], "the cluster doesn't have the template's tag")
	return arns[0]
}

// mskBootstrapBrokers returns the IAM broker addresses AWS returns for the cluster.
func mskBootstrapBrokers(ctx context.Context, t *testing.T, client *kafka.Client, arn string) string {
	out, err := client.GetBootstrapBrokers(ctx, &kafka.GetBootstrapBrokersInput{ClusterArn: aws.String(arn)})
	require.NoError(t, err)
	return aws.ToString(out.BootstrapBrokerStringSaslIam)
}

// cleanupMSKClusters deletes the MSK clusters named name, and waits until they are gone, when the test ends.
func cleanupMSKClusters(t *testing.T, client *kafka.Client, name string) {
	t.Cleanup(func() {
		ctx, cancel := cleanupContext(t)
		defer cancel()

		err := poll(ctx, 15*time.Second, func() (bool, error) {
			arns, err := listMSKClusters(ctx, client, name)
			if err != nil || len(arns) == 0 {
				return len(arns) == 0, err
			}
			for _, arn := range arns {
				t.Logf("deleting MSK cluster %s", arn)
				_, err := client.DeleteCluster(ctx, &kafka.DeleteClusterInput{ClusterArn: aws.String(arn)})
				// AWS rejects a delete while the cluster is still being created or is already being deleted.
				if err != nil && !slices.Contains([]string{"BadRequestException", "NotFoundException"}, errorCode(err)) {
					return false, err
				}
			}
			return false, nil
		})
		if err != nil {
			t.Errorf("failed to delete the MSK clusters named %s: %v", name, err)
		}
	})
}
