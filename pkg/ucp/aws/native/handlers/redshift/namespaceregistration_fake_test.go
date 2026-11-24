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
	"slices"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/redshift/types"
	"github.com/aws/aws-sdk-go-v2/service/redshiftserverless"
	serverlesstypes "github.com/aws/aws-sdk-go-v2/service/redshiftserverless/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	// registrationClusterType, registrationWorkgroupType, and registrationNamespaceType keep the fake's clusters,
	// serverless workgroups, and serverless namespaces in the store, apart from the registrations the handler serves.
	registrationClusterType   = "AWS::Redshift::Cluster"
	registrationWorkgroupType = "AWS::RedshiftServerless::Workgroup"
	registrationNamespaceType = "AWS::RedshiftServerless::Namespace"

	// registrationValidation is the code of a value outside the limits of the API reference, and registrationInvalid
	// of a consumer that isn't an account ID.
	registrationValidation = "ValidationError"
	registrationInvalid    = "InvalidParameterValue"

	// registrationBusyReads is how many reads of the cluster, or of the serverless namespace and workgroup, report it
	// busy after a registration changes it.
	registrationBusyReads = 2

	// registrationArnPrefix starts the fake's namespace ARNs.
	registrationArnPrefix = "arn:aws:redshift-serverless:us-west-2:123456789012:namespace/"
	registrationClusterNS = "arn:aws:redshift:us-west-2:123456789012:namespace:"

	// registrationConsumer is a consumer account, and registrationOtherConsumer the consumer of a data share that
	// isn't a registration.
	registrationConsumer      = "444455556666"
	registrationOtherConsumer = "777788889999"
)

// registrationFault returns the client fault AWS returns with the code.
func registrationFault(code string, message string) error {
	return &smithy.GenericAPIError{Code: code, Message: message, Fault: smithy.FaultClient}
}

// addRegistrationParents adds the fake's serverless namespaces analytics and sales, each with its one workgroup,
// analytics-wg and sales-wg, and the cluster warehouse, to store.
func addRegistrationParents(store *awsfake.CloudControl) {
	for _, name := range []string{"analytics", "sales"} {
		store.Put(registrationNamespaceType, name, map[string]any{"NamespaceName": name, "NamespaceArn": registrationArnPrefix + "ns-" + name})
		store.Put(registrationWorkgroupType, name+"-wg", map[string]any{"WorkgroupName": name + "-wg", "NamespaceName": name})
	}
	store.Put(registrationClusterType, "warehouse", map[string]any{"ClusterIdentifier": "warehouse", "ClusterNamespaceArn": registrationClusterNS + "ns-warehouse"})
}

// registrationFake is an in-memory Redshift and Redshift Serverless API for namespace registrations. It keeps them
// in an awsfake store, in the type's property format, so the suite's Cloud Control sees the same resources, and the
// clusters, workgroups, and namespaces under their own types. A registration is the association of the producer
// namespace's INTERNAL data share with the consumer, which DescribeDataSharesForProducer returns. It follows the rules
// in registrationAPI, and where AWS's docs are silent it takes the side that is harder for the handler:
//
//   - A registration or deregistration leaves the cluster, or the serverless namespace and workgroup, busy for
//     registrationBusyReads reads, and AWS rejects either while it is.
//   - AWS doesn't list a registration while its cluster or workgroup is still busy with it, and still lists a
//     deregistered one for one read.
//   - Every namespace has a data share that isn't a registration, whose consumer looks like one.
type registrationFake struct {
	store *awsfake.CloudControl

	mu sync.Mutex

	// busy counts the reads each parent, by store type and name, still reports busy, and held are parents busy until
	// a test frees them.
	busy map[string]int
	held map[string]bool

	// hidden holds registrations AWS doesn't list until their parent is free, and lingering deregistered ones AWS
	// still lists for one read, by producer ARN and consumer.
	hidden    map[string]bool
	lingering map[string]bool

	// errs maps a method name to an error the call returns instead of acting, and calls counts the calls of each.
	errs  map[string]error
	calls map[string]int
}

var (
	_ RegistrationAPI           = (*registrationFake)(nil)
	_ RegistrationServerlessAPI = (*registrationFake)(nil)
)

