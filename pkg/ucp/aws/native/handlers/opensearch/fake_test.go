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
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/opensearch/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	// account is the fake's caller, and otherAccount another account.
	account      = "123456789012"
	otherAccount = "210987654321"

	region = "us-west-2"

	// pageSize is how many items a list page has. AWS doesn't document its page size.
	pageSize = 2

	// settleReads is how many reads of a changing endpoint return it in its changing state before it settles. AWS
	// says a new endpoint is active within 2 to 5 minutes, so the first read never sees it settled.
	settleReads = 2

	// maxEndpointsPerDomain, maxEndpointsPerAccount, and maxPrincipals are AWS's documented limits.
	maxEndpointsPerDomain  = 10
	maxEndpointsPerAccount = 50
	maxPrincipals          = 10

	// validationCode is the error the Common Errors page gives for input that fails a constraint, which the fake
	// returns where a call's page names no error for it.
	validationCode = "ValidationError"

	// baseCode is the error each call lists for any other failure, which the fake returns where the docs state a
	// rule but name no error for it.
	baseCode = "BaseException"
)

// domain is an OpenSearch Service domain the fake knows: its owner, and whether it is a VPC domain.
type domain struct {
	owner string
	vpc   bool
}

// domains are the domains in us-west-2, by name.
var domains = map[string]domain{
	"logs":    {owner: account, vpc: true},
	"metrics": {owner: account, vpc: true},
	"public":  {owner: account, vpc: false},
	"shared":  {owner: otherAccount, vpc: true},
}

func domainArn(name string) string {
	return fmt.Sprintf("arn:aws:es:%s:%s:domain/%s", region, domains[name].owner, name)
}

// subnet is a subnet in the caller's account.
type subnet struct {
	vpc, zone string
}

var (
	subnets = map[string]subnet{
		"subnet-a": {vpc: "vpc-1", zone: "us-west-2a"},
		"subnet-b": {vpc: "vpc-1", zone: "us-west-2b"},
		"subnet-c": {vpc: "vpc-2", zone: "us-west-2a"},
	}

	// securityGroups maps each security group to its VPC. Each VPC's default group is sg-default-<vpc>.
	securityGroups = map[string]string{
		"sg-default-vpc-1": "vpc-1",
		"sg-web":           "vpc-1",
		"sg-db":            "vpc-1",
		"sg-default-vpc-2": "vpc-2",
	}
)

var (
	domainArnPattern = regexp.MustCompile(`^arn:aws[a-z\-]*:[a-z]+:[a-z0-9\-]+:[0-9]+:domain/[a-z0-9\-]+$`)
	arnParts         = regexp.MustCompile(`^arn:aws[a-z\-]*:[a-z]+:([a-z0-9\-]+):([0-9]+):domain/([a-z0-9\-]+)$`)
)

// tokenUse is a client token's first create: the request it came with, without the token, and the endpoint it made.
type tokenUse struct {
	request map[string]any
	id      string
}

// fakeAPI is an in-memory Amazon OpenSearch Service API for VPC endpoints and VPC endpoint access, with the EC2 call
// that finds a VPC's default security group. It keeps endpoints and authorizations in an awsfake store, in their
// types' property format, so the suite's Cloud Control sees the same resources. It follows the rules in endpointAPI
// and accessAPI, and where AWS's docs are silent it takes the pessimistic side:
//
//   - A created, updated, or deleted endpoint reports CREATING, UPDATING, or DELETING for settleReads reads before it
//     settles, and its Endpoint DNS name appears only once it is ACTIVE.
//   - While any endpoint of a domain is changing, or the domain is processing a change of its own, AWS rejects a
//     create or update of an endpoint of that domain with ConflictException, and an authorization change with
//     BaseException.
//   - A client token sent again with other values is rejected, and an endpoint that is changing can't be deleted.
type fakeAPI struct {
	store  *awsfake.CloudControl
	failer nativetest.Failer

	mu sync.Mutex

	// status holds each endpoint's status while it isn't ACTIVE, and reads how many more reads it stays so.
	status map[string]types.VpcEndpointStatus
	reads  map[string]int

	tokens map[string]tokenUse
	nextID int

	// processing is how many more calls each domain stays processing a change of its own, such as an update
	// outside Radius.
	processing map[string]int

	// failCreate makes the next created endpoint end CREATE_FAILED, and failing holds the endpoints that will.
	failCreate bool
	failing    map[string]bool

	// hold keeps changing endpoints changing, as for a slow change.
	hold bool

	// rootARN makes ListVpcEndpointAccess return an account principal as its root ARN instead of its ID.
	rootARN bool

	// calls counts the calls that reached the fake, by method.
	calls map[string]int

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error
}

