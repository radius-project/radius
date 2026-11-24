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

package kms

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	apiReference = "https://docs.aws.amazon.com/kms/latest/APIReference/"
	guide        = "https://docs.aws.amazon.com/kms/latest/developerguide/"
	keyStates    = guide + "key-state.html"
	creating     = guide + "create-grant-overview.html"
	retiring     = guide + "grant-delete.html"
	consistency  = guide + "grants.html#terms-eventual-consistency"
	constraints  = apiReference + "API_GrantConstraints.html"

	// validationNote is what every call's page leaves unclear about its constraints.
	validationNote = " The page names no error for a value outside its length, pattern, or range, or for parameters " +
		"that can't be combined; the fake returns ValidationError, the error the CreateGrant page gives for an " +
		"operation the key doesn't support."

	grantee = "arn:aws:iam::" + account + ":role/app"
)

// apiCall is a call against a fake KMS API, after any calls it needs to set up.
type apiCall func(ctx context.Context, f *fakeAPI) error

// against returns calls as a rule's Break or Check, made in order against one fake KMS API backed by store. It stops
// at the first call that fails, and returns its error.
func against(calls ...apiCall) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		f := newFakeAPI(store)
		for _, call := range calls {
			if err := call(ctx, f); err != nil {
				return err
			}
		}
		return nil
	}
}

// createInput returns the input of a grant that lets the app role decrypt with keyID, with the changes edits make.
func createInput(edits ...func(*kms.CreateGrantInput)) *kms.CreateGrantInput {
	in := &kms.CreateGrantInput{
		KeyId:            aws.String(keyID),
		GranteePrincipal: aws.String(grantee),
		Operations:       []types.GrantOperation{types.GrantOperationDecrypt},
	}
	for _, edit := range edits {
		edit(in)
	}
	return in
}

// createGrant creates the grant createInput gives with edits.
func createGrant(edits ...func(*kms.CreateGrantInput)) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		_, err := f.CreateGrant(ctx, createInput(edits...))
		return err
	}
}

// named returns an edit that names the grant.
func named(name string) func(*kms.CreateGrantInput) {
	return func(in *kms.CreateGrantInput) { in.Name = aws.String(name) }
}

// onKey returns an edit that sets the grant's key.
func onKey(key string) func(*kms.CreateGrantInput) {
	return func(in *kms.CreateGrantInput) { in.KeyId = aws.String(key) }
}

// allowing returns an edit that sets the grant's operations.
func allowing(ops ...types.GrantOperation) func(*kms.CreateGrantInput) {
	return func(in *kms.CreateGrantInput) { in.Operations = ops }
}

// constrained returns an edit that sets the grant's constraints.
func constrained(c types.GrantConstraints) func(*kms.CreateGrantInput) {
	return func(in *kms.CreateGrantInput) { in.Constraints = &c }
}

// toService returns an edit that gives the grant to a service principal instead of the app role.
func toService(in *kms.CreateGrantInput) {
	in.GranteePrincipal, in.GranteeServicePrincipal = nil, aws.String("rds.amazonaws.com")
}

// pairs returns an encryption context of n pairs.
func pairs(n int) map[string]string {
	out := map[string]string{}
	for i := range n {
		out[fmt.Sprint("k", i)] = "v"
	}
	return out
}

// listAll returns the key's grants that in names, from every page.
func listAll(ctx context.Context, f *fakeAPI, in *kms.ListGrantsInput) ([]types.GrantListEntry, error) {
	out := []types.GrantListEntry{}
	for {
		page, err := f.ListGrants(ctx, in)
		if err != nil {
			return nil, err
		}
		out = append(out, page.Grants...)
		if !page.Truncated {
			return out, nil
		}
		in.Marker = page.NextMarker
	}
}

// expect returns an error unless got is want.
func expect[T comparable](what string, got T, want T) error {
	if got != want {
		return fmt.Errorf("%s is %v, not %v", what, got, want)
	}
	return nil
}

// countOn returns the number of grants ListGrants lists on the key.
func countOn(ctx context.Context, f *fakeAPI, key string) (int, error) {
	list, err := listAll(ctx, f, &kms.ListGrantsInput{KeyId: aws.String(key)})
	return len(list), err
}

