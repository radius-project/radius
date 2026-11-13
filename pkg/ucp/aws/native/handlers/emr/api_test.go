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

package emr

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/emr"
	"github.com/aws/aws-sdk-go-v2/service/emr/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	apiReference    = "https://docs.aws.amazon.com/emr/latest/APIReference/"
	commonErrors    = apiReference + "CommonErrors.html"
	managementGuide = "https://docs.aws.amazon.com/emr/latest/ManagementGuide/"
	tagRules        = managementGuide + "emr-plan-tags.html"
	terminationDoc  = managementGuide + "UsingEMR_TerminationProtection.html"
)

var (
	// longID is a cluster ID longer than the 256 characters EMR allows.
	longID = "j-" + strings.Repeat("A", maxID)

	// longJobFlowID is a job flow ID longer than the 10280 characters EMR allows.
	longJobFlowID = "j-" + strings.Repeat("A", maxJobFlowID)
)

// wide returns n copies of é, which UTF-8 encodes in two bytes, so a limit counted in bytes instead of characters
// rejects it too early.
func wide(n int) string {
	return strings.Repeat("é", n)
}

// runInput returns a RunJobFlow request EMR accepts, with a master and a core instance group.
func runInput() *emr.RunJobFlowInput {
	return &emr.RunJobFlowInput{
		Name:         aws.String("app-emr"),
		ReleaseLabel: aws.String("emr-7.1.0"),
		Instances: &types.JobFlowInstancesConfig{InstanceGroups: []types.InstanceGroupConfig{
			{InstanceRole: types.InstanceRoleTypeMaster, InstanceCount: aws.Int32(1), InstanceType: aws.String("m5.xlarge")},
			{InstanceRole: types.InstanceRoleTypeCore, InstanceCount: aws.Int32(1), InstanceType: aws.String("m5.xlarge")},
		}},
	}
}

// withFleets changes a RunJobFlow request to use a master and a core instance fleet instead of instance groups.
func withFleets(in *emr.RunJobFlowInput) {
	in.Instances = &types.JobFlowInstancesConfig{InstanceFleets: []types.InstanceFleetConfig{
		{InstanceFleetType: types.InstanceFleetTypeMaster, TargetOnDemandCapacity: aws.Int32(1), InstanceTypeConfigs: []types.InstanceTypeConfig{{InstanceType: aws.String("m5.xlarge")}}},
		{InstanceFleetType: types.InstanceFleetTypeCore, TargetSpotCapacity: aws.Int32(2), InstanceTypeConfigs: []types.InstanceTypeConfig{{InstanceType: aws.String("m5.xlarge")}}},
	}}
}

// coreFleet returns the core instance fleet of a request withFleets changed.
func coreFleet(in *emr.RunJobFlowInput) *types.InstanceFleetConfig {
	return &in.Instances.InstanceFleets[1]
}

// typeConfigs returns n instance type configurations.
func typeConfigs(n int) []types.InstanceTypeConfig {
	out := []types.InstanceTypeConfig{}
	for i := range n {
		out = append(out, types.InstanceTypeConfig{InstanceType: aws.String(fmt.Sprintf("m5.%dxlarge", i+1))})
	}
	return out
}

// run starts a cluster with f from runInput changed by edit, which may be nil, and returns its ID.
func run(ctx context.Context, f *fakeAPI, edit func(in *emr.RunJobFlowInput)) (string, error) {
	in := runInput()
	if edit != nil {
		edit(in)
	}
	out, err := f.RunJobFlow(ctx, in)
	if err != nil {
		return "", err
	}
	return aws.ToString(out.JobFlowId), nil
}

// runCall returns a call that starts a cluster from runInput changed by edit, against a fake EMR API backed by store.
func runCall(edit func(in *emr.RunJobFlowInput)) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		_, err := run(ctx, newFakeAPI(store), edit)
		return err
	}
}

// started starts a cluster with f and describes it once, so it runs, and returns its ID.
func started(ctx context.Context, f *fakeAPI, edit func(in *emr.RunJobFlowInput)) (string, error) {
	id, err := run(ctx, f, edit)
	if err != nil {
		return "", err
	}
	_, err = describeCluster(ctx, f, id)
	return id, err
}

func describeCluster(ctx context.Context, f *fakeAPI, id string) (*types.Cluster, error) {
	out, err := f.DescribeCluster(ctx, &emr.DescribeClusterInput{ClusterId: aws.String(id)})
	if err != nil {
		return nil, err
	}
	return out.Cluster, nil
}

// onCluster returns a call that makes call on a cluster that runs, against a fake EMR API backed by store.
func onCluster(call func(ctx context.Context, f *fakeAPI, id string) error) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		f := newFakeAPI(store)
		id, err := started(ctx, f, nil)
		if err != nil {
			return err
		}
		return call(ctx, f, id)
	}
}

// idRule returns the rule that a call's ClusterId is at most 256 characters, for call, which makes the call on the
// cluster with the ID.
func idRule(call func(ctx context.Context, f *fakeAPI, id string) error) nativetest.Rule {
	return nativetest.Rule{
		Rule: "ClusterId is at most 256 characters",
		Break: func(ctx context.Context, store *awsfake.CloudControl) error {
			return call(ctx, newFakeAPI(store), longID)
		},
		Code: "ValidationError",
		Doc:  commonErrors,
	}
}

// jobFlowRules returns the rules for a call that takes JobFlowIds and a required Boolean, for call, which makes the
// call on the clusters with the IDs, with value as the Boolean.
func jobFlowRules(name string, call func(ctx context.Context, f *fakeAPI, ids []string, value *bool) error) []nativetest.Rule {
	return []nativetest.Rule{
		{
			Rule: "each of JobFlowIds is at most 10280 characters",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				return call(ctx, newFakeAPI(store), []string{longJobFlowID}, aws.Bool(true))
			},
			Code: "ValidationError",
			Doc:  commonErrors,
		},
		{
			Rule: name + " is required",
			Break: onCluster(func(ctx context.Context, f *fakeAPI, id string) error {
				return call(ctx, f, []string{id}, nil)
			}),
			Code: "ValidationError",
			Doc:  commonErrors,
		},
	}
}