var (
	_ API    = (*fakeAPI)(nil)
	_ EC2API = (*fakeAPI)(nil)
)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{
		store:      store,
		status:     map[string]types.VpcEndpointStatus{},
		reads:      map[string]int{},
		tokens:     map[string]tokenUse{},
		processing: map[string]int{},
		failing:    map[string]bool{},
		calls:      map[string]int{},
		errs:       map[string]error{},
	}
}

func fault(code string, format string, args ...any) error {
	return &smithy.GenericAPIError{Code: code, Message: fmt.Sprintf(format, args...), Fault: smithy.FaultClient}
}

// begin counts a call and returns the error errs holds for it.
func (f *fakeAPI) begin(method string) error {
	f.calls[method]++
	return f.errs[method]
}

func (f *fakeAPI) change(apply func() error) error {
	if err := f.failer.Before(); err != nil {
		return err
	}
	if err := apply(); err != nil {
		return err
	}
	return f.failer.After()
}

// tick passes time for the domains processing a change of their own.
func (f *fakeAPI) tick() {
	for name, n := range f.processing {
		if n <= 1 {
			delete(f.processing, name)
		} else {
			f.processing[name] = n - 1
		}
	}
}

// statusOf returns the endpoint's status: ACTIVE unless the fake keeps another for it.
func (f *fakeAPI) statusOf(id string) types.VpcEndpointStatus {
	if status, ok := f.status[id]; ok {
		return status
	}
	return types.VpcEndpointStatusActive
}

// endpointDomain returns the name of the domain the endpoint connects to.
func (f *fakeAPI) endpointDomain(id string) string {
	arn, _ := f.store.Resource(vpcEndpointType, id)["DomainArn"].(string)
	return arn[strings.LastIndex(arn, "/")+1:]
}

// domainBusy reports whether the domain is processing a change, or AWS is changing one of its endpoints other than
// except.
func (f *fakeAPI) domainBusy(name string, except string) bool {
	if f.processing[name] > 0 {
		return true
	}
	for _, id := range f.store.List(vpcEndpointType) {
		if id != except && f.endpointDomain(id) == name && slices.Contains([]types.VpcEndpointStatus{
			types.VpcEndpointStatusCreating, types.VpcEndpointStatusUpdating, types.VpcEndpointStatusDeleting,
		}, f.statusOf(id)) {
			return true
		}
	}
	return false
}

// settle moves the status of an endpoint a read returned on, once it was read settleReads times while changing.
func (f *fakeAPI) settle(id string) {
	status, ok := f.status[id]
	if !ok || !changingStatus(status) || f.hold {
		return
	}
	if f.reads[id]++; f.reads[id] < settleReads {
		return
	}
	delete(f.reads, id)
	switch status {
	case types.VpcEndpointStatusDeleting:
		delete(f.status, id)
		f.store.Remove(vpcEndpointType, id)
	case types.VpcEndpointStatusCreating:
		if f.failing[id] {
			delete(f.failing, id)
			f.status[id] = types.VpcEndpointStatusCreateFailed
			return
		}
		delete(f.status, id)
	default:
		delete(f.status, id)
	}
}

func changingStatus(status types.VpcEndpointStatus) bool {
	return status == types.VpcEndpointStatusCreating || status == types.VpcEndpointStatusUpdating || status == types.VpcEndpointStatusDeleting
}

