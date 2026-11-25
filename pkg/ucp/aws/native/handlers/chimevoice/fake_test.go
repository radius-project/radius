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

package chimevoice

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/chimesdkvoice"
	"github.com/aws/aws-sdk-go-v2/service/chimesdkvoice/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	// outsideType is the pseudo type the store keeps, per Voice Connector, the credentials added outside Radius under.
	outsideType = "outside::" + credentialsType

	// missingPrefix starts the IDs of Voice Connectors AWS doesn't have, and noTerminationPrefix the IDs of Voice
	// Connectors without termination settings.
	missingPrefix       = "missing"
	noTerminationPrefix = "notermination"

	badRequestCode = "BadRequestException"

	// validationCode is the error AWS returns for a value outside the limits of its API model. The SDK doesn't model
	// it, so its fault is unknown.
	validationCode = "ValidationException"

	// maxCredentials is how many credentials a Voice Connector can have.
	maxCredentials = 10

	// staleReads is how many lists after a Put or a delete still return the earlier usernames.
	staleReads = 2
)

// voiceConnectorIDPattern is the form of a Voice Connector ID AWS takes, from its ValidationException.
var voiceConnectorIDPattern = regexp.MustCompile(`^([a-z0-9]{21,22}|[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`)

// stale is what lists of a Voice Connector's usernames return for a while after a Put or a delete changed them:
// before, for reads more lists, while AWS still has after.
type stale struct {
	before []string
	after  []string
	reads  int
}

// fakeAPI is an in-memory Amazon Chime SDK Voice API for the credentials of Voice Connectors. The store keeps one
// entry per identifier, holding the credentials that alias added, with their passwords, and one entry per Voice
// Connector under outsideType for credentials added outside Radius. A Voice Connector's credentials are those of all
// its entries. A Put sets the password of a username in the entry that has it, and adds a new username to the entry
// of the current identifier, which the tracked handler sets. A delete takes the username out of every entry. An entry
// with no credentials is removed.
//
// Every Voice Connector exists, except those whose ID starts with missingPrefix, and has termination settings, except
// those whose ID starts with noTerminationPrefix. As AWS does, a list right after a Put or a delete returns the
// earlier usernames, for staleReads lists.
type fakeAPI struct {
	store  *awsfake.CloudControl
	failer nativetest.Failer

	mu      sync.Mutex
	current string
	calls   []string
	stale   map[string]*stale

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{store: store, stale: map[string]*stale{}, errs: map[string]error{}}
}

func (f *fakeAPI) setCurrent(identifier string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.current = identifier
}

// apiError returns the error AWS returns with HTTP status and code, a client fault for a 4xx status and a server
// fault otherwise.
func apiError(status int, code string, message string) error {
	fault := smithy.FaultClient
	if status >= http.StatusInternalServerError {
		fault = smithy.FaultServer
	}
	return &smithy.OperationError{
		ServiceID:     "Chime SDK Voice",
		OperationName: "Call",
		Err: &smithyhttp.ResponseError{
			Response: &smithyhttp.Response{Response: &http.Response{StatusCode: status}},
			Err:      &smithy.GenericAPIError{Code: code, Message: message, Fault: fault},
		},
	}
}

func badRequest(format string, args ...any) error {
	return apiError(http.StatusBadRequest, badRequestCode, fmt.Sprintf(format, args...))
}

// invalid returns AWS's ValidationException. The SDK doesn't model it, so in production its fault is unknown, which
// testdata/errors.json covers. The fake marks it a client fault, as the suite's API rules require.
func invalid(format string, args ...any) error {
	return apiError(http.StatusBadRequest, validationCode, fmt.Sprintf(format, args...))
}

func notFoundError(format string, args ...any) error {
	return apiError(http.StatusNotFound, notFoundCode, fmt.Sprintf(format, args...))
}

// missing returns the error the SDK returns, before it sends anything, for a call without a required parameter.
func missing(operation string, parameter string) error {
	err := smithy.InvalidParamsError{Context: operation}
	err.Add(smithy.NewErrParamRequired(parameter))
	return err
}

// voiceConnector returns the error AWS returns for a call on the Voice Connector with id, or nil when AWS has it.
func voiceConnector(id *string, operation string) error {
	switch {
	case id == nil:
		return missing(operation, "VoiceConnectorId")
	case !voiceConnectorIDPattern.MatchString(*id):
		return invalid("1 validation error detected: Value '%s' at 'voiceConnectorId' failed to satisfy constraint", *id)
	case strings.HasPrefix(*id, missingPrefix):
		return notFoundError("Could not find VoiceConnector for %s", *id)
	}
	return nil
}

// entry is an entry of the store.
type entry struct {
	typeName   string
	identifier string
	properties map[string]any
}

