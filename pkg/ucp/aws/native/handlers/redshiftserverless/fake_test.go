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

package redshiftserverless

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/redshiftserverless"
	"github.com/aws/aws-sdk-go-v2/service/redshiftserverless/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	account   = "123456789012"
	arnPrefix = "arn:aws:redshift-serverless:us-west-2:" + account + ":"

	// workgroupType keeps the fake's workgroups in the store, apart from the resources the handlers serve.
	workgroupType = "AWS::RedshiftServerless::Workgroup"

	// defaultGroup is the security group AWS gives an endpoint whose create sets none: the VPC's default group.
	defaultGroup = "sg-default"

	// pageSize is how many items a list returns when it doesn't set maxResults.
	pageSize = 100

	// busyReads is how many reads of a workgroup report MODIFYING after a change that changes it. A change AWS
	// documents nothing about leaves the workgroup busy too, because a real one was busy after a usage limit change.
	busyReads = 2

	// maxLimitsPerType is how many usage limits of one usage type a workgroup can have.
	maxLimitsPerType = 4
)

// workgroups are the workgroups of this account the fake has, and sharedWorkgroup one of the account otherAccount,
// which the fake's own reads of workgroups don't return.
var workgroups = []string{"analytics", "reports"}

const (
	sharedWorkgroup = "shared"
	otherAccount    = "444455556666"

	// ungrantedWorkgroup is a workgroup of otherAccount that hasn't granted this account access.
	ungrantedWorkgroup = "private"
)

// certificates are the ACM certificates Amazon issued that the fake knows. Any other ARN is not one.
var certificates = []string{
	"arn:aws:acm:us-west-2:" + account + ":certificate/11111111-1111-1111-1111-111111111111",
	"arn:aws:acm:us-west-2:" + account + ":certificate/22222222-2222-2222-2222-222222222222",
}

// namespaceArn is the ARN of a namespace, which CreateUsageLimit doesn't take.
const namespaceArn = arnPrefix + "namespace/ns-analytics"

// workgroupArn returns the ARN of the fake's workgroup with the name.
func workgroupArn(name string) string {
	return arnPrefix + "workgroup/wg-" + name
}

// addWorkgroups adds the fake's workgroups to store.
func addWorkgroups(store *awsfake.CloudControl) {
	for _, name := range workgroups {
		store.Put(workgroupType, name, map[string]any{"WorkgroupName": name, "WorkgroupArn": workgroupArn(name)})
	}
	store.Put(workgroupType, sharedWorkgroup, map[string]any{"WorkgroupName": sharedWorkgroup, "Owner": otherAccount, "Grantees": []any{account}})
	store.Put(workgroupType, ungrantedWorkgroup, map[string]any{"WorkgroupName": ungrantedWorkgroup, "Owner": otherAccount})
}

// granted reports whether the workgroup with the name is this account's, or its owner granted this account access.
func (f *fakeAPI) granted(name string) bool {
	props := f.store.Resource(workgroupType, name)
	return awsapi.String(props, "Owner") == "" || slices.Contains(awsapi.Strings(props, "Grantees"), account)
}

// ownWorkgroups returns the names of the workgroups of this account.
func (f *fakeAPI) ownWorkgroups() []string {
	out := []string{}
	for _, name := range f.store.List(workgroupType) {
		if awsapi.String(f.store.Resource(workgroupType, name), "Owner") == "" {
			out = append(out, name)
		}
	}
	return out
}

// hasWorkgroup reports whether owner, or this account when owner is "", has a workgroup with the name.
func (f *fakeAPI) hasWorkgroup(name string, owner string) bool {
	props := f.store.Resource(workgroupType, name)
	if props == nil {
		return false
	}
	if owner == account {
		owner = ""
	}
	return awsapi.String(props, "Owner") == owner
}

