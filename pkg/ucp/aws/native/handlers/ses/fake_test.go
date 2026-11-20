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

package ses

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	sestypes "github.com/aws/aws-sdk-go-v2/service/ses/types"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	// account is the fake's account.
	account = "123456789012"

	// missingPrefix starts the names of identities and SNS topics SES doesn't have, and noForwardingPrefix the names
	// of identities whose email feedback forwarding is off.
	missingPrefix      = "missing"
	noForwardingPrefix = "noforward"

	badRequestCode = "BadRequestException"
	limitCode      = "LimitExceededException"

	// invalidParameterCode and validationErrorCode are the SES v1 API's errors for a value it doesn't take. The SDK
	// doesn't model them, so their fault is unknown.
	invalidParameterCode = "InvalidParameterValue"
	validationErrorCode  = "ValidationError"

	// maxPolicies is how many policies an identity can have, maxPolicyBytes how long a policy can be, and
	// maxIdentities how many identities GetIdentityNotificationAttributes reads at once.
	maxPolicies    = 20
	maxPolicyBytes = 4000
	maxIdentities  = 100

	// staleReads is how many reads after a change still return the earlier value.
	staleReads = 2

	// defaultPool and sharedPool are the pools SES manages: the one an address is in until it is moved, and the one
	// every SES customer shares.
	defaultPool = "ses-default-dedicated-pool"
	sharedPool  = "ses-shared-pool"
)

// leased are the dedicated IP addresses the fake account leases. Each is in ses-default-dedicated-pool until a call
// moves it, and the store has the pool of each address a call moved.
var leased = []string{"198.51.100.7", "198.51.100.8"}

// pools are the dedicated IP pools the fake account has, and whether SES manages each one's addresses.
var pools = map[string]bool{"pool-a": false, "pool-b": false, "managed-pool": true}

var policyNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// stale is what reads of a resource return for a while after a change: before, or nil for none, for reads more
// reads, while AWS still has after, or nil for none.
type stale struct {
	before map[string]any
	after  map[string]any
	reads  int
}

// fakeAPI is an in-memory SES API, both v1 and v2, for identity policies, notification topics, and dedicated IP
// addresses. It keeps them in an awsfake store, in their types' property format, so the suite's Cloud Control sees
// the same resources.
//
// Every identity exists, except those whose name starts with missingPrefix. Email feedback forwarding is on, except
// for identities whose name starts with noForwardingPrefix. Every SNS topic in the fake's account and region exists,
// except those whose name starts with missingPrefix. The account leases the dedicated IP addresses in leased.
//
// As SES may, a read right after a change returns the earlier value, for staleReads reads by default.
type fakeAPI struct {
	store *awsfake.CloudControl

	mu     sync.Mutex
	calls  []string
	stale  map[string]*stale
	failer nativetest.Failer

	// staleReads is how many reads after a change still return the earlier value.
	staleReads int

	// headers are the identities and notification types whose headers setting is on while they have no topic.
	headers map[string]bool

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error
}

var (
	_ V1API = (*fakeAPI)(nil)
	_ V2API = (*fakeAPI)(nil)
)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{store: store, stale: map[string]*stale{}, staleReads: staleReads, headers: map[string]bool{}, errs: map[string]error{}}
}

// noWait is a pause that doesn't wait.
func noWait(context.Context, time.Duration) {}

// apiError returns the error AWS returns with HTTP status and code. The fake marks every error a client fault, as
// the suite's API rules require, even those the SDK doesn't model, whose fault is unknown in production.
func apiError(service string, status int, code string, message string) error {
	return &smithy.OperationError{
		ServiceID:     service,
		OperationName: "Call",
		Err: &smithyhttp.ResponseError{
			Response: &smithyhttp.Response{Response: &http.Response{StatusCode: status}},
			Err:      &smithy.GenericAPIError{Code: code, Message: message, Fault: smithy.FaultClient},
		},
	}
}

// unmodeled returns an error with HTTP status 400 and code, whose fault is unknown, as the SDK reads an SES v1 error it
// doesn't model.
func unmodeled(code string, message string) error {
	return &smithy.OperationError{
		ServiceID:     "SES",
		OperationName: "Call",
		Err: &smithyhttp.ResponseError{
			Response: &smithyhttp.Response{Response: &http.Response{StatusCode: http.StatusBadRequest}},
			Err:      &smithy.GenericAPIError{Code: code, Message: message},
		},
	}
}