func newRegistrationFake(store *awsfake.CloudControl) *registrationFake {
	return &registrationFake{
		store:     store,
		busy:      map[string]int{},
		held:      map[string]bool{},
		hidden:    map[string]bool{},
		lingering: map[string]bool{},
		errs:      map[string]error{},
		calls:     map[string]int{},
	}
}

func (f *registrationFake) begin(method string) error {
	f.calls[method]++
	return f.errs[method]
}

// isBusy reports whether the parent is busy, without counting a read.
func (f *registrationFake) isBusy(typeName string, name string) bool {
	return f.held[typeName+"/"+name] || f.busy[typeName+"/"+name] > 0
}

// readBusy reports whether the parent is busy, counting a read of its status.
func (f *registrationFake) readBusy(typeName string, name string) bool {
	busy := f.isBusy(typeName, name)
	if f.busy[typeName+"/"+name] > 0 {
		f.busy[typeName+"/"+name]--
	}
	return busy
}

// target is the producer of a registration call: its namespace ARN, the parents a change makes busy, and the
// identifier of a registration of the consumer.
type registrationTarget struct {
	arn        string
	parents    [][2]string
	identifier func(consumer string) string
}

// target returns the producer a call's NamespaceIdentifier names, or ClusterNotFound. A serverless identifier names
// the namespace of its workgroup, whatever namespace it names.
func (f *registrationFake) target(namespace types.NamespaceIdentifierUnion) (registrationTarget, error) {
	switch v := namespace.(type) {
	case *types.NamespaceIdentifierUnionMemberProvisionedIdentifier:
		cluster := aws.ToString(v.Value.ClusterIdentifier)
		props := f.store.Resource(registrationClusterType, cluster)
		if props == nil {
			return registrationTarget{}, registrationFault(registrationClusterNotFound, "Cluster "+cluster+" can't be found.")
		}
		return registrationTarget{
			arn:        awsapi.String(props, "ClusterNamespaceArn"),
			parents:    [][2]string{{registrationClusterType, cluster}},
			identifier: func(consumer string) string { return cluster + "|" + consumer },
		}, nil
	case *types.NamespaceIdentifierUnionMemberServerlessIdentifier:
		workgroup := aws.ToString(v.Value.WorkgroupIdentifier)
		props := f.store.Resource(registrationWorkgroupType, workgroup)
		if props == nil {
			return registrationTarget{}, registrationFault(registrationClusterNotFound, "Could not find serverless workgroup "+workgroup)
		}
		name := awsapi.String(props, "NamespaceName")
		return registrationTarget{
			arn:        awsapi.String(f.store.Resource(registrationNamespaceType, name), "NamespaceArn"),
			parents:    [][2]string{{registrationWorkgroupType, workgroup}, {registrationNamespaceType, name}},
			identifier: func(consumer string) string { return name + "|" + workgroup + "|" + consumer },
		}, nil
	}
	return registrationTarget{}, registrationFault(registrationValidation, "NamespaceIdentifier is required")
}

// change checks a RegisterNamespace or DeregisterNamespace call and returns its producer and consumer.
func (f *registrationFake) change(namespace types.NamespaceIdentifierUnion, consumers []string) (registrationTarget, string, error) {
	if len(consumers) != 1 {
		return registrationTarget{}, "", registrationFault(registrationValidation, "Value at 'consumerIdentifiers' failed to satisfy constraint: Member must have length less than or equal to 1")
	}
	t, err := f.target(namespace)
	if err != nil {
		return t, "", err
	}
	if !registrationAccount.MatchString(consumers[0]) {
		return t, "", registrationFault(registrationInvalid, "The consumer identifier isn't valid.")
	}
	for _, parent := range t.parents {
		if f.isBusy(parent[0], parent[1]) {
			return t, "", registrationFault(registrationBusy, "There is an operation running on the Cluster. Please try again at a later time.")
		}
	}
	return t, consumers[0], nil
}

// changed marks the parents of t busy after a change.
func (f *registrationFake) changed(t registrationTarget) {
	for _, parent := range t.parents {
		f.busy[parent[0]+"/"+parent[1]] = registrationBusyReads
	}
}