// fakeAPI is an in-memory Redshift Serverless API for usage limits, managed VPC endpoints, and custom domain
// associations. It keeps them in an awsfake store, in each type's property format, so the suite's Cloud Control sees
// the same resources, and the workgroups under workgroupType. It follows the rules in usageLimitAPI, endpointAccessAPI,
// and customDomainAPI, and where AWS's docs are silent it takes the side that is harder for the handler:
//
//   - A change to a usage limit or a custom domain leaves the workgroup MODIFYING for busyReads reads, and a change
//     AWS rejects while a workgroup is busy is rejected while it is.
//   - A new or changed endpoint is CREATING or MODIFYING for one read before it is ACTIVE, and a deleted one is
//     DELETING for one read before it is gone.
//   - A deleted usage limit is still read for one read.
type fakeAPI struct {
	store *awsfake.CloudControl

	mu sync.Mutex

	// busy counts the reads each workgroup still reports MODIFYING, and held are workgroups busy until a test frees
	// them.
	busy map[string]int
	held map[string]bool

	// status holds an endpoint's transitional status, by name. An endpoint without one is ACTIVE.
	status map[string]string

	// deleted holds usage limits deleted but still read once, by ID.
	deleted map[string]map[string]any

	next int

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error

	// calls counts the calls of each method.
	calls map[string]int
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{
		store:   store,
		busy:    map[string]int{},
		held:    map[string]bool{},
		status:  map[string]string{},
		deleted: map[string]map[string]any{},
		errs:    map[string]error{},
		calls:   map[string]int{},
	}
}

func fault(code string, message string) error {
	return &smithy.GenericAPIError{Code: code, Message: message, Fault: smithy.FaultClient}
}

// rules collects the first rule a call breaks, as the client fault AWS returns for it.
type rules struct {
	err error
}

func (r *rules) check(ok bool, code string, message string) {
	if r.err == nil && !ok {
		r.err = fault(code, message)
	}
}

// begin records a call of method and returns the error a test set for it.
func (f *fakeAPI) begin(method string) error {
	f.calls[method]++
	return f.errs[method]
}

// workgroupOf returns the name of the workgroup with the ARN, or "".
func (f *fakeAPI) workgroupOf(arn string) string {
	for _, name := range f.ownWorkgroups() {
		if awsapi.String(f.store.Resource(workgroupType, name), "WorkgroupArn") == arn {
			return name
		}
	}
	return ""
}

// workgroupBusy reports whether the workgroup with the name is MODIFYING, without counting a read.
func (f *fakeAPI) workgroupBusy(name string) bool {
	return f.held[name] || f.busy[name] > 0
}

// change marks the workgroup with the name busy after a change that changes it.
func (f *fakeAPI) change(name string) {
	f.busy[name] = busyReads
}

// workgroup returns the workgroup with the name as AWS returns it, counting a read of its status.
func (f *fakeAPI) workgroup(name string) types.Workgroup {
	status := types.WorkgroupStatusAvailable
	if f.workgroupBusy(name) {
		status = types.WorkgroupStatusModifying
		if f.busy[name] > 0 {
			f.busy[name]--
		}
	}
	props := f.store.Resource(workgroupType, name)
	return types.Workgroup{
		WorkgroupName:    aws.String(name),
		WorkgroupArn:     aws.String(awsapi.String(props, "WorkgroupArn")),
		Status:           status,
		CustomDomainName: aws.String(f.domainOf(name)),
	}
}

// domainOf returns the custom domain of the workgroup with the name, or "".
func (f *fakeAPI) domainOf(name string) string {
	for _, id := range f.store.List(customDomainType) {
		if workgroup, domain, _ := strings.Cut(id, "|"); workgroup == name {
			return domain
		}
	}
	return ""
}

// page returns the items of list from the position token names, up to maxResults, and the token of the rest, as a
// Redshift Serverless list does. A token is at least 8 characters.
func page[T any](r *rules, list []T, token *string, maxResults *int32) ([]T, *string) {
	size := pageSize
	if maxResults != nil {
		size = int(*maxResults)
	}
	r.check(size >= 1 && size <= 100, validation, "maxResults must be 1 to 100")
	start := 0
	if token != nil {
		n, err := strconv.Atoi(strings.TrimPrefix(aws.ToString(token), "token-"))
		r.check(err == nil && len(aws.ToString(token)) >= 8 && n <= len(list), "InvalidPaginationException", "The pagination token is invalid")
		start = n
	}
	if r.err != nil {
		return nil, nil
	}
	end := min(start+size, len(list))
	var next *string
	if end < len(list) {
		next = aws.String(fmt.Sprintf("token-%04d", end))
	}
	return list[start:end], next
}

