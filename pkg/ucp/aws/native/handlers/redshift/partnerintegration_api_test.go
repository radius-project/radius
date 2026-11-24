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

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	partnerReference = "https://docs.aws.amazon.com/redshift/latest/APIReference/"

	// partnerLimitsUnclear is what the pages leave unclear about the limits of the values.
	partnerLimitsUnclear = "The page gives each value's length and pattern without the error code of a value outside " +
		"them; AWS answered such values with ValidationError in a run against real AWS. The page names " +
		"UnauthorizedPartnerIntegration and UnsupportedOperation without saying when AWS returns them, so the fake " +
		"never does."
)

// partnerCall is a call against a fake Redshift API for partner integrations, after any calls it needs to set up.
type partnerCall func(ctx context.Context, f *partnerFake) error

// partnerAgainst returns calls as a rule's Break or Check, made in order against one fake backed by store, which has
// the fake's clusters. It stops at the first call that fails, and returns its error.
func partnerAgainst(calls ...partnerCall) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		addPartnerClusters(store)
		f := newPartnerFake(store)
		for _, call := range calls {
			if err := call(ctx, f); err != nil {
				return err
			}
		}
		return nil
	}
}

// partnerInput is the values of a partner call: the fake's account and cluster, the database dev, and Fivetran, with
// the changes edits make.
type partnerInput struct {
	account, cluster, database, name *string
}

func newPartnerInput(edits ...func(*partnerInput)) partnerInput {
	in := partnerInput{account: aws.String(partnerAccount), cluster: aws.String(partnerCluster), database: aws.String("dev"), name: aws.String("Fivetran")}
	for _, edit := range edits {
		edit(&in)
	}
	return in
}

func addPartner(edits ...func(*partnerInput)) partnerCall {
	return func(ctx context.Context, f *partnerFake) error {
		in := newPartnerInput(edits...)
		_, err := f.AddPartner(ctx, &redshift.AddPartnerInput{AccountId: in.account, ClusterIdentifier: in.cluster, DatabaseName: in.database, PartnerName: in.name})
		return err
	}
}

func deletePartner(edits ...func(*partnerInput)) partnerCall {
	return func(ctx context.Context, f *partnerFake) error {
		in := newPartnerInput(edits...)
		_, err := f.DeletePartner(ctx, &redshift.DeletePartnerInput{AccountId: in.account, ClusterIdentifier: in.cluster, DatabaseName: in.database, PartnerName: in.name})
		return err
	}
}

// describePartners returns a call that lists the integrations of the input, and fails unless it lists want of them.
func describePartners(want int, edits ...func(*partnerInput)) partnerCall {
	return func(ctx context.Context, f *partnerFake) error {
		in := newPartnerInput(edits...)
		out, err := f.DescribePartners(ctx, &redshift.DescribePartnersInput{AccountId: in.account, ClusterIdentifier: in.cluster, DatabaseName: in.database, PartnerName: in.name})
		if err != nil {
			return err
		}
		if len(out.PartnerIntegrationInfoList) != want {
			return fmt.Errorf("AWS lists %d integrations, not %d", len(out.PartnerIntegrationInfoList), want)
		}
		return nil
	}
}

func partnerName(name string) func(*partnerInput) {
	return func(in *partnerInput) { in.name = aws.String(name) }
}

func partnerDatabase(name *string) func(*partnerInput) {
	return func(in *partnerInput) { in.database = name }
}

// partnerAnyName leaves the partner name out of a call.
func partnerAnyName(in *partnerInput) {
	in.name = nil
}

