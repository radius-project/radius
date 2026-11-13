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

package dax

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dax"
	"github.com/aws/aws-sdk-go-v2/service/dax/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	apiReference  = "https://docs.aws.amazon.com/amazondynamodb/latest/APIReference/"
	createPage    = apiReference + "API_dax_CreateCluster.html"
	tagPage       = apiReference + "API_dax_Tag.html"
	developer     = "https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/"
	clusterGuide  = developer + "DAX.concepts.cluster.html"
	settingsGuide = developer + "DAX.cluster-management.html#DAX.cluster-management.custom-settings"
	stackTemplate = "https://docs.aws.amazon.com/AWSCloudFormation/latest/TemplateReference/aws-resource-dax-cluster.html"
)

// createInput returns a valid CreateCluster input for a three-node cluster named app-dax, changed by change.
func createInput(change func(in *dax.CreateClusterInput)) *dax.CreateClusterInput {
	in := &dax.CreateClusterInput{
		ClusterName:       aws.String("app-dax"),
		NodeType:          aws.String("dax.t3.small"),
		ReplicationFactor: 3,
		IamRoleArn:        aws.String("arn:aws:iam::123456789012:role/dax"),
		AvailabilityZones: []string{"us-west-2a", "us-west-2b", "us-west-2c"},
	}
	if change != nil {
		change(in)
	}
	return in
}

// create creates a cluster from createInput(change) with a fake DAX API backed by store, and returns the API.
func create(ctx context.Context, store *awsfake.CloudControl, change func(in *dax.CreateClusterInput)) (*fakeAPI, error) {
	f := newFakeAPI(store)
	_, err := f.CreateCluster(ctx, createInput(change))
	return f, err
}

// describe returns the cluster with the name.
func (f *fakeAPI) describe(ctx context.Context, name string) (types.Cluster, error) {
	out, err := f.DescribeClusters(ctx, &dax.DescribeClustersInput{ClusterNames: []string{name}})
	if err != nil {
		return types.Cluster{}, err
	}
	return out.Clusters[0], nil
}

// available creates the cluster app-dax from createInput(change) and reads it until it is available.
func available(ctx context.Context, store *awsfake.CloudControl, change func(in *dax.CreateClusterInput)) (*fakeAPI, error) {
	f, err := create(ctx, store, change)
	if err != nil {
		return nil, err
	}
	for range 5 {
		c, err := f.describe(ctx, "app-dax")
		if err != nil {
			return nil, err
		}
		if aws.ToString(c.Status) == "available" {
			return f, nil
		}
	}
	return nil, fmt.Errorf("the cluster isn't available")
}

// createBreak returns a Break that creates a cluster from createInput(change).
func createBreak(change func(in *dax.CreateClusterInput)) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		_, err := create(ctx, store, change)
		return err
	}
}

// named returns a change of createInput that sets the cluster name.
func named(name string) func(in *dax.CreateClusterInput) {
	return func(in *dax.CreateClusterInput) { in.ClusterName = aws.String(name) }
}

// windowBreak returns a Break that makes call with the maintenance window, on an available cluster.
func windowBreak(window string, call func(ctx context.Context, f *fakeAPI, window *string) error) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		f, err := available(ctx, store, nil)
		if err != nil {
			return err
		}
		return call(ctx, f, aws.String(window))
	}
}

// createWithWindow creates a second cluster with the maintenance window.
func createWithWindow(ctx context.Context, f *fakeAPI, window *string) error {
	_, err := f.CreateCluster(ctx, createInput(func(in *dax.CreateClusterInput) {
		in.ClusterName = aws.String("other")
		in.PreferredMaintenanceWindow = window
	}))
	return err
}

// updateWithWindow updates app-dax's maintenance window.
func updateWithWindow(ctx context.Context, f *fakeAPI, window *string) error {
	_, err := f.UpdateCluster(ctx, &dax.UpdateClusterInput{ClusterName: aws.String("app-dax"), PreferredMaintenanceWindow: window})
	return err
}