func (f *fakeAPI) GetWorkgroup(ctx context.Context, in *redshiftserverless.GetWorkgroupInput, _ ...func(*redshiftserverless.Options)) (*redshiftserverless.GetWorkgroupOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("GetWorkgroup"); err != nil {
		return nil, err
	}
	name := aws.ToString(in.WorkgroupName)
	if !f.hasWorkgroup(name, "") {
		return nil, fault(notFound, "Serverless workgroup "+name+" not found.")
	}
	w := f.workgroup(name)
	return &redshiftserverless.GetWorkgroupOutput{Workgroup: &w}, nil
}

func (f *fakeAPI) ListWorkgroups(ctx context.Context, in *redshiftserverless.ListWorkgroupsInput, _ ...func(*redshiftserverless.Options)) (*redshiftserverless.ListWorkgroupsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("ListWorkgroups"); err != nil {
		return nil, err
	}
	r := &rules{}
	names, next := page(r, f.ownWorkgroups(), in.NextToken, in.MaxResults)
	if r.err != nil {
		return nil, r.err
	}
	out := []types.Workgroup{}
	for _, name := range names {
		out = append(out, f.workgroup(name))
	}
	return &redshiftserverless.ListWorkgroupsOutput{Workgroups: out, NextToken: next}, nil
}

// usageLimitOf returns the usage limit with the properties as AWS returns it.
func usageLimitOf(props map[string]any) types.UsageLimit {
	n, _ := amount(props["Amount"])
	return types.UsageLimit{
		UsageLimitId:  aws.String(awsapi.String(props, "UsageLimitId")),
		UsageLimitArn: aws.String(awsapi.String(props, "UsageLimitArn")),
		ResourceArn:   aws.String(awsapi.String(props, "ResourceArn")),
		UsageType:     types.UsageLimitUsageType(awsapi.String(props, "UsageType")),
		Period:        types.UsageLimitPeriod(awsapi.String(props, "Period")),
		BreachAction:  types.UsageLimitBreachAction(awsapi.String(props, "BreachAction")),
		Amount:        aws.Int64(n),
	}
}

// limitRules checks an amount and a breach action of a create or an update.
func limitRules(r *rules, n *int64, action types.UsageLimitBreachAction) {
	r.check(n == nil || *n >= 1, validation, "Usage limit amount can not be a non-positive value.")
	r.check(action == "" || slices.Contains(action.Values(), action), validation, "The breach action is not valid.")
}

func (f *fakeAPI) CreateUsageLimit(ctx context.Context, in *redshiftserverless.CreateUsageLimitInput, _ ...func(*redshiftserverless.Options)) (*redshiftserverless.CreateUsageLimitOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("CreateUsageLimit"); err != nil {
		return nil, err
	}
	arn := aws.ToString(in.ResourceArn)
	workgroup := f.workgroupOf(arn)
	r := &rules{}
	r.check(in.Amount != nil, validation, "amount is required")
	limitRules(r, in.Amount, in.BreachAction)
	r.check(slices.Contains(in.UsageType.Values(), in.UsageType), validation, "The usage type is not valid.")
	r.check(in.Period == "" || slices.Contains(in.Period.Values(), in.Period), validation, "The period is not valid.")
	r.check(workgroup != "", validation, "The Resource ARN is not valid with the resource type.")
	same := 0
	for _, id := range f.store.List(usageLimitType) {
		props := f.store.Resource(usageLimitType, id)
		if awsapi.String(props, "ResourceArn") == arn && awsapi.String(props, "UsageType") == string(in.UsageType) {
			same++
		}
	}
	r.check(same < maxLimitsPerType, "ServiceQuotaExceededException", fmt.Sprintf("Cannot create more than %d usage limits for %s usage type", maxLimitsPerType, in.UsageType))
	if r.err != nil {
		return nil, r.err
	}
	f.next++
	id := fmt.Sprintf("%08d-0000-4000-8000-000000000000", f.next)
	period, action := string(in.Period), string(in.BreachAction)
	if period == "" {
		period = defaultPeriod
	}
	if action == "" {
		action = defaultBreachAction
	}
	props := map[string]any{
		"UsageLimitId":  id,
		"UsageLimitArn": arnPrefix + "usagelimit/" + id,
		"ResourceArn":   arn,
		"UsageType":     string(in.UsageType),
		"Amount":        float64(*in.Amount),
		"Period":        period,
		"BreachAction":  action,
	}
	f.store.Put(usageLimitType, id, props)
	f.change(workgroup)
	limit := usageLimitOf(props)
	return &redshiftserverless.CreateUsageLimitOutput{UsageLimit: &limit}, nil
}