// entries returns the store's entries of the Voice Connector.
func (f *fakeAPI) entries(vc string) []entry {
	out := []entry{}
	for _, id := range f.store.List(credentialsType) {
		if props := f.store.Resource(credentialsType, id); awsapi.String(props, voiceConnectorIDProperty) == vc {
			out = append(out, entry{credentialsType, id, props})
		}
	}
	if props := f.store.Resource(outsideType, vc); props != nil {
		out = append(out, entry{outsideType, vc, props})
	}
	return out
}

// items returns the credentials of properties.
func items(properties map[string]any) []map[string]any {
	out := []map[string]any{}
	list, _ := properties[credentialsProperty].([]any)
	for _, item := range list {
		if object, ok := item.(map[string]any); ok {
			out = append(out, object)
		}
	}
	return out
}

// usernames returns the Voice Connector's usernames, sorted, as AWS lists them.
func (f *fakeAPI) usernames(vc string) []string {
	out := []string{}
	for _, e := range f.entries(vc) {
		for _, item := range items(e.properties) {
			if name, _ := item[usernameField].(string); !slices.Contains(out, name) {
				out = append(out, name)
			}
		}
	}
	slices.Sort(out)
	return out
}

// password returns the password AWS has for the username of the Voice Connector, or "" when it has none.
func (f *fakeAPI) password(vc string, username string) string {
	for _, e := range f.entries(vc) {
		for _, item := range items(e.properties) {
			if item[usernameField] == username {
				password, _ := item[passwordField].(string)
				return password
			}
		}
	}
	return ""
}

func (f *fakeAPI) put(e entry, list []map[string]any) {
	if len(list) == 0 {
		f.store.Remove(e.typeName, e.identifier)
		return
	}
	out := []any{}
	for _, item := range list {
		out = append(out, item)
	}
	e.properties[credentialsProperty] = out
	f.store.Put(e.typeName, e.identifier, e.properties)
}

// change makes a call that changes the Voice Connector's credentials, failing it as the failer says. Lists return the
// earlier usernames for staleReads lists.
func (f *fakeAPI) change(vc string, apply func()) error {
	if err := f.failer.Before(); err != nil {
		return err
	}
	before := f.usernames(vc)
	apply()
	f.stale[vc] = &stale{before: before, after: f.usernames(vc), reads: staleReads}
	return f.failer.After()
}

// begin records a call and returns the error errs sets for it.
func (f *fakeAPI) begin(method string) error {
	f.calls = append(f.calls, method)
	return f.errs[method]
}

func (f *fakeAPI) ListVoiceConnectorTerminationCredentials(ctx context.Context, in *chimesdkvoice.ListVoiceConnectorTerminationCredentialsInput, _ ...func(*chimesdkvoice.Options)) (*chimesdkvoice.ListVoiceConnectorTerminationCredentialsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("ListVoiceConnectorTerminationCredentials"); err != nil {
		return nil, err
	}
	if err := voiceConnector(in.VoiceConnectorId, "ListVoiceConnectorTerminationCredentialsInput"); err != nil {
		return nil, err
	}
	vc := *in.VoiceConnectorId
	names := f.usernames(vc)
	if s := f.stale[vc]; s != nil {
		if s.reads > 0 && slices.Equal(names, s.after) {
			s.reads--
			return &chimesdkvoice.ListVoiceConnectorTerminationCredentialsOutput{Usernames: slices.Clone(s.before)}, nil
		}
		delete(f.stale, vc)
	}
	return &chimesdkvoice.ListVoiceConnectorTerminationCredentialsOutput{Usernames: names}, nil
}

func (f *fakeAPI) PutVoiceConnectorTerminationCredentials(ctx context.Context, in *chimesdkvoice.PutVoiceConnectorTerminationCredentialsInput, _ ...func(*chimesdkvoice.Options)) (*chimesdkvoice.PutVoiceConnectorTerminationCredentialsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("PutVoiceConnectorTerminationCredentials"); err != nil {
		return nil, err
	}
	if err := voiceConnector(in.VoiceConnectorId, "PutVoiceConnectorTerminationCredentialsInput"); err != nil {
		return nil, err
	}
	vc := *in.VoiceConnectorId
	switch {
	case in.Credentials == nil:
		return nil, badRequest("1 validation error detected: Value null at 'credentials' failed to satisfy constraint: Member must not be null")
	case len(in.Credentials) == 0:
		return nil, badRequest("Given credential list is empty, voiceConnectorId: %s", vc)
	case slices.ContainsFunc(in.Credentials, func(c types.Credential) bool { return aws.ToString(c.Username) == "" || aws.ToString(c.Password) == "" }):
		return nil, badRequest("Credential item should have both username & password for voiceConnectorId: %s", vc)
	case strings.HasPrefix(vc, noTerminationPrefix):
		return nil, badRequest("Could not put termination credentials for non-existing termination. Request voice connector id is '%s'", vc)
	}
	present := f.usernames(vc)
	added := 0
	for _, c := range in.Credentials {
		if !slices.Contains(present, aws.ToString(c.Username)) {
			added++
		}
	}
	if len(present)+added > maxCredentials {
		return nil, badRequest("Exceeded allowable TerminationCredential limits: %d; Current total numbers of TerminationCredential are: %d; Numbers of new TerminationCredential to add are: %d;", maxCredentials, len(present), added)
	}
	return &chimesdkvoice.PutVoiceConnectorTerminationCredentialsOutput{}, f.change(vc, func() {
		for _, c := range in.Credentials {
			f.set(vc, aws.ToString(c.Username), aws.ToString(c.Password))
		}
	})
}

