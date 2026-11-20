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

package connect

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/connect"
	"github.com/aws/aws-sdk-go-v2/service/connect/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
)

const (
	region  = "us-west-2"
	account = "123456789012"

	invalidParameterCode = "InvalidParameterException"

	// testInstance has the phone numbers testPhone and otherPhone, and the flows testFlow and otherFlow.
	// otherInstance has none. missingInstance, missingPhone, and missingFlow are IDs AWS doesn't have.
	testInstance    = "11111111-1111-4111-8111-111111111111"
	otherInstance   = "22222222-2222-4222-8222-222222222222"
	missingInstance = "99999999-9999-4999-8999-999999999999"
	testPhone       = "aaaaaaaa-0000-4000-8000-000000000001"
	otherPhone      = "aaaaaaaa-0000-4000-8000-000000000002"
	missingPhone    = "aaaaaaaa-0000-4000-8000-00000000dead"
	testFlow        = "ffffffff-0000-4000-8000-000000000001"
	otherFlow       = "ffffffff-0000-4000-8000-000000000002"
	missingFlow     = "ffffffff-0000-4000-8000-00000000dead"

	// tdgPhone is claimed to a traffic distribution group created in tdgRegion, with testInstance's Region as the
	// other Region, where AWS finds it only by its ARN.
	tdgPhone  = "aaaaaaaa-0000-4000-8000-000000000003"
	tdgRegion = "us-east-1"

	// staleLists is how many lists after a change still return the association as it was.
	staleLists = 10

	// pageSize is how many associations a page of ListFlowAssociations has.
	pageSize = 2
)

