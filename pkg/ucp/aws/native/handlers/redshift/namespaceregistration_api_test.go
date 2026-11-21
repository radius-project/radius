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

package redshift

import (
	"context"
	"fmt"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/redshift/types"
	"github.com/aws/aws-sdk-go-v2/service/redshiftserverless"
	serverlesstypes "github.com/aws/aws-sdk-go-v2/service/redshiftserverless/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	registrationReference  = "https://docs.aws.amazon.com/redshift/latest/APIReference/"
	registrationServerless = "https://docs.aws.amazon.com/redshift-serverless/latest/APIReference/"
)

// registrationProbes is what a run against real AWS showed about RegisterNamespace and DeregisterNamespace that the
// pages don't state.
var registrationProbes = "In a run against real AWS, AWS answered two consumers with ValidationError and a consumer " +
	"that isn't an account ID with InvalidParameterValue, took any 12-digit account, answered a workgroup it didn't " +
	"have with ClusterNotFound, and answered a call while the cluster or the serverless namespace was busy with " +
	"InvalidClusterState (\"There is an operation running on the Cluster\"). Each call left the cluster, or the " +
	"serverless namespace and workgroup, busy for about 10 to 20 seconds; the fake leaves them busy for " +
	fmt.Sprint(registrationBusyReads) + " reads of their status."

// registrationCall is a call against a fake API for namespace registrations, after any calls it needs to set up.
type registrationCall func(ctx context.Context, f *registrationFake) error

// registrationAgainst returns calls as a rule's Break or Check, made in order against one fake backed by store,
// which has the fake's namespaces, workgroups, and cluster. It stops at the first call that fails, and returns its
// error.
func registrationAgainst(calls ...registrationCall) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		addRegistrationParents(store)
		f := newRegistrationFake(store)
		for _, call := range calls {
			if err := call(ctx, f); err != nil {
				return err
			}
		}
		return nil
	}
}

// serverlessIdentifier and provisionedIdentifier return the NamespaceIdentifier of a serverless namespace and
// workgroup, and of a cluster.
func serverlessIdentifier(namespace string, workgroup string) types.NamespaceIdentifierUnion {
	return &types.NamespaceIdentifierUnionMemberServerlessIdentifier{Value: types.ServerlessIdentifier{
		NamespaceIdentifier: aws.String(namespace),
		WorkgroupIdentifier: aws.String(workgroup),
	}}
}

func provisionedIdentifier(cluster string) types.NamespaceIdentifierUnion {
	return &types.NamespaceIdentifierUnionMemberProvisionedIdentifier{Value: types.ProvisionedIdentifier{ClusterIdentifier: aws.String(cluster)}}
}

// analyticsNamespace is the NamespaceIdentifier of the fake's serverless namespace analytics.
var analyticsNamespace = serverlessIdentifier("analytics", "analytics-wg")

func registerNamespace(namespace types.NamespaceIdentifierUnion, consumers ...string) registrationCall {
	return func(ctx context.Context, f *registrationFake) error {
		_, err := f.RegisterNamespace(ctx, &redshift.RegisterNamespaceInput{NamespaceIdentifier: namespace, ConsumerIdentifiers: consumers})
		return err
	}
}

func deregisterNamespace(namespace types.NamespaceIdentifierUnion, consumers ...string) registrationCall {
	return func(ctx context.Context, f *registrationFake) error {
		_, err := f.DeregisterNamespace(ctx, &redshift.DeregisterNamespaceInput{NamespaceIdentifier: namespace, ConsumerIdentifiers: consumers})
		return err
	}
}

// holdParent returns a call that keeps a parent of the fake busy until the test ends.
func holdParent(typeName string, name string) registrationCall {
	return func(ctx context.Context, f *registrationFake) error {
		f.held[typeName+"/"+name] = true
		return nil
	}
}

// settleParents reads the status of each parent of the fake until it is free.
func settleParents(ctx context.Context, f *registrationFake) error {
	for range registrationBusyReads {
		if _, err := f.GetWorkgroup(ctx, &redshiftserverless.GetWorkgroupInput{WorkgroupName: aws.String("analytics-wg")}); err != nil {
			return err
		}
		if _, err := f.GetNamespace(ctx, &redshiftserverless.GetNamespaceInput{NamespaceName: aws.String("analytics")}); err != nil {
			return err
		}
		if _, err := f.DescribeClusters(ctx, &redshift.DescribeClustersInput{ClusterIdentifier: aws.String("warehouse")}); err != nil {
			return err
		}
	}
	return nil
}

// expectWorkgroupStatus returns a call that reads the workgroup analytics-wg and fails unless its status is want.
func expectWorkgroupStatus(want serverlesstypes.WorkgroupStatus) registrationCall {
	return func(ctx context.Context, f *registrationFake) error {
		out, err := f.GetWorkgroup(ctx, &redshiftserverless.GetWorkgroupInput{WorkgroupName: aws.String("analytics-wg")})
		if err != nil {
			return err
		}
		if out.Workgroup.Status != want {
			return fmt.Errorf("the workgroup is %s, not %s", out.Workgroup.Status, want)
		}
		return nil
	}
}

