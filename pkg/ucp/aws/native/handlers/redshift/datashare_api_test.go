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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/redshift/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	dataShareReference    = "https://docs.aws.amazon.com/redshift/latest/APIReference/"
	dataShareCommonErrors = dataShareReference + "CommonErrors.html"
	dataShareStatusGuide  = "https://docs.aws.amazon.com/redshift/latest/dg/datashare_status.html"
	dataShareGrantGuide   = "https://docs.aws.amazon.com/redshift/latest/dg/producer-cluster-admin.html"

	// dataShareRun is how the records name what a run against real AWS, with a producer and a consumer account,
	// showed where the pages are silent.
	dataShareRun = "in a run against real AWS"

	// dataShareListUnclear is what the describe pages leave unclear about pages.
	dataShareListUnclear = "The page doesn't give MaxRecords' default or limits; AWS accepted 0, 1, and 1000 " + dataShareRun +
		". The fake returns 2 data shares a page, so a handler that reads one page fails."
)

// dataShareCall is a call against a fake Redshift API, after any calls it needs to set up.
type dataShareCall func(ctx context.Context, f *dataShareFake) error

// dataShareAgainst returns calls as a rule's Break or Check, made in order as the producer account against one fake
// backed by store. It stops at the first call that fails, and returns its error.
func dataShareAgainst(calls ...dataShareCall) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		return dataShareCalls(ctx, newDataShareFake(store, dataShareProducer), calls)
	}
}

// dataShareAsConsumer is dataShareAgainst for the consumer account, with the data shares sales and orders authorized
// for it.
func dataShareAsConsumer(calls ...dataShareCall) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		return dataShareCalls(ctx, newDataShareConsumerFake(store), calls)
	}
}

func dataShareCalls(ctx context.Context, f *dataShareFake, calls []dataShareCall) error {
	for _, call := range calls {
		if err := call(ctx, f); err != nil {
			return err
		}
	}
	return nil
}

// dataShareAs returns a call that makes the next calls as the account.
func dataShareAs(account string) dataShareCall {
	return func(ctx context.Context, f *dataShareFake) error {
		f.caller = account
		return nil
	}
}

func dataShareAuthorize(arn string, consumer string, allowWrites *bool) dataShareCall {
	return func(ctx context.Context, f *dataShareFake) error {
		_, err := f.AuthorizeDataShare(ctx, &redshift.AuthorizeDataShareInput{DataShareArn: aws.String(arn), ConsumerIdentifier: aws.String(consumer), AllowWrites: allowWrites})
		return err
	}
}

func dataShareDeauthorize(arn string, consumer string) dataShareCall {
	return func(ctx context.Context, f *dataShareFake) error {
		_, err := f.DeauthorizeDataShare(ctx, &redshift.DeauthorizeDataShareInput{DataShareArn: aws.String(arn), ConsumerIdentifier: aws.String(consumer)})
		return err
	}
}

// dataShareExpectProducer returns a call that reads the data share as its producer, and fails unless the consumer's
// association has the status, and, when writes isn't nil, ProducerAllowedWrites writes.
func dataShareExpectProducer(arn string, consumer string, status types.DataShareStatus, writes *bool) dataShareCall {
	return func(ctx context.Context, f *dataShareFake) error {
		out, err := f.DescribeDataShares(ctx, &redshift.DescribeDataSharesInput{DataShareArn: aws.String(arn)})
		if err != nil {
			return err
		}
		a := dataShareAuthorizationOf(&out.DataShares[0], consumer)
		if a == nil || a.Status != status {
			return fmt.Errorf("the association of %s is %v, not %s", consumer, a, status)
		}
		if writes != nil && aws.ToBool(a.ProducerAllowedWrites) != *writes {
			return fmt.Errorf("ProducerAllowedWrites is %v, not %v", aws.ToBool(a.ProducerAllowedWrites), *writes)
		}
		return nil
	}
}

func dataShareAssociate(in redshift.AssociateDataShareConsumerInput) dataShareCall {
	return func(ctx context.Context, f *dataShareFake) error {
		if in.DataShareArn == nil {
			in.DataShareArn = aws.String(dataShareSales)
		}
		_, err := f.AssociateDataShareConsumer(ctx, &in)
		return err
	}
}

