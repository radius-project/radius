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
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/redshift/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	// dataShareProducer is the account that produces the fake's data shares, dataShareConsumerAccount the account
	// they are granted to, and dataShareOtherConsumer another account granted the data share sales.
	dataShareProducer        = "123456789012"
	dataShareConsumerAccount = "444455556666"
	dataShareOtherConsumer   = "777788889999"

	dataShareProducerArn = "arn:aws:redshift-serverless:us-west-2:" + dataShareProducer + ":namespace/11111111-1111-1111-1111-111111111111"
	dataSharePrefix      = "arn:aws:redshift:us-west-2:" + dataShareProducer + ":datashare:11111111-1111-1111-1111-111111111111/"

	// dataShareNamespace and dataShareCluster are namespaces of the consumer account: a Redshift Serverless one, and a
	// provisioned cluster's.
	dataShareNamespace = "arn:aws:redshift-serverless:us-west-2:" + dataShareConsumerAccount + ":namespace/22222222-2222-2222-2222-222222222222"
	dataShareCluster   = "arn:aws:redshift:us-west-2:" + dataShareConsumerAccount + ":namespace:33333333-3333-3333-3333-333333333333"

	// dataSharePageSize is how many data shares a describe call returns when it doesn't set MaxRecords. AWS's pages
	// don't say, so the fake uses a small one, so a handler that reads only the first page fails.
	dataSharePageSize = 2

	// dataShareInternalMarker marks a store entry of a data share Redshift made for itself.
	dataShareInternalMarker = "DataShareType"
)

var (
	dataShareSales    = dataSharePrefix + "sales"
	dataShareOrders   = dataSharePrefix + "orders"
	dataShareUngranted = dataSharePrefix + "ungranted"

	// dataShareGrants are the consumers each data share is granted to in SQL when a fake starts.
	dataShareGrants = map[string][]string{
		dataShareSales:    {dataShareConsumerAccount, dataShareOtherConsumer},
		dataShareOrders:   {dataShareConsumerAccount},
		dataShareUngranted: nil,
	}

	// dataShareFakeConsumerID is the form of a consumer AWS takes: an account ID, ADX, or DataCatalog/<account ID>.
	dataShareFakeConsumerID = regexp.MustCompile(`^([0-9]{12}|ADX|DataCatalog/[0-9]{12})$`)
)

// dataShareFake is an in-memory Redshift API for data shares, shared by a producer account and a consumer account,
// which caller says. It keeps each authorization, while AUTHORIZED or ACTIVE, and each consumer association, while
// ACTIVE, in an awsfake store in its type's property format, so the suite's Cloud Control sees the same resources. It
// follows the rules in dataShareAuthorizationAPI and dataShareConsumerAPI, and where AWS's docs are silent it takes
// the side that is harder for the handler:
//
//   - A removed authorization or association reads as it was once more. A new one reads as made at once, as in a run
//     against real AWS: the handler can't tell a slow one from none, because both read the same, so it never waits
//     for one (see the Progress of each handler).
//   - Describe calls return at most dataSharePageSize data shares.
type dataShareFake struct {
	store *awsfake.CloudControl

	mu sync.Mutex

	// caller is the account that makes the calls.
	caller string

	// ended holds the status of an authorization that is no longer AUTHORIZED or ACTIVE, by identifier:
	// DEAUTHORIZED or REJECTED. One that isn't there is PENDING_AUTHORIZATION.
	ended map[string]types.DataShareStatus

	// lag holds the status the next read of an authorization or association reports, by type and identifier.
	lag map[string]types.DataShareStatus

	// grants are the consumers each data share is granted to in SQL, which the API can't change.
	grants map[string][]string

	// errs maps a method name to an error the call returns instead of acting, and calls counts the calls of each.
	errs  map[string]error
	calls map[string]int
}

var _ DataShareAPI = (*dataShareFake)(nil)

func newDataShareFake(store *awsfake.CloudControl, caller string) *dataShareFake {
	return &dataShareFake{
		store:  store,
		caller: caller,
		grants: maps.Clone(dataShareGrants),
		ended:  map[string]types.DataShareStatus{},
		lag:    map[string]types.DataShareStatus{},
		errs:   map[string]error{},
		calls:  map[string]int{},
	}
}