func v2Error(status int, code string, format string, args ...any) error {
	return apiError("SESv2", status, code, fmt.Sprintf(format, args...))
}

func notFound(format string, args ...any) error {
	return v2Error(http.StatusNotFound, notFoundCode, format, args...)
}

func badRequest(format string, args ...any) error {
	return v2Error(http.StatusBadRequest, badRequestCode, format, args...)
}

func invalidParameter(format string, args ...any) error {
	return apiError("SES", http.StatusBadRequest, invalidParameterCode, fmt.Sprintf(format, args...))
}

// missing returns the error the SDK returns, before it sends anything, for a call without a required parameter.
func missing(operation string, parameter string) error {
	err := smithy.InvalidParamsError{Context: operation}
	err.Add(smithy.NewErrParamRequired(parameter))
	return err
}

// begin records a call and returns the error errs sets for it.
func (f *fakeAPI) begin(method string) error {
	f.calls = append(f.calls, method)
	return f.errs[method]
}

// change runs a call that changes AWS, with the failer.
func (f *fakeAPI) change(apply func()) error {
	if err := f.failer.Before(); err != nil {
		return err
	}
	apply()
	return f.failer.After()
}

// read returns the resource of typeName with id as a read sees it, and whether there is one.
func (f *fakeAPI) read(typeName string, id string) (map[string]any, bool) {
	current, ok := f.store.Read(typeName, id)
	key := typeName + "|" + id
	if s := f.stale[key]; s != nil {
		if s.reads > 0 && reflect.DeepEqual(current, s.after) {
			s.reads--
			return s.before, s.before != nil
		}
		delete(f.stale, key)
	}
	return current, ok
}

// write sets the resource of typeName with id to props, or removes it when props is nil. Reads return the earlier
// resource for staleReads reads.
func (f *fakeAPI) write(typeName string, id string, props map[string]any) {
	before, _ := f.store.Read(typeName, id)
	if props == nil {
		f.store.Remove(typeName, id)
	} else {
		f.store.Put(typeName, id, props)
	}
	after, _ := f.store.Read(typeName, id)
	f.stale[typeName+"|"+id] = &stale{before: before, after: after, reads: f.staleReads}
}

// identityExists reports whether SES has the identity.
func identityExists(identity string) bool {
	return !strings.HasPrefix(identity, missingPrefix)
}

// identityARN returns the ARN of the identity in the fake's account.
func identityARN(identity string) string {
	return "arn:aws:ses:" + region + ":" + account + ":identity/" + identity
}

// storedPolicy returns policy as SES stores it, or AWS's error for a policy SES doesn't take for identity: one that
// isn't JSON, is longer than maxPolicyBytes, has a statement without an Effect of Allow or Deny, a principal, an
// action, or a resource, has a statement whose resource isn't the identity, or allows an action of another service. SES stores an account ID in a principal as the account's root ARN, a statement's list of one
// value as the value, without whitespace, and with Version 2008-10-17 when the policy has none. SES also keeps its
// own key order and reorders lists; the fake orders keys by name, sorts a statement's AWS principals in descending
// order, as a live call returned them, and keeps other lists as they are.
func storedPolicy(identity string, policy string) (string, error) {
	if len(policy) > maxPolicyBytes {
		return "", badRequest("The policy size, %d, is larger than the max policy size, %d.", len(policy), maxPolicyBytes)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(policy), &doc); err != nil || doc == nil {
		return "", badRequest("Unable to parse policy.")
	}
	if _, ok := doc["Version"]; !ok {
		doc["Version"] = defaultPolicyVersion
	}
	statements, _ := doc["Statement"].([]any)
	if s, ok := doc["Statement"].(map[string]any); ok {
		statements = []any{s}
	}
	if len(statements) == 0 {
		return "", badRequest("Unable to parse policy.")
	}
	for _, s := range statements {
		statement, _ := s.(map[string]any)
		switch {
		case statement["Effect"] != "Allow" && statement["Effect"] != "Deny":
			return "", badRequest("Unable to parse policy.")
		case !hasPrincipal(statement["Principal"]):
			return "", badRequest("There must be at least one principal associated to each statement.")
		case len(values(statement["Action"])) == 0:
			return "", badRequest("Each statement must have at least one action associated to it.")
		case len(values(statement["Resource"])) == 0:
			return "", badRequest("SES resource policies must be explicitly associated to the resource ARN they apply to.")
		}
		for _, resource := range values(statement["Resource"]) {
			if resource != identityARN(identity) {
				return "", badRequest("Expected the resource to be '%s' but was '%s'.", identityARN(identity), resource)
			}
		}
		for _, action := range values(statement["Action"]) {
			if !strings.HasPrefix(strings.ToLower(action), "ses:") {
				return "", badRequest("Invalid action '%s'.", action)
			}
		}
		if principal, ok := statement["Principal"].(map[string]any); ok {
			if ids := values(principal["AWS"]); len(ids) > 0 {
				arns := []any{}
				for _, id := range ids {
					if len(id) == 12 && strings.Trim(id, "0123456789") == "" {
						id = "arn:aws:iam::" + id + ":root"
					}
					arns = append(arns, id)
				}
				// In a live call SES returned a list of principals in descending order.
				slices.SortFunc(arns, func(a, b any) int { return strings.Compare(fmt.Sprint(b), fmt.Sprint(a)) })
				principal["AWS"] = arns
			}
			for key, value := range principal {
				principal[key] = single(value)
			}
		}
		for _, key := range []string{"Action", "NotAction", "Resource", "NotResource"} {
			if value, ok := statement[key]; ok {
				statement[key] = single(value)
			}
		}
	}
	b, _ := json.Marshal(doc)
	return string(b), nil
}