var (
	// phoneNumbers maps each phone number the fake has to the instance it is claimed to.
	phoneNumbers = map[string]string{testPhone: testInstance, otherPhone: testInstance, tdgPhone: testInstance}

	// flows maps each flow the fake has to its instance.
	flows = map[string]string{testFlow: testInstance, otherFlow: testInstance}

	instances = []string{testInstance, otherInstance}

	uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

func instanceARN(instance string) string {
	return fmt.Sprintf("arn:aws:connect:%s:%s:instance/%s", region, account, instance)
}

func phoneARN(phone string) string {
	r := region
	if phone == tdgPhone {
		r = tdgRegion
	}
	return fmt.Sprintf("arn:aws:connect:%s:%s:phone-number/%s", r, account, phone)
}

func flowARN(flow string) string {
	return instanceARN(flows[flow]) + "/contact-flow/" + flow
}

// stale is what lists of a phone number's association return for a while after a change: before, or nil for none,
// for lists more lists, while AWS has after, or nil for none.
type stale struct {
	before map[string]any
	after  map[string]any
	lists  int
}

// fakeAPI is an in-memory Amazon Connect API for phone number flow associations. It keeps them in an awsfake store, in
// the type's property format, under the identifier of the template that made them, so the suite's Cloud Control sees
// the same resources. It lists a flow in the form the association gave it, so the suite's templates read back as they
// are; the suite's OtherForm, and testdata, cover the flow's ARN, which AWS returns.
//
// The docs don't say when a change shows, so a list right after an association changes returns it as it was, for
// staleLists lists.
type fakeAPI struct {
	store *awsfake.CloudControl

	mu    sync.Mutex
	calls []string
	stale map[string]*stale

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{store: store, stale: map[string]*stale{}, errs: map[string]error{}}
}

// noWait is a pause that doesn't wait.
func noWait(context.Context, time.Duration) {}

// apiError returns the error AWS returns with HTTP status and code.
func apiError(status int, code string, message string) error {
	return &smithy.OperationError{
		ServiceID:     "Connect",
		OperationName: "Call",
		Err: &smithyhttp.ResponseError{
			Response: &smithyhttp.Response{Response: &http.Response{StatusCode: status}},
			Err:      &smithy.GenericAPIError{Code: code, Message: message, Fault: smithy.FaultClient},
		},
	}
}

func resourceNotFound(format string, args ...any) error {
	return apiError(http.StatusNotFound, notFoundCode, fmt.Sprintf(format, args...))
}

func invalid(parameter string) error {
	return apiError(http.StatusBadRequest, invalidParameterCode, "Invalid request parameter: "+parameter)
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

// lastSegment returns the part of id after its last "/", the ID an ARN gives, or id when it isn't an ARN.
func lastSegment(id string) string {
	if strings.HasPrefix(id, "arn:") {
		return id[strings.LastIndex(id, "/")+1:]
	}
	return id
}

// instance returns the ID of the instance id gives, by ID or ARN, or the error AWS returns for one it doesn't have.
func instance(id *string, operation string) (string, error) {
	switch {
	case id == nil:
		return "", missing(operation, "InstanceId")
	case len(*id) == 0 || len(*id) > 100:
		return "", invalid("InstanceId")
	}
	got := lastSegment(*id)
	if !slices.Contains(instances, got) || (strings.HasPrefix(*id, "arn:") && *id != instanceARN(got)) {
		return "", resourceNotFound("Instance not found: %s", *id)
	}
	return got, nil
}

// phoneNumber returns the ID of the phone number id gives, by ID or ARN, claimed to instance, or the error AWS
// returns. A number claimed to a traffic distribution group is found in the group's other Region only by ARN.
func phoneNumber(id *string, instance string, operation string) (string, error) {
	if id == nil {
		return "", missing(operation, "PhoneNumberId")
	}
	got := lastSegment(*id)
	switch {
	case !uuidPattern.MatchString(got):
		return "", invalid("PhoneNumberId")
	case phoneNumbers[got] != instance:
		return "", resourceNotFound("Phone number not found: %s", got)
	case got == tdgPhone && !strings.HasPrefix(*id, "arn:"):
		return "", resourceNotFound("Phone number not found: %s", got)
	case strings.HasPrefix(*id, "arn:") && *id != phoneARN(got):
		return "", resourceNotFound("Phone number not found: %s", got)
	}
	return got, nil
}

// associations returns the identifiers in store of the associations of the phone number, in any form.
func (f *fakeAPI) associations(phone string) []string {
	out := []string{}
	for _, identifier := range f.store.List(associationType) {
		number, _, _ := strings.Cut(identifier, "|")
		if lastSegment(number) == phone {
			out = append(out, identifier)
		}
	}
	return out
}

// read returns the association with identifier as a list sees it, and whether there is one.
func (f *fakeAPI) read(identifier string) (map[string]any, bool) {
	current, ok := f.store.Read(associationType, identifier)
	if s := f.stale[identifier]; s != nil {
		if s.lists > 0 && reflect.DeepEqual(current, s.after) {
			s.lists--
			return s.before, s.before != nil
		}
		delete(f.stale, identifier)
	}
	return current, ok
}

// write sets the association with identifier to props, or deletes it when props is nil. Lists return it as it was
// for staleLists lists.
func (f *fakeAPI) write(identifier string, props map[string]any) {
	before, _ := f.store.Read(associationType, identifier)
	if props == nil {
		f.store.Remove(associationType, identifier)
	} else {
		f.store.Put(associationType, identifier, props)
	}
	after, _ := f.store.Read(associationType, identifier)
	f.stale[identifier] = &stale{before: before, after: after, lists: staleLists}
}

func (f *fakeAPI) AssociatePhoneNumberContactFlow(ctx context.Context, in *connect.AssociatePhoneNumberContactFlowInput, _ ...func(*connect.Options)) (*connect.AssociatePhoneNumberContactFlowOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("AssociatePhoneNumberContactFlow"); err != nil {
		return nil, err
	}
	if in.ContactFlowId == nil {
		return nil, missing("AssociatePhoneNumberContactFlowInput", "ContactFlowId")
	}
	if len(*in.ContactFlowId) > 500 {
		return nil, invalid("ContactFlowId")
	}
	inst, err := instance(in.InstanceId, "AssociatePhoneNumberContactFlowInput")
	if err != nil {
		return nil, err
	}
	phone, err := phoneNumber(in.PhoneNumberId, inst, "AssociatePhoneNumberContactFlowInput")
	if err != nil {
		return nil, err
	}
	flow := lastSegment(*in.ContactFlowId)
	if flows[flow] != inst || (strings.HasPrefix(*in.ContactFlowId, "arn:") && *in.ContactFlowId != flowARN(flow)) {
		return nil, resourceNotFound("Contact flow not found: %s", *in.ContactFlowId)
	}
	// A number has one flow, so the association replaces any other, whatever form named the number.
	identifier := *in.PhoneNumberId + "|" + *in.InstanceId
	for _, other := range f.associations(phone) {
		if other != identifier {
			f.write(other, nil)
		}
	}
	f.write(identifier, map[string]any{phoneNumberIDProperty: *in.PhoneNumberId, instanceIDProperty: *in.InstanceId, contactFlowIDProperty: *in.ContactFlowId})
	return &connect.AssociatePhoneNumberContactFlowOutput{}, nil
}

func (f *fakeAPI) DisassociatePhoneNumberContactFlow(ctx context.Context, in *connect.DisassociatePhoneNumberContactFlowInput, _ ...func(*connect.Options)) (*connect.DisassociatePhoneNumberContactFlowOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DisassociatePhoneNumberContactFlow"); err != nil {
		return nil, err
	}
	inst, err := instance(in.InstanceId, "DisassociatePhoneNumberContactFlowInput")
	if err != nil {
		return nil, err
	}
	phone, err := phoneNumber(in.PhoneNumberId, inst, "DisassociatePhoneNumberContactFlowInput")
	if err != nil {
		return nil, err
	}
	for _, identifier := range f.associations(phone) {
		f.write(identifier, nil)
	}
	return &connect.DisassociatePhoneNumberContactFlowOutput{}, nil
}

func (f *fakeAPI) ListFlowAssociations(ctx context.Context, in *connect.ListFlowAssociationsInput, _ ...func(*connect.Options)) (*connect.ListFlowAssociationsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("ListFlowAssociations"); err != nil {
		return nil, err
	}
	inst, err := instance(in.InstanceId, "ListFlowAssociationsInput")
	if err != nil {
		return nil, err
	}
	if in.MaxResults != nil && (*in.MaxResults < 1 || *in.MaxResults > 1000) {
		return nil, invalid("MaxResults")
	}
	// Only voice phone numbers have associations in the fake, so a list of another type is empty.
	if in.ResourceType != "" && in.ResourceType != types.ListFlowAssociationResourceTypeVoicePhoneNumber {
		return &connect.ListFlowAssociationsOutput{}, nil
	}
	// An association removed moments ago is still listed.
	identifiers := f.store.List(associationType)
	for identifier, s := range f.stale {
		if s.before != nil && !slices.Contains(identifiers, identifier) {
			identifiers = append(identifiers, identifier)
		}
	}
	slices.Sort(identifiers)
	summaries := []types.FlowAssociationSummary{}
	for _, identifier := range identifiers {
		props, ok := f.read(identifier)
		if !ok || lastSegment(fmt.Sprint(props[instanceIDProperty])) != inst {
			continue
		}
		summaries = append(summaries, types.FlowAssociationSummary{
			ResourceId:   aws.String(phoneARN(lastSegment(fmt.Sprint(props[phoneNumberIDProperty])))),
			FlowId:       aws.String(fmt.Sprint(props[contactFlowIDProperty])),
			ResourceType: types.ListFlowAssociationResourceTypeVoicePhoneNumber,
		})
	}
	start := 0
	if in.NextToken != nil {
		if start, err = strconv.Atoi(*in.NextToken); err != nil || start > len(summaries) {
			return nil, invalid("NextToken")
		}
	}
	out := &connect.ListFlowAssociationsOutput{FlowAssociationSummaryList: summaries[start:min(start+pageSize, len(summaries))]}
	if start+pageSize < len(summaries) {
		out.NextToken = aws.String(strconv.Itoa(start + pageSize))
	}
	return out, nil
}