// set sets the password of the username in the entry of the Voice Connector that has it, or adds the credential to
// the entry of the current identifier.
func (f *fakeAPI) set(vc string, username string, password string) {
	for _, e := range f.entries(vc) {
		list := items(e.properties)
		if i := slices.IndexFunc(list, func(item map[string]any) bool { return item[usernameField] == username }); i >= 0 {
			list[i] = map[string]any{usernameField: username, passwordField: password}
			f.put(e, list)
			return
		}
	}
	e := entry{outsideType, vc, nil}
	if f.current != "" {
		e = entry{credentialsType, f.current, nil}
	}
	e.properties = f.store.Resource(e.typeName, e.identifier)
	if e.properties == nil {
		e.properties = map[string]any{voiceConnectorIDProperty: vc}
		if e.typeName == credentialsType {
			e.properties["Id"] = e.identifier
		}
	}
	f.put(e, append(items(e.properties), map[string]any{usernameField: username, passwordField: password}))
}

func (f *fakeAPI) DeleteVoiceConnectorTerminationCredentials(ctx context.Context, in *chimesdkvoice.DeleteVoiceConnectorTerminationCredentialsInput, _ ...func(*chimesdkvoice.Options)) (*chimesdkvoice.DeleteVoiceConnectorTerminationCredentialsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DeleteVoiceConnectorTerminationCredentials"); err != nil {
		return nil, err
	}
	if in.Usernames == nil {
		return nil, missing("DeleteVoiceConnectorTerminationCredentialsInput", "Usernames")
	}
	if err := voiceConnector(in.VoiceConnectorId, "DeleteVoiceConnectorTerminationCredentialsInput"); err != nil {
		return nil, err
	}
	vc := *in.VoiceConnectorId
	if len(in.Usernames) == 0 {
		return nil, badRequest("Given credential list is empty, voiceConnectorId: %s", vc)
	}
	return &chimesdkvoice.DeleteVoiceConnectorTerminationCredentialsOutput{}, f.change(vc, func() {
		for _, e := range f.entries(vc) {
			f.put(e, slices.DeleteFunc(items(e.properties), func(item map[string]any) bool {
				name, _ := item[usernameField].(string)
				return slices.Contains(in.Usernames, name)
			}))
		}
	})
}

// tracked is the handler with its fake API, which it tells the identifier of each call, so a Put adds a username to
// that identifier's entry.
type tracked struct {
	*TerminationCredentials
	api *fakeAPI
}

func newTracked(store *awsfake.CloudControl) *tracked {
	api := newFakeAPI(store)
	h := NewTerminationCredentials(api)
	h.wait = noWait
	return &tracked{TerminationCredentials: h, api: api}
}

// noWait is a pause that doesn't wait.
func noWait(context.Context, time.Duration) {}

func (t *tracked) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	t.api.setCurrent(awsapi.String(request.Properties, voiceConnectorIDProperty) + "|" + native.GenerateName(request.ClientToken))
	return t.TerminationCredentials.Create(ctx, request)
}

func (t *tracked) Update(ctx context.Context, request native.UpdateRequest) error {
	t.api.setCurrent(request.Identifier)
	return t.TerminationCredentials.Update(ctx, request)
}

func (t *tracked) ReadScoped(ctx context.Context, region string, identifier string, managed []map[string]any) (map[string]any, error) {
	t.api.setCurrent(identifier)
	return t.TerminationCredentials.ReadScoped(ctx, region, identifier, managed)
}

func (t *tracked) DeleteScoped(ctx context.Context, region string, identifier string, managed []map[string]any) error {
	t.api.setCurrent(identifier)
	return t.TerminationCredentials.DeleteScoped(ctx, region, identifier, managed)
}
