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

package opensearch

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/opensearch/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	apiReference  = "https://docs.aws.amazon.com/opensearch-service/latest/APIReference/"
	commonErrors  = apiReference + "CommonErrors.html"
	vpcOptionsDoc = apiReference + "API_VPCOptions.html"
	errorDoc      = apiReference + "API_VpcEndpointError.html"
	endpointGuide = "https://docs.aws.amazon.com/opensearch-service/latest/developerguide/vpc-interface-endpoints.html"

	// constraintNote is what every call's page leaves unclear about its constraints.
	constraintNote = " The page names no error for a value outside its length, pattern, or valid values, or for a required " +
		"value left out; the fake returns ValidationError, from the Common Errors page."

	// busyNote is what the docs leave unclear about a domain that is changing.
	busyNote = " The docs don't say whether AWS takes the call while it is changing the domain or another of its VPC " +
		"endpoints. The fake rejects it then, with "
)

// apiCall is a call against a fake OpenSearch Service API, after any calls it needs to set up.
type apiCall func(ctx context.Context, f *fakeAPI) error

// against returns calls as a rule's Break or Check, made in order against one fake API backed by store. It stops at
// the first call that fails, and returns its error.
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

// createInput returns the input of a create of an endpoint to the domain logs in subnet-a, with the changes edits
// make.
func createInput(edits ...func(*opensearch.CreateVpcEndpointInput)) *opensearch.CreateVpcEndpointInput {
	in := &opensearch.CreateVpcEndpointInput{
		DomainArn:   aws.String(domainArn("logs")),
		VpcOptions:  &types.VPCOptions{SubnetIds: []string{"subnet-a"}},
		ClientToken: aws.String("token-1"),
	}
	for _, edit := range edits {
		edit(in)
	}
	return in
}

func createEndpoint(edits ...func(*opensearch.CreateVpcEndpointInput)) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		_, err := f.CreateVpcEndpoint(ctx, createInput(edits...))
		return err
	}
}

// token returns an edit that sends the client token t.
func token(t string) func(*opensearch.CreateVpcEndpointInput) {
	return func(in *opensearch.CreateVpcEndpointInput) { in.ClientToken = aws.String(t) }
}

// toDomain returns an edit that names the domain.
func toDomain(name string) func(*opensearch.CreateVpcEndpointInput) {
	return func(in *opensearch.CreateVpcEndpointInput) { in.DomainArn = aws.String(domainArn(name)) }
}

// endpointsOf returns the IDs of every endpoint, from every page of ListVpcEndpoints.
func endpointsOf(ctx context.Context, f *fakeAPI) ([]string, error) {
	ids := []string{}
	in := &opensearch.ListVpcEndpointsInput{}
	for {
		page, err := f.ListVpcEndpoints(ctx, in)
		if err != nil {
			return nil, err
		}
		for _, s := range page.VpcEndpointSummaryList {
			ids = append(ids, aws.ToString(s.VpcEndpointId))
		}
		if in.NextToken = page.NextToken; in.NextToken == nil {
			return ids, nil
		}
	}
}

// settleAll reads every endpoint until none is changing.
func settleAll(ctx context.Context, f *fakeAPI) error {
	for range settleReads {
		ids, err := endpointsOf(ctx, f)
		if err != nil || len(ids) == 0 {
			return err
		}
		if _, err := f.DescribeVpcEndpoints(ctx, &opensearch.DescribeVpcEndpointsInput{VpcEndpointIds: ids}); err != nil {
			return err
		}
	}
	return nil
}