func dataShareDisassociate(in redshift.DisassociateDataShareConsumerInput) dataShareCall {
	return func(ctx context.Context, f *dataShareFake) error {
		if in.DataShareArn == nil {
			in.DataShareArn = aws.String(dataShareSales)
		}
		_, err := f.DisassociateDataShareConsumer(ctx, &in)
		return err
	}
}

// dataShareExpectConsumer returns a call that reads the data share sales as the consumer account, and fails unless
// it has exactly the associations want, by consumer, with their statuses.
func dataShareExpectConsumer(want map[string]types.DataShareStatus) dataShareCall {
	return func(ctx context.Context, f *dataShareFake) error {
		out, err := f.DescribeDataSharesForConsumer(ctx, &redshift.DescribeDataSharesForConsumerInput{})
		if err != nil {
			return err
		}
		got := map[string]types.DataShareStatus{}
		for _, share := range out.DataShares {
			if aws.ToString(share.DataShareArn) != dataShareSales {
				continue
			}
			for i := range share.DataShareAssociations {
				c, _ := dataShareConsumerOf(&share.DataShareAssociations[i])
				got[c.String()] = share.DataShareAssociations[i].Status
			}
		}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			return fmt.Errorf("the associations are %v, not %v", got, want)
		}
		return nil
	}
}

// dataShareNoLag reads the data share sales as the caller, so the next read reports what AWS has.
func dataShareNoLag(ctx context.Context, f *dataShareFake) error {
	if f.caller == dataShareConsumerAccount {
		_, err := f.DescribeDataSharesForConsumer(ctx, &redshift.DescribeDataSharesForConsumerInput{})
		return err
	}
	_, err := f.DescribeDataShares(ctx, &redshift.DescribeDataSharesInput{DataShareArn: aws.String(dataShareSales)})
	return err
}

// dataShareEntire, dataShareInNs, and dataShareInRegionX associate the data share sales with the entire account, the consumer namespace, and a Region.
var (
	dataShareEntire    = redshift.AssociateDataShareConsumerInput{AssociateEntireAccount: aws.Bool(true)}
	dataShareInNs      = redshift.AssociateDataShareConsumerInput{ConsumerArn: aws.String(dataShareNamespace)}
	dataShareInRegionX = redshift.AssociateDataShareConsumerInput{ConsumerRegion: aws.String("us-east-1")}
)

// dataSharePaging returns the rule that describe calls return their data shares in pages. list makes the call with
// marker, and returns how many data shares the page has and its marker.
func dataSharePaging(consumer bool, list func(ctx context.Context, f *dataShareFake, marker *string) (int, *string, error)) nativetest.Rule {
	return nativetest.Rule{
		Rule: "a call returns a Marker when there are more data shares, and the next call with it returns the rest",
		Check: func(ctx context.Context, store *awsfake.CloudControl) error {
			f := newDataShareFake(store, dataShareProducer)
			if consumer {
				f = newDataShareConsumerFake(store)
				store.Put(dataShareAuthorizationType, dataShareUngranted+"|"+dataShareConsumerAccount, map[string]any{"DataShareArn": dataShareUngranted})
			} else {
				f.grants[dataShareUngranted] = []string{dataShareConsumerAccount}
			}
			total := 0
			var marker *string
			for range 3 {
				n, next, err := list(ctx, f, marker)
				if err != nil {
					return err
				}
				total += n
				if marker = next; marker == nil {
					break
				}
			}
			if total != 3 || marker != nil {
				return fmt.Errorf("the pages have %d data shares", total)
			}
			return nil
		},
	}
}