// windowRules returns the rules CreateCluster's page gives for a maintenance window, which call sets.
func windowRules(call func(ctx context.Context, f *fakeAPI, window *string) error, doc string) []nativetest.Rule {
	return []nativetest.Rule{
		{
			Rule:  "PreferredMaintenanceWindow is ddd:hh24:mi-ddd:hh24:mi, where ddd is sun, mon, tue, wed, thu, fri, or sat",
			Break: windowBreak("sun:05:00-day:06:00", call),
			Code:  "InvalidParameterValueException",
			Doc:   doc,
		},
		{
			Rule:  "the maintenance window is at least 60 minutes",
			Break: windowBreak("sun:05:00-sun:05:30", call),
			Code:  "InvalidParameterValueException",
			Doc:   doc,
		},
		{
			Rule:  "the maintenance window is at most 24 hours",
			Break: windowBreak("sun:05:00-mon:06:00", call),
			Code:  "InvalidParameterValueException",
			Doc:   clusterGuide + "#DAX.concepts.maintenance-window",
		},
	}
}

// stateRule returns the rule that a call needs the cluster available: call is made right after the create, while the
// cluster is creating.
func stateRule(call func(ctx context.Context, f *fakeAPI) error) nativetest.Rule {
	return nativetest.Rule{
		Rule: "the cluster is available",
		Break: func(ctx context.Context, store *awsfake.CloudControl) error {
			f, err := create(ctx, store, nil)
			if err != nil {
				return err
			}
			return call(ctx, f)
		},
		Code: "InvalidClusterStateFault",
	}
}

// missingRule returns the rule that the cluster call names exists.
func missingRule(call func(ctx context.Context, f *fakeAPI) error) nativetest.Rule {
	return nativetest.Rule{
		Rule: "the cluster exists",
		Break: func(ctx context.Context, store *awsfake.CloudControl) error {
			return call(ctx, newFakeAPI(store))
		},
		Code: "ClusterNotFoundFault",
	}
}

// tagCallRules returns the rules of a tag call: ResourceName is the ARN of a cluster that exists, and, when
// needsAvailable is set, the cluster is available.
func tagCallRules(needsAvailable bool, call func(ctx context.Context, f *fakeAPI, resource string) error) []nativetest.Rule {
	rules := []nativetest.Rule{
		{
			Rule: "ResourceName is the cluster's ARN, not its name",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				f, err := available(ctx, store, nil)
				if err != nil {
					return err
				}
				return call(ctx, f, "app-dax")
			},
			Code: "InvalidARNFault",
		},
		missingRule(func(ctx context.Context, f *fakeAPI) error { return call(ctx, f, arnPrefix+"missing") }),
	}
	if needsAvailable {
		rules = append(rules, stateRule(func(ctx context.Context, f *fakeAPI) error { return call(ctx, f, arnPrefix+"app-dax") }))
	}
	return rules
}

// tagCluster adds tags to app-dax.
func tagCluster(ctx context.Context, f *fakeAPI, tags ...types.Tag) error {
	_, err := f.TagResource(ctx, &dax.TagResourceInput{ResourceName: aws.String(arnPrefix + "app-dax"), Tags: tags})
	return err
}

func tag(key string, value string) types.Tag {
	return types.Tag{Key: aws.String(key), Value: aws.String(value)}
}

// tags returns n tags.
func tags(n int) []types.Tag {
	out := []types.Tag{}
	for i := range n {
		out = append(out, tag(fmt.Sprintf("k%d", i), "v"))
	}
	return out
}