// endpointAPI is the record of the AWS calls the VPC endpoint handler makes.
var endpointAPI = map[string]nativetest.Call{
	"CreateVpcEndpoint": {
		Doc: apiReference + "API_CreateVpcEndpoint.html",
		Rules: []nativetest.Rule{
			{
				Rule: "DomainArn is an OpenSearch Service domain ARN of 1 to 512 characters",
				Break: against(createEndpoint(func(in *opensearch.CreateVpcEndpointInput) {
					in.DomainArn = aws.String("arn:aws:es:us-west-2:123456789012:collection/logs")
				})),
				Code: validationCode,
			},
			{
				Rule:  "VpcOptions is required",
				Break: against(createEndpoint(func(in *opensearch.CreateVpcEndpointInput) { in.VpcOptions = nil })),
				Code:  validationCode,
			},
			{
				Rule:  "ClientToken is 1 to 64 characters",
				Break: against(createEndpoint(token(string(make([]byte, 65))))),
				Code:  validationCode,
			},
			{
				Rule: "ClientToken makes the request idempotent, so the same request sent again returns the first endpoint",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					first, err := f.CreateVpcEndpoint(ctx, createInput())
					if err != nil {
						return err
					}
					again, err := f.CreateVpcEndpoint(ctx, createInput())
					if err != nil {
						return err
					}
					if a, b := aws.ToString(first.VpcEndpoint.VpcEndpointId), aws.ToString(again.VpcEndpoint.VpcEndpointId); a != b || len(store.List(vpcEndpointType)) != 1 {
						return fmt.Errorf("the request sent again made the endpoint %s, not %s", b, a)
					}
					return nil
				},
			},
			{
				Rule: "an endpoint created without security groups uses the VPC's default security group",
				Doc:  vpcOptionsDoc,
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					out, err := newFakeAPI(store).CreateVpcEndpoint(ctx, createInput())
					if err != nil {
						return err
					}
					if groups := out.VpcEndpoint.VpcOptions.SecurityGroupIds; !slices.Equal(groups, []string{"sg-default-vpc-1"}) {
						return fmt.Errorf("the endpoint has the security groups %v", groups)
					}
					return nil
				},
			},
			{
				Rule: "a domain can have at most 10 endpoints",
				Doc:  endpointGuide,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					for i := range maxEndpointsPerDomain + 1 {
						if err := createEndpoint(token(fmt.Sprint(i)))(ctx, f); err != nil {
							return err
						}
						if err := settleAll(ctx, f); err != nil {
							return err
						}
					}
					return nil
				},
				Code: "LimitExceededException",
			},
			{
				Rule: "an account can have at most 50 endpoints",
				Doc:  endpointGuide,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					for i := range maxEndpointsPerAccount + 1 {
						arn := fmt.Sprintf("arn:aws:es:us-west-2:%s:domain/other-%d", account, i)
						store.Put(vpcEndpointType, fmt.Sprintf("aos-x%d", i), map[string]any{"DomainArn": arn})
					}
					return createEndpoint()(ctx, f)
				},
				Code: "LimitExceededException",
			},
		},
		Unclear: "How long AWS keeps a client token, and what a token sent again with other values does: the fake keeps " +
			"tokens for good and rejects other values with ValidationError. The developer guide says endpoints connect only to " +
			"VPC domains in the same Region, and to a domain in another account only once its owner authorizes this account, " +
			"but names no error: the fake returns ValidationError for a domain it doesn't have and BaseException for the " +
			"others. An endpoint without subnets fails with ValidationError, and subnets or security groups in more than one " +
			"VPC too." + busyNote + "ConflictException, the conflict this page lists." + constraintNote,
	},
	"DescribeVpcEndpoints": {
		Doc: apiReference + "API_DescribeVpcEndpoints.html",
		Rules: []nativetest.Rule{
			{
				Rule: "VpcEndpointIds is required",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.DescribeVpcEndpoints(ctx, &opensearch.DescribeVpcEndpointsInput{})
					return err
				}),
				Code: validationCode,
			},
			{
				Rule: "each ID matches ^aos-[a-zA-Z0-9]*$ and is 5 to 256 characters",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.DescribeVpcEndpoints(ctx, &opensearch.DescribeVpcEndpointsInput{VpcEndpointIds: []string{"vpce-0123"}})
					return err
				}),
				Code: validationCode,
			},
			{
				Rule: "an ID AWS has no endpoint for comes back in VpcEndpointErrors with ENDPOINT_NOT_FOUND, not as an error",
				Doc:  errorDoc,
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					out, err := newFakeAPI(store).DescribeVpcEndpoints(ctx, &opensearch.DescribeVpcEndpointsInput{VpcEndpointIds: []string{"aos-missing"}})
					if err != nil {
						return err
					}
					if len(out.VpcEndpoints) != 0 || len(out.VpcEndpointErrors) != 1 || out.VpcEndpointErrors[0].ErrorCode != types.VpcEndpointErrorCodeEndpointNotFound {
						return fmt.Errorf("a missing endpoint came back as %+v", out)
					}
					return nil
				},
			},
		},
		Unclear: "How many IDs one call takes, and how soon a change shows: the fake reports a changing endpoint in its " +
			"changing state for two reads, and its Endpoint DNS name only once it is ACTIVE." + constraintNote,
	},
	"UpdateVpcEndpoint": {
		Doc: apiReference + "API_UpdateVpcEndpoint.html",
		Rules: []nativetest.Rule{
			{
				Rule: "VpcEndpointId matches ^aos-[a-zA-Z0-9]*$",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.UpdateVpcEndpoint(ctx, &opensearch.UpdateVpcEndpointInput{VpcEndpointId: aws.String("vpce-1"), VpcOptions: &types.VPCOptions{}})
					return err
				}),
				Code: validationCode,
			},
			{
				Rule: "VpcOptions is required",
				Break: against(createEndpoint(), settleAll, func(ctx context.Context, f *fakeAPI) error {
					_, err := f.UpdateVpcEndpoint(ctx, &opensearch.UpdateVpcEndpointInput{VpcEndpointId: aws.String("aos-0000000000000001")})
					return err
				}),
				Code: validationCode,
			},
			{
				Rule: "an endpoint that doesn't exist fails with ResourceNotFoundException",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.UpdateVpcEndpoint(ctx, &opensearch.UpdateVpcEndpointInput{VpcEndpointId: aws.String("aos-missing"), VpcOptions: &types.VPCOptions{SubnetIds: []string{"subnet-a"}}})
					return err
				}),
				Code: "ResourceNotFoundException",
			},
		},
		Unclear: "What an update that leaves out SecurityGroupIds does: the fake keeps the endpoint's groups, so a handler " +
			"that means to remove them must send the default group. Whether AWS takes an update while the endpoint is still " +
			"changing: the fake rejects it with ConflictException, the conflict this page lists. Subnets in another VPC fail " +
			"with ValidationError." + busyNote + "ConflictException." + constraintNote,
	},
	"DeleteVpcEndpoint": {
		Doc: apiReference + "API_DeleteVpcEndpoint.html",
		Rules: []nativetest.Rule{
			{
				Rule: "an endpoint that doesn't exist fails with ResourceNotFoundException",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.DeleteVpcEndpoint(ctx, &opensearch.DeleteVpcEndpointInput{VpcEndpointId: aws.String("aos-missing")})
					return err
				}),
				Code: "ResourceNotFoundException",
			},
			{
				Rule: "the response reports the endpoint as DELETING",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					if err := createEndpoint()(ctx, f); err != nil {
						return err
					}
					if err := settleAll(ctx, f); err != nil {
						return err
					}
					out, err := f.DeleteVpcEndpoint(ctx, &opensearch.DeleteVpcEndpointInput{VpcEndpointId: aws.String(store.List(vpcEndpointType)[0])})
					if err != nil {
						return err
					}
					if out.VpcEndpointSummary.Status != types.VpcEndpointStatusDeleting {
						return fmt.Errorf("the deleted endpoint is %s", out.VpcEndpointSummary.Status)
					}
					return nil
				},
			},
		},
		Unclear: "Whether AWS deletes an endpoint it is still creating, updating, or deleting, and with what error: the page " +
			"lists no conflict, so the fake rejects it with BaseException.",
	},
	"ListVpcEndpoints": {
		Doc: apiReference + "API_ListVpcEndpoints.html",
		Rules: []nativetest.Rule{
			{
				Rule: "the list has every endpoint in the account and Region, across pages",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					for i, name := range []string{"logs", "metrics", "logs"} {
						if err := createEndpoint(token(fmt.Sprint(i)), toDomain(name))(ctx, f); err != nil {
							return err
						}
						if err := settleAll(ctx, f); err != nil {
							return err
						}
					}
					ids, err := endpointsOf(ctx, f)
					if err != nil {
						return err
					}
					if len(ids) != 3 {
						return fmt.Errorf("the list has %d endpoints, not 3", len(ids))
					}
					return nil
				},
			},
		},
		Unclear: "The page size: the fake returns two endpoints a page.",
	},
	"DescribeSecurityGroups": {
		Doc: "https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeSecurityGroups.html",
		Rules: []nativetest.Rule{
			{
				Rule: "the vpc-id and group-name filters select the groups with that VPC and name, such as a VPC's default group",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					out, err := newFakeAPI(store).DescribeSecurityGroups(ctx, &awsec2.DescribeSecurityGroupsInput{Filters: []ec2types.Filter{
						{Name: aws.String("vpc-id"), Values: []string{"vpc-2"}},
						{Name: aws.String("group-name"), Values: []string{"default"}},
					}})
					if err != nil {
						return err
					}
					if len(out.SecurityGroups) != 1 || aws.ToString(out.SecurityGroups[0].GroupId) != "sg-default-vpc-2" {
						return fmt.Errorf("the filters selected %+v", out.SecurityGroups)
					}
					return nil
				},
			},
		},
	},
}