// checkVpcOptions checks the endpoint's subnets and security groups: every subnet exists and they are in one VPC,
// and every security group exists in that VPC. It returns the VPC and the subnets' zones.
func checkVpcOptions(o *types.VPCOptions) (string, []string, error) {
	if o == nil {
		return "", nil, fault(validationCode, "VpcOptions is required")
	}
	if len(o.SubnetIds) == 0 {
		return "", nil, fault(validationCode, "an endpoint needs at least one subnet")
	}
	vpc, zones := "", []string{}
	for _, id := range o.SubnetIds {
		s, ok := subnets[id]
		if !ok || (vpc != "" && s.vpc != vpc) {
			return "", nil, fault(validationCode, "the subnets must exist and be in one VPC: %s", id)
		}
		vpc = s.vpc
		if !slices.Contains(zones, s.zone) {
			zones = append(zones, s.zone)
		}
	}
	for _, id := range o.SecurityGroupIds {
		if securityGroups[id] != vpc {
			return "", nil, fault(validationCode, "the security group %s isn't in %s", id, vpc)
		}
	}
	return vpc, zones, nil
}

// authorized reports whether the domain lets the account create endpoints to it.
func (f *fakeAPI) authorized(name string, caller string) bool {
	if domains[name].owner == caller {
		return true
	}
	return f.store.Resource(vpcEndpointAccessType, name+"|"+caller) != nil
}

func anyList(list []string) []any {
	out := []any{}
	for _, v := range list {
		out = append(out, v)
	}
	return out
}