// clusterTags returns the tags of app-dax.
func clusterTags(ctx context.Context, f *fakeAPI) (map[string]string, error) {
	out, err := f.ListTags(ctx, &dax.ListTagsInput{ResourceName: aws.String(arnPrefix + "app-dax")})
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	for _, t := range out.Tags {
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return m, nil
}

func updateApp(ctx context.Context, f *fakeAPI, in *dax.UpdateClusterInput) error {
	in.ClusterName = aws.String("app-dax")
	_, err := f.UpdateCluster(ctx, in)
	return err
}

func increaseApp(ctx context.Context, f *fakeAPI, factor int32) error {
	_, err := f.IncreaseReplicationFactor(ctx, &dax.IncreaseReplicationFactorInput{ClusterName: aws.String("app-dax"), NewReplicationFactor: factor})
	return err
}

func decreaseApp(ctx context.Context, f *fakeAPI, in *dax.DecreaseReplicationFactorInput) error {
	in.ClusterName = aws.String("app-dax")
	_, err := f.DecreaseReplicationFactor(ctx, in)
	return err
}

// onAvailable returns a Break that makes call on the available three-node cluster app-dax.
func onAvailable(call func(ctx context.Context, f *fakeAPI) error) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		f, err := available(ctx, store, nil)
		if err != nil {
			return err
		}
		return call(ctx, f)
	}
}