// dataShareAuthorizationAPI records the Redshift calls DataShareAuthorization makes.
var dataShareAuthorizationAPI = map[string]nativetest.Call{
	"AuthorizeDataShare": {
		Doc: dataShareReference + "API_AuthorizeDataShare.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "ConsumerIdentifier is an AWS account ID or a keyword, such as ADX",
				Doc:   dataShareCommonErrors,
				Break: dataShareAgainst(dataShareAuthorize(dataShareSales, "not-an-account", nil)),
				Code:  dataShareParameterFault,
			},
			{
				Rule:  "the producer grants the data share to the consumer in SQL before it authorizes it",
				Doc:   dataShareGrantGuide,
				Break: dataShareAgainst(dataShareAuthorize(dataShareSales, "111122223333", nil)),
				Code:  dataShareFault,
			},
			{
				Rule:  "the data share exists, and has a grant",
				Break: dataShareAgainst(dataShareAuthorize(dataShareUngranted, dataShareConsumerAccount, nil)),
				Code:  dataShareFault,
			},
			{
				Rule: "an authorized data share is AUTHORIZED, and AllowWrites is false when the call leaves it out",
				Doc:  dataShareStatusGuide,
				Check: dataShareAgainst(dataShareAuthorize(dataShareSales, dataShareConsumerAccount, nil),
					dataShareExpectProducer(dataShareSales, dataShareConsumerAccount, types.DataShareStatusAuthorized, aws.Bool(false))),
			},
			{
				Rule: "authorizing again sets AllowWrites, and keeps the consumer's association ACTIVE",
				Check: dataShareAgainst(dataShareAuthorize(dataShareSales, dataShareConsumerAccount, nil),
					dataShareAs(dataShareConsumerAccount), dataShareAssociate(dataShareEntire), dataShareAs(dataShareProducer), dataShareNoLag,
					dataShareAuthorize(dataShareSales, dataShareConsumerAccount, aws.Bool(true)),
					dataShareExpectProducer(dataShareSales, dataShareConsumerAccount, types.DataShareStatusAuthorized, aws.Bool(true)),
					dataShareExpectProducer(dataShareSales, dataShareConsumerAccount, types.DataShareStatusActive, aws.Bool(true))),
			},
			{
				Rule: "a data share the consumer rejected can be authorized again",
				Doc:  dataShareStatusGuide,
				Check: dataShareAgainst(dataShareAuthorize(dataShareSales, dataShareConsumerAccount, nil), func(ctx context.Context, f *dataShareFake) error {
					f.reject(dataShareSales)
					return nil
				}, dataShareNoLag, dataShareAuthorize(dataShareSales, dataShareConsumerAccount, nil), dataShareNoLag,
					dataShareExpectProducer(dataShareSales, dataShareConsumerAccount, types.DataShareStatusAuthorized, nil)),
			},
		},
		Unclear: "The page lists only InvalidDataShareFault, \"There is an error with the datashare\". " + dataShareRun + ", AWS " +
			"answered a data share without a grant to the consumer, or none at all, with InvalidDataShareFault, and a consumer " +
			"identifier that isn't one with InvalidParameterValue, which the SDK doesn't model. It answered an authorization again " +
			"with the same one, set ProducerAllowedWrites to the call's AllowWrites (false when left out), and reported an ACTIVE " +
			"association AUTHORIZED in its response and ACTIVE again on the next read, with the consumer's association kept. " +
			"The page doesn't say whether that interrupts the consumer's queries. It authorized a REJECTED and a DEAUTHORIZED data " +
			"share again. The call takes no idempotency token. The pages don't say what AWS does for a data share Redshift made " +
			"for itself (DataShareType INTERNAL); the handler never sends one, and the fake accepts it.",
	},
	"DeauthorizeDataShare": {
		Doc: dataShareReference + "API_DeauthorizeDataShare.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "the data share is authorized for the consumer",
				Break: dataShareAgainst(dataShareDeauthorize(dataShareSales, dataShareConsumerAccount)),
				Code:  dataShareFault,
			},
			{
				Rule: "an authorization removed once can't be removed again",
				Break: dataShareAgainst(dataShareAuthorize(dataShareSales, dataShareConsumerAccount, nil),
					dataShareDeauthorize(dataShareSales, dataShareConsumerAccount), dataShareDeauthorize(dataShareSales, dataShareConsumerAccount)),
				Code: dataShareFault,
			},
			{
				Rule: "a removed authorization is DEAUTHORIZED, after a read that still reports it, and the consumer's associations are gone",
				Doc:  dataShareStatusGuide,
				Check: dataShareAgainst(dataShareAuthorize(dataShareSales, dataShareConsumerAccount, nil),
					dataShareAs(dataShareConsumerAccount), dataShareAssociate(dataShareInNs), dataShareAs(dataShareProducer), dataShareNoLag,
					dataShareDeauthorize(dataShareSales, dataShareConsumerAccount),
					dataShareExpectProducer(dataShareSales, dataShareConsumerAccount, types.DataShareStatusActive, nil),
					dataShareExpectProducer(dataShareSales, dataShareConsumerAccount, types.DataShareStatusDeauthorized, nil),
					func(ctx context.Context, f *dataShareFake) error {
						if ids := f.associations(dataShareSales); len(ids) != 0 {
							return fmt.Errorf("the consumer still has %v", ids)
						}
						return nil
					}),
			},
		},
		Unclear: "The page lists only InvalidDataShareFault. " + dataShareRun + ", AWS answered a data share without a grant to " +
			"the consumer, and an authorization it had already removed, with InvalidDataShareFault, removed the consumer's " +
			"associations with an ACTIVE data share, and removed the authorization of a data share the consumer had REJECTED. " +
			"The fake rejects a grant the producer never authorized, which the run didn't try. The handler takes " +
			"InvalidDataShareFault as not found only when a read then finds no authorization.",
	},
	"DescribeDataShares": {
		Doc: dataShareReference + "API_DescribeDataShares.html",
		Rules: []nativetest.Rule{
			{
				Rule: "an ARN of no data share is invalid",
				Break: dataShareAgainst(func(ctx context.Context, f *dataShareFake) error {
					_, err := f.DescribeDataShares(ctx, &redshift.DescribeDataSharesInput{DataShareArn: aws.String(dataSharePrefix + "missing")})
					return err
				}),
				Code: dataShareFault,
			},
			{
				Rule:  "a data share the producer granted but hasn't authorized is PENDING_AUTHORIZATION",
				Doc:   dataShareStatusGuide,
				Check: dataShareAgainst(dataShareExpectProducer(dataShareSales, dataShareConsumerAccount, types.DataShareStatusPendingAuthorization, nil)),
			},
		},
		Unclear: "The page doesn't say what each account sees. " + dataShareRun + ", the producer saw one association for each " +
			"consumer account it granted the data share to, without ConsumerRegion, and the consumer saw its own associations: one " +
			"for the account, AVAILABLE or ACTIVE, and one for each namespace or Region. AWS answered a data share granted to no " +
			"consumer with InvalidDataShareFault, as for one that doesn't exist, and returned the INTERNAL data share a namespace " +
			"registration makes. A malformed ARN got AccessDenied, so the handler checks the ARN's form first.",
	},
	"DescribeDataSharesForProducer": {
		Doc: dataShareReference + "API_DescribeDataSharesForProducer.html",
		Rules: []nativetest.Rule{
			{
				Rule: "ProducerArn names a namespace that exists",
				Break: dataShareAgainst(func(ctx context.Context, f *dataShareFake) error {
					_, err := f.DescribeDataSharesForProducer(ctx, &redshift.DescribeDataSharesForProducerInput{ProducerArn: aws.String(dataShareProducerArn + "0")})
					return err
				}),
				Code: dataShareNamespaceFault,
			},
			{
				Rule: "ProducerArn names a namespace of the account",
				Doc:  dataShareCommonErrors,
				Break: dataShareAsConsumer(func(ctx context.Context, f *dataShareFake) error {
					_, err := f.DescribeDataSharesForProducer(ctx, &redshift.DescribeDataSharesForProducerInput{ProducerArn: aws.String(dataShareProducerArn)})
					return err
				}),
				Code: dataShareParameterFault,
			},
			dataSharePaging(false, func(ctx context.Context, f *dataShareFake, marker *string) (int, *string, error) {
				out, err := f.DescribeDataSharesForProducer(ctx, &redshift.DescribeDataSharesForProducerInput{Marker: marker})
				if err != nil {
					return 0, nil, err
				}
				return len(out.DataShares), out.Marker, nil
			}),
		},
		Unclear: dataShareListUnclear + " " + dataShareRun + ", it returned only data shares granted to a consumer, and not the " +
			"INTERNAL one a namespace registration makes; the handler leaves out any it returns. It answered the producer namespace " +
			"of a data share from the consumer account with InvalidParameterValue, which the SDK doesn't model, so the handler " +
			"takes that, or InvalidNamespaceFault, as a data share this account doesn't produce.",
	},
}