// authorize authorizes the account on the domain logs, with the changes edits make.
func authorize(edits ...func(*opensearch.AuthorizeVpcEndpointAccessInput)) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		in := &opensearch.AuthorizeVpcEndpointAccessInput{DomainName: aws.String("logs"), Account: aws.String(otherAccount)}
		for _, edit := range edits {
			edit(in)
		}
		_, err := f.AuthorizeVpcEndpointAccess(ctx, in)
		return err
	}
}

// accessAPI is the record of the AWS calls the VPC endpoint access handler makes.
var accessAPI = map[string]nativetest.Call{
	"AuthorizeVpcEndpointAccess": {
		Doc: apiReference + "API_AuthorizeVpcEndpointAccess.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "DomainName is 3 to 28 characters matching [a-z][a-z0-9\\-]+",
				Break: against(authorize(func(in *opensearch.AuthorizeVpcEndpointAccessInput) { in.DomainName = aws.String("Logs") })),
				Code:  validationCode,
			},
			{
				Rule:  "Account matches ^[0-9]+$",
				Break: against(authorize(func(in *opensearch.AuthorizeVpcEndpointAccessInput) { in.Account = aws.String("acct") })),
				Code:  validationCode,
			},
			{
				Rule: "Service is application.opensearchservice.amazonaws.com",
				Break: against(authorize(func(in *opensearch.AuthorizeVpcEndpointAccessInput) {
					in.Account, in.Service = nil, "es.amazonaws.com"
				})),
				Code: validationCode,
			},
			{
				Rule:  "a domain that doesn't exist fails with ResourceNotFoundException",
				Break: against(authorize(func(in *opensearch.AuthorizeVpcEndpointAccessInput) { in.DomainName = aws.String("missing") })),
				Code:  "ResourceNotFoundException",
			},
			{
				Rule: "a domain can have at most 10 authorized principals",
				Doc:  endpointGuide,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					for i := range maxPrincipals + 1 {
						if err := authorize(func(in *opensearch.AuthorizeVpcEndpointAccessInput) {
							in.Account = aws.String(fmt.Sprintf("1000000000%02d", i))
						})(ctx, f); err != nil {
							return err
						}
					}
					return nil
				},
				Code: "LimitExceededException",
			},
		},
		Unclear: "What a call with both Account and Service, or neither, does: the fake rejects it with ValidationError. What " +
			"authorizing a principal the domain already lists does: the fake rejects it with BaseException, so a handler " +
			"must read the list first." + busyNote + "BaseException, because the page lists no conflict." + constraintNote,
	},
	"RevokeVpcEndpointAccess": {
		Doc: apiReference + "API_RevokeVpcEndpointAccess.html",
		Rules: []nativetest.Rule{
			{
				Rule: "DomainName is 3 to 28 characters matching [a-z][a-z0-9\\-]+",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.RevokeVpcEndpointAccess(ctx, &opensearch.RevokeVpcEndpointAccessInput{DomainName: aws.String("x"), Account: aws.String(otherAccount)})
					return err
				}),
				Code: validationCode,
			},
			{
				Rule: "a domain that doesn't exist fails with ResourceNotFoundException",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.RevokeVpcEndpointAccess(ctx, &opensearch.RevokeVpcEndpointAccessInput{DomainName: aws.String("missing"), Account: aws.String(otherAccount)})
					return err
				}),
				Code: "ResourceNotFoundException",
			},
		},
		Unclear: "What revoking a principal the domain doesn't list does: the fake fails with ResourceNotFoundException. What " +
			"ServiceOptions changes in a revoke, and what happens to endpoints the principal already made. " + busyNote +
			"BaseException." + constraintNote,
	},
	"ListVpcEndpointAccess": {
		Doc: apiReference + "API_ListVpcEndpointAccess.html",
		Rules: []nativetest.Rule{
			{
				Rule: "a domain that doesn't exist fails with ResourceNotFoundException",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.ListVpcEndpointAccess(ctx, &opensearch.ListVpcEndpointAccessInput{DomainName: aws.String("missing")})
					return err
				}),
				Code: "ResourceNotFoundException",
			},
			{
				Rule: "the list has every principal the domain authorized, across pages",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					for i := range 3 {
						if err := authorize(func(in *opensearch.AuthorizeVpcEndpointAccessInput) {
							in.Account = aws.String(fmt.Sprintf("10000000000%d", i))
						})(ctx, f); err != nil {
							return err
						}
					}
					count := 0
					in := &opensearch.ListVpcEndpointAccessInput{DomainName: aws.String("logs")}
					for {
						page, err := f.ListVpcEndpointAccess(ctx, in)
						if err != nil {
							return err
						}
						count += len(page.AuthorizedPrincipalList)
						if in.NextToken = page.NextToken; in.NextToken == nil {
							break
						}
					}
					if count != 3 {
						return errors.New("the list doesn't have every principal")
					}
					return nil
				},
			},
		},
		Unclear: "Whether Principal holds an account's ID or its root ARN, since the page calls it an IAM principal: the fake " +
			"returns the ID, and the handler's tests also run with the ARN. PrincipalType: the page lists AWS_ACCOUNT and " +
			"AWS_SERVICE, but other AWS clients report that AWS returns \"AWS Account\" for an account, so the fake returns " +
			"\"AWS Account\" and \"AWS Service\", and the handler's tests also run with the documented values. The page size: " +
			"the fake returns two principals a page.",
	},
}