// clusterAPI records the DAX calls Cluster makes.
var clusterAPI = map[string]nativetest.Call{
	"CreateCluster": {
		Doc: createPage,
		Rules: append([]nativetest.Rule{
			{
				Rule:  "ClusterName has at most 20 characters",
				Break: createBreak(named("a" + strings.Repeat("b", 20))),
				Code:  "InvalidParameterValueException",
			},
			{
				Rule:  "ClusterName starts with a letter",
				Break: createBreak(named("1-dax")),
				Code:  "InvalidParameterValueException",
			},
			{
				Rule:  "ClusterName has only letters, digits, and hyphens",
				Break: createBreak(named("app_dax")),
				Code:  "InvalidParameterValueException",
			},
			{
				Rule:  "ClusterName doesn't end with a hyphen",
				Break: createBreak(named("app-")),
				Code:  "InvalidParameterValueException",
			},
			{
				Rule:  "ClusterName has no two hyphens in a row",
				Break: createBreak(named("app--dax")),
				Code:  "InvalidParameterValueException",
			},
			{
				Rule: "DAX stores ClusterName in lowercase",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f, err := create(ctx, store, named("App-DAX"))
					if err != nil {
						return err
					}
					c, err := f.describe(ctx, "app-dax")
					if err != nil {
						return err
					}
					if name := aws.ToString(c.ClusterName); name != "app-dax" {
						return fmt.Errorf("the cluster is named %q", name)
					}
					return nil
				},
			},
			{
				Rule: "a cluster with the name exists, in any case, since DAX stores names in lowercase",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					f, err := create(ctx, store, nil)
					if err != nil {
						return err
					}
					_, err = f.CreateCluster(ctx, createInput(named("APP-DAX")))
					return err
				},
				Code: "ClusterAlreadyExistsFault",
			},
			{
				Rule:  "AvailabilityZones, when set, has as many zones as ReplicationFactor",
				Break: createBreak(func(in *dax.CreateClusterInput) { in.AvailabilityZones = []string{"us-west-2a", "us-west-2b"} }),
				Code:  "InvalidParameterCombinationException",
			},
			{
				Rule:  "AvailabilityZones, when set, has as many zones as ReplicationFactor, so an empty list fails",
				Break: createBreak(func(in *dax.CreateClusterInput) { in.AvailabilityZones = []string{} }),
				Code:  "InvalidParameterCombinationException",
			},
			{
				Rule: "a cluster has at most 11 nodes: a primary and 10 read replicas",
				Break: createBreak(func(in *dax.CreateClusterInput) {
					in.ReplicationFactor = 12
					in.AvailabilityZones = nil
				}),
				Code: "NodeQuotaForClusterExceededFault",
				Doc:  clusterGuide + "#DAX.concepts.clusters",
			},
			{
				Rule:  "ClusterEndpointEncryptionType is NONE or TLS",
				Break: createBreak(func(in *dax.CreateClusterInput) { in.ClusterEndpointEncryptionType = "SSL" }),
				Code:  "InvalidParameterValueException",
			},
			{
				Rule: "ClusterEndpointEncryptionType is NONE when left out",
				Doc:  stackTemplate,
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f, err := create(ctx, store, nil)
					if err != nil {
						return err
					}
					c, err := f.describe(ctx, "app-dax")
					if err != nil {
						return err
					}
					if c.ClusterEndpointEncryptionType != types.ClusterEndpointEncryptionTypeNone {
						return fmt.Errorf("the encryption type is %q", c.ClusterEndpointEncryptionType)
					}
					return nil
				},
			},
			{
				Rule:  "NetworkType is ipv4, ipv6, or dual_stack",
				Break: createBreak(func(in *dax.CreateClusterInput) { in.NetworkType = "ipv5" }),
				Code:  "InvalidParameterValueException",
			},
			{
				Rule: "DAX assigns a 60-minute maintenance window when PreferredMaintenanceWindow is left out",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f, err := create(ctx, store, nil)
					if err != nil {
						return err
					}
					c, err := f.describe(ctx, "app-dax")
					if err != nil {
						return err
					}
					if minutes, ok := windowMinutes(aws.ToString(c.PreferredMaintenanceWindow)); !ok || minutes != 60 {
						return fmt.Errorf("the maintenance window is %q", aws.ToString(c.PreferredMaintenanceWindow))
					}
					return nil
				},
			},
			{
				Rule: "DAX assigns the VPC's default security group to each node when SecurityGroupIds is left out",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f, err := create(ctx, store, nil)
					if err != nil {
						return err
					}
					c, err := f.describe(ctx, "app-dax")
					if err != nil {
						return err
					}
					if len(c.SecurityGroups) != 1 {
						return fmt.Errorf("the cluster has security groups %v", c.SecurityGroups)
					}
					return nil
				},
			},
			{
				Rule: "notifications are off when NotificationTopicArn is left out",
				Doc:  settingsGuide,
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f, err := create(ctx, store, nil)
					if err != nil {
						return err
					}
					c, err := f.describe(ctx, "app-dax")
					if err != nil {
						return err
					}
					if n := c.NotificationConfiguration; n != nil && aws.ToString(n.TopicStatus) == "active" {
						return fmt.Errorf("notifications go to %s", aws.ToString(n.TopicArn))
					}
					return nil
				},
			},
			{
				Rule:  "a cluster has at most 50 tags",
				Break: createBreak(func(in *dax.CreateClusterInput) { in.Tags = tags(51) }),
				Code:  "TagQuotaPerResourceExceeded",
				Doc:   tagPage,
			},
			{
				Rule:  "a tag key has no aws: prefix, which only AWS assigns",
				Break: createBreak(func(in *dax.CreateClusterInput) { in.Tags = []types.Tag{tag("aws:team", "a")} }),
				Code:  "InvalidParameterValueException",
				Doc:   tagPage,
			},
		}, windowRules(createWithWindow, "")...),
		Unclear: "The page doesn't name the error for each limit it states. The fake returns InvalidParameterValueException " +
			"for an invalid value, and InvalidParameterCombinationException for AvailabilityZones that don't match " +
			"ReplicationFactor. The page says a cluster with replicas has 3 to 10 nodes, while the developer guide allows " +
			"two nodes, and up to 11; the fake allows 1 to 11. It lists the days of a maintenance window in lowercase " +
			"without saying whether other cases work; the fake accepts any case. The docs don't say what DAX reports " +
			"when SSESpecification or ParameterGroupName is left out: the fake reports encryption at rest DISABLED and no " +
			"parameter group. The handler sends default.dax1.0 for a removed parameter group, as the recorded responses " +
			"show, while the developer guide names it default.dax.1.0. NetworkType comes from the subnet group, which " +
			"the fake doesn't have, so it reports none. Encryption at rest isn't supported on dax.r3 node types, but the " +
			"docs don't name the error, so the fake doesn't check it. The fake doesn't check that the subnet group, " +
			"parameter group, or topic exists.",
	},
	"DescribeClusters": {
		Doc: apiReference + "API_dax_DescribeClusters.html",
		Rules: []nativetest.Rule{
			{
				Rule: "MaxResults is 20 to 100",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).DescribeClusters(ctx, &dax.DescribeClustersInput{MaxResults: aws.Int32(10)})
					return err
				},
				Code: "InvalidParameterValueException",
			},
			missingRule(func(ctx context.Context, f *fakeAPI) error {
				_, err := f.describe(ctx, "missing")
				return err
			}),
			{
				Rule: "with more clusters than MaxResults, DAX returns MaxResults clusters and a NextToken",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					for i := range 21 {
						if _, err := f.CreateCluster(ctx, createInput(named(fmt.Sprintf("app-%d", i)))); err != nil {
							return err
						}
					}
					out, err := f.DescribeClusters(ctx, &dax.DescribeClustersInput{MaxResults: aws.Int32(20)})
					if err != nil {
						return err
					}
					if len(out.Clusters) != 20 || out.NextToken == nil {
						return fmt.Errorf("the fake returned %d clusters and NextToken %v", len(out.Clusters), out.NextToken)
					}
					return nil
				},
			},
			{
				Rule: "a creating or deleting cluster has only cluster level information, without its nodes",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f, err := create(ctx, store, nil)
					if err != nil {
						return err
					}
					c, err := f.describe(ctx, "app-dax")
					if err != nil {
						return err
					}
					if aws.ToString(c.Status) != "creating" || len(c.Nodes) > 0 {
						return fmt.Errorf("the %s cluster has nodes %v", aws.ToString(c.Status), c.Nodes)
					}
					if _, err := f.describe(ctx, "app-dax"); err != nil {
						return err
					}
					if _, err := f.DeleteCluster(ctx, &dax.DeleteClusterInput{ClusterName: aws.String("app-dax")}); err != nil {
						return err
					}
					if c, err = f.describe(ctx, "app-dax"); err != nil {
						return err
					}
					if aws.ToString(c.Status) != "deleting" || len(c.Nodes) > 0 {
						return fmt.Errorf("the %s cluster has nodes %v", aws.ToString(c.Status), c.Nodes)
					}
					return nil
				},
			},
		},
		Unclear: "The page doesn't say whether ClusterNames match in any case; the fake lowercases them, as DAX stores " +
			"names. It doesn't say whether TotalNodes counts nodes being removed; the handler subtracts NodeIdsToRemove, " +
			"and the fake removes nodes at once.",
	},
	"UpdateCluster": {
		Doc: apiReference + "API_dax_UpdateCluster.html",
		Rules: append([]nativetest.Rule{
			stateRule(func(ctx context.Context, f *fakeAPI) error {
				return updateApp(ctx, f, &dax.UpdateClusterInput{Description: aws.String("cache")})
			}),
			missingRule(func(ctx context.Context, f *fakeAPI) error {
				_, err := f.UpdateCluster(ctx, &dax.UpdateClusterInput{ClusterName: aws.String("missing"), Description: aws.String("cache")})
				return err
			}),
			{
				Rule: "the call changes only the settings it names",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f, err := available(ctx, store, func(in *dax.CreateClusterInput) { in.SecurityGroupIds = []string{"sg-1"} })
					if err != nil {
						return err
					}
					if err := updateApp(ctx, f, &dax.UpdateClusterInput{Description: aws.String("cache")}); err != nil {
						return err
					}
					c, err := f.describe(ctx, "app-dax")
					if err != nil {
						return err
					}
					if len(c.SecurityGroups) != 1 || aws.ToString(c.SecurityGroups[0].SecurityGroupIdentifier) != "sg-1" {
						return fmt.Errorf("the cluster has security groups %v", c.SecurityGroups)
					}
					return nil
				},
			},
		}, windowRules(updateWithWindow, createPage)...),
		Unclear: "The page says DAX assigns the default VPC security group when SecurityGroupIds is left out, the words " +
			"of CreateCluster's page, without saying whether an update without it replaces the cluster's groups. The " +
			"fake keeps them, as the handler expects. The page states no format for PreferredMaintenanceWindow; the fake " +
			"applies CreateCluster's. It doesn't list the values of NotificationTopicStatus beyond active and inactive, " +
			"and the fake doesn't check them or that the parameter group exists.",
	},
	"IncreaseReplicationFactor": {
		Doc: apiReference + "API_dax_IncreaseReplicationFactor.html",
		Rules: []nativetest.Rule{
			stateRule(func(ctx context.Context, f *fakeAPI) error { return increaseApp(ctx, f, 4) }),
			missingRule(func(ctx context.Context, f *fakeAPI) error {
				_, err := f.IncreaseReplicationFactor(ctx, &dax.IncreaseReplicationFactorInput{ClusterName: aws.String("missing"), NewReplicationFactor: 4})
				return err
			}),
			{
				Rule:  "the call adds nodes, so NewReplicationFactor is more than the cluster's nodes",
				Break: onAvailable(func(ctx context.Context, f *fakeAPI) error { return increaseApp(ctx, f, 3) }),
				Code:  "InvalidParameterValueException",
			},
			{
				Rule:  "a cluster has at most 11 nodes: a primary and 10 read replicas",
				Break: onAvailable(func(ctx context.Context, f *fakeAPI) error { return increaseApp(ctx, f, 12) }),
				Code:  "NodeQuotaForClusterExceededFault",
				Doc:   clusterGuide + "#DAX.concepts.clusters",
			},
		},
		Unclear: "The page says all the cluster's nodes are placed in AvailabilityZones, without saying whether the list " +
			"has a zone for every node or only for the new ones, or what a list of another length returns. The handler " +
			"sends a zone for every node, and the fake takes the list as the zones of all the nodes. The page doesn't name " +
			"the error for a NewReplicationFactor that adds no node; the fake returns InvalidParameterValueException.",
	},
	"DecreaseReplicationFactor": {
		Doc: apiReference + "API_dax_DecreaseReplicationFactor.html",
		Rules: []nativetest.Rule{
			stateRule(func(ctx context.Context, f *fakeAPI) error {
				return decreaseApp(ctx, f, &dax.DecreaseReplicationFactorInput{NewReplicationFactor: 2})
			}),
			missingRule(func(ctx context.Context, f *fakeAPI) error {
				_, err := f.DecreaseReplicationFactor(ctx, &dax.DecreaseReplicationFactorInput{ClusterName: aws.String("missing"), NewReplicationFactor: 1})
				return err
			}),
			{
				Rule: "the call can't remove the last node",
				Break: onAvailable(func(ctx context.Context, f *fakeAPI) error {
					return decreaseApp(ctx, f, &dax.DecreaseReplicationFactorInput{NewReplicationFactor: 0})
				}),
				Code: "InvalidParameterValueException",
			},
			{
				Rule: "the call removes nodes, so NewReplicationFactor is less than the cluster's nodes",
				Break: onAvailable(func(ctx context.Context, f *fakeAPI) error {
					return decreaseApp(ctx, f, &dax.DecreaseReplicationFactorInput{NewReplicationFactor: 3})
				}),
				Code: "InvalidParameterValueException",
			},
			{
				Rule: "each node of NodeIdsToRemove is in the cluster",
				Break: onAvailable(func(ctx context.Context, f *fakeAPI) error {
					return decreaseApp(ctx, f, &dax.DecreaseReplicationFactorInput{NewReplicationFactor: 2, NodeIdsToRemove: []string{"app-dax-9"}})
				}),
				Code: "NodeNotFoundFault",
			},
		},
		Unclear: "The page doesn't say how AvailabilityZones and NodeIdsToRemove combine, or whether AvailabilityZones " +
			"names a zone for each node removed. The handler sends one zone per node it removes; the fake removes a node " +
			"per zone listed, and the last nodes when the call names too few. The page doesn't name the error for a " +
			"NewReplicationFactor below 1 or one that removes no node; the fake returns InvalidParameterValueException.",
	},
	"DeleteCluster": {
		Doc: apiReference + "API_dax_DeleteCluster.html",
		Rules: []nativetest.Rule{
			stateRule(func(ctx context.Context, f *fakeAPI) error {
				_, err := f.DeleteCluster(ctx, &dax.DeleteClusterInput{ClusterName: aws.String("app-dax")})
				return err
			}),
			missingRule(func(ctx context.Context, f *fakeAPI) error {
				_, err := f.DeleteCluster(ctx, &dax.DeleteClusterInput{ClusterName: aws.String("missing")})
				return err
			}),
		},
		Unclear: "The page lists InvalidClusterStateFault, a cluster not available, without saying which states it " +
			"rejects. The fake rejects a creating, modifying, or deleting cluster, and the handler treats a deleting one " +
			"as deleted.",
	},
	"ListTags": {
		Doc: apiReference + "API_dax_ListTags.html",
		Rules: tagCallRules(false, func(ctx context.Context, f *fakeAPI, resource string) error {
			_, err := f.ListTags(ctx, &dax.ListTagsInput{ResourceName: aws.String(resource)})
			return err
		}),
		Unclear: "The page lists InvalidClusterStateFault, a cluster not available, so DAX rejects ListTags while the " +
			"cluster is creating, modifying, or deleting. The fake doesn't enforce it yet: the handler's Read and List " +
			"call ListTags on every cluster DescribeClusters returns, whatever its state, and fixing that needs a " +
			"decision on what Read reports for Tags while the cluster isn't available.",
	},
	"TagResource": {
		Doc: apiReference + "API_dax_TagResource.html",
		Rules: append(tagCallRules(true, func(ctx context.Context, f *fakeAPI, resource string) error {
			_, err := f.TagResource(ctx, &dax.TagResourceInput{ResourceName: aws.String(resource), Tags: []types.Tag{tag("team", "a")}})
			return err
		}),
			nativetest.Rule{
				Rule: "a cluster has at most 50 tags",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					f, err := available(ctx, store, func(in *dax.CreateClusterInput) { in.Tags = tags(50) })
					if err != nil {
						return err
					}
					return tagCluster(ctx, f, tag("team", "a"))
				},
				Code: "TagQuotaPerResourceExceeded",
				Doc:  tagPage,
			},
			nativetest.Rule{
				Rule:  "a tag key has no aws: prefix, which only AWS assigns",
				Break: onAvailable(func(ctx context.Context, f *fakeAPI) error { return tagCluster(ctx, f, tag("aws:team", "a")) }),
				Code:  "InvalidParameterValueException",
				Doc:   tagPage,
			},
			nativetest.Rule{
				Rule: "a tag with a key the cluster has replaces its value, and keys are case sensitive",
				Doc:  tagPage,
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f, err := available(ctx, store, func(in *dax.CreateClusterInput) { in.Tags = []types.Tag{tag("team", "a")} })
					if err != nil {
						return err
					}
					if err := tagCluster(ctx, f, tag("team", "b"), tag("Team", "c")); err != nil {
						return err
					}
					got, err := clusterTags(ctx, f)
					if err != nil {
						return err
					}
					if len(got) != 2 || got["team"] != "b" || got["Team"] != "c" {
						return fmt.Errorf("the cluster has tags %v", got)
					}
					return nil
				},
			},
		),
		Unclear: "The Tag page doesn't name the error for a key with the aws: prefix; the fake returns " +
			"InvalidParameterValueException.",
	},
	"UntagResource": {
		Doc: apiReference + "API_dax_UntagResource.html",
		Rules: tagCallRules(true, func(ctx context.Context, f *fakeAPI, resource string) error {
			_, err := f.UntagResource(ctx, &dax.UntagResourceInput{ResourceName: aws.String(resource), TagKeys: []string{"team"}})
			return err
		}),
		Unclear: "The page lists TagNotFoundFault, but says only that tags with the keys are removed, not " +
			"whether a key the cluster doesn't have fails. The fake accepts it and changes nothing, as a retry of a " +
			"lost response expects.",
	},
}