func (f *fakeAPI) GetUsageLimit(ctx context.Context, in *redshiftserverless.GetUsageLimitInput, _ ...func(*redshiftserverless.Options)) (*redshiftserverless.GetUsageLimitOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("GetUsageLimit"); err != nil {
		return nil, err
	}
	id := aws.ToString(in.UsageLimitId)
	props := f.store.Resource(usageLimitType, id)
	if props == nil {
		props = f.deleted[id]
		delete(f.deleted, id)
	}
	if props == nil {
		return nil, fault(validation, "The usage limit ID is not valid. "+id+" does not exist")
	}
	limit := usageLimitOf(props)
	return &redshiftserverless.GetUsageLimitOutput{UsageLimit: &limit}, nil
}

func (f *fakeAPI) UpdateUsageLimit(ctx context.Context, in *redshiftserverless.UpdateUsageLimitInput, _ ...func(*redshiftserverless.Options)) (*redshiftserverless.UpdateUsageLimitOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("UpdateUsageLimit"); err != nil {
		return nil, err
	}
	id := aws.ToString(in.UsageLimitId)
	props := f.store.Resource(usageLimitType, id)
	if props == nil {
		return nil, fault(notFound, "Usage limit "+id+" is not found")
	}
	r := &rules{}
	limitRules(r, in.Amount, in.BreachAction)
	if r.err != nil {
		return nil, r.err
	}
	if in.Amount != nil {
		props["Amount"] = float64(*in.Amount)
	}
	if in.BreachAction != "" {
		props["BreachAction"] = string(in.BreachAction)
	}
	f.store.Put(usageLimitType, id, props)
	f.change(f.workgroupOf(awsapi.String(props, "ResourceArn")))
	limit := usageLimitOf(props)
	return &redshiftserverless.UpdateUsageLimitOutput{UsageLimit: &limit}, nil
}

func (f *fakeAPI) DeleteUsageLimit(ctx context.Context, in *redshiftserverless.DeleteUsageLimitInput, _ ...func(*redshiftserverless.Options)) (*redshiftserverless.DeleteUsageLimitOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DeleteUsageLimit"); err != nil {
		return nil, err
	}
	id := aws.ToString(in.UsageLimitId)
	props := f.store.Resource(usageLimitType, id)
	if props == nil {
		return nil, fault(notFound, "Usage limit "+id+" is not found")
	}
	f.store.Remove(usageLimitType, id)
	f.deleted[id] = props
	f.change(f.workgroupOf(awsapi.String(props, "ResourceArn")))
	limit := usageLimitOf(props)
	return &redshiftserverless.DeleteUsageLimitOutput{UsageLimit: &limit}, nil
}

func (f *fakeAPI) ListUsageLimits(ctx context.Context, in *redshiftserverless.ListUsageLimitsInput, _ ...func(*redshiftserverless.Options)) (*redshiftserverless.ListUsageLimitsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("ListUsageLimits"); err != nil {
		return nil, err
	}
	r := &rules{}
	arn := aws.ToString(in.ResourceArn)
	r.check(in.ResourceArn == nil || f.workgroupOf(arn) != "", validation, "The Resource ARN is not valid.")
	r.check(in.UsageType == "" || slices.Contains(in.UsageType.Values(), in.UsageType), validation, "The usage type is not valid.")
	matches := []types.UsageLimit{}
	for _, id := range f.store.List(usageLimitType) {
		props := f.store.Resource(usageLimitType, id)
		if (arn == "" || awsapi.String(props, "ResourceArn") == arn) && (in.UsageType == "" || awsapi.String(props, "UsageType") == string(in.UsageType)) {
			matches = append(matches, usageLimitOf(props))
		}
	}
	list, next := page(r, matches, in.NextToken, in.MaxResults)
	if r.err != nil {
		return nil, r.err
	}
	return &redshiftserverless.ListUsageLimitsOutput{UsageLimits: list, NextToken: next}, nil
}