func (f *registrationFake) RegisterNamespace(ctx context.Context, in *redshift.RegisterNamespaceInput, _ ...func(*redshift.Options)) (*redshift.RegisterNamespaceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("RegisterNamespace"); err != nil {
		return nil, err
	}
	t, consumer, err := f.change(in.NamespaceIdentifier, in.ConsumerIdentifiers)
	if err != nil {
		return nil, err
	}
	identifier := t.identifier(consumer)
	r, _ := splitRegistration(identifier)
	f.store.Put(registrationType, identifier, r.properties())
	f.hidden[t.arn+"|"+consumer] = true
	delete(f.lingering, t.arn+"|"+consumer)
	f.changed(t)
	return &redshift.RegisterNamespaceOutput{Status: types.NamespaceRegistrationStatusRegistering}, nil
}

func (f *registrationFake) DeregisterNamespace(ctx context.Context, in *redshift.DeregisterNamespaceInput, _ ...func(*redshift.Options)) (*redshift.DeregisterNamespaceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DeregisterNamespace"); err != nil {
		return nil, err
	}
	t, consumer, err := f.change(in.NamespaceIdentifier, in.ConsumerIdentifiers)
	if err != nil {
		return nil, err
	}
	identifier := t.identifier(consumer)
	if f.store.Resource(registrationType, identifier) != nil {
		f.store.Remove(registrationType, identifier)
		if !f.hidden[t.arn+"|"+consumer] {
			f.lingering[t.arn+"|"+consumer] = true
		}
	}
	delete(f.hidden, t.arn+"|"+consumer)
	f.changed(t)
	return &redshift.DeregisterNamespaceOutput{Status: types.NamespaceRegistrationStatusDeregistering}, nil
}

// producer returns the parents of the namespace with the ARN, and false when AWS has no such namespace.
func (f *registrationFake) producer(arn string) ([][2]string, bool) {
	for _, cluster := range f.store.List(registrationClusterType) {
		if awsapi.String(f.store.Resource(registrationClusterType, cluster), "ClusterNamespaceArn") == arn {
			return [][2]string{{registrationClusterType, cluster}}, true
		}
	}
	for _, workgroup := range f.store.List(registrationWorkgroupType) {
		name := awsapi.String(f.store.Resource(registrationWorkgroupType, workgroup), "NamespaceName")
		if awsapi.String(f.store.Resource(registrationNamespaceType, name), "NamespaceArn") == arn {
			return [][2]string{{registrationWorkgroupType, workgroup}, {registrationNamespaceType, name}}, true
		}
	}
	return nil, false
}

// arnOf returns the producer namespace ARN of the registration with the identifier.
func (f *registrationFake) arnOf(identifier string) string {
	r, err := splitRegistration(identifier)
	if err != nil {
		return ""
	}
	if r.Cluster != "" {
		return awsapi.String(f.store.Resource(registrationClusterType, r.Cluster), "ClusterNamespaceArn")
	}
	return awsapi.String(f.store.Resource(registrationNamespaceType, r.Namespace), "NamespaceArn")
}