// requireFails returns an error unless err is one, for a Check that a call fails.
func requireFails(err error, what string) error {
	if err == nil {
		return fmt.Errorf("the fake accepts %s", what)
	}
	return nil
}

// terminated starts a cluster tagged team=a with f, shuts it down, and describes it once, so it is TERMINATED but EMR
// still has it, and returns its ID.
func terminated(ctx context.Context, f *fakeAPI) (string, error) {
	id, err := started(ctx, f, func(in *emr.RunJobFlowInput) {
		in.Tags = []types.Tag{{Key: aws.String("team"), Value: aws.String("a")}}
	})
	if err != nil {
		return "", err
	}
	if _, err := f.TerminateJobFlows(ctx, &emr.TerminateJobFlowsInput{JobFlowIds: []string{id}}); err != nil {
		return "", err
	}
	_, err = describeCluster(ctx, f, id)
	return id, err
}

// terminatedTagsRule returns the rule that a terminated cluster's tags can't change, for call, which changes the tags
// of the cluster with the ID.
func terminatedTagsRule(call func(ctx context.Context, f *fakeAPI, id string) error) nativetest.Rule {
	return nativetest.Rule{
		Rule: "a terminated cluster's tags can't change",
		Doc:  tagRules,
		Check: func(ctx context.Context, store *awsfake.CloudControl) error {
			f := newFakeAPI(store)
			id, err := terminated(ctx, f)
			if err != nil {
				return err
			}
			before := awsapi.Tags(store.Resource(clusterType, id))
			if err := requireFails(call(ctx, f, id), "a tag change on a terminated cluster"); err != nil {
				return err
			}
			if after := awsapi.Tags(store.Resource(clusterType, id)); len(after) != len(before) {
				return fmt.Errorf("the tags changed from %v to %v", before, after)
			}
			return nil
		},
	}
}

// pagesRule returns the rule that a list call may return part of the list with a Marker for the rest, for list,
// which lists the items of a cluster started from runInput changed by edit and returns the Marker of the first page.
func pagesRule(edit func(in *emr.RunJobFlowInput), list func(ctx context.Context, f *fakeAPI, id string) (*string, error)) nativetest.Rule {
	return nativetest.Rule{
		Rule: "AWS may return part of the list, with a Marker for the rest",
		Check: func(ctx context.Context, store *awsfake.CloudControl) error {
			f := newFakeAPI(store)
			id, err := started(ctx, f, edit)
			if err != nil {
				return err
			}
			marker, err := list(ctx, f, id)
			if err != nil {
				return err
			}
			if marker == nil {
				return errors.New("the fake returned the whole list on one page")
			}
			return nil
		},
	}
}

// withTasks adds two task instance groups to a RunJobFlow request, so the cluster has four groups.
func withTasks(in *emr.RunJobFlowInput) {
	for range 2 {
		in.Instances.InstanceGroups = append(in.Instances.InstanceGroups, types.InstanceGroupConfig{
			InstanceRole: types.InstanceRoleTypeTask, InstanceCount: aws.Int32(1), InstanceType: aws.String("m5.xlarge"),
		})
	}
}

// withTaskFleet adds a task instance fleet to a RunJobFlow request withFleets changed, so the cluster has three
// fleets.
func withTaskFleet(in *emr.RunJobFlowInput) {
	withFleets(in)
	in.Instances.InstanceFleets = append(in.Instances.InstanceFleets, types.InstanceFleetConfig{
		InstanceFleetType: types.InstanceFleetTypeTask, TargetSpotCapacity: aws.Int32(1), InstanceTypeConfigs: typeConfigs(1),
	})
}

// scalingPolicy returns a managed scaling policy with the compute limits EMR requires.
func scalingPolicy() *types.ManagedScalingPolicy {
	return &types.ManagedScalingPolicy{ComputeLimits: &types.ComputeLimits{
		MaximumCapacityUnits: aws.Int32(8), MinimumCapacityUnits: aws.Int32(2), UnitType: types.ComputeLimitsUnitTypeInstances,
	}}
}

// requireScalingDefaults returns an error unless the managed scaling policy of the cluster with the ID has the
// On-Demand and core limits EMR sets to MaximumCapacityUnits.
func requireScalingDefaults(ctx context.Context, f *fakeAPI, id string) error {
	out, err := f.GetManagedScalingPolicy(ctx, &emr.GetManagedScalingPolicyInput{ClusterId: aws.String(id)})
	if err != nil {
		return err
	}
	l := out.ManagedScalingPolicy.ComputeLimits
	if aws.ToInt32(l.MaximumOnDemandCapacityUnits) != 8 || aws.ToInt32(l.MaximumCoreCapacityUnits) != 8 {
		return fmt.Errorf("the limits are %+v", l)
	}
	return nil
}

// requireIdleTimeout returns an error unless the auto-termination policy of the cluster with the ID has the idle
// timeout EMR uses when the policy sets none.
func requireIdleTimeout(ctx context.Context, f *fakeAPI, id string) error {
	out, err := f.GetAutoTerminationPolicy(ctx, &emr.GetAutoTerminationPolicyInput{ClusterId: aws.String(id)})
	if err != nil {
		return err
	}
	if p := out.AutoTerminationPolicy; p == nil || aws.ToInt64(p.IdleTimeout) != defaultIdleTimeout {
		return fmt.Errorf("the policy is %+v", p)
	}
	return nil
}

// runDefault returns the rule for a value EMR gives a cluster when RunJobFlow leaves it out, for require, which
// returns an error unless the cluster with the ID has it.
func runDefault(rule string, doc string, edit func(in *emr.RunJobFlowInput), require func(ctx context.Context, f *fakeAPI, id string) error) nativetest.Rule {
	return nativetest.Rule{
		Rule: rule,
		Doc:  doc,
		Check: func(ctx context.Context, store *awsfake.CloudControl) error {
			f := newFakeAPI(store)
			id, err := started(ctx, f, edit)
			if err != nil {
				return err
			}
			return require(ctx, f, id)
		},
	}
}