// endpointOf returns the endpoint with the properties as AWS returns it, with the status status.
func endpointOf(props map[string]any, status string) types.EndpointAccess {
	name := awsapi.String(props, "EndpointName")
	groups := []types.VpcSecurityGroupMembership{}
	ids := awsapi.Strings(props, "VpcSecurityGroupIds")
	slices.Sort(ids)
	for _, id := range ids {
		groups = append(groups, types.VpcSecurityGroupMembership{VpcSecurityGroupId: aws.String(id), Status: aws.String("active")})
	}
	// AWS returns the subnets in its own order, not the create's.
	subnets := awsapi.Strings(props, "SubnetIds")
	slices.Sort(subnets)
	e := types.EndpointAccess{
		EndpointName:       aws.String(name),
		EndpointArn:        aws.String(awsapi.String(props, "EndpointArn")),
		EndpointStatus:     aws.String(status),
		WorkgroupName:      aws.String(awsapi.String(props, "WorkgroupName")),
		SubnetIds:          subnets,
		VpcSecurityGroups:  groups,
		Address:            aws.String(name + "-endpoint.abc123.us-west-2.redshift-serverless.amazonaws.com"),
		Port:               aws.Int32(5439),
		EndpointCreateTime: aws.Time(time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)),
		VpcEndpoint: &types.VpcEndpoint{
			VpcEndpointId: aws.String("vpce-" + name),
			VpcId:         aws.String("vpc-1"),
			NetworkInterfaces: []types.NetworkInterface{{
				NetworkInterfaceId: aws.String("eni-" + name),
				SubnetId:           aws.String(subnets[0]),
				PrivateIpAddress:   aws.String("10.0.0.10"),
				AvailabilityZone:   aws.String("us-west-2a"),
			}},
		},
	}
	if status == "CREATING" {
		// AWS returns no address, create time, or VPC endpoint until the endpoint is made.
		e.Address, e.EndpointCreateTime, e.VpcEndpoint = nil, nil, nil
	}
	return e
}

// endpointState returns the endpoint's status, and moves a transitional status on: a create or update finishes, and
// a delete removes the endpoint, which then returns false.
func (f *fakeAPI) endpointState(name string) (string, bool) {
	status, ok := f.status[name]
	if !ok {
		return endpointActive, true
	}
	delete(f.status, name)
	if status == endpointDeleting {
		f.store.Remove(endpointAccessType, name)
	}
	return status, true
}