func (f *registrationFake) DescribeDataSharesForProducer(ctx context.Context, in *redshift.DescribeDataSharesForProducerInput, _ ...func(*redshift.Options)) (*redshift.DescribeDataSharesForProducerOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DescribeDataSharesForProducer"); err != nil {
		return nil, err
	}
	arn := aws.ToString(in.ProducerArn)
	parents, ok := f.producer(arn)
	if !ok {
		return nil, registrationFault(registrationNoNamespace, "The namespace isn't valid because the namespace doesn't exist.")
	}
	busy := false
	for _, parent := range parents {
		busy = busy || f.isBusy(parent[0], parent[1])
	}
	associations := []types.DataShareAssociation{}
	consumers := []string{}
	for _, identifier := range f.store.List(registrationType) {
		if f.arnOf(identifier) != arn {
			continue
		}
		consumer := awsapi.Strings(f.store.Resource(registrationType, identifier), "ConsumerIdentifiers")
		if len(consumer) == 1 && !(busy && f.hidden[arn+"|"+consumer[0]]) {
			delete(f.hidden, arn+"|"+consumer[0])
			consumers = append(consumers, consumer[0])
		}
	}
	for key := range f.lingering {
		if consumer, ok := strings.CutPrefix(key, arn+"|"); ok {
			delete(f.lingering, key)
			consumers = append(consumers, consumer)
		}
	}
	slices.Sort(consumers)
	for _, consumer := range slices.Compact(consumers) {
		associations = append(associations, types.DataShareAssociation{
			ConsumerIdentifier: aws.String(registrationConsumerPrefix + consumer),
			Status:             types.DataShareStatusAuthorized,
		})
	}
	shares := []types.DataShare{{
		DataShareArn:          aws.String("arn:aws:redshift:us-west-2:123456789012:datashare:" + arn + "/sales"),
		ProducerArn:           aws.String(arn),
		DataShareAssociations: []types.DataShareAssociation{{ConsumerIdentifier: aws.String(registrationConsumerPrefix + registrationOtherConsumer), Status: types.DataShareStatusActive}},
	}}
	if len(associations) > 0 {
		shares = append(shares, types.DataShare{
			DataShareArn:          aws.String("arn:aws:redshift:us-west-2:123456789012:datashare:" + arn + "/ds_internal_namespace"),
			ProducerArn:           aws.String(arn),
			DataShareType:         types.DataShareTypeInternal,
			DataShareAssociations: associations,
		})
	}
	return &redshift.DescribeDataSharesForProducerOutput{DataShares: shares}, nil
}

func (f *registrationFake) DescribeClusters(ctx context.Context, in *redshift.DescribeClustersInput, _ ...func(*redshift.Options)) (*redshift.DescribeClustersOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DescribeClusters"); err != nil {
		return nil, err
	}
	name := aws.ToString(in.ClusterIdentifier)
	props := f.store.Resource(registrationClusterType, name)
	if props == nil {
		return nil, registrationFault(registrationClusterNotFound, "Cluster "+name+" not found.")
	}
	status := "available"
	if f.readBusy(registrationClusterType, name) {
		status = "modifying"
	}
	return &redshift.DescribeClustersOutput{Clusters: []types.Cluster{{
		ClusterIdentifier:   aws.String(name),
		ClusterNamespaceArn: aws.String(awsapi.String(props, "ClusterNamespaceArn")),
		ClusterStatus:       aws.String(status),
	}}}, nil
}

func (f *registrationFake) GetWorkgroup(ctx context.Context, in *redshiftserverless.GetWorkgroupInput, _ ...func(*redshiftserverless.Options)) (*redshiftserverless.GetWorkgroupOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("GetWorkgroup"); err != nil {
		return nil, err
	}
	name := aws.ToString(in.WorkgroupName)
	props := f.store.Resource(registrationWorkgroupType, name)
	if props == nil {
		return nil, registrationFault(registrationServerlessNotFound, "Serverless workgroup "+name+" not found.")
	}
	status := serverlesstypes.WorkgroupStatusAvailable
	if f.readBusy(registrationWorkgroupType, name) {
		status = serverlesstypes.WorkgroupStatusModifying
	}
	return &redshiftserverless.GetWorkgroupOutput{Workgroup: &serverlesstypes.Workgroup{
		WorkgroupName: aws.String(name),
		NamespaceName: aws.String(awsapi.String(props, "NamespaceName")),
		Status:        status,
	}}, nil
}

func (f *registrationFake) GetNamespace(ctx context.Context, in *redshiftserverless.GetNamespaceInput, _ ...func(*redshiftserverless.Options)) (*redshiftserverless.GetNamespaceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("GetNamespace"); err != nil {
		return nil, err
	}
	name := aws.ToString(in.NamespaceName)
	props := f.store.Resource(registrationNamespaceType, name)
	if props == nil {
		return nil, registrationFault(registrationServerlessNotFound, "Namespace "+name+" not found.")
	}
	status := serverlesstypes.NamespaceStatusAvailable
	if f.readBusy(registrationNamespaceType, name) {
		status = serverlesstypes.NamespaceStatusModifying
	}
	return &redshiftserverless.GetNamespaceOutput{Namespace: &serverlesstypes.Namespace{
		NamespaceName: aws.String(name),
		NamespaceArn:  aws.String(awsapi.String(props, "NamespaceArn")),
		Status:        status,
	}}, nil
}