// expectConsumers returns a call that lists the data shares of the namespace with the ARN, and fails unless the
// consumers of its INTERNAL data share are want.
func expectConsumers(arn string, want ...string) registrationCall {
	return func(ctx context.Context, f *registrationFake) error {
		out, err := f.DescribeDataSharesForProducer(ctx, &redshift.DescribeDataSharesForProducerInput{ProducerArn: aws.String(arn)})
		if err != nil {
			return err
		}
		got := []string{}
		for _, share := range out.DataShares {
			if share.DataShareType != types.DataShareTypeInternal {
				continue
			}
			for _, association := range share.DataShareAssociations {
				got = append(got, aws.ToString(association.ConsumerIdentifier))
			}
		}
		if !slices.Equal(got, want) {
			return fmt.Errorf("the INTERNAL data share has the consumers %v, not %v", got, want)
		}
		return nil
	}
}

const (
	analyticsArn = registrationArnPrefix + "ns-analytics"
	warehouseArn = registrationClusterNS + "ns-warehouse"
)

// registrationChangeRules are the rules RegisterNamespace and DeregisterNamespace share, for a call made with
// namespace and consumers.
func registrationChangeRules(call func(namespace types.NamespaceIdentifierUnion, consumers ...string) registrationCall) []nativetest.Rule {
	return []nativetest.Rule{
		{
			Rule:  "ConsumerIdentifiers has exactly 1 item",
			Break: registrationAgainst(call(analyticsNamespace, registrationConsumer, registrationOtherConsumer)),
			Code:  registrationValidation,
		},
		{
			Rule:  "the cluster exists",
			Break: registrationAgainst(call(provisionedIdentifier("missing"), registrationConsumer)),
			Code:  registrationClusterNotFound,
		},
		{
			Rule:  "the cluster is available",
			Break: registrationAgainst(holdParent(registrationClusterType, "warehouse"), call(provisionedIdentifier("warehouse"), registrationConsumer)),
			Code:  registrationBusy,
		},
		{
			Rule:  "the serverless workgroup exists",
			Break: registrationAgainst(call(serverlessIdentifier("analytics", "missing"), registrationConsumer)),
			Code:  registrationClusterNotFound,
		},
		{
			Rule:  "the serverless namespace and workgroup are free",
			Break: registrationAgainst(holdParent(registrationNamespaceType, "analytics"), call(analyticsNamespace, registrationConsumer)),
			Code:  registrationBusy,
		},
		{
			Rule:  "the consumer is an account ID",
			Break: registrationAgainst(call(analyticsNamespace, "consumer")),
			Code:  registrationInvalid,
		},
	}
}