// requireCluster returns a require for runDefault that describes the cluster and returns an error unless ok says the
// cluster has the value.
func requireCluster(ok func(c *types.Cluster) bool) func(ctx context.Context, f *fakeAPI, id string) error {
	return func(ctx context.Context, f *fakeAPI, id string) error {
		c, err := describeCluster(ctx, f, id)
		if err != nil {
			return err
		}
		if !ok(c) {
			return fmt.Errorf("the cluster is %+v", c)
		}
		return nil
	}
}

// requireTypeConfig returns a require for runDefault that lists the instance fleets and returns an error unless ok
// says the core fleet's instance type has the value.
func requireTypeConfig(ok func(s types.InstanceTypeSpecification) bool) func(ctx context.Context, f *fakeAPI, id string) error {
	return func(ctx context.Context, f *fakeAPI, id string) error {
		out, err := f.ListInstanceFleets(ctx, &emr.ListInstanceFleetsInput{ClusterId: aws.String(id)})
		if err != nil {
			return err
		}
		if s := out.InstanceFleets[1].InstanceTypeSpecifications[0]; !ok(s) {
			return fmt.Errorf("the instance type is %+v", s)
		}
		return nil
	}
}

func modifyGroups(ctx context.Context, f *fakeAPI, id string, group *string) error {
	_, err := f.ModifyInstanceGroups(ctx, &emr.ModifyInstanceGroupsInput{ClusterId: aws.String(id), InstanceGroups: []types.InstanceGroupModifyConfig{
		{InstanceGroupId: group, InstanceCount: aws.Int32(2)},
	}})
	return err
}

func addTag(ctx context.Context, f *fakeAPI, id string, key string, value string) error {
	_, err := f.AddTags(ctx, &emr.AddTagsInput{ResourceId: aws.String(id), Tags: []types.Tag{{Key: aws.String(key), Value: aws.String(value)}}})
	return err
}

func terminate(ctx context.Context, f *fakeAPI, ids []string) error {
	_, err := f.TerminateJobFlows(ctx, &emr.TerminateJobFlowsInput{JobFlowIds: ids})
	return err
}

func protect(ctx context.Context, f *fakeAPI, ids []string, value *bool) error {
	_, err := f.SetTerminationProtection(ctx, &emr.SetTerminationProtectionInput{JobFlowIds: ids, TerminationProtected: value})
	return err
}