func dataShareError(code string, message string) error {
	return &smithy.GenericAPIError{Code: code, Message: message, Fault: smithy.FaultClient}
}

// dataShareRules collects the first rule a call breaks, as the client fault AWS returns for it.
type dataShareRules struct {
	err error
}

func (r *dataShareRules) check(ok bool, code string, message string) {
	if r.err == nil && !ok {
		r.err = dataShareError(code, message)
	}
}

// begin records a call of method and returns the error a test set for it.
func (f *dataShareFake) begin(method string) error {
	f.calls[method]++
	return f.errs[method]
}

// dataShareMissing is the error AWS returns for a data share a call can't act on.
func dataShareMissing() error {
	return dataShareError(dataShareFault, "The data share isn't available because it doesn't exist.")
}

// granted reports whether the data share is granted to consumer in SQL.
func (f *dataShareFake) granted(arn string, consumer string) bool {
	return slices.Contains(f.grants[arn], consumer)
}

// internal reports whether Redshift made the data share for itself, which a test marks on a store entry.
func (f *dataShareFake) internal(arn string) bool {
	for _, typeName := range []string{dataShareAuthorizationType, dataShareConsumerType} {
		for _, id := range f.store.List(typeName) {
			props := f.store.Resource(typeName, id)
			if awsapi.String(props, "DataShareArn") == arn && awsapi.String(props, dataShareInternalMarker) != "" {
				return true
			}
		}
	}
	return false
}

// read returns the status a read of the entry reports, using up a lag.
func (f *dataShareFake) read(typeName string, id string, status types.DataShareStatus) types.DataShareStatus {
	key := typeName + "|" + id
	if lagged, ok := f.lag[key]; ok {
		delete(f.lag, key)
		return lagged
	}
	return status
}

// associations returns the identifiers of the consumer associations of the data share.
func (f *dataShareFake) associations(arn string) []string {
	out := []string{}
	for _, id := range f.store.List(dataShareConsumerType) {
		if awsapi.String(f.store.Resource(dataShareConsumerType, id), "DataShareArn") == arn {
			out = append(out, id)
		}
	}
	return out
}

// producerView returns the data share as its producer sees it: an association for each consumer it is granted to.
func (f *dataShareFake) producerView(arn string) types.DataShare {
	share := types.DataShare{DataShareArn: aws.String(arn), ProducerArn: aws.String(dataShareProducerArn), AllowPubliclyAccessibleConsumers: aws.Bool(false)}
	if f.internal(arn) {
		share.DataShareType = types.DataShareTypeInternal
	}
	for _, consumer := range f.grants[arn] {
		id := arn + "|" + consumer
		a := types.DataShareAssociation{ConsumerIdentifier: aws.String(consumer), Status: types.DataShareStatusPendingAuthorization}
		if props := f.store.Resource(dataShareAuthorizationType, id); props != nil {
			a.Status = types.DataShareStatusAuthorized
			a.ProducerAllowedWrites = aws.Bool(dataShareBool(props, "AllowWrites"))
			if consumer == dataShareConsumerAccount && len(f.associations(arn)) > 0 {
				a.Status = types.DataShareStatusActive
			}
		} else if status, ok := f.ended[id]; ok {
			a.Status = status
			a.ProducerAllowedWrites = aws.Bool(false)
		}
		a.Status = f.read(dataShareAuthorizationType, id, a.Status)
		share.DataShareAssociations = append(share.DataShareAssociations, a)
	}
	return share
}

// authorized reports whether the producer authorized the data share for the consumer account.
func (f *dataShareFake) authorized(arn string) bool {
	return f.store.Resource(dataShareAuthorizationType, arn+"|"+dataShareConsumerAccount) != nil
}