func (f *fakeAPI) CreateEndpointAccess(ctx context.Context, in *redshiftserverless.CreateEndpointAccessInput, _ ...func(*redshiftserverless.Options)) (*redshiftserverless.CreateEndpointAccessOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("CreateEndpointAccess"); err != nil {
		return nil, err
	}
	name := aws.ToString(in.EndpointName)
	workgroup := aws.ToString(in.WorkgroupName)
	owner := aws.ToString(in.OwnerAccount)
	r := &rules{}
	r.check(len(name) <= maxEndpointName, validation, "The VPC endpoint name has a limit of 30 characters.")
	r.check(endpointName.MatchString(name) && !strings.Contains(name, "--") && !strings.HasSuffix(name, "-"), validation,
		"The VPC endpoint name can contain: lowercase letters, numbers, and hyphens. The first character must be a letter. It can't end with a hyphen or contain two consecutive hyphens.")
	r.check(len(in.SubnetIds) > 0, validation, "subnetIds is required")
	r.check(in.OwnerAccount == nil || ownerAccount.MatchString(owner), validation, "ownerAccount must be an account ID")
	r.check(f.store.Resource(endpointAccessType, name) == nil, validation, "The VPC endpoint name "+name+" already exists.")
	r.check(f.hasWorkgroup(workgroup, owner), notFound, "Serverless workgroup "+workgroup+" not found.")
	r.check(f.granted(workgroup), "AccessDeniedException", "The owner of the workgroup "+workgroup+" hasn't granted this account access.")
	r.check(!f.workgroupBusy(workgroup), conflict, "An operation is running on the serverless workgroup.")
	if r.err != nil {
		return nil, r.err
	}
	groups := in.VpcSecurityGroupIds
	if len(groups) == 0 {
		groups = []string{defaultGroup}
	}
	props := map[string]any{
		"EndpointName":        name,
		"WorkgroupName":       workgroup,
		"SubnetIds":           anyList(in.SubnetIds),
		"VpcSecurityGroupIds": anyList(groups),
		"EndpointArn":         arnPrefix + "managedvpcendpoint/vpce-" + name,
	}
	if owner != "" {
		props["OwnerAccount"] = owner
	}
	f.store.Put(endpointAccessType, name, props)
	f.status[name] = "CREATING"
	e := endpointOf(props, "CREATING")
	return &redshiftserverless.CreateEndpointAccessOutput{Endpoint: &e}, nil
}

func (f *fakeAPI) GetEndpointAccess(ctx context.Context, in *redshiftserverless.GetEndpointAccessInput, _ ...func(*redshiftserverless.Options)) (*redshiftserverless.GetEndpointAccessOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("GetEndpointAccess"); err != nil {
		return nil, err
	}
	name := aws.ToString(in.EndpointName)
	props := f.store.Resource(endpointAccessType, name)
	if props == nil {
		return nil, fault(notFound, "The VPC endpoint name "+name+" can't be found for customer "+account)
	}
	status, _ := f.endpointState(name)
	e := endpointOf(props, status)
	return &redshiftserverless.GetEndpointAccessOutput{Endpoint: &e}, nil
}

func (f *fakeAPI) UpdateEndpointAccess(ctx context.Context, in *redshiftserverless.UpdateEndpointAccessInput, _ ...func(*redshiftserverless.Options)) (*redshiftserverless.UpdateEndpointAccessOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("UpdateEndpointAccess"); err != nil {
		return nil, err
	}
	name := aws.ToString(in.EndpointName)
	props := f.store.Resource(endpointAccessType, name)
	if props == nil {
		return nil, fault(notFound, "The VPC endpoint name "+name+" can't be found for customer "+account)
	}
	r := &rules{}
	r.check(f.status[name] == "", conflict, "The VPC endpoint "+name+" status must be active.")
	r.check(len(in.VpcSecurityGroupIds) > 0, validation, "Specify a security group ID.")
	r.check(!sameSet(in.VpcSecurityGroupIds, awsapi.Strings(props, "VpcSecurityGroupIds")), validation,
		"The VPC security group IDs are already associated with the VPC endpoint "+name+".")
	if r.err != nil {
		return nil, r.err
	}
	props["VpcSecurityGroupIds"] = anyList(in.VpcSecurityGroupIds)
	f.store.Put(endpointAccessType, name, props)
	f.status[name] = "MODIFYING"
	e := endpointOf(props, "MODIFYING")
	return &redshiftserverless.UpdateEndpointAccessOutput{Endpoint: &e}, nil
}

func (f *fakeAPI) DeleteEndpointAccess(ctx context.Context, in *redshiftserverless.DeleteEndpointAccessInput, _ ...func(*redshiftserverless.Options)) (*redshiftserverless.DeleteEndpointAccessOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DeleteEndpointAccess"); err != nil {
		return nil, err
	}
	name := aws.ToString(in.EndpointName)
	props := f.store.Resource(endpointAccessType, name)
	if props == nil {
		return nil, fault(notFound, "The VPC endpoint name "+name+" can't be found for customer "+account)
	}
	if f.status[name] != "" {
		return nil, fault(conflict, "The VPC endpoint "+name+" status must be active.")
	}
	f.status[name] = endpointDeleting
	e := endpointOf(props, endpointDeleting)
	return &redshiftserverless.DeleteEndpointAccessOutput{Endpoint: &e}, nil
}