// hasPrincipal reports whether principal, a statement's Principal, names at least one: "*", or a type with a value.
func hasPrincipal(principal any) bool {
	if principal == "*" {
		return true
	}
	principals, _ := principal.(map[string]any)
	for _, value := range principals {
		if len(values(value)) > 0 {
			return true
		}
	}
	return false
}

// values returns v, a string or a list of strings, as a list.
func values(v any) []string {
	switch v := v.(type) {
	case string:
		return []string{v}
	case []any:
		out := []string{}
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// single returns v, or its one value when it is a list of one value.
func single(v any) any {
	if list, ok := v.([]any); ok && len(list) == 1 {
		return list[0]
	}
	return v
}

// policyArgs returns the error SES returns for a policy call's identity and name, or nil.
func policyArgs(operation string, identity *string, name *string) error {
	switch {
	case identity == nil:
		return missing(operation, "EmailIdentity")
	case name == nil:
		return missing(operation, "PolicyName")
	case len(*name) > 64:
		return badRequest("1 validation error detected: Value at 'policyName' failed to satisfy constraint: Member must have length less than or equal to 64")
	case !policyNamePattern.MatchString(*name):
		return badRequest("PolicyName is invalid. Policy names must only include alpha-numeric characters, dashes, and underscores.")
	case !identityExists(*identity):
		return notFound("Email identity <%s> does not exist.", *identity)
	}
	return nil
}

func (f *fakeAPI) CreateEmailIdentityPolicy(ctx context.Context, in *sesv2.CreateEmailIdentityPolicyInput, _ ...func(*sesv2.Options)) (*sesv2.CreateEmailIdentityPolicyOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("CreateEmailIdentityPolicy"); err != nil {
		return nil, err
	}
	if err := policyArgs("CreateEmailIdentityPolicyInput", in.EmailIdentity, in.PolicyName); err != nil {
		return nil, err
	}
	if in.Policy == nil {
		return nil, missing("CreateEmailIdentityPolicyInput", "Policy")
	}
	identity, name := *in.EmailIdentity, *in.PolicyName
	if _, ok := f.store.Read(identityPolicyType, identity+"|"+name); ok {
		return nil, v2Error(http.StatusBadRequest, alreadyExistsCode, "Policy <%s> already exists", name)
	}
	if len(f.policies(identity, false)) >= maxPolicies {
		return nil, v2Error(http.StatusBadRequest, limitCode, "Maximum number of policies (%d) reached for identity <%s>.", maxPolicies, identity)
	}
	policy, err := storedPolicy(identity, *in.Policy)
	if err != nil {
		return nil, err
	}
	err = f.change(func() {
		f.write(identityPolicyType, identity+"|"+name, map[string]any{"EmailIdentity": identity, "PolicyName": name, "Policy": policy})
	})
	if err != nil {
		return nil, err
	}
	return &sesv2.CreateEmailIdentityPolicyOutput{}, nil
}

func (f *fakeAPI) UpdateEmailIdentityPolicy(ctx context.Context, in *sesv2.UpdateEmailIdentityPolicyInput, _ ...func(*sesv2.Options)) (*sesv2.UpdateEmailIdentityPolicyOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("UpdateEmailIdentityPolicy"); err != nil {
		return nil, err
	}
	if err := policyArgs("UpdateEmailIdentityPolicyInput", in.EmailIdentity, in.PolicyName); err != nil {
		return nil, err
	}
	if in.Policy == nil {
		return nil, missing("UpdateEmailIdentityPolicyInput", "Policy")
	}
	identity, name := *in.EmailIdentity, *in.PolicyName
	policy, err := storedPolicy(identity, *in.Policy)
	if err != nil {
		return nil, err
	}
	// The page says the call succeeds for a policy SES doesn't have, but live calls got NotFoundException.
	if _, ok := f.store.Read(identityPolicyType, identity+"|"+name); !ok {
		return nil, notFound("Policy <%s> does not exist", name)
	}
	err = f.change(func() {
		f.write(identityPolicyType, identity+"|"+name, map[string]any{"EmailIdentity": identity, "PolicyName": name, "Policy": policy})
	})
	if err != nil {
		return nil, err
	}
	return &sesv2.UpdateEmailIdentityPolicyOutput{}, nil
}

// policies returns the policies of identity by name. When stale is true, they are as a read sees them.
func (f *fakeAPI) policies(identity string, stale bool) map[string]string {
	ids := f.store.List(identityPolicyType)
	if stale {
		for key := range f.stale {
			if id, ok := strings.CutPrefix(key, identityPolicyType+"|"); ok && !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
	}
	out := map[string]string{}
	for _, id := range ids {
		if !strings.HasPrefix(id, identity+"|") {
			continue
		}
		props, ok := f.store.Read(identityPolicyType, id)
		if stale {
			props, ok = f.read(identityPolicyType, id)
		}
		if ok {
			out[fmt.Sprint(props["PolicyName"])] = fmt.Sprint(props["Policy"])
		}
	}
	return out
}

func (f *fakeAPI) GetEmailIdentityPolicies(ctx context.Context, in *sesv2.GetEmailIdentityPoliciesInput, _ ...func(*sesv2.Options)) (*sesv2.GetEmailIdentityPoliciesOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("GetEmailIdentityPolicies"); err != nil {
		return nil, err
	}
	switch {
	case in.EmailIdentity == nil:
		return nil, missing("GetEmailIdentityPoliciesInput", "EmailIdentity")
	case !identityExists(*in.EmailIdentity):
		return nil, notFound("Email identity <%s> does not exist.", *in.EmailIdentity)
	}
	return &sesv2.GetEmailIdentityPoliciesOutput{Policies: f.policies(*in.EmailIdentity, true)}, nil
}

func (f *fakeAPI) DeleteEmailIdentityPolicy(ctx context.Context, in *sesv2.DeleteEmailIdentityPolicyInput, _ ...func(*sesv2.Options)) (*sesv2.DeleteEmailIdentityPolicyOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DeleteEmailIdentityPolicy"); err != nil {
		return nil, err
	}
	if err := policyArgs("DeleteEmailIdentityPolicyInput", in.EmailIdentity, in.PolicyName); err != nil {
		return nil, err
	}
	id := *in.EmailIdentity + "|" + *in.PolicyName
	// The page says the call succeeds for a policy SES doesn't have, but live calls got NotFoundException.
	if _, ok := f.store.Read(identityPolicyType, id); !ok {
		return nil, notFound("Policy <%s> does not exist", *in.PolicyName)
	}
	if err := f.change(func() { f.write(identityPolicyType, id, nil) }); err != nil {
		return nil, err
	}
	return &sesv2.DeleteEmailIdentityPolicyOutput{}, nil
}

// notificationArgs returns the error SES returns for a notification call's identity and type, or nil.
func notificationArgs(operation string, identity *string, kind sestypes.NotificationType) error {
	switch {
	case identity == nil:
		return missing(operation, "Identity")
	case kind == "":
		return missing(operation, "NotificationType")
	case !slices.Contains(kind.Values(), kind):
		return apiError("SES", http.StatusBadRequest, validationErrorCode, "1 validation error detected: Value at 'notificationType' failed to satisfy constraint: Member must satisfy enum value set: [Bounce, Complaint, Delivery]")
	case !identityExists(*identity):
		return invalidParameter("Identity %s is invalid. Must be a verified email address or domain.", *identity)
	}
	return nil
}

// topicExists reports whether SNS has the topic in the fake's account and region.
func topicExists(topic string) bool {
	name, ok := strings.CutPrefix(topic, "arn:aws:sns:"+region+":"+account+":")
	return ok && name != "" && !strings.HasPrefix(name, missingPrefix)
}

func (f *fakeAPI) SetIdentityNotificationTopic(ctx context.Context, in *ses.SetIdentityNotificationTopicInput, _ ...func(*ses.Options)) (*ses.SetIdentityNotificationTopicOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("SetIdentityNotificationTopic"); err != nil {
		return nil, err
	}
	if err := notificationArgs("SetIdentityNotificationTopicInput", in.Identity, in.NotificationType); err != nil {
		return nil, err
	}
	identity, kind, topic := *in.Identity, in.NotificationType, aws.ToString(in.SnsTopic)
	if topic != "" && !topicExists(topic) {
		return nil, invalidParameter("SNS topic %s is invalid.", topic)
	}
	if topic == "" && kind != sestypes.NotificationTypeDelivery && strings.HasPrefix(identity, noForwardingPrefix) {
		return nil, invalidParameter("Identity %s must have SNS topics for bounces and complaints while email feedback forwarding is disabled.", identity)
	}
	id := identity + "|" + string(kind)
	current, ok := f.store.Read(notificationTopicType, id)
	err := f.change(func() {
		switch {
		case topic == "" && ok:
			// SES keeps the headers setting when it clears the topic.
			f.headers[id], _ = current[headersProperty].(bool)
			f.write(notificationTopicType, id, nil)
		case topic != "":
			headers := f.headers[id]
			if ok {
				headers, _ = current[headersProperty].(bool)
			}
			f.write(notificationTopicType, id, topicProps(identity, kind, topic, headers))
		}
	})
	if err != nil {
		return nil, err
	}
	return &ses.SetIdentityNotificationTopicOutput{}, nil
}

func (f *fakeAPI) SetIdentityHeadersInNotificationsEnabled(ctx context.Context, in *ses.SetIdentityHeadersInNotificationsEnabledInput, _ ...func(*ses.Options)) (*ses.SetIdentityHeadersInNotificationsEnabledOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("SetIdentityHeadersInNotificationsEnabled"); err != nil {
		return nil, err
	}
	if err := notificationArgs("SetIdentityHeadersInNotificationsEnabledInput", in.Identity, in.NotificationType); err != nil {
		return nil, err
	}
	id := *in.Identity + "|" + string(in.NotificationType)
	current, ok := f.store.Read(notificationTopicType, id)
	if !ok {
		return nil, invalidParameter("Identity %s has no SNS topic for %s notifications.", *in.Identity, in.NotificationType)
	}
	err := f.change(func() {
		f.write(notificationTopicType, id, topicProps(*in.Identity, in.NotificationType, fmt.Sprint(current["SnsTopic"]), in.Enabled))
	})
	if err != nil {
		return nil, err
	}
	return &ses.SetIdentityHeadersInNotificationsEnabledOutput{}, nil
}

func (f *fakeAPI) GetIdentityNotificationAttributes(ctx context.Context, in *ses.GetIdentityNotificationAttributesInput, _ ...func(*ses.Options)) (*ses.GetIdentityNotificationAttributesOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("GetIdentityNotificationAttributes"); err != nil {
		return nil, err
	}
	switch {
	case in.Identities == nil:
		return nil, missing("GetIdentityNotificationAttributesInput", "Identities")
	case len(in.Identities) > maxIdentities:
		return nil, invalidParameter("Can only get notification attributes for up to %d identities at a time.", maxIdentities)
	}
	out := map[string]sestypes.IdentityNotificationAttributes{}
	for _, identity := range in.Identities {
		// SES leaves out an identity it doesn't have, without an error.
		if !identityExists(identity) {
			continue
		}
		attributes := sestypes.IdentityNotificationAttributes{ForwardingEnabled: !strings.HasPrefix(identity, noForwardingPrefix)}
		for _, kind := range sestypes.NotificationType("").Values() {
			id := identity + "|" + string(kind)
			topic, headers := "", f.headers[id]
			if props, ok := f.read(notificationTopicType, id); ok {
				topic = fmt.Sprint(props["SnsTopic"])
				headers, _ = props[headersProperty].(bool)
			}
			switch kind {
			case sestypes.NotificationTypeBounce:
				attributes.BounceTopic, attributes.HeadersInBounceNotificationsEnabled = optional(topic), headers
			case sestypes.NotificationTypeComplaint:
				attributes.ComplaintTopic, attributes.HeadersInComplaintNotificationsEnabled = optional(topic), headers
			case sestypes.NotificationTypeDelivery:
				attributes.DeliveryTopic, attributes.HeadersInDeliveryNotificationsEnabled = optional(topic), headers
			}
		}
		out[identity] = attributes
	}
	return &ses.GetIdentityNotificationAttributesOutput{NotificationAttributes: out}, nil
}

// topicProps returns the properties the fake stores for a topic. It leaves out the headers setting when it is off,
// SES's default.
func topicProps(identity string, kind sestypes.NotificationType, topic string, headers bool) map[string]any {
	props := map[string]any{"Identity": identity, "NotificationType": string(kind), "SnsTopic": topic}
	if headers {
		props[headersProperty] = true
	}
	return props
}

// optional returns s, or nil when it is empty, as SES leaves out a topic an identity doesn't have.
func optional(s string) *string {
	if s == "" {
		return nil
	}
	return aws.String(s)
}

// ipArg returns the error SES returns for a dedicated IP call's address, or nil.
func ipArg(operation string, ip *string) error {
	switch {
	case ip == nil:
		return missing(operation, "Ip")
	case net.ParseIP(*ip) == nil:
		return badRequest("Provided Ip is not a valid inet address.")
	}
	return nil
}

func (f *fakeAPI) GetDedicatedIp(ctx context.Context, in *sesv2.GetDedicatedIpInput, _ ...func(*sesv2.Options)) (*sesv2.GetDedicatedIpOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("GetDedicatedIp"); err != nil {
		return nil, err
	}
	if err := ipArg("GetDedicatedIpInput", in.Ip); err != nil {
		return nil, err
	}
	if !slices.Contains(leased, *in.Ip) {
		return nil, notFound("Could not find dedicated IP <%s> under this account.", *in.Ip)
	}
	// The page marks PoolName optional, so the fake leaves it out for an address in ses-default-dedicated-pool.
	var pool *string
	if props, ok := f.read(dedicatedIPType, *in.Ip); ok {
		pool = aws.String(fmt.Sprint(props[poolProperty]))
	}
	return &sesv2.GetDedicatedIpOutput{DedicatedIp: &types.DedicatedIp{
		Ip:               in.Ip,
		PoolName:         pool,
		WarmupPercentage: aws.Int32(100),
		WarmupStatus:     types.WarmupStatusDone,
	}}, nil
}

func (f *fakeAPI) PutDedicatedIpInPool(ctx context.Context, in *sesv2.PutDedicatedIpInPoolInput, _ ...func(*sesv2.Options)) (*sesv2.PutDedicatedIpInPoolOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("PutDedicatedIpInPool"); err != nil {
		return nil, err
	}
	if err := ipArg("PutDedicatedIpInPoolInput", in.Ip); err != nil {
		return nil, err
	}
	if in.DestinationPoolName == nil {
		return nil, missing("PutDedicatedIpInPoolInput", "DestinationPoolName")
	}
	ip, pool := *in.Ip, *in.DestinationPoolName
	if !slices.Contains(leased, ip) {
		return nil, notFound("Could not find dedicated IP <%s> under this account.", ip)
	}
	current, _ := f.store.Read(dedicatedIPType, ip)
	managed, exists := pools[pool]
	switch {
	case pool == defaultPool || pool == sharedPool:
		return nil, badRequest("Pool name describes either a shared or a default dedicated pool.")
	case !exists:
		return nil, notFound("Pool <%s> does not exist.", pool)
	case managed:
		return nil, badRequest("Pool <%s> is managed by SES.", pool)
	case pools[fmt.Sprint(current[poolProperty])]:
		return nil, badRequest("Dedicated IP <%s> is in a pool managed by SES.", ip)
	}
	if err := f.change(func() { f.write(dedicatedIPType, ip, map[string]any{"Ip": ip, poolProperty: pool}) }); err != nil {
		return nil, err
	}
	return &sesv2.PutDedicatedIpInPoolOutput{}, nil
}