// partnerIdentityRules are the rules of the values that identify an integration, for a call made with them.
func partnerIdentityRules(call func(edits ...func(*partnerInput)) partnerCall) []nativetest.Rule {
	return []nativetest.Rule{
		{
			Rule:  "the account ID is 12 digits",
			Break: partnerAgainst(call(func(in *partnerInput) { in.account = aws.String("12345") })),
			Code:  partnerValidation,
		},
		{
			Rule:  "the cluster identifier is at most 63 letters, digits, and hyphens",
			Break: partnerAgainst(call(func(in *partnerInput) { in.cluster = aws.String("my_cluster") })),
			Code:  partnerValidation,
		},
		{
			Rule:  "the cluster exists",
			Break: partnerAgainst(call(func(in *partnerInput) { in.cluster = aws.String("missing") })),
			Code:  partnerClusterNotFound,
		},
		{
			Rule:  "the account owns the cluster",
			Break: partnerAgainst(call(func(in *partnerInput) { in.account = aws.String("111122223333") })),
			Code:  partnerClusterNotFound,
		},
		{
			Rule:  "the database name starts with a letter or _, then letters, digits, and @$#_",
			Break: partnerAgainst(call(partnerDatabase(aws.String("1dev")))),
			Code:  partnerValidation,
		},
		{
			Rule:  "the partner name is letters, digits, hyphens, and underscores",
			Break: partnerAgainst(call(partnerName("Five tran"))),
			Code:  partnerValidation,
		},
	}
}

// partnerAPI records the Redshift calls PartnerIntegration makes.
var partnerAPI = map[string]nativetest.Call{
	"AddPartner": {
		Doc: partnerReference + "API_AddPartner.html",
		Rules: append(partnerIdentityRules(addPartner), []nativetest.Rule{
			{
				Rule:  "the partner is one AWS supports",
				Break: partnerAgainst(addPartner(partnerName("Sisu"))),
				Code:  partnerNotFound,
			},
			{
				Rule:  "AWS accepts an integration it already has, and keeps one",
				Check: partnerAgainst(addPartner(), addPartner(), describePartners(1)),
			},
		}...),
		Unclear: partnerLimitsUnclear + " The page doesn't list the partners AWS supports; AWS added Datacoral, Etleap, " +
			"Fivetran, Informatica, and Matillion, and answered Sisu and fivetran with PartnerNotFound, in a run against " +
			"real AWS, accepted an integration it already had, accepted a database the cluster doesn't have, took a " +
			"cluster identifier in upper case as the same cluster, and added an integration while the cluster was " +
			"modifying; the fake does the same. The page doesn't say how soon a new integration is listed. AWS listed it " +
			"at once in that run, and so does the fake: the handler takes an integration AWS doesn't list after a create " +
			"as gone, so the deploy fails and the next one adds it again.",
	},
	"DeletePartner": {
		Doc: partnerReference + "API_DeletePartner.html",
		Rules: append(partnerIdentityRules(deletePartner), []nativetest.Rule{
			{
				Rule:  "the integration exists",
				Break: partnerAgainst(deletePartner()),
				Code:  partnerNotFound,
			},
		}...),
		Unclear: partnerLimitsUnclear + " The page doesn't say how soon a deleted integration is gone. AWS no longer listed " +
			"it in a run against real AWS; the fake still lists it for one read.",
	},
	"DescribePartners": {
		Doc: partnerReference + "API_DescribePartners.html",
		Rules: append(partnerIdentityRules(func(edits ...func(*partnerInput)) partnerCall { return describePartners(0, edits...) }), []nativetest.Rule{
			{
				Rule: "a call without the database name or partner name lists every integration of the cluster",
				Check: partnerAgainst(addPartner(), addPartner(partnerName("Etleap")), addPartner(partnerDatabase(aws.String("sales"))),
					describePartners(3, partnerDatabase(nil), partnerAnyName), describePartners(2, partnerAnyName), describePartners(1)),
			},
			{
				Rule:  "names match in their own case",
				Check: partnerAgainst(addPartner(), describePartners(0, partnerDatabase(aws.String("DEV")))),
			},
		}...),
		Unclear: partnerLimitsUnclear + " The page doesn't say whether the filters match names in any case. In a run " +
			"against real AWS, a call with the database name DEV listed none of the integrations of dev. AWS returned no " +
			"Status until the partner reported one, and so does the fake.",
	},
}