func (f *fakeAPI) ListEndpointAccess(ctx context.Context, in *redshiftserverless.ListEndpointAccessInput, _ ...func(*redshiftserverless.Options)) (*redshiftserverless.ListEndpointAccessOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("ListEndpointAccess"); err != nil {
		return nil, err
	}
	all := []types.EndpointAccess{}
	for _, name := range f.store.List(endpointAccessType) {
		props := f.store.Resource(endpointAccessType, name)
		if in.WorkgroupName != nil && awsapi.String(props, "WorkgroupName") != aws.ToString(in.WorkgroupName) {
			continue
		}
		status, _ := f.endpointState(name)
		all = append(all, endpointOf(props, status))
	}
	r := &rules{}
	list, next := page(r, all, in.NextToken, in.MaxResults)
	if r.err != nil {
		return nil, r.err
	}
	return &redshiftserverless.ListEndpointAccessOutput{Endpoints: list, NextToken: next}, nil
}

// associationOf returns the association with the properties as AWS returns it.
func associationOf(props map[string]any) types.Association {
	return types.Association{
		WorkgroupName:                     aws.String(awsapi.String(props, "WorkgroupName")),
		CustomDomainName:                  aws.String(awsapi.String(props, "CustomDomainName")),
		CustomDomainCertificateArn:        aws.String(awsapi.String(props, "CustomDomainCertificateArn")),
		CustomDomainCertificateExpiryTime: aws.Time(time.Date(2027, 11, 20, 0, 0, 0, 0, time.UTC)),
	}
}

// associationNotFound is the error AWS returns for a change of an association it doesn't have.
func associationNotFound() error {
	return fault(notFound, "There isn't a custom domain association to update. Create a custom domain association first.")
}

// certificateRule checks the certificate ARN of a create or an update.
func certificateRule(r *rules, arn string) {
	r.check(slices.Contains(certificates, arn), validation, "Can't find the certificate ["+arn+"].")
}

func (f *fakeAPI) CreateCustomDomainAssociation(ctx context.Context, in *redshiftserverless.CreateCustomDomainAssociationInput, _ ...func(*redshiftserverless.Options)) (*redshiftserverless.CreateCustomDomainAssociationOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("CreateCustomDomainAssociation"); err != nil {
		return nil, err
	}
	workgroup, domain := aws.ToString(in.WorkgroupName), aws.ToString(in.CustomDomainName)
	certificate := aws.ToString(in.CustomDomainCertificateArn)
	r := &rules{}
	r.check(workgroupName.MatchString(workgroup), validation, "The workgroup name is not valid.")
	r.check(len(domain) <= maxDomainName && domainName.MatchString(strings.ToLower(domain)), validation, "The custom domain name is not valid.")
	r.check(f.hasWorkgroup(workgroup, ""), notFound, "Serverless workgroup "+workgroup+" not found.")
	certificateRule(r, certificate)
	r.check(!f.workgroupBusy(workgroup), conflict, "An operation is running on the serverless workgroup.")
	r.check(f.domainOf(workgroup) == "", conflict, "The workgroup "+workgroup+" already has a custom domain name.")
	for _, id := range f.store.List(customDomainType) {
		_, other, _ := strings.Cut(id, "|")
		r.check(other != domain, conflict, "The custom domain name "+domain+" is already in use.")
	}
	if r.err != nil {
		return nil, r.err
	}
	props := map[string]any{"WorkgroupName": workgroup, "CustomDomainName": domain, "CustomDomainCertificateArn": certificate}
	f.store.Put(customDomainType, workgroup+"|"+domain, props)
	f.change(workgroup)
	a := associationOf(props)
	return &redshiftserverless.CreateCustomDomainAssociationOutput{
		WorkgroupName:                     a.WorkgroupName,
		CustomDomainName:                  a.CustomDomainName,
		CustomDomainCertificateArn:        a.CustomDomainCertificateArn,
		CustomDomainCertificateExpiryTime: a.CustomDomainCertificateExpiryTime,
	}, nil
}