// consumerView returns the data share as the consumer account sees it: an association for the account, AVAILABLE or
// ACTIVE with the entire account, and one for each namespace or Region it is associated with.
func (f *dataShareFake) consumerView(arn string) types.DataShare {
	share := types.DataShare{DataShareArn: aws.String(arn), ProducerArn: aws.String(dataShareProducerArn), AllowPubliclyAccessibleConsumers: aws.Bool(false)}
	if f.internal(arn) {
		share.DataShareType = types.DataShareTypeInternal
	}
	writes := dataShareBool(f.store.Resource(dataShareAuthorizationType, arn+"|"+dataShareConsumerAccount), "AllowWrites")
	account := types.DataShareAssociation{ConsumerIdentifier: aws.String(dataShareConsumerAccount), Status: types.DataShareStatusAvailable, ProducerAllowedWrites: aws.Bool(writes)}
	entries := []types.DataShareAssociation{}
	for _, id := range f.associations(arn) {
		props := f.store.Resource(dataShareConsumerType, id)
		status := f.read(dataShareConsumerType, id, types.DataShareStatusActive)
		a := types.DataShareAssociation{
			ConsumerIdentifier:     aws.String(dataShareConsumerAccount),
			Status:                 status,
			ProducerAllowedWrites:  aws.Bool(writes),
			ConsumerAcceptedWrites: aws.Bool(dataShareBool(props, "AllowWrites")),
		}
		switch {
		case dataShareBool(props, "AssociateEntireAccount"):
			account = a
			continue
		case awsapi.String(props, "ConsumerArn") != "":
			a.ConsumerIdentifier = aws.String(awsapi.String(props, "ConsumerArn"))
			a.ConsumerRegion = aws.String("us-west-2")
		default:
			a.ConsumerRegion = aws.String(awsapi.String(props, "ConsumerRegion"))
		}
		entries = append(entries, a)
	}
	// A removed association reads as ACTIVE once more.
	for key, status := range f.lag {
		typeName, id, _ := strings.Cut(key, "|")
		if typeName != dataShareConsumerType || !strings.HasPrefix(id, arn+"|") || f.store.Resource(dataShareConsumerType, id) != nil {
			continue
		}
		delete(f.lag, key)
		a := types.DataShareAssociation{ConsumerIdentifier: aws.String(dataShareConsumerAccount), Status: status}
		switch last := strings.TrimPrefix(id, arn+"|"); {
		case last == dataShareEntireAccount:
			account = a
			continue
		case strings.HasPrefix(last, "arn:"):
			a.ConsumerIdentifier, a.ConsumerRegion = aws.String(last), aws.String("us-west-2")
		default:
			a.ConsumerRegion = aws.String(last)
		}
		entries = append(entries, a)
	}
	share.DataShareAssociations = append([]types.DataShareAssociation{account}, entries...)
	return share
}

// dataSharePage returns the data shares of list from the position marker names, up to maxRecords, and the marker
// of the rest.
func dataSharePage(list []types.DataShare, marker *string, maxRecords *int32) ([]types.DataShare, *string) {
	size := dataSharePageSize
	if maxRecords != nil && *maxRecords > 0 {
		size = int(*maxRecords)
	}
	start, _ := strconv.Atoi(strings.TrimPrefix(aws.ToString(marker), "marker-"))
	start = min(max(start, 0), len(list))
	end := min(start+size, len(list))
	var next *string
	if end < len(list) {
		next = aws.String(fmt.Sprintf("marker-%d", end))
	}
	return list[start:end], next
}

// produced returns the data shares the caller produces that are granted to a consumer, which are the ones AWS
// reports.
func (f *dataShareFake) produced() []string {
	if f.caller != dataShareProducer {
		return nil
	}
	out := []string{}
	for _, arn := range slices.Sorted(maps.Keys(f.grants)) {
		if len(f.grants[arn]) > 0 {
			out = append(out, arn)
		}
	}
	return out
}

// inbound returns the data shares a producer authorized for the caller.
func (f *dataShareFake) inbound() []string {
	if f.caller != dataShareConsumerAccount {
		return nil
	}
	out := []string{}
	for _, arn := range slices.Sorted(maps.Keys(f.grants)) {
		if f.authorized(arn) {
			out = append(out, arn)
		}
	}
	return out
}