// grantAPI records the KMS calls Grant makes.
var grantAPI = map[string]nativetest.Call{
	"CreateGrant": {
		Doc: apiReference + "API_CreateGrant.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "the key exists in the caller's account and the call's region",
				Break: against(createGrant(onKey("ffffffff-ffff-ffff-ffff-ffffffffffff"))),
				Code:  "NotFoundException",
			},
			{
				Rule: "a key ID names a key in the call's region only",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.CreateGrant(ctx, createInput(), func(o *kms.Options) { o.Region = "us-east-1" })
					return err
				}),
				Code: "NotFoundException",
			},
			{
				Rule:  "a KeyId ARN is a valid ARN",
				Break: against(createGrant(onKey("arn:aws:kms:us-west-2:123:key"))),
				Code:  "InvalidArnException",
			},
			{
				Rule:  "KeyId is a key ID or key ARN, not an alias",
				Break: against(createGrant(onKey("alias/app"))),
				Code:  "NotFoundException",
			},
			{
				Rule:  "a disabled key takes no grant",
				Doc:   keyStates,
				Break: against(createGrant(onKey(disabledKeyID))),
				Code:  "DisabledException",
			},
			{
				Rule:  "a key pending deletion takes no grant",
				Doc:   keyStates,
				Break: against(createGrant(onKey(deletingKeyID))),
				Code:  "KMSInvalidStateException",
			},
			{
				Rule:  "Operations lists only operations the key supports, so a symmetric key takes no Sign",
				Break: against(createGrant(allowing(types.GrantOperationSign))),
				Code:  validationCode,
			},
			{
				Rule:  "an asymmetric signing key takes no GenerateDataKey",
				Break: against(createGrant(onKey(signingKeyID), allowing(types.GrantOperationGenerateDataKey))),
				Code:  validationCode,
			},
			{
				Rule: "every key takes CreateGrant, RetireGrant, and DescribeKey",
				Check: against(createGrant(onKey(signingKeyID), allowing(types.GrantOperationSign, types.GrantOperationRetireGrant,
					types.GrantOperationDescribeKey, types.GrantOperationCreateGrant))),
			},
			{
				Rule:  "Operations is required",
				Break: against(createGrant(allowing())),
				Code:  validationCode,
			},
			{
				Rule:  "the grant has GranteePrincipal or GranteeServicePrincipal, not both",
				Break: against(createGrant(func(in *kms.CreateGrantInput) { in.GranteeServicePrincipal = aws.String("rds.amazonaws.com") })),
				Code:  validationCode,
			},
			{
				Rule:  "the grant has a grantee",
				Break: against(createGrant(func(in *kms.CreateGrantInput) { in.GranteePrincipal = nil })),
				Code:  validationCode,
			},
			{
				Rule: "the grant has RetiringPrincipal or RetiringServicePrincipal, not both",
				Break: against(createGrant(func(in *kms.CreateGrantInput) {
					in.RetiringPrincipal, in.RetiringServicePrincipal = aws.String(grantee), aws.String("rds.amazonaws.com")
				})),
				Code: validationCode,
			},
			{
				Rule: "a grantee service principal needs a SourceArn constraint",
				Doc:  creating,
				Break: against(createGrant(toService, func(in *kms.CreateGrantInput) {
					in.RetiringServicePrincipal = aws.String("rds.amazonaws.com")
				})),
				Code: validationCode,
			},
			{
				Rule:  "a grantee service principal needs a retiring principal",
				Doc:   creating,
				Break: against(createGrant(toService, constrained(types.GrantConstraints{SourceArn: aws.String("arn:aws:rds:us-west-2:" + account + ":db:app")}))),
				Code:  validationCode,
			},
			{
				Rule: "a grantee service principal with a SourceArn constraint and a retiring principal is a grant",
				Doc:  creating,
				Check: against(createGrant(toService, constrained(types.GrantConstraints{SourceArn: aws.String("arn:aws:rds:us-west-2:" + account + ":db:app")}),
					func(in *kms.CreateGrantInput) { in.RetiringServicePrincipal = aws.String("rds.amazonaws.com") })),
			},
			{
				Rule: "GranteePrincipal is 1 to 256 characters of letters, digits, and +=,.@:/_-",
				Break: against(createGrant(func(in *kms.CreateGrantInput) {
					in.GranteePrincipal = aws.String("arn:aws:iam::" + account + ":role/a b")
				})),
				Code: validationCode,
			},
			{
				Rule:  "a service principal is a dotted name, such as rds.amazonaws.com",
				Break: against(createGrant(func(in *kms.CreateGrantInput) { in.RetiringServicePrincipal = aws.String("rds") })),
				Code:  validationCode,
			},
			{
				Rule:  "Name is 1 to 256 characters of letters, digits, and :/_-",
				Break: against(createGrant(named("app grant"))),
				Code:  validationCode,
			},
			{
				Rule:  "Name is at most 256 characters",
				Break: against(createGrant(named(strings.Repeat("a", 257)))),
				Code:  validationCode,
			},
			{
				Rule:  "an encryption context constraint has at most 8 pairs",
				Break: against(createGrant(constrained(types.GrantConstraints{EncryptionContextEquals: pairs(9)}))),
				Code:  validationCode,
			},
			{
				Rule:  "a grant has EncryptionContextEquals or EncryptionContextSubset, not both",
				Doc:   creating,
				Break: against(createGrant(constrained(types.GrantConstraints{EncryptionContextEquals: pairs(1), EncryptionContextSubset: pairs(1)}))),
				Code:  validationCode,
			},
			{
				Rule:  "an encryption context value is at most 384 characters",
				Break: against(createGrant(constrained(types.GrantConstraints{EncryptionContextSubset: map[string]string{"k": strings.Repeat("v", 385)}}))),
				Code:  validationCode,
			},
			{
				Rule:  "an encryption context constraint is only for a symmetric encryption key",
				Doc:   constraints,
				Break: against(createGrant(onKey(signingKeyID), allowing(types.GrantOperationSign), constrained(types.GrantConstraints{EncryptionContextEquals: pairs(1)}))),
				Code:  validationCode,
			},
			{
				Rule:  "SourceArn is an ARN of 20 to 512 characters",
				Doc:   constraints,
				Break: against(createGrant(constrained(types.GrantConstraints{SourceArn: aws.String("arn:aws:s3:::bucket")}))),
				Code:  validationCode,
			},
			{
				Rule: "a key has at most 50,000 grants",
				Doc:  guide + "resource-limits.html",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					f.max = 1
					return nil
				}, createGrant(), createGrant()),
				Code: "LimitExceededException",
			},
			{
				Rule: "a create with the same Name and values as a grant returns that grant's ID, with a new grant token",
				Doc:  creating,
				Check: against(func(ctx context.Context, f *fakeAPI) error {
					first, err := f.CreateGrant(ctx, createInput(named("app")))
					if err != nil {
						return err
					}
					again, err := f.CreateGrant(ctx, createInput(named("app")))
					if err != nil {
						return err
					}
					if aws.ToString(again.GrantToken) == aws.ToString(first.GrantToken) {
						return fmt.Errorf("the grant token is the same")
					}
					if err := expect("the grant ID", aws.ToString(again.GrantId), aws.ToString(first.GrantId)); err != nil {
						return err
					}
					n, err := countOn(ctx, f, keyID)
					if err != nil {
						return err
					}
					return expect("the number of grants", n, 1)
				}),
			},
			{
				Rule: "a create with the same Name and other values, or without a Name, makes a new grant",
				Doc:  creating,
				Check: against(createGrant(named("app")), createGrant(named("app"), allowing(types.GrantOperationEncrypt)),
					createGrant(), createGrant(), func(ctx context.Context, f *fakeAPI) error {
						n, err := countOn(ctx, f, keyID)
						if err != nil {
							return err
						}
						return expect("the number of grants", n, 4)
					}),
			},
		},
		Unclear: "The docs say a create with the same properties as an existing grant, including the name, returns " +
			"that grant, without saying whether operations in another order are the same properties; the fake treats " +
			"them as the same. The key state table gives DisabledException or KMSInvalidStateException for a key " +
			"pending deletion; the fake returns KMSInvalidStateException. The page says KeyId is a key ID or key ARN, " +
			"without naming the error for an alias; the fake returns NotFoundException, and the handler rejects an " +
			"alias before any call. It says an encryption context constraint applies only to operations that take an " +
			"encryption context, without saying whether a grant for another key with one is rejected; the fake rejects " +
			"it. The page doesn't say how long a new grant takes to show in ListGrants; the fake lists it at once " +
			"unless a test asks for a lag." + validationNote,
	},
	"ListGrants": {
		Doc: apiReference + "API_ListGrants.html",
		Rules: []nativetest.Rule{
			{
				Rule: "the key exists",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.ListGrants(ctx, &kms.ListGrantsInput{KeyId: aws.String("ffffffff-ffff-ffff-ffff-ffffffffffff")})
					return err
				}),
				Code: "NotFoundException",
			},
			{
				Rule: "Limit is 1 to 100",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.ListGrants(ctx, &kms.ListGrantsInput{KeyId: aws.String(keyID), Limit: aws.Int32(101)})
					return err
				}),
				Code: validationCode,
			},
			{
				Rule: "the call filters by GranteePrincipal or GranteeServicePrincipal, not both",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.ListGrants(ctx, &kms.ListGrantsInput{KeyId: aws.String(keyID), GranteePrincipal: aws.String(grantee), GranteeServicePrincipal: aws.String("rds.amazonaws.com")})
					return err
				}),
				Code: validationCode,
			},
			{
				Rule: "GrantId is 1 to 128 characters",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.ListGrants(ctx, &kms.ListGrantsInput{KeyId: aws.String(keyID), GrantId: aws.String(strings.Repeat("a", 129))})
					return err
				}),
				Code: validationCode,
			},
			{
				Rule: "Marker is a NextMarker ListGrants returned",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.ListGrants(ctx, &kms.ListGrantsInput{KeyId: aws.String(keyID), Marker: aws.String("other")})
					return err
				}),
				Code: "InvalidMarkerException",
			},
			{
				Rule: "a page has at most Limit grants and may have fewer; Truncated and NextMarker give the next page",
				Check: against(createGrant(), createGrant(), createGrant(), func(ctx context.Context, f *fakeAPI) error {
					page, err := f.ListGrants(ctx, &kms.ListGrantsInput{KeyId: aws.String(keyID), Limit: aws.Int32(1)})
					if err != nil {
						return err
					}
					if len(page.Grants) != 1 || !page.Truncated || page.NextMarker == nil {
						return fmt.Errorf("the first page is %+v", page)
					}
					n, err := countOn(ctx, f, keyID)
					if err != nil {
						return err
					}
					return expect("the number of grants on every page", n, 3)
				}),
			},
			{
				Rule: "GrantId and GranteePrincipal return only the grants they name",
				Check: against(createGrant(named("app")), createGrant(func(in *kms.CreateGrantInput) {
					in.GranteePrincipal = aws.String("arn:aws:iam::" + account + ":role/other")
				}), func(ctx context.Context, f *fakeAPI) error {
					list, err := listAll(ctx, f, &kms.ListGrantsInput{KeyId: aws.String(keyID), GranteePrincipal: aws.String(grantee)})
					if err != nil {
						return err
					}
					if len(list) != 1 {
						return fmt.Errorf("the grantee has %d grants", len(list))
					}
					list, err = listAll(ctx, f, &kms.ListGrantsInput{KeyId: aws.String(keyID), GrantId: list[0].GrantId})
					if err != nil {
						return err
					}
					if err := expect("the number of grants with the ID", len(list), 1); err != nil {
						return err
					}
					return expect("the name", aws.ToString(list[0].Name), "app")
				}),
			},
			{
				Rule: "a grant lists its key by ARN, even when it was created by key ID, and its issuing account",
				Check: against(createGrant(), func(ctx context.Context, f *fakeAPI) error {
					list, err := listAll(ctx, f, &kms.ListGrantsInput{KeyId: aws.String(keyID)})
					if err != nil {
						return err
					}
					if err := expect("the key", aws.ToString(list[0].KeyId), keyARN(keyID)); err != nil {
						return err
					}
					if err := expect("the name of a grant without one", aws.ToString(list[0].Name), ""); err != nil {
						return err
					}
					return expect("the issuing account", aws.ToString(list[0].IssuingAccount), "arn:aws:iam::"+account+":root")
				}),
			},
			{
				Rule: "the grants of a key in any state can be listed",
				Doc:  keyStates,
				Check: against(func(ctx context.Context, f *fakeAPI) error {
					for _, key := range []string{disabledKeyID, deletingKeyID} {
						if _, err := countOn(ctx, f, key); err != nil {
							return err
						}
					}
					return nil
				}),
			},
			{
				Rule: "a retired grant can still be listed for a short while",
				Doc:  consistency,
				Check: against(func(ctx context.Context, f *fakeAPI) error {
					out, err := f.CreateGrant(ctx, createInput())
					if err != nil {
						return err
					}
					if _, err := f.RetireGrant(ctx, &kms.RetireGrantInput{KeyId: aws.String(keyID), GrantId: out.GrantId}); err != nil {
						return err
					}
					for _, want := range []int{1, 0} {
						n, err := countOn(ctx, f, keyID)
						if err != nil {
							return err
						}
						if err := expect("the number of grants listed", n, want); err != nil {
							return err
						}
					}
					return nil
				}),
			},
		},
		Unclear: "The page doesn't say what a GrantId filter that names no grant returns, or when it gets " +
			"InvalidGrantIdException; the fake returns an empty list, and the handler treats both as no grant. It doesn't " +
			"give the order of the grants or of their operations; the fake lists grants by identifier and operations in " +
			"the order the create sent them. It doesn't say how long a change takes to show; the fake lists a retired " +
			"grant for one more read, and with lagNew hides a new one from its first." + validationNote,
	},
	"RetireGrant": {
		Doc: apiReference + "API_RetireGrant.html",
		Rules: []nativetest.Rule{
			{
				Rule: "the grant exists",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.RetireGrant(ctx, &kms.RetireGrantInput{KeyId: aws.String(keyARN(keyID)), GrantId: aws.String("missing")})
					return err
				}),
				Code: "NotFoundException",
			},
			{
				Rule: "the call names the grant with a grant token, or with both the key and the grant ID",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.RetireGrant(ctx, &kms.RetireGrantInput{GrantId: aws.String("missing")})
					return err
				}),
				Code: validationCode,
			},
			{
				Rule: "a grant token is one CreateGrant returned",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.RetireGrant(ctx, &kms.RetireGrantInput{GrantToken: aws.String("other")})
					return err
				}),
				Code: "InvalidGrantTokenException",
			},
			{
				Rule: "the account that created the grant can retire it, by key and grant ID or by grant token",
				Doc:  retiring,
				Check: against(func(ctx context.Context, f *fakeAPI) error {
					first, err := f.CreateGrant(ctx, createInput())
					if err != nil {
						return err
					}
					second, err := f.CreateGrant(ctx, createInput())
					if err != nil {
						return err
					}
					if _, err := f.RetireGrant(ctx, &kms.RetireGrantInput{KeyId: aws.String(keyARN(keyID)), GrantId: first.GrantId}); err != nil {
						return err
					}
					if _, err := f.RetireGrant(ctx, &kms.RetireGrantInput{GrantToken: second.GrantToken}); err != nil {
						return err
					}
					return expect("the number of grants", len(f.grantsOf(keyID)), 0)
				}),
			},
		},
		Unclear: "The page says KeyId is the key ARN, and also that a key ID works; the fake takes both. It lists " +
			"NotFoundException and InvalidGrantIdException without saying which one a grant AWS already retired gets; " +
			"the fake returns NotFoundException, and the handler treats both as a retired grant. It doesn't say whether " +
			"the caller needs a kms:RetireGrant IAM permission; the developer guide says the account that created the " +
			"grant can delegate it, so the schema lists it. The fake's caller is the account that created every grant." +
			validationNote,
	},
	"DescribeKey": {
		Doc: apiReference + "API_DescribeKey.html",
		Rules: []nativetest.Rule{
			{
				Rule: "the key exists",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.DescribeKey(ctx, &kms.DescribeKeyInput{KeyId: aws.String("alias/missing")})
					return err
				}),
				Code: "NotFoundException",
			},
			{
				Rule: "KeyId is a key ID, key ARN, alias name, or alias ARN, and the answer has the key ARN",
				Check: against(func(ctx context.Context, f *fakeAPI) error {
					for _, key := range []string{keyID, keyARN(keyID), "alias/app", "arn:aws:kms:" + region + ":" + account + ":alias/app"} {
						out, err := f.DescribeKey(ctx, &kms.DescribeKeyInput{KeyId: aws.String(key)})
						if err != nil {
							return err
						}
						if err := expect("the ARN of "+key, aws.ToString(out.KeyMetadata.Arn), keyARN(keyID)); err != nil {
							return err
						}
					}
					return nil
				}),
			},
			{
				Rule: "a key in any state can be described",
				Doc:  keyStates,
				Check: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.DescribeKey(ctx, &kms.DescribeKeyInput{KeyId: aws.String(deletingKeyID)})
					return err
				}),
			},
		},
		Unclear: "The handler calls DescribeKey only for the ARN of a key a template or name gives by key ID." + validationNote,
	},
}