// dataShareConsumerAPI records the Redshift calls DataShareConsumerAssociation makes.
var dataShareConsumerAPI = map[string]nativetest.Call{
	"AssociateDataShareConsumer": {
		Doc: dataShareReference + "API_AssociateDataShareConsumer.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "the producer authorized the data share for the account",
				Break: dataShareAsConsumer(dataShareAssociate(redshift.AssociateDataShareConsumerInput{DataShareArn: aws.String(dataShareUngranted), AssociateEntireAccount: aws.Bool(true)})),
				Code:  dataShareFault,
			},
			{
				Rule:  "ConsumerArn names a namespace that exists",
				Break: dataShareAsConsumer(dataShareAssociate(redshift.AssociateDataShareConsumerInput{ConsumerArn: aws.String(dataShareNamespace + "0")})),
				Code:  dataShareNamespaceFault,
			},
			{
				Rule:  "ConsumerArn names a namespace of the account",
				Doc:   dataShareCommonErrors,
				Break: dataShareAsConsumer(dataShareAssociate(redshift.AssociateDataShareConsumerInput{ConsumerArn: aws.String("arn:aws:redshift-serverless:us-west-2:" + dataShareProducer + ":namespace/x")})),
				Code:  dataShareParameterFault,
			},
			{
				Rule:  "the call names the entire account, a namespace, or a Region",
				Doc:   dataShareCommonErrors,
				Break: dataShareAsConsumer(dataShareAssociate(redshift.AssociateDataShareConsumerInput{AssociateEntireAccount: aws.Bool(false)})),
				Code:  dataShareParameterFault,
			},
			{
				Rule:  "the call names only one of the entire account and a namespace",
				Doc:   dataShareCommonErrors,
				Break: dataShareAsConsumer(dataShareAssociate(redshift.AssociateDataShareConsumerInput{AssociateEntireAccount: aws.Bool(true), ConsumerArn: aws.String(dataShareNamespace)})),
				Code:  dataShareParameterFault,
			},
			{
				Rule:  "the call names only one of the entire account and a Region",
				Doc:   dataShareCommonErrors,
				Break: dataShareAsConsumer(dataShareAssociate(redshift.AssociateDataShareConsumerInput{AssociateEntireAccount: aws.Bool(true), ConsumerRegion: aws.String("us-east-1")})),
				Code:  dataShareParameterFault,
			},
			{
				Rule:  "the call names only one of a namespace and a Region",
				Doc:   dataShareCommonErrors,
				Break: dataShareAsConsumer(dataShareAssociate(redshift.AssociateDataShareConsumerInput{ConsumerArn: aws.String(dataShareNamespace), ConsumerRegion: aws.String("us-east-1")})),
				Code:  dataShareParameterFault,
			},
			{
				Rule: "an associated data share is ACTIVE",
				Doc:  dataShareStatusGuide,
				Check: dataShareAsConsumer(dataShareAssociate(dataShareInNs),
					dataShareExpectConsumer(map[string]types.DataShareStatus{dataShareEntireAccount: types.DataShareStatusAvailable, dataShareNamespace: types.DataShareStatusActive})),
			},
			{
				Rule: "an association with the entire account replaces those with a namespace or a Region",
				Check: dataShareAsConsumer(dataShareAssociate(dataShareInNs), dataShareAssociate(dataShareInRegionX), dataShareNoLag, dataShareAssociate(dataShareEntire), dataShareNoLag,
					dataShareExpectConsumer(map[string]types.DataShareStatus{dataShareEntireAccount: types.DataShareStatusActive})),
			},
			{
				Rule: "an association with a namespace or a Region replaces the one with the entire account, and they keep each other",
				Check: dataShareAsConsumer(dataShareAssociate(dataShareEntire), dataShareNoLag, dataShareAssociate(dataShareInNs), dataShareAssociate(dataShareInRegionX), dataShareNoLag,
					dataShareExpectConsumer(map[string]types.DataShareStatus{
						dataShareEntireAccount: types.DataShareStatusAvailable, dataShareNamespace: types.DataShareStatusActive, "us-east-1": types.DataShareStatusActive,
					})),
			},
			{
				Rule: "associating again sets AllowWrites, which is false when the call leaves it out, and keeps the association",
				Check: dataShareAsConsumer(dataShareAssociate(redshift.AssociateDataShareConsumerInput{ConsumerArn: aws.String(dataShareNamespace), AllowWrites: aws.Bool(true)}),
					dataShareNoLag, dataShareAssociate(dataShareInNs), func(ctx context.Context, f *dataShareFake) error {
						if dataShareBool(f.store.Resource(dataShareConsumerType, dataShareSales+"|"+dataShareNamespace), "AllowWrites") {
							return fmt.Errorf("ConsumerAcceptedWrites is still true")
						}
						return nil
					}, dataShareExpectConsumer(map[string]types.DataShareStatus{dataShareEntireAccount: types.DataShareStatusAvailable, dataShareNamespace: types.DataShareStatusActive})),
			},
		},
		Unclear: "The page lists InvalidDataShareFault and InvalidNamespaceFault without the cases they cover. " + dataShareRun + ", AWS " +
			"answered a data share no producer authorized for the account, including from the producer account itself, with " +
			"InvalidDataShareFault, a namespace of the account that doesn't exist with InvalidNamespaceFault, a namespace ARN of " +
			"another account with InvalidParameterValue, and a call that named none, or more " +
			"than one, of the entire account, a namespace, and a Region with InvalidParameterValue. It replaced the namespace and " +
			"Region associations with an entire-account one, and the other way around, which the page doesn't say. It answered an " +
			"association again with the same one, with ConsumerAcceptedWrites set to the call's AllowWrites. The call takes no " +
			"idempotency token. The run associated only Redshift Serverless namespaces, never an AWS Glue Data Catalog, so the " +
			"handler takes only namespace ARNs.",
	},
	"DisassociateDataShareConsumer": {
		Doc: dataShareReference + "API_DisassociateDataShareConsumer.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "the data share is associated the way the call names",
				Break: dataShareAsConsumer(dataShareAssociate(dataShareEntire), dataShareDisassociate(redshift.DisassociateDataShareConsumerInput{ConsumerRegion: aws.String("us-east-1")})),
				Code:  dataShareFault,
			},
			{
				Rule: "an association removed once can't be removed again",
				Break: dataShareAsConsumer(dataShareAssociate(dataShareInNs),
					dataShareDisassociate(redshift.DisassociateDataShareConsumerInput{ConsumerArn: aws.String(dataShareNamespace)}),
					dataShareDisassociate(redshift.DisassociateDataShareConsumerInput{ConsumerArn: aws.String(dataShareNamespace)})),
				Code: dataShareFault,
			},
			{
				Rule: "a removed association is gone, after a read that still reports it, and leaves the data share AVAILABLE",
				Doc:  dataShareStatusGuide,
				Check: dataShareAsConsumer(dataShareAssociate(dataShareEntire), dataShareNoLag,
					dataShareDisassociate(redshift.DisassociateDataShareConsumerInput{DisassociateEntireAccount: aws.Bool(true)}),
					dataShareExpectConsumer(map[string]types.DataShareStatus{dataShareEntireAccount: types.DataShareStatusActive}),
					dataShareExpectConsumer(map[string]types.DataShareStatus{dataShareEntireAccount: types.DataShareStatusAvailable})),
			},
		},
		Unclear: "The page lists InvalidDataShareFault without the cases it covers. " + dataShareRun + ", AWS answered an association " +
			"it didn't have, or had already removed, with InvalidDataShareFault. The page doesn't say what AWS does with a call " +
			"that names none, or more than one, of the entire account, a namespace, and a Region; the fake rejects it as " +
			"AssociateDataShareConsumer does, and the handler always names one. The handler takes InvalidDataShareFault as not " +
			"found only when a read then finds no association.",
	},
	"DescribeDataSharesForConsumer": {
		Doc: dataShareReference + "API_DescribeDataSharesForConsumer.html",
		Rules: []nativetest.Rule{
			{
				Rule: "ConsumerArn names a namespace that exists",
				Break: dataShareAsConsumer(func(ctx context.Context, f *dataShareFake) error {
					_, err := f.DescribeDataSharesForConsumer(ctx, &redshift.DescribeDataSharesForConsumerInput{ConsumerArn: aws.String(dataShareNamespace + "0")})
					return err
				}),
				Code: dataShareNamespaceFault,
			},
			{
				Rule: "the producer account sees none of its own data shares",
				Check: dataShareAsConsumer(dataShareAs(dataShareProducer), func(ctx context.Context, f *dataShareFake) error {
					out, err := f.DescribeDataSharesForConsumer(ctx, &redshift.DescribeDataSharesForConsumerInput{})
					if err != nil || len(out.DataShares) != 0 {
						return fmt.Errorf("the producer sees %d data shares: %v", len(out.DataShares), err)
					}
					return nil
				}),
			},
			dataSharePaging(true, func(ctx context.Context, f *dataShareFake, marker *string) (int, *string, error) {
				out, err := f.DescribeDataSharesForConsumer(ctx, &redshift.DescribeDataSharesForConsumerInput{Marker: marker})
				if err != nil {
					return 0, nil, err
				}
				return len(out.DataShares), out.Marker, nil
			}),
		},
		Unclear: dataShareListUnclear + " " + dataShareRun + ", the producer account got an empty list, so the handler reads a " +
			"consumer association only from what the consumer account sees, and never takes the producer's authorization for one.",
	},
}