func (f *dataShareFake) AuthorizeDataShare(ctx context.Context, in *redshift.AuthorizeDataShareInput, _ ...func(*redshift.Options)) (*redshift.AuthorizeDataShareOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("AuthorizeDataShare"); err != nil {
		return nil, err
	}
	arn, consumer := aws.ToString(in.DataShareArn), aws.ToString(in.ConsumerIdentifier)
	r := &dataShareRules{}
	r.check(dataShareFakeConsumerID.MatchString(consumer), dataShareParameterFault, "The consumer identifier isn't valid.")
	r.check(slices.Contains(f.produced(), arn) && f.granted(arn, consumer), dataShareFault, "The data share isn't available because it doesn't exist.")
	if r.err != nil {
		return nil, r.err
	}
	id := arn + "|" + consumer
	if f.store.Resource(dataShareAuthorizationType, id) != nil {
		// AWS reports an authorization it authorized again as AUTHORIZED, even with an ACTIVE association, before it
		// reports it ACTIVE again.
		f.lag[dataShareAuthorizationType+"|"+id] = types.DataShareStatusAuthorized
	}
	delete(f.ended, id)
	f.store.Put(dataShareAuthorizationType, id, map[string]any{
		"DataShareArn":       arn,
		"ConsumerIdentifier": consumer,
		"AllowWrites":        aws.ToBool(in.AllowWrites),
		"ProducerArn":        dataShareProducerArn,
	})
	return &redshift.AuthorizeDataShareOutput{DataShareArn: aws.String(arn), ProducerArn: aws.String(dataShareProducerArn)}, nil
}

func (f *dataShareFake) DeauthorizeDataShare(ctx context.Context, in *redshift.DeauthorizeDataShareInput, _ ...func(*redshift.Options)) (*redshift.DeauthorizeDataShareOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DeauthorizeDataShare"); err != nil {
		return nil, err
	}
	arn, consumer := aws.ToString(in.DataShareArn), aws.ToString(in.ConsumerIdentifier)
	id := arn + "|" + consumer
	if !slices.Contains(f.produced(), arn) || !f.granted(arn, consumer) {
		return nil, dataShareMissing()
	}
	props := f.store.Resource(dataShareAuthorizationType, id)
	switch {
	case props == nil && f.ended[id] == types.DataShareStatusDeauthorized:
		return nil, dataShareError(dataShareFault, "You can't remove authorization because you have already removed authorization from the data share.")
	case props == nil && f.ended[id] != types.DataShareStatusRejected:
		return nil, dataShareError(dataShareFault, "You can't remove authorization because the data share isn't authorized.")
	}
	if props != nil {
		previous := types.DataShareStatusAuthorized
		if consumer == dataShareConsumerAccount && len(f.associations(arn)) > 0 {
			previous = types.DataShareStatusActive
		}
		f.lag[dataShareAuthorizationType+"|"+id] = previous
		f.store.Remove(dataShareAuthorizationType, id)
		if consumer == dataShareConsumerAccount {
			for _, association := range f.associations(arn) {
				f.store.Remove(dataShareConsumerType, association)
			}
		}
	}
	f.ended[id] = types.DataShareStatusDeauthorized
	return &redshift.DeauthorizeDataShareOutput{DataShareArn: aws.String(arn), ProducerArn: aws.String(dataShareProducerArn)}, nil
}

// reject is the consumer account declining the data share, which it does with RejectDataShare, a call Radius doesn't
// make.
func (f *dataShareFake) reject(arn string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := arn + "|" + dataShareConsumerAccount
	f.store.Remove(dataShareAuthorizationType, id)
	for _, association := range f.associations(arn) {
		f.store.Remove(dataShareConsumerType, association)
	}
	f.ended[id] = types.DataShareStatusRejected
}