func (f *fakeAPI) CreateVpcEndpoint(ctx context.Context, in *opensearch.CreateVpcEndpointInput, _ ...func(*opensearch.Options)) (*opensearch.CreateVpcEndpointOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("CreateVpcEndpoint"); err != nil {
		return nil, err
	}
	arn := aws.ToString(in.DomainArn)
	if len(arn) < 1 || len(arn) > 512 || !domainArnPattern.MatchString(arn) {
		return nil, fault(validationCode, "DomainArn %q doesn't match the pattern", arn)
	}
	if token := aws.ToString(in.ClientToken); in.ClientToken != nil && (len(token) < 1 || len(token) > 64) {
		return nil, fault(validationCode, "ClientToken must be 1 to 64 characters")
	}
	request := map[string]any{"DomainArn": arn, "VpcOptions": in.VpcOptions}
	key := region + "/" + aws.ToString(in.ClientToken)
	if earlier, ok := f.tokens[key]; ok && in.ClientToken != nil {
		if !reflect.DeepEqual(earlier.request, request) {
			return nil, fault(validationCode, "the client token was used with other values")
		}
		if e := f.store.Resource(vpcEndpointType, earlier.id); e != nil {
			return &opensearch.CreateVpcEndpointOutput{VpcEndpoint: f.endpoint(earlier.id)}, nil
		}
	}
	vpc, zones, err := checkVpcOptions(in.VpcOptions)
	if err != nil {
		return nil, err
	}
	m := arnParts.FindStringSubmatch(arn)
	name := m[3]
	d, ok := domains[name]
	if !ok || m[1] != region || m[2] != d.owner {
		return nil, fault(validationCode, "the domain %q doesn't exist in %s", arn, region)
	}
	if !d.vpc {
		return nil, fault(baseCode, "the domain %q isn't a VPC domain", name)
	}
	if !f.authorized(name, account) {
		return nil, fault(baseCode, "the account %s isn't authorized to create endpoints to %q", account, name)
	}
	if f.domainBusy(name, "") {
		return nil, fault("ConflictException", "the domain %q is processing a change", name)
	}
	ofDomain := 0
	for _, id := range f.store.List(vpcEndpointType) {
		if f.endpointDomain(id) == name {
			ofDomain++
		}
	}
	if ofDomain >= maxEndpointsPerDomain || len(f.store.List(vpcEndpointType)) >= maxEndpointsPerAccount {
		return nil, fault("LimitExceededException", "too many VPC endpoints")
	}
	groups := in.VpcOptions.SecurityGroupIds
	if len(groups) == 0 {
		groups = []string{"sg-default-" + vpc}
	}
	f.nextID++
	id := fmt.Sprintf("aos-%016d", f.nextID)
	err = f.change(func() error {
		properties := map[string]any{
			"VpcEndpointId":    id,
			"DomainArn":        arn,
			"VpcEndpointOwner": account,
			"VpcOptions": map[string]any{
				"SubnetIds":         anyList(in.VpcOptions.SubnetIds),
				"SecurityGroupIds":  anyList(groups),
				"VPCId":             vpc,
				"AvailabilityZones": anyList(zones),
			},
		}
		if f.failCreate {
			f.failing[id] = true
			f.failCreate = false
		}
		f.store.Put(vpcEndpointType, id, properties)
		f.status[id] = types.VpcEndpointStatusCreating
		if in.ClientToken != nil {
			f.tokens[key] = tokenUse{request: request, id: id}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &opensearch.CreateVpcEndpointOutput{VpcEndpoint: f.endpoint(id)}, nil
}

// endpoint returns the endpoint as AWS returns it.
func (f *fakeAPI) endpoint(id string) *types.VpcEndpoint {
	p := f.store.Resource(vpcEndpointType, id)
	o, _ := p["VpcOptions"].(map[string]any)
	strs := func(name string) []string {
		out := []string{}
		list, _ := o[name].([]any)
		for _, v := range list {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	str := func(m map[string]any, name string) *string {
		if v, ok := m[name].(string); ok {
			return aws.String(v)
		}
		return nil
	}
	status := f.statusOf(id)
	e := &types.VpcEndpoint{
		VpcEndpointId:    aws.String(id),
		DomainArn:        str(p, "DomainArn"),
		VpcEndpointOwner: str(p, "VpcEndpointOwner"),
		Status:           status,
		VpcOptions: &types.VPCDerivedInfo{
			SubnetIds:         strs("SubnetIds"),
			SecurityGroupIds:  strs("SecurityGroupIds"),
			VPCId:             str(o, "VPCId"),
			AvailabilityZones: strs("AvailabilityZones"),
		},
	}
	if status == types.VpcEndpointStatusActive || status == types.VpcEndpointStatusUpdating {
		e.Endpoint = aws.String("vpc-" + id + "." + region + ".es.amazonaws.com")
	}
	return e
}

func (f *fakeAPI) DescribeVpcEndpoints(ctx context.Context, in *opensearch.DescribeVpcEndpointsInput, _ ...func(*opensearch.Options)) (*opensearch.DescribeVpcEndpointsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DescribeVpcEndpoints"); err != nil {
		return nil, err
	}
	if len(in.VpcEndpointIds) == 0 {
		return nil, fault(validationCode, "VpcEndpointIds is required")
	}
	for _, id := range in.VpcEndpointIds {
		if len(id) < 5 || len(id) > 256 || !endpointIDPattern.MatchString(id) {
			return nil, fault(validationCode, "%q isn't a VPC endpoint ID", id)
		}
	}
	f.tick()
	out := &opensearch.DescribeVpcEndpointsOutput{VpcEndpoints: []types.VpcEndpoint{}, VpcEndpointErrors: []types.VpcEndpointError{}}
	for _, id := range in.VpcEndpointIds {
		if f.store.Resource(vpcEndpointType, id) == nil {
			out.VpcEndpointErrors = append(out.VpcEndpointErrors, types.VpcEndpointError{
				VpcEndpointId: aws.String(id), ErrorCode: types.VpcEndpointErrorCodeEndpointNotFound, ErrorMessage: aws.String("not found"),
			})
			continue
		}
		out.VpcEndpoints = append(out.VpcEndpoints, *f.endpoint(id))
		f.settle(id)
	}
	return out, nil
}

func (f *fakeAPI) UpdateVpcEndpoint(ctx context.Context, in *opensearch.UpdateVpcEndpointInput, _ ...func(*opensearch.Options)) (*opensearch.UpdateVpcEndpointOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("UpdateVpcEndpoint"); err != nil {
		return nil, err
	}
	id := aws.ToString(in.VpcEndpointId)
	if len(id) < 5 || len(id) > 256 || !endpointIDPattern.MatchString(id) {
		return nil, fault(validationCode, "%q isn't a VPC endpoint ID", id)
	}
	if in.VpcOptions == nil {
		return nil, fault(validationCode, "VpcOptions is required")
	}
	p := f.store.Resource(vpcEndpointType, id)
	if p == nil {
		return nil, fault("ResourceNotFoundException", "the endpoint %q doesn't exist", id)
	}
	if status := f.statusOf(id); changingStatus(status) {
		return nil, fault("ConflictException", "the endpoint %q is %s", id, status)
	}
	if name := f.endpointDomain(id); f.domainBusy(name, id) {
		return nil, fault("ConflictException", "the domain %q is processing a change", name)
	}
	current, _ := p["VpcOptions"].(map[string]any)
	options := *in.VpcOptions
	if options.SecurityGroupIds == nil {
		// The docs don't say what an update that leaves out security groups does, so the fake keeps them.
		groups, _ := current["SecurityGroupIds"].([]any)
		for _, g := range groups {
			options.SecurityGroupIds = append(options.SecurityGroupIds, fmt.Sprint(g))
		}
	}
	vpc, zones, err := checkVpcOptions(&options)
	if err != nil {
		return nil, err
	}
	if vpc != current["VPCId"] {
		return nil, fault(validationCode, "the subnets must be in the endpoint's VPC %s", current["VPCId"])
	}
	err = f.change(func() error {
		p["VpcOptions"] = map[string]any{
			"SubnetIds":         anyList(options.SubnetIds),
			"SecurityGroupIds":  anyList(options.SecurityGroupIds),
			"VPCId":             vpc,
			"AvailabilityZones": anyList(zones),
		}
		f.store.Put(vpcEndpointType, id, p)
		f.status[id] = types.VpcEndpointStatusUpdating
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &opensearch.UpdateVpcEndpointOutput{VpcEndpoint: f.endpoint(id)}, nil
}

func (f *fakeAPI) DeleteVpcEndpoint(ctx context.Context, in *opensearch.DeleteVpcEndpointInput, _ ...func(*opensearch.Options)) (*opensearch.DeleteVpcEndpointOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DeleteVpcEndpoint"); err != nil {
		return nil, err
	}
	id := aws.ToString(in.VpcEndpointId)
	if len(id) < 5 || len(id) > 256 || !endpointIDPattern.MatchString(id) {
		return nil, fault(validationCode, "%q isn't a VPC endpoint ID", id)
	}
	if f.store.Resource(vpcEndpointType, id) == nil {
		return nil, fault("ResourceNotFoundException", "the endpoint %q doesn't exist", id)
	}
	if status := f.statusOf(id); changingStatus(status) {
		return nil, fault(baseCode, "the endpoint %q is %s", id, status)
	}
	err := f.change(func() error {
		f.status[id] = types.VpcEndpointStatusDeleting
		return nil
	})
	if err != nil {
		return nil, err
	}
	summary := f.endpoint(id)
	return &opensearch.DeleteVpcEndpointOutput{VpcEndpointSummary: &types.VpcEndpointSummary{
		VpcEndpointId: summary.VpcEndpointId, DomainArn: summary.DomainArn, VpcEndpointOwner: summary.VpcEndpointOwner, Status: summary.Status,
	}}, nil
}

// page returns the items of the page that token starts, and the next page's token, or nil on the last page.
func page[T any](items []T, token *string) ([]T, *string) {
	start, _ := strconv.Atoi(aws.ToString(token))
	end := min(start+pageSize, len(items))
	var next *string
	if end < len(items) {
		next = aws.String(strconv.Itoa(end))
	}
	return items[min(start, end):end], next
}

func (f *fakeAPI) ListVpcEndpoints(ctx context.Context, in *opensearch.ListVpcEndpointsInput, _ ...func(*opensearch.Options)) (*opensearch.ListVpcEndpointsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("ListVpcEndpoints"); err != nil {
		return nil, err
	}
	f.tick()
	summaries := []types.VpcEndpointSummary{}
	for _, id := range f.store.List(vpcEndpointType) {
		e := f.endpoint(id)
		summaries = append(summaries, types.VpcEndpointSummary{VpcEndpointId: e.VpcEndpointId, DomainArn: e.DomainArn, VpcEndpointOwner: e.VpcEndpointOwner, Status: e.Status})
	}
	items, next := page(summaries, in.NextToken)
	return &opensearch.ListVpcEndpointsOutput{VpcEndpointSummaryList: items, NextToken: next}, nil
}

// checkDomainName returns ValidationException for a name outside the documented length and pattern, and
// ResourceNotFoundException for a domain the fake doesn't have.
func checkDomainName(name string) error {
	if len(name) < 3 || len(name) > 28 || !domainNamePattern.MatchString(name) {
		return fault(validationCode, "DomainName %q doesn't match the pattern", name)
	}
	if _, ok := domains[name]; !ok {
		return fault("ResourceNotFoundException", "the domain %q doesn't exist", name)
	}
	return nil
}

// principal returns the principal and its type an authorize or revoke names.
func principal(account *string, service types.AWSServicePrincipal) (string, types.PrincipalType, error) {
	switch {
	case account != nil && service != "":
		return "", "", fault(validationCode, "set Account or Service, not both")
	case account != nil:
		if !accountPattern.MatchString(*account) {
			return "", "", fault(validationCode, "Account %q doesn't match the pattern", *account)
		}
		return *account, types.PrincipalTypeAwsAccount, nil
	case service != "":
		if !slices.Contains(types.AWSServicePrincipal("").Values(), service) {
			return "", "", fault(validationCode, "Service %q isn't a valid value", service)
		}
		return string(service), types.PrincipalTypeAwsService, nil
	}
	return "", "", fault(validationCode, "set Account or Service")
}

// principals returns the identifiers of the domain's authorizations, in the order the store lists them.
func (f *fakeAPI) principals(name string) []string {
	out := []string{}
	for _, id := range f.store.List(vpcEndpointAccessType) {
		if strings.HasPrefix(id, name+"|") {
			out = append(out, id)
		}
	}
	return out
}

func (f *fakeAPI) AuthorizeVpcEndpointAccess(ctx context.Context, in *opensearch.AuthorizeVpcEndpointAccessInput, _ ...func(*opensearch.Options)) (*opensearch.AuthorizeVpcEndpointAccessOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("AuthorizeVpcEndpointAccess"); err != nil {
		return nil, err
	}
	name := aws.ToString(in.DomainName)
	if err := checkDomainName(name); err != nil {
		return nil, err
	}
	p, kind, err := principal(in.Account, in.Service)
	if err != nil {
		return nil, err
	}
	id := name + "|" + p
	if f.store.Resource(vpcEndpointAccessType, id) != nil {
		return nil, fault(baseCode, "%s is already authorized", p)
	}
	if f.domainBusy(name, "") {
		return nil, fault(baseCode, "the domain %q is processing a change", name)
	}
	if len(f.principals(name)) >= maxPrincipals {
		return nil, fault("LimitExceededException", "a domain can have at most %d authorized principals", maxPrincipals)
	}
	properties := map[string]any{"DomainName": name, "Principal": p, "PrincipalType": string(kind)}
	if kind == types.PrincipalTypeAwsAccount {
		properties["Account"] = p
	} else {
		properties["Service"] = p
		if in.ServiceOptions != nil && len(in.ServiceOptions.SupportedRegions) > 0 {
			properties["ServiceOptions"] = map[string]any{"SupportedRegions": anyList(in.ServiceOptions.SupportedRegions)}
		}
	}
	if err := f.change(func() error {
		f.store.Put(vpcEndpointAccessType, id, properties)
		return nil
	}); err != nil {
		return nil, err
	}
	return &opensearch.AuthorizeVpcEndpointAccessOutput{AuthorizedPrincipal: f.authorizedPrincipal(id)}, nil
}

// authorizedPrincipal returns the authorization as AWS returns it.
func (f *fakeAPI) authorizedPrincipal(id string) *types.AuthorizedPrincipal {
	p := f.store.Resource(vpcEndpointAccessType, id)
	out := &types.AuthorizedPrincipal{Principal: aws.String(p["Principal"].(string)), PrincipalType: types.PrincipalType(p["PrincipalType"].(string))}
	if f.rootARN && out.PrincipalType == types.PrincipalTypeAwsAccount {
		out.Principal = aws.String("arn:aws:iam::" + aws.ToString(out.Principal) + ":root")
	}
	if o, ok := p["ServiceOptions"].(map[string]any); ok {
		out.ServiceOptions = &types.ServiceOptions{}
		for _, r := range o["SupportedRegions"].([]any) {
			out.ServiceOptions.SupportedRegions = append(out.ServiceOptions.SupportedRegions, r.(string))
		}
	}
	return out
}

func (f *fakeAPI) RevokeVpcEndpointAccess(ctx context.Context, in *opensearch.RevokeVpcEndpointAccessInput, _ ...func(*opensearch.Options)) (*opensearch.RevokeVpcEndpointAccessOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("RevokeVpcEndpointAccess"); err != nil {
		return nil, err
	}
	name := aws.ToString(in.DomainName)
	if err := checkDomainName(name); err != nil {
		return nil, err
	}
	p, _, err := principal(in.Account, in.Service)
	if err != nil {
		return nil, err
	}
	id := name + "|" + p
	if f.store.Resource(vpcEndpointAccessType, id) == nil {
		return nil, fault("ResourceNotFoundException", "%s isn't authorized", p)
	}
	if f.domainBusy(name, "") {
		return nil, fault(baseCode, "the domain %q is processing a change", name)
	}
	if err := f.change(func() error {
		f.store.Remove(vpcEndpointAccessType, id)
		return nil
	}); err != nil {
		return nil, err
	}
	return &opensearch.RevokeVpcEndpointAccessOutput{}, nil
}

func (f *fakeAPI) ListVpcEndpointAccess(ctx context.Context, in *opensearch.ListVpcEndpointAccessInput, _ ...func(*opensearch.Options)) (*opensearch.ListVpcEndpointAccessOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("ListVpcEndpointAccess"); err != nil {
		return nil, err
	}
	name := aws.ToString(in.DomainName)
	if err := checkDomainName(name); err != nil {
		return nil, err
	}
	f.tick()
	list := []types.AuthorizedPrincipal{}
	for _, id := range f.principals(name) {
		list = append(list, *f.authorizedPrincipal(id))
	}
	items, next := page(list, in.NextToken)
	return &opensearch.ListVpcEndpointAccessOutput{AuthorizedPrincipalList: items, NextToken: next}, nil
}

func (f *fakeAPI) DescribeSecurityGroups(ctx context.Context, in *awsec2.DescribeSecurityGroupsInput, _ ...func(*awsec2.Options)) (*awsec2.DescribeSecurityGroupsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DescribeSecurityGroups"); err != nil {
		return nil, err
	}
	filters := map[string][]string{}
	for _, filter := range in.Filters {
		filters[aws.ToString(filter.Name)] = filter.Values
	}
	out := &awsec2.DescribeSecurityGroupsOutput{}
	for _, id := range slices.Sorted(maps.Keys(securityGroups)) {
		vpc := securityGroups[id]
		name := id
		if strings.HasPrefix(id, "sg-default-") {
			name = "default"
		}
		if v, ok := filters["vpc-id"]; ok && !slices.Contains(v, vpc) {
			continue
		}
		if v, ok := filters["group-name"]; ok && !slices.Contains(v, name) {
			continue
		}
		out.SecurityGroups = append(out.SecurityGroups, ec2types.SecurityGroup{GroupId: aws.String(id), GroupName: aws.String(name), VpcId: aws.String(vpc)})
	}
	return out, nil
}