// clusterAPI records the EMR calls Cluster makes.
var clusterAPI = map[string]nativetest.Call{
	"RunJobFlow": {
		Doc: apiReference + "API_RunJobFlow.html",
		Rules: []nativetest.Rule{
			{Rule: "Name is required", Break: runCall(func(in *emr.RunJobFlowInput) { in.Name = nil }), Code: "ValidationError", Doc: commonErrors},
			{Rule: "Name is at most 256 characters", Break: runCall(func(in *emr.RunJobFlowInput) { in.Name = aws.String(wide(maxName + 1)) }), Code: "ValidationError", Doc: commonErrors},
			{Rule: "Name's limit counts characters, so 256 non-ASCII characters are accepted", Check: runCall(func(in *emr.RunJobFlowInput) { in.Name = aws.String(wide(maxName)) })},
			{
				Rule:  "a cluster's name can't contain <, >, $, |, or `",
				Doc:   apiReference + "API_Cluster.html",
				Break: runCall(func(in *emr.RunJobFlowInput) { in.Name = aws.String("app|emr") }),
				Code:  "ValidationError",
			},
			{
				Rule: "AdditionalInfo is at most 10280 characters",
				Break: runCall(func(in *emr.RunJobFlowInput) {
					in.AdditionalInfo = aws.String(wide(maxAdditionalInfo + 1))
				}),
				Code: "ValidationError",
				Doc:  commonErrors,
			},
			{
				Rule: "AdditionalInfo's limit counts characters, so 10280 non-ASCII characters are accepted",
				Check: runCall(func(in *emr.RunJobFlowInput) {
					in.AdditionalInfo = aws.String(wide(maxAdditionalInfo))
				}),
			},
			{
				Rule: "an instance type is 1 to 256 characters",
				Doc:  apiReference + "API_InstanceGroupConfig.html",
				Break: runCall(func(in *emr.RunJobFlowInput) {
					in.Instances.InstanceGroups[1].InstanceType = aws.String("")
				}),
				Code: "ValidationError",
			},
			{
				Rule: "a job flow has at most 256 steps",
				Break: runCall(func(in *emr.RunJobFlowInput) {
					for i := range maxSteps + 1 {
						in.Steps = append(in.Steps, types.StepConfig{Name: aws.String(fmt.Sprint(i)), HadoopJarStep: &types.HadoopJarStepConfig{Jar: aws.String("command-runner.jar")}})
					}
				}),
				Code: "ValidationError",
				Doc:  commonErrors,
			},
			{
				Rule:  "StepConcurrencyLevel is at most 256",
				Break: runCall(func(in *emr.RunJobFlowInput) { in.StepConcurrencyLevel = aws.Int32(maxStepConcurrency + 1) }),
				Code:  "ValidationError",
				Doc:   commonErrors,
			},
			runDefault("StepConcurrencyLevel defaults to 1", "", nil, requireCluster(func(c *types.Cluster) bool {
				return aws.ToInt32(c.StepConcurrencyLevel) == 1
			})),
			runDefault("VisibleToAllUsers defaults to true", "", nil, requireCluster(func(c *types.Cluster) bool {
				return aws.ToBool(c.VisibleToAllUsers)
			})),
			{
				Rule: "Instances sets InstanceGroups or InstanceFleets",
				Doc:  apiReference + "API_JobFlowInstancesConfig.html",
				Break: runCall(func(in *emr.RunJobFlowInput) {
					in.Instances = &types.JobFlowInstancesConfig{Ec2SubnetId: aws.String("subnet-0a1")}
				}),
				Code: "ValidationError",
			},
			{
				Rule: "Instances can't set both InstanceGroups and InstanceFleets",
				Break: runCall(func(in *emr.RunJobFlowInput) {
					groups := in.Instances.InstanceGroups
					withFleets(in)
					in.Instances.InstanceGroups = groups
				}),
				Code: "ValidationError",
				Doc:  commonErrors,
			},
			{
				Rule:  "EmrManagedMasterSecurityGroup and EmrManagedSlaveSecurityGroup are set together",
				Doc:   apiReference + "API_JobFlowInstancesConfig.html",
				Break: runCall(func(in *emr.RunJobFlowInput) { in.Instances.EmrManagedMasterSecurityGroup = aws.String("sg-0a1") }),
				Code:  "ValidationError",
			},
			runDefault("KeepJobFlowAliveWhenNoSteps defaults to false, so the cluster terminates when its steps finish", apiReference+"API_JobFlowInstancesConfig.html", nil,
				requireCluster(func(c *types.Cluster) bool { return aws.ToBool(c.AutoTerminate) })),
			runDefault("termination protection is off unless TerminationProtected is true", terminationDoc, nil,
				requireCluster(func(c *types.Cluster) bool { return c.TerminationProtected != nil && !*c.TerminationProtected })),
			runDefault("unhealthy node replacement is on unless UnhealthyNodeReplacement is set", managementGuide+"emr-plan-node-replacement.html", nil,
				requireCluster(func(c *types.Cluster) bool { return aws.ToBool(c.UnhealthyNodeReplacement) })),
			{
				Rule:  "an instance group sets InstanceCount, InstanceRole, and InstanceType",
				Doc:   apiReference + "API_InstanceGroupConfig.html",
				Break: runCall(func(in *emr.RunJobFlowInput) { in.Instances.InstanceGroups[1].InstanceCount = nil }),
				Code:  "ValidationError",
			},
			{
				Rule: "a master instance fleet sets one of TargetSpotCapacity and TargetOnDemandCapacity, to 1",
				Doc:  apiReference + "API_InstanceFleetConfig.html",
				Break: runCall(func(in *emr.RunJobFlowInput) {
					withFleets(in)
					in.Instances.InstanceFleets[0].TargetOnDemandCapacity = aws.Int32(2)
				}),
				Code: "ValidationError",
			},
			{
				Rule: "an instance fleet has a target capacity greater than 0",
				Doc:  apiReference + "API_InstanceFleetConfig.html",
				Break: runCall(func(in *emr.RunJobFlowInput) {
					withFleets(in)
					coreFleet(in).TargetSpotCapacity = aws.Int32(0)
				}),
				Code: "ValidationError",
			},
			{
				Rule: "an instance fleet has at most 5 instance type configurations without an allocation strategy",
				Doc:  apiReference + "API_InstanceTypeConfig.html",
				Break: runCall(func(in *emr.RunJobFlowInput) {
					withFleets(in)
					coreFleet(in).InstanceTypeConfigs = typeConfigs(maxTypeConfigs + 1)
				}),
				Code: "ValidationError",
			},
			{
				Rule: "an instance fleet has at most 30 instance type configurations with an allocation strategy",
				Doc:  apiReference + "API_InstanceTypeConfig.html",
				Break: runCall(func(in *emr.RunJobFlowInput) {
					withFleets(in)
					coreFleet(in).LaunchSpecifications = &types.InstanceFleetProvisioningSpecifications{SpotSpecification: &types.SpotProvisioningSpecification{
						AllocationStrategy: types.SpotProvisioningAllocationStrategyCapacityOptimized, TimeoutAction: types.SpotProvisioningTimeoutActionSwitchToOnDemand, TimeoutDurationMinutes: aws.Int32(10),
					}}
					coreFleet(in).InstanceTypeConfigs = typeConfigs(maxStrategyConfigs + 1)
				}),
				Code: "ValidationError",
			},
			{
				Rule: "WeightedCapacity is 1 for a master instance fleet",
				Doc:  apiReference + "API_InstanceTypeConfig.html",
				Break: runCall(func(in *emr.RunJobFlowInput) {
					withFleets(in)
					in.Instances.InstanceFleets[0].InstanceTypeConfigs[0].WeightedCapacity = aws.Int32(2)
				}),
				Code: "ValidationError",
			},
			{
				Rule: "WeightedCapacity is at least 1 for a core or task instance fleet",
				Doc:  apiReference + "API_InstanceTypeConfig.html",
				Break: runCall(func(in *emr.RunJobFlowInput) {
					withFleets(in)
					coreFleet(in).InstanceTypeConfigs[0].WeightedCapacity = aws.Int32(0)
				}),
				Code: "ValidationError",
			},
			runDefault("WeightedCapacity defaults to 1", apiReference+"API_InstanceTypeConfig.html", withFleets,
				requireTypeConfig(func(s types.InstanceTypeSpecification) bool { return aws.ToInt32(s.WeightedCapacity) == 1 })),
			runDefault("BidPriceAsPercentageOfOnDemandPrice defaults to 100 when neither it nor BidPrice is set", apiReference+"API_InstanceTypeConfig.html", withFleets,
				requireTypeConfig(func(s types.InstanceTypeSpecification) bool {
					return aws.ToFloat64(s.BidPriceAsPercentageOfOnDemandPrice) == 100
				})),
			{
				Rule: "an EBS volume is 1 to 1024 GiB",
				Doc:  apiReference + "API_VolumeSpecification.html",
				Break: runCall(func(in *emr.RunJobFlowInput) {
					in.Instances.InstanceGroups[1].EbsConfiguration = &types.EbsConfiguration{EbsBlockDeviceConfigs: []types.EbsBlockDeviceConfig{
						{VolumeSpecification: &types.VolumeSpecification{SizeInGB: aws.Int32(maxVolumeSize + 1), VolumeType: aws.String("gp3")}},
					}}
				}),
				Code: "ValidationError",
			},
			{
				Rule: "an EBS volume type is gp3, gp2, io1, st1, sc1, or standard",
				Doc:  apiReference + "API_VolumeSpecification.html",
				Break: runCall(func(in *emr.RunJobFlowInput) {
					in.Instances.InstanceGroups[1].EbsConfiguration = &types.EbsConfiguration{EbsBlockDeviceConfigs: []types.EbsBlockDeviceConfig{
						{VolumeSpecification: &types.VolumeSpecification{SizeInGB: aws.Int32(32), VolumeType: aws.String("io2")}},
					}}
				}),
				Code: "ValidationError",
			},
			{
				Rule: "an EBS volume's Throughput is 125 to 1000 MiB/s, and only for gp3",
				Doc:  apiReference + "API_VolumeSpecification.html",
				Break: runCall(func(in *emr.RunJobFlowInput) {
					withFleets(in)
					coreFleet(in).InstanceTypeConfigs[0].EbsConfiguration = &types.EbsConfiguration{EbsBlockDeviceConfigs: []types.EbsBlockDeviceConfig{
						{VolumeSpecification: &types.VolumeSpecification{SizeInGB: aws.Int32(32), VolumeType: aws.String("gp2"), Throughput: aws.Int32(250)}},
					}}
				}),
				Code: "ValidationError",
			},
			{
				Rule: "an auto-termination policy's IdleTimeout is 60 to 604800 seconds",
				Doc:  apiReference + "API_AutoTerminationPolicy.html",
				Break: runCall(func(in *emr.RunJobFlowInput) {
					in.AutoTerminationPolicy = &types.AutoTerminationPolicy{IdleTimeout: aws.Int64(minIdleTimeout - 1)}
				}),
				Code: "ValidationError",
			},
			runDefault("an auto-termination policy's IdleTimeout defaults to one hour", managementGuide+"emr-auto-termination-policy.html",
				func(in *emr.RunJobFlowInput) { in.AutoTerminationPolicy = &types.AutoTerminationPolicy{} }, requireIdleTimeout),
			{
				Rule: "a managed scaling policy's ComputeLimits sets MaximumCapacityUnits, MinimumCapacityUnits, and UnitType",
				Doc:  apiReference + "API_ComputeLimits.html",
				Break: runCall(func(in *emr.RunJobFlowInput) {
					in.ManagedScalingPolicy = scalingPolicy()
					in.ManagedScalingPolicy.ComputeLimits.UnitType = ""
				}),
				Code: "ValidationError",
			},
			{
				Rule: "a managed scaling policy's UtilizationPerformanceIndex is 1 to 100",
				Doc:  apiReference + "API_ManagedScalingPolicy.html",
				Break: runCall(func(in *emr.RunJobFlowInput) {
					in.ManagedScalingPolicy = scalingPolicy()
					in.ManagedScalingPolicy.UtilizationPerformanceIndex = aws.Int32(101)
				}),
				Code: "ValidationError",
			},
			runDefault("a managed scaling policy's MaximumOnDemandCapacityUnits and MaximumCoreCapacityUnits default to MaximumCapacityUnits",
				managementGuide+"emr-managed-scaling.html", func(in *emr.RunJobFlowInput) { in.ManagedScalingPolicy = scalingPolicy() }, requireScalingDefaults),
			{
				Rule: "a tag has a key of up to 128 characters and a value of up to 256",
				Doc:  apiReference + "API_AddTags.html",
				Break: runCall(func(in *emr.RunJobFlowInput) {
					in.Tags = []types.Tag{{Key: aws.String(wide(maxTagKey + 1)), Value: aws.String("")}}
				}),
				Code: "ValidationError",
			},
			{
				Rule: "the tag limits count characters, so a key of 128 and a value of 256 non-ASCII characters are accepted",
				Doc:  apiReference + "API_AddTags.html",
				Check: runCall(func(in *emr.RunJobFlowInput) {
					in.Tags = []types.Tag{{Key: aws.String(wide(maxTagKey)), Value: aws.String(wide(maxTagValue))}}
				}),
			},
		},
		Unclear: "The page names no error for a request that breaks a rule. The fake returns ValidationError, which the " +
			"Common Errors page gives for input that doesn't meet constraints. RunJobFlow takes no idempotency token. " +
			"The docs say instance fleets need release 4.8.0 or later but not 5.0.x, and that auto-termination, managed " +
			"scaling, log encryption, custom AMIs, EBS root volume IOPS and throughput, and placement group roles other " +
			"than MASTER depend on the release, but not whether AWS rejects them on other releases; the fake accepts " +
			"them. The schema has no AmiVersion, so the handler can't send it with ReleaseLabel. The page says " +
			"VisibleToAllUsers false has no effect, but not whether DescribeCluster then reports false; the fake reports " +
			"what was sent. A cluster without KeepJobFlowAliveWhenNoSteps stops once its steps finish; the fake keeps it " +
			"WAITING. The page and the Management Guide disagree on ScaleDownBehavior's default, so the fake sets none. " +
			"The docs don't say whether DescribeCluster lists applications EMR installs that the request doesn't name, " +
			"or their versions; the fake returns the request's. For release 7.0 and lower, the Management Guide says " +
			"unhealthy node replacement's default depends on termination protection in some clusters; the fake turns it " +
			"on. The docs give no limit on the number of tags.",
	},
	"DescribeCluster": {
		Doc: apiReference + "API_DescribeCluster.html",
		Rules: []nativetest.Rule{idRule(func(ctx context.Context, f *fakeAPI, id string) error {
			_, err := describeCluster(ctx, f, id)
			return err
		})},
		Unclear: "The page names InvalidRequestException for wrong user input without listing the cases. The fake returns " +
			"it for a cluster ID it doesn't know, which the handler reads as not found. AWS describes an ended cluster " +
			"for about two months; the fake describes it as TERMINATED.",
	},
	"ListClusters": {
		Doc: apiReference + "API_ListClusters.html",
		Rules: []nativetest.Rule{{
			Rule: "AWS returns at most 50 clusters per call, with a Marker for the rest",
			Check: func(ctx context.Context, store *awsfake.CloudControl) error {
				f := newFakeAPI(store)
				for range 2 {
					if _, err := run(ctx, f, nil); err != nil {
						return err
					}
				}
				out, err := f.ListClusters(ctx, &emr.ListClustersInput{})
				if err != nil {
					return err
				}
				if len(out.Clusters) > 50 || out.Marker == nil {
					return errors.New("the fake returned every cluster on one page")
				}
				return nil
			},
		}},
		Unclear: "The page says clusters that change state during the call may not be listed as expected; the fake lists " +
			"each cluster by its state when called.",
	},
	"ListInstanceGroups": {
		Doc: apiReference + "API_ListInstanceGroups.html",
		Rules: []nativetest.Rule{
			idRule(func(ctx context.Context, f *fakeAPI, id string) error {
				_, err := f.ListInstanceGroups(ctx, &emr.ListInstanceGroupsInput{ClusterId: aws.String(id)})
				return err
			}),
			pagesRule(withTasks, func(ctx context.Context, f *fakeAPI, id string) (*string, error) {
				out, err := f.ListInstanceGroups(ctx, &emr.ListInstanceGroupsInput{ClusterId: aws.String(id)})
				if err != nil {
					return nil, err
				}
				return out.Marker, nil
			}),
		},
		Unclear: "The page gives no page size; the fake returns two groups per page.",
	},
	"ListInstanceFleets": {
		Doc: apiReference + "API_ListInstanceFleets.html",
		Rules: []nativetest.Rule{
			idRule(func(ctx context.Context, f *fakeAPI, id string) error {
				_, err := f.ListInstanceFleets(ctx, &emr.ListInstanceFleetsInput{ClusterId: aws.String(id)})
				return err
			}),
			pagesRule(withTaskFleet, func(ctx context.Context, f *fakeAPI, id string) (*string, error) {
				out, err := f.ListInstanceFleets(ctx, &emr.ListInstanceFleetsInput{ClusterId: aws.String(id)})
				if err != nil {
					return nil, err
				}
				return out.Marker, nil
			}),
		},
		Unclear: "The page gives no page size; the fake returns two fleets per page.",
	},
	"GetManagedScalingPolicy": {
		Doc: apiReference + "API_GetManagedScalingPolicy.html",
		Rules: []nativetest.Rule{idRule(func(ctx context.Context, f *fakeAPI, id string) error {
			_, err := f.GetManagedScalingPolicy(ctx, &emr.GetManagedScalingPolicyInput{ClusterId: aws.String(id)})
			return err
		})},
		Unclear: "The page doesn't say what AWS returns for a cluster without a policy; the fake returns no policy.",
	},
	"GetAutoTerminationPolicy": {
		Doc: apiReference + "API_GetAutoTerminationPolicy.html",
		Rules: []nativetest.Rule{idRule(func(ctx context.Context, f *fakeAPI, id string) error {
			_, err := f.GetAutoTerminationPolicy(ctx, &emr.GetAutoTerminationPolicyInput{ClusterId: aws.String(id)})
			return err
		})},
		Unclear: "The page doesn't say what AWS returns for a cluster without a policy; the fake returns no policy.",
	},
	"ModifyCluster": {
		Doc: apiReference + "API_ModifyCluster.html",
		Rules: []nativetest.Rule{{
			Rule: "StepConcurrencyLevel is 1 to 256",
			Break: onCluster(func(ctx context.Context, f *fakeAPI, id string) error {
				_, err := f.ModifyCluster(ctx, &emr.ModifyClusterInput{ClusterId: aws.String(id), StepConcurrencyLevel: aws.Int32(0)})
				return err
			}),
			Code: "ValidationError",
			Doc:  commonErrors,
		}},
		Unclear: "The page recommends not changing StepConcurrencyLevel while steps run, without saying AWS rejects it; the " +
			"fake accepts it. It lists InvalidRequestException for wrong user input; the fake returns ValidationError for " +
			"a value out of range, as the Common Errors page gives.",
	},
	"ModifyInstanceGroups": {
		Doc: apiReference + "API_ModifyInstanceGroups.html",
		Rules: []nativetest.Rule{
			idRule(func(ctx context.Context, f *fakeAPI, id string) error {
				return modifyGroups(ctx, f, id, aws.String("ig-core"))
			}),
			{
				Rule: "InstanceGroupId is required",
				Doc:  apiReference + "API_InstanceGroupModifyConfig.html",
				Break: onCluster(func(ctx context.Context, f *fakeAPI, id string) error {
					return modifyGroups(ctx, f, id, nil)
				}),
				Code: "ValidationError",
			},
			{
				Rule: "a cluster must be running to resize: one that is still starting can't",
				Doc:  managementGuide + "emr-manage-resize.html",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					id, err := run(ctx, f, nil)
					if err != nil {
						return err
					}
					if err := requireFails(modifyGroups(ctx, f, id, aws.String("ig-core")), "a resize of a starting cluster"); err != nil {
						return err
					}
					if _, err := describeCluster(ctx, f, id); err != nil {
						return err
					}
					return modifyGroups(ctx, f, id, aws.String("ig-core"))
				},
			},
		},
		Unclear: "The docs don't name the error for a cluster that isn't running; the fake returns ValidationException. " +
			"The Management Guide says you can add core nodes but describes removing only task nodes, and that graceful " +
			"reduction doesn't take core nodes below the HDFS replication factor, without saying whether the call fails; " +
			"the fake accepts any count. It only knows the group IDs its ListInstanceGroups returns.",
	},
	"RemoveAutoScalingPolicy": {
		Doc: apiReference + "API_RemoveAutoScalingPolicy.html",
		Rules: []nativetest.Rule{
			idRule(func(ctx context.Context, f *fakeAPI, id string) error {
				_, err := f.RemoveAutoScalingPolicy(ctx, &emr.RemoveAutoScalingPolicyInput{ClusterId: aws.String(id), InstanceGroupId: aws.String("ig-core")})
				return err
			}),
			{
				Rule: "InstanceGroupId is at most 256 characters",
				Break: onCluster(func(ctx context.Context, f *fakeAPI, id string) error {
					_, err := f.RemoveAutoScalingPolicy(ctx, &emr.RemoveAutoScalingPolicyInput{ClusterId: aws.String(id), InstanceGroupId: aws.String(longID)})
					return err
				}),
				Code: "ValidationError",
				Doc:  commonErrors,
			},
		},
		Unclear: "The page doesn't say what AWS does for a group without a policy; the fake accepts it and changes nothing.",
	},
	"SetVisibleToAllUsers": {
		Doc: apiReference + "API_SetVisibleToAllUsers.html",
		Rules: jobFlowRules("VisibleToAllUsers", func(ctx context.Context, f *fakeAPI, ids []string, value *bool) error {
			_, err := f.SetVisibleToAllUsers(ctx, &emr.SetVisibleToAllUsersInput{JobFlowIds: ids, VisibleToAllUsers: value})
			return err
		}),
		Unclear: "The page says the setting is no longer supported and the call works on running clusters, without naming " +
			"an error for other states or saying whether false still takes effect. The fake accepts it until the cluster " +
			"ends and stores the value.",
	},
	"SetTerminationProtection": {
		Doc: apiReference + "API_SetTerminationProtection.html",
		Rules: append(jobFlowRules("TerminationProtected", protect), nativetest.Rule{
			Rule: "a protected cluster can be shut down once the call sets TerminationProtected to false",
			Check: func(ctx context.Context, store *awsfake.CloudControl) error {
				f := newFakeAPI(store)
				id, err := started(ctx, f, nil)
				if err != nil {
					return err
				}
				if err := protect(ctx, f, []string{id}, aws.Bool(true)); err != nil {
					return err
				}
				if err := requireFails(terminate(ctx, f, []string{id}), "a shutdown of a protected cluster"); err != nil {
					return err
				}
				if err := protect(ctx, f, []string{id}, aws.Bool(false)); err != nil {
					return err
				}
				return terminate(ctx, f, []string{id})
			},
		}),
		Unclear: "The page doesn't say which cluster states allow the call; the fake allows any until the cluster ends.",
	},
	"SetUnhealthyNodeReplacement": {
		Doc: apiReference + "API_SetUnhealthyNodeReplacement.html",
		Rules: jobFlowRules("UnhealthyNodeReplacement", func(ctx context.Context, f *fakeAPI, ids []string, value *bool) error {
			_, err := f.SetUnhealthyNodeReplacement(ctx, &emr.SetUnhealthyNodeReplacementInput{JobFlowIds: ids, UnhealthyNodeReplacement: value})
			return err
		}),
		Unclear: "The page doesn't say which cluster states allow the call; the fake allows any until the cluster ends.",
	},
	"PutManagedScalingPolicy": {
		Doc: apiReference + "API_PutManagedScalingPolicy.html",
		Rules: []nativetest.Rule{
			idRule(func(ctx context.Context, f *fakeAPI, id string) error {
				_, err := f.PutManagedScalingPolicy(ctx, &emr.PutManagedScalingPolicyInput{ClusterId: aws.String(id), ManagedScalingPolicy: scalingPolicy()})
				return err
			}),
			{
				Rule: "ManagedScalingPolicy is required",
				Break: onCluster(func(ctx context.Context, f *fakeAPI, id string) error {
					_, err := f.PutManagedScalingPolicy(ctx, &emr.PutManagedScalingPolicyInput{ClusterId: aws.String(id)})
					return err
				}),
				Code: "ValidationError",
				Doc:  commonErrors,
			},
			{
				Rule: "ComputeLimits sets MaximumCapacityUnits, MinimumCapacityUnits, and UnitType",
				Doc:  apiReference + "API_ComputeLimits.html",
				Break: onCluster(func(ctx context.Context, f *fakeAPI, id string) error {
					policy := scalingPolicy()
					policy.ComputeLimits.MinimumCapacityUnits = nil
					_, err := f.PutManagedScalingPolicy(ctx, &emr.PutManagedScalingPolicyInput{ClusterId: aws.String(id), ManagedScalingPolicy: policy})
					return err
				}),
				Code: "ValidationError",
			},
			{
				Rule: "MaximumOnDemandCapacityUnits and MaximumCoreCapacityUnits default to MaximumCapacityUnits",
				Doc:  managementGuide + "emr-managed-scaling.html",
				Check: onCluster(func(ctx context.Context, f *fakeAPI, id string) error {
					_, err := f.PutManagedScalingPolicy(ctx, &emr.PutManagedScalingPolicyInput{ClusterId: aws.String(id), ManagedScalingPolicy: scalingPolicy()})
					if err != nil {
						return err
					}
					return requireScalingDefaults(ctx, f, id)
				}),
			},
		},
		Unclear: "The docs don't say MinimumCapacityUnits must be at most MaximumCapacityUnits, or name the error for a " +
			"release without managed scaling; the fake accepts both.",
	},
	"RemoveManagedScalingPolicy": {
		Doc: apiReference + "API_RemoveManagedScalingPolicy.html",
		Rules: []nativetest.Rule{idRule(func(ctx context.Context, f *fakeAPI, id string) error {
			_, err := f.RemoveManagedScalingPolicy(ctx, &emr.RemoveManagedScalingPolicyInput{ClusterId: aws.String(id)})
			return err
		})},
		Unclear: "The page doesn't say what AWS does for a cluster without a policy; the fake accepts it.",
	},
	"PutAutoTerminationPolicy": {
		Doc: apiReference + "API_PutAutoTerminationPolicy.html",
		Rules: []nativetest.Rule{
			idRule(func(ctx context.Context, f *fakeAPI, id string) error {
				_, err := f.PutAutoTerminationPolicy(ctx, &emr.PutAutoTerminationPolicyInput{ClusterId: aws.String(id)})
				return err
			}),
			{
				Rule: "IdleTimeout is 60 to 604800 seconds",
				Doc:  apiReference + "API_AutoTerminationPolicy.html",
				Break: onCluster(func(ctx context.Context, f *fakeAPI, id string) error {
					_, err := f.PutAutoTerminationPolicy(ctx, &emr.PutAutoTerminationPolicyInput{ClusterId: aws.String(id), AutoTerminationPolicy: &types.AutoTerminationPolicy{IdleTimeout: aws.Int64(maxIdleTimeout + 1)}})
					return err
				}),
				Code: "ValidationError",
			},
			{
				Rule: "IdleTimeout defaults to one hour",
				Doc:  managementGuide + "emr-auto-termination-policy.html",
				Check: onCluster(func(ctx context.Context, f *fakeAPI, id string) error {
					_, err := f.PutAutoTerminationPolicy(ctx, &emr.PutAutoTerminationPolicyInput{ClusterId: aws.String(id), AutoTerminationPolicy: &types.AutoTerminationPolicy{}})
					if err != nil {
						return err
					}
					return requireIdleTimeout(ctx, f, id)
				}),
			},
		},
		Unclear: "The page doesn't say what AWS does when AutoTerminationPolicy is left out; the fake removes the policy. " +
			"The handler always sends one. The note that auto-termination needs release 5.30.0 or 6.1.0 or later names " +
			"no error; the fake accepts any release.",
	},
	"RemoveAutoTerminationPolicy": {
		Doc: apiReference + "API_RemoveAutoTerminationPolicy.html",
		Rules: []nativetest.Rule{idRule(func(ctx context.Context, f *fakeAPI, id string) error {
			_, err := f.RemoveAutoTerminationPolicy(ctx, &emr.RemoveAutoTerminationPolicyInput{ClusterId: aws.String(id)})
			return err
		})},
		Unclear: "The page doesn't say what AWS does for a cluster without a policy; the fake accepts it.",
	},
	"AddTags": {
		Doc: apiReference + "API_AddTags.html",
		Rules: []nativetest.Rule{
			{
				Rule: "Tags is required",
				Break: onCluster(func(ctx context.Context, f *fakeAPI, id string) error {
					_, err := f.AddTags(ctx, &emr.AddTagsInput{ResourceId: aws.String(id)})
					return err
				}),
				Code: "ValidationError",
				Doc:  commonErrors,
			},
			{
				Rule: "a tag has a key of up to 128 characters and a value of up to 256",
				Break: onCluster(func(ctx context.Context, f *fakeAPI, id string) error {
					return addTag(ctx, f, id, "team", wide(maxTagValue+1))
				}),
				Code: "ValidationError",
				Doc:  commonErrors,
			},
			{
				Rule: "the tag limits count characters, so a key of 128 and a value of 256 non-ASCII characters are accepted",
				Check: onCluster(func(ctx context.Context, f *fakeAPI, id string) error {
					return addTag(ctx, f, id, wide(maxTagKey), wide(maxTagValue))
				}),
			},
			terminatedTagsRule(func(ctx context.Context, f *fakeAPI, id string) error {
				return addTag(ctx, f, id, "env", "dev")
			}),
		},
		Unclear: "The docs don't name the error for a terminated cluster; the fake returns ValidationException. They give " +
			"no limit on the number of tags.",
	},
	"RemoveTags": {
		Doc: apiReference + "API_RemoveTags.html",
		Rules: []nativetest.Rule{
			{
				Rule: "TagKeys is required",
				Break: onCluster(func(ctx context.Context, f *fakeAPI, id string) error {
					_, err := f.RemoveTags(ctx, &emr.RemoveTagsInput{ResourceId: aws.String(id)})
					return err
				}),
				Code: "ValidationError",
				Doc:  commonErrors,
			},
			terminatedTagsRule(func(ctx context.Context, f *fakeAPI, id string) error {
				_, err := f.RemoveTags(ctx, &emr.RemoveTagsInput{ResourceId: aws.String(id), TagKeys: []string{"team"}})
				return err
			}),
		},
		Unclear: "The docs don't name the error for a terminated cluster; the fake returns ValidationException. The page " +
			"doesn't say what AWS does for a key the resource doesn't have; the fake accepts it.",
	},
	"TerminateJobFlows": {
		Doc: apiReference + "API_TerminateJobFlows.html",
		Rules: []nativetest.Rule{
			{
				Rule: "a call shuts down at most 10 clusters",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					ids := []string{}
					for range maxTerminate + 1 {
						id, err := started(ctx, f, nil)
						if err != nil {
							return err
						}
						ids = append(ids, id)
					}
					return terminate(ctx, f, ids)
				},
				Code: "ValidationError",
				Doc:  commonErrors,
			},
			{
				Rule: "each of JobFlowIds is at most 10280 characters",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return terminate(ctx, newFakeAPI(store), []string{longJobFlowID})
				},
				Code: "ValidationError",
				Doc:  commonErrors,
			},
			{
				Rule: "AWS returns an error for a cluster with termination protection, and the cluster keeps running",
				Doc:  terminationDoc,
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					id, err := started(ctx, f, func(in *emr.RunJobFlowInput) {
						in.Instances.TerminationProtected = aws.Bool(true)
					})
					if err != nil {
						return err
					}
					if err := requireFails(terminate(ctx, f, []string{id}), "a shutdown of a protected cluster"); err != nil {
						return err
					}
					c, err := describeCluster(ctx, f, id)
					if err != nil {
						return err
					}
					if s := c.Status.State; s != types.ClusterStateWaiting {
						return fmt.Errorf("the cluster is %s", s)
					}
					return nil
				},
			},
		},
		Unclear: "The docs say AWS returns an error for a protected cluster without naming it; the fake returns " +
			"ValidationException with AWS's message. The page doesn't say what AWS does for a cluster that already ended; " +
			"the fake returns ValidationException for an ID it no longer has.",
	},
}