func (f *dataShareFake) DescribeDataShares(ctx context.Context, in *redshift.DescribeDataSharesInput, _ ...func(*redshift.Options)) (*redshift.DescribeDataSharesOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DescribeDataShares"); err != nil {
		return nil, err
	}
	arn := aws.ToString(in.DataShareArn)
	if f.caller == dataShareConsumerAccount {
		// The consumer sees only the data shares authorized for it, and an empty list for any other.
		if !f.authorized(arn) {
			return &redshift.DescribeDataSharesOutput{}, nil
		}
		return &redshift.DescribeDataSharesOutput{DataShares: []types.DataShare{f.consumerView(arn)}}, nil
	}
	if !slices.Contains(f.produced(), arn) {
		return nil, dataShareError(dataShareFault, "The datashare Amazon Resource Name (ARN) isn't valid because the ARN doesn't exist. Use a valid datashare ARN.")
	}
	return &redshift.DescribeDataSharesOutput{DataShares: []types.DataShare{f.producerView(arn)}}, nil
}

func (f *dataShareFake) DescribeDataSharesForProducer(ctx context.Context, in *redshift.DescribeDataSharesForProducerInput, _ ...func(*redshift.Options)) (*redshift.DescribeDataSharesForProducerOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DescribeDataSharesForProducer"); err != nil {
		return nil, err
	}
	r := &dataShareRules{}
	r.check(in.Status == "" || slices.Contains(in.Status.Values(), in.Status), dataShareParameterFault, "The status isn't valid.")
	r.check(in.ProducerArn == nil || strings.Contains(aws.ToString(in.ProducerArn), ":"+f.caller+":"), dataShareParameterFault,
		"You can't access this namespace because the namespace doesn't exist in your AWS account.")
	r.check(in.ProducerArn == nil || aws.ToString(in.ProducerArn) == dataShareProducerArn, dataShareNamespaceFault, "The namespace doesn't exist.")
	if r.err != nil {
		return nil, r.err
	}
	all := []types.DataShare{}
	for _, arn := range f.produced() {
		if f.internal(arn) || (in.ProducerArn != nil && aws.ToString(in.ProducerArn) != dataShareProducerArn) {
			continue
		}
		if share := f.producerView(arn); dataShareHasStatus(share, string(in.Status)) {
			all = append(all, share)
		}
	}
	list, next := dataSharePage(all, in.Marker, in.MaxRecords)
	return &redshift.DescribeDataSharesForProducerOutput{DataShares: list, Marker: next}, nil
}

func (f *dataShareFake) DescribeDataSharesForConsumer(ctx context.Context, in *redshift.DescribeDataSharesForConsumerInput, _ ...func(*redshift.Options)) (*redshift.DescribeDataSharesForConsumerOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DescribeDataSharesForConsumer"); err != nil {
		return nil, err
	}
	r := &dataShareRules{}
	r.check(in.Status == "" || slices.Contains(in.Status.Values(), in.Status), dataShareParameterFault, "The status isn't valid.")
	r.check(in.ConsumerArn == nil || slices.Contains([]string{dataShareNamespace, dataShareCluster}, aws.ToString(in.ConsumerArn)), dataShareNamespaceFault,
		"The namespace isn't valid because the namespace doesn't exist. Provide a valid namespace.")
	if r.err != nil {
		return nil, r.err
	}
	all := []types.DataShare{}
	for _, arn := range f.inbound() {
		if share := f.consumerView(arn); dataShareHasStatus(share, string(in.Status)) {
			all = append(all, share)
		}
	}
	list, next := dataSharePage(all, in.Marker, in.MaxRecords)
	return &redshift.DescribeDataSharesForConsumerOutput{DataShares: list, Marker: next}, nil
}

// dataShareHasStatus reports whether status is "", or one of the share's associations has it, which is what a
// describe call's Status keeps.
func dataShareHasStatus(share types.DataShare, status string) bool {
	return status == "" || slices.ContainsFunc(share.DataShareAssociations, func(a types.DataShareAssociation) bool { return string(a.Status) == status })
}

// dataShareOneConsumer checks that a call names exactly one of the entire account, a namespace, or a Region.
func dataShareOneConsumer(r *dataShareRules, entire bool, arn string, region string) {
	r.check(entire || arn != "" || region != "", dataShareParameterFault, "No association was specified.")
	r.check(!entire || arn == "", dataShareParameterFault, "You can't associate a datashare with both an entire AWS account and a consumer Amazon Resource Name (ARN).")
	r.check(!entire || region == "", dataShareParameterFault, "You can't associate a datashare with both an entire AWS account and an AWS Region.")
	r.check(arn == "" || region == "", dataShareParameterFault, "You can't associate a datashare with both a consumer Amazon Resource Name (ARN) and an AWS Region.")
}