func (f *fakeAPI) GetCustomDomainAssociation(ctx context.Context, in *redshiftserverless.GetCustomDomainAssociationInput, _ ...func(*redshiftserverless.Options)) (*redshiftserverless.GetCustomDomainAssociationOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("GetCustomDomainAssociation"); err != nil {
		return nil, err
	}
	props := f.store.Resource(customDomainType, aws.ToString(in.WorkgroupName)+"|"+aws.ToString(in.CustomDomainName))
	if props == nil {
		return nil, fault(notFound, "The custom domain name is not used.")
	}
	a := associationOf(props)
	return &redshiftserverless.GetCustomDomainAssociationOutput{
		WorkgroupName:                     a.WorkgroupName,
		CustomDomainName:                  a.CustomDomainName,
		CustomDomainCertificateArn:        a.CustomDomainCertificateArn,
		CustomDomainCertificateExpiryTime: a.CustomDomainCertificateExpiryTime,
	}, nil
}

func (f *fakeAPI) UpdateCustomDomainAssociation(ctx context.Context, in *redshiftserverless.UpdateCustomDomainAssociationInput, _ ...func(*redshiftserverless.Options)) (*redshiftserverless.UpdateCustomDomainAssociationOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("UpdateCustomDomainAssociation"); err != nil {
		return nil, err
	}
	workgroup := aws.ToString(in.WorkgroupName)
	id := workgroup + "|" + aws.ToString(in.CustomDomainName)
	props := f.store.Resource(customDomainType, id)
	if props == nil {
		return nil, associationNotFound()
	}
	r := &rules{}
	certificateRule(r, aws.ToString(in.CustomDomainCertificateArn))
	r.check(!f.workgroupBusy(workgroup), conflict, "An operation is running on the serverless workgroup.")
	if r.err != nil {
		return nil, r.err
	}
	props["CustomDomainCertificateArn"] = aws.ToString(in.CustomDomainCertificateArn)
	f.store.Put(customDomainType, id, props)
	f.change(workgroup)
	a := associationOf(props)
	return &redshiftserverless.UpdateCustomDomainAssociationOutput{
		WorkgroupName:                     a.WorkgroupName,
		CustomDomainName:                  a.CustomDomainName,
		CustomDomainCertificateArn:        a.CustomDomainCertificateArn,
		CustomDomainCertificateExpiryTime: a.CustomDomainCertificateExpiryTime,
	}, nil
}

func (f *fakeAPI) DeleteCustomDomainAssociation(ctx context.Context, in *redshiftserverless.DeleteCustomDomainAssociationInput, _ ...func(*redshiftserverless.Options)) (*redshiftserverless.DeleteCustomDomainAssociationOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DeleteCustomDomainAssociation"); err != nil {
		return nil, err
	}
	workgroup := aws.ToString(in.WorkgroupName)
	id := workgroup + "|" + aws.ToString(in.CustomDomainName)
	if f.store.Resource(customDomainType, id) == nil {
		return nil, associationNotFound()
	}
	if f.workgroupBusy(workgroup) {
		return nil, fault(conflict, "An operation is running on the serverless workgroup.")
	}
	f.store.Remove(customDomainType, id)
	f.change(workgroup)
	return &redshiftserverless.DeleteCustomDomainAssociationOutput{}, nil
}

func (f *fakeAPI) ListCustomDomainAssociations(ctx context.Context, in *redshiftserverless.ListCustomDomainAssociationsInput, _ ...func(*redshiftserverless.Options)) (*redshiftserverless.ListCustomDomainAssociationsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("ListCustomDomainAssociations"); err != nil {
		return nil, err
	}
	all := []types.Association{}
	for _, id := range f.store.List(customDomainType) {
		props := f.store.Resource(customDomainType, id)
		if in.CustomDomainName != nil && awsapi.String(props, "CustomDomainName") != aws.ToString(in.CustomDomainName) {
			continue
		}
		all = append(all, associationOf(props))
	}
	r := &rules{}
	list, next := page(r, all, in.NextToken, in.MaxResults)
	if r.err != nil {
		return nil, r.err
	}
	return &redshiftserverless.ListCustomDomainAssociationsOutput{Associations: list, NextToken: next}, nil
}