// registrationAPI records the Redshift and Redshift Serverless calls NamespaceRegistration makes.
var registrationAPI = map[string]nativetest.Call{
	"RegisterNamespace": {
		Doc: registrationReference + "API_RegisterNamespace.html",
		Rules: append(registrationChangeRules(registerNamespace), []nativetest.Rule{
			{
				Rule: "a registration is an association of the consumer's AWS Glue Data Catalog with the namespace's INTERNAL " +
					"data share, which AWS lists once the namespace and workgroup are free",
				Check: registrationAgainst(registerNamespace(analyticsNamespace, registrationConsumer), expectConsumers(analyticsArn),
					expectWorkgroupStatus(serverlesstypes.WorkgroupStatusModifying), settleParents,
					expectWorkgroupStatus(serverlesstypes.WorkgroupStatusAvailable), expectConsumers(analyticsArn, "DataCatalog/"+registrationConsumer)),
			},
			{
				Rule:  "AWS registers the workgroup's namespace, whatever NamespaceIdentifier names",
				Check: registrationAgainst(registerNamespace(serverlessIdentifier("sales", "analytics-wg"), registrationConsumer), settleParents, expectConsumers(analyticsArn, "DataCatalog/"+registrationConsumer)),
			},
			{
				Rule: "AWS accepts a consumer it has registered, once the namespace is free",
				Check: registrationAgainst(registerNamespace(provisionedIdentifier("warehouse"), registrationConsumer), settleParents,
					registerNamespace(provisionedIdentifier("warehouse"), registrationConsumer), settleParents, expectConsumers(warehouseArn, "DataCatalog/"+registrationConsumer)),
			},
		}...),
		Unclear: "The page names ClusterNotFound, InvalidClusterState, and InvalidNamespaceFault without the rules of " +
			"ConsumerIdentifiers beyond its one item, or what AWS does with a NamespaceIdentifier of another namespace than " +
			"the workgroup's. " + registrationProbes + " AWS registered the workgroup's namespace when NamespaceIdentifier named " +
			"a namespace it didn't have, so the handler checks the workgroup's namespace before it registers. AWS listed a " +
			"serverless registration about 10 seconds later, once the workgroup was free, and a cluster's at once; the " +
			"fake lists either only once the cluster or workgroup is free.",
	},
	"DeregisterNamespace": {
		Doc: registrationReference + "API_DeregisterNamespace.html",
		Rules: append(registrationChangeRules(deregisterNamespace), []nativetest.Rule{
			{
				Rule:  "AWS accepts a consumer that isn't registered",
				Check: registrationAgainst(deregisterNamespace(analyticsNamespace, registrationConsumer)),
			},
			{
				Rule: "a deregistration ends the association and leaves the namespace and workgroup busy",
				Check: registrationAgainst(registerNamespace(analyticsNamespace, registrationConsumer), settleParents, expectConsumers(analyticsArn, "DataCatalog/"+registrationConsumer),
					deregisterNamespace(analyticsNamespace, registrationConsumer), expectWorkgroupStatus(serverlesstypes.WorkgroupStatusModifying),
					expectConsumers(analyticsArn, "DataCatalog/"+registrationConsumer), expectConsumers(analyticsArn)),
			},
		}...),
		Unclear: registrationProbes + " AWS answered a consumer it had no registration for with success, so the handler " +
			"finds a registration that is already gone by reading it. AWS still listed a deregistered serverless " +
			"registration a few seconds later; the fake lists it for one more read.",
	},
	"DescribeDataSharesForProducer": {
		Doc: registrationReference + "API_DescribeDataSharesForProducer.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "the producer ARN names a namespace",
				Break: registrationAgainst(expectConsumers(registrationArnPrefix + "ns-missing")),
				Code:  registrationNoNamespace,
			},
			{
				Rule:  "AWS lists the namespace's other data shares, whose consumers can look like a registration's",
				Check: registrationAgainst(expectConsumers(analyticsArn)),
			},
		},
		Unclear: "The page doesn't say what RegisterNamespace's data share looks like. In a run against real AWS it was " +
			"ds_internal_namespace, of type INTERNAL, with one association per consumer, DataCatalog/ and the account, " +
			"with status AUTHORIZED, and AWS removed the data share with its last association. The fake returns every " +
			"data share in one page.",
	},
	"DescribeClusters": {
		Doc: registrationReference + "API_DescribeClusters.html",
		Rules: []nativetest.Rule{
			{
				Rule: "the cluster exists",
				Break: registrationAgainst(func(ctx context.Context, f *registrationFake) error {
					_, err := f.DescribeClusters(ctx, &redshift.DescribeClustersInput{ClusterIdentifier: aws.String("missing")})
					return err
				}),
				Code: registrationClusterNotFound,
			},
		},
		Unclear: "The page doesn't list the statuses; the Cluster data type's page does. AWS reported modifying while a " +
			"registration changed the cluster in a run against real AWS. The handler waits only while the status is one of " +
			"a change that ends on its own, such as modifying or rebooting, and the fake reports available or modifying.",
	},
	"GetWorkgroup": {
		Doc: registrationServerless + "API_GetWorkgroup.html",
		Rules: []nativetest.Rule{
			{
				Rule: "the workgroup exists",
				Break: registrationAgainst(func(ctx context.Context, f *registrationFake) error {
					_, err := f.GetWorkgroup(ctx, &redshiftserverless.GetWorkgroupInput{WorkgroupName: aws.String("missing")})
					return err
				}),
				Code: registrationServerlessNotFound,
			},
			{
				Rule: "a namespace has one workgroup, and a workgroup one namespace",
				Doc:  "https://docs.aws.amazon.com/redshift/latest/mgmt/serverless-workgroup-namespace.html",
				Check: registrationAgainst(func(ctx context.Context, f *registrationFake) error {
					namespaces := map[string]bool{}
					for _, workgroup := range f.store.List(registrationWorkgroupType) {
						out, err := f.GetWorkgroup(ctx, &redshiftserverless.GetWorkgroupInput{WorkgroupName: aws.String(workgroup)})
						if err != nil {
							return err
						}
						name := aws.ToString(out.Workgroup.NamespaceName)
						if namespaces[name] {
							return fmt.Errorf("the namespace %s has more than one workgroup", name)
						}
						namespaces[name] = true
					}
					return nil
				}),
			},
		},
	},
	"GetNamespace": {
		Doc: registrationServerless + "API_GetNamespace.html",
		Rules: []nativetest.Rule{
			{
				Rule: "the namespace exists",
				Break: registrationAgainst(func(ctx context.Context, f *registrationFake) error {
					_, err := f.GetNamespace(ctx, &redshiftserverless.GetNamespaceInput{NamespaceName: aws.String("missing")})
					return err
				}),
				Code: registrationServerlessNotFound,
			},
		},
	},
}