// dataShareAssociationID returns the identifier of the association a call names.
func dataShareAssociationID(arn string, entire bool, consumerArn string, region string) string {
	return arn + "|" + dataShareConsumer{entireAccount: entire, arn: consumerArn, region: region}.String()
}

func (f *dataShareFake) AssociateDataShareConsumer(ctx context.Context, in *redshift.AssociateDataShareConsumerInput, _ ...func(*redshift.Options)) (*redshift.AssociateDataShareConsumerOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("AssociateDataShareConsumer"); err != nil {
		return nil, err
	}
	arn, entire := aws.ToString(in.DataShareArn), aws.ToBool(in.AssociateEntireAccount)
	consumerArn, region := aws.ToString(in.ConsumerArn), aws.ToString(in.ConsumerRegion)
	r := &dataShareRules{}
	dataShareOneConsumer(r, entire, consumerArn, region)
	r.check(slices.Contains(f.inbound(), arn), dataShareFault, "The data share isn't available because it doesn't exist.")
	r.check(consumerArn == "" || strings.Contains(consumerArn, ":"+f.caller+":"), dataShareParameterFault,
		"The cluster namespace specified doesn't exist. Try associating the datashare with another cluster namespace.")
	r.check(consumerArn == "" || slices.Contains([]string{dataShareNamespace, dataShareCluster}, consumerArn), dataShareNamespaceFault,
		"The consumer's Amazon Resource Name (ARN) isn't valid because the namespace doesn't exist. Provide a valid namespace.")
	if r.err != nil {
		return nil, r.err
	}
	// An association with the entire account replaces those with a namespace or a Region, and the other way around.
	for _, other := range f.associations(arn) {
		if dataShareBool(f.store.Resource(dataShareConsumerType, other), "AssociateEntireAccount") != entire {
			f.store.Remove(dataShareConsumerType, other)
		}
	}
	id := dataShareAssociationID(arn, entire, consumerArn, region)
	props := map[string]any{"DataShareArn": arn, "AssociateEntireAccount": entire, "AllowWrites": aws.ToBool(in.AllowWrites), "ProducerArn": dataShareProducerArn}
	if consumerArn != "" {
		props["ConsumerArn"] = consumerArn
	}
	if region != "" {
		props["ConsumerRegion"] = region
	}
	f.store.Put(dataShareConsumerType, id, props)
	return &redshift.AssociateDataShareConsumerOutput{DataShareArn: aws.String(arn), ProducerArn: aws.String(dataShareProducerArn)}, nil
}

func (f *dataShareFake) DisassociateDataShareConsumer(ctx context.Context, in *redshift.DisassociateDataShareConsumerInput, _ ...func(*redshift.Options)) (*redshift.DisassociateDataShareConsumerOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DisassociateDataShareConsumer"); err != nil {
		return nil, err
	}
	arn, entire := aws.ToString(in.DataShareArn), aws.ToBool(in.DisassociateEntireAccount)
	consumerArn, region := aws.ToString(in.ConsumerArn), aws.ToString(in.ConsumerRegion)
	r := &dataShareRules{}
	dataShareOneConsumer(r, entire, consumerArn, region)
	id := dataShareAssociationID(arn, entire, consumerArn, region)
	r.check(slices.Contains(f.inbound(), arn) && f.store.Resource(dataShareConsumerType, id) != nil, dataShareFault,
		"The data share either doesn't exist or isn't associated with this data consumer cluster's Amazon Resource Name (ARN).")
	if r.err != nil {
		return nil, r.err
	}
	f.store.Remove(dataShareConsumerType, id)
	f.lag[dataShareConsumerType+"|"+id] = types.DataShareStatusActive
	return &redshift.DisassociateDataShareConsumerOutput{DataShareArn: aws.String(arn), ProducerArn: aws.String(dataShareProducerArn)}, nil
}
