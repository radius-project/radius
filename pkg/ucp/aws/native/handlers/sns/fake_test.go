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

package sns

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
)

const (
	invalidParameterCode = "InvalidParameter"

	// account is the fake's account, and invalidCredential a credential the push notification services reject.
	account           = "123456789012"
	invalidCredential = "invalid-credential"

	// staleReads is how many lists after a change still return the application as it was. In live calls a change
	// showed after up to 9 lists half a second apart.
	staleReads = 10

	// pageSize is how many applications a page of ListPlatformApplications has.
	pageSize = 2

	// certificateExpiration is the expiry the fake reports for every APNs certificate.
	certificateExpiration = "2030-01-01T00:00:00Z"
)

// platforms are the platforms AWS takes: those CreatePlatformApplication's Platform names, and BAIDU, MPNS, and WNS,
// which the page's credential list names.
var platforms = []string{"ADM", "APNS", "APNS_SANDBOX", "GCM", "BAIDU", "MPNS", "WNS"}

// allAttributes are the attributes a create or update can send.
var allAttributes = append(slices.Clone(credentialAttributes), append(slices.Clone(settingAttributes), "Enabled")...)

// secretAttributes are the attributes AWS never returns.
var secretAttributes = []string{"PlatformPrincipal", "PlatformCredential"}

// stale is what reads of an application return for a while after a change: before, or nil when it didn't exist, for
// reads more reads, while AWS still has after, or nil when it no longer exists.
type stale struct {
	before map[string]any
	after  map[string]any
	reads  int
}

// fakeAPI is an in-memory Amazon SNS API for platform applications. It keeps them in an awsfake store, in the type's
// property format with the secrets AWS keeps, so the suite's Cloud Control sees the same resources.
//
// The docs don't say when a change shows, so, as AWS did in live calls, a list right after a create, an update, or a
// delete returns the application as it was, for staleReads lists.
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
		ServiceID:     "SNS",
		OperationName: "Call",
		Err: &smithyhttp.ResponseError{
			Response: &smithyhttp.Response{Response: &http.Response{StatusCode: status}},
			Err:      &smithy.GenericAPIError{Code: code, Message: message, Fault: smithy.FaultClient},
		},
	}
}

func invalid(format string, args ...any) error {
	return apiError(http.StatusBadRequest, invalidParameterCode, "Invalid parameter: "+fmt.Sprintf(format, args...))
}

func notFound() error {
	return apiError(http.StatusNotFound, notFoundCode, "PlatformApplication does not exist")
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

// identifierOf returns the identifier of the application with arn, or an error when the ARN names none.
func identifierOf(arn *string) (string, error) {
	if arn == nil {
		return "", missing("PlatformApplicationArn", "PlatformApplicationArn")
	}
	platform, name, ok := platformAndName(*arn)
	if !ok || !strings.HasPrefix(*arn, "arn:aws:sns:") {
		return "", invalid("PlatformApplicationArn Reason: Wrong number of slashes in relative portion of the ARN.")
	}
	return platform + "|" + name, nil
}

// read returns the application with identifier as a read sees it, and whether there is one.
func (f *fakeAPI) read(identifier string) (map[string]any, bool) {
	current, ok := f.store.Read(platformApplicationType, identifier)
	if s := f.stale[identifier]; s != nil {
		if s.reads > 0 && reflect.DeepEqual(current, s.after) {
			s.reads--
			return s.before, s.before != nil
		}
		delete(f.stale, identifier)
	}
	return current, ok
}

// write sets the application with identifier to props, or deletes it when props is nil. Reads return it as it was
// for staleReads reads.
func (f *fakeAPI) write(identifier string, props map[string]any) {
	before, _ := f.store.Read(platformApplicationType, identifier)
	if props == nil {
		f.store.Remove(platformApplicationType, identifier)
	} else {
		f.store.Put(platformApplicationType, identifier, props)
	}
	after, _ := f.store.Read(platformApplicationType, identifier)
	f.stale[identifier] = &stale{before: before, after: after, reads: staleReads}
}

// attributesOf returns the attributes AWS returns for the application with props: every one it has, except the
// secrets. AWS returns a feedback role it cleared as "".
func attributesOf(props map[string]any) map[string]string {
	out := map[string]string{}
	for key, value := range props {
		if s, ok := value.(string); ok && !slices.Contains(secretAttributes, key) && key != platformProperty && key != nameProperty && key != arnProperty {
			out[key] = s
		}
	}
	return out
}

// checkAttributes returns the error AWS returns for attributes it doesn't take.
func checkAttributes(attributes map[string]string) error {
	for key, value := range attributes {
		switch {
		case !slices.Contains(allAttributes, key):
			return invalid("Attributes Reason: Invalid attribute name: %s", key)
		case key == sampleRateAttribute:
			if rate, err := strconv.Atoi(value); err != nil || rate < 0 || rate > 100 {
				return invalid("Attributes Reason: Invalid value for attribute: SuccessFeedbackSampleRate: %s value provided is not an integer between 0-100", value)
			}
		case strings.HasPrefix(key, "Event") && value != "" && !strings.HasPrefix(value, "arn:aws:sns:"):
			return invalid("Attributes Reason: Invalid Topic ARN: %s for event attribute: %s", value, key)
		case key == "Enabled" && value != "true" && value != "false":
			return invalid("Attributes Reason: Invalid value for attribute: Enabled")
		}
	}
	return nil
}

// checkCredentials returns the error AWS returns for the credentials of an application of platform, given every
// attribute it would have.
func checkCredentials(platform string, attributes map[string]string) error {
	credential, principal := attributes["PlatformCredential"], attributes["PlatformPrincipal"]
	team, bundle := attributes["ApplePlatformTeamID"], attributes["ApplePlatformBundleID"]
	apns := platform == "APNS" || platform == "APNS_SANDBOX"
	switch {
	case apns && (team != "" || bundle != "") && (credential == "" || principal == "" || team == "" || bundle == ""):
		return invalid("Attributes Reason: Missing one or more of the required attributes: PlatformPrincipal, PlatformCredential, ApplePlatformTeamID, ApplePlatformBundleID.")
	case credential == "" || (principal == "" && platform != "GCM"):
		return invalid("Attributes Reason: Missing one or more of the required attributes: PlatformPrincipal, PlatformCredential.")
	case credential == invalidCredential:
		return invalid("Attributes Reason: Platform credentials are invalid")
	}
	return nil
}

// authenticate sets the attributes AWS reports about the credentials of an application of platform in props.
func authenticate(platform string, props map[string]any) {
	delete(props, "AuthenticationMethod")
	delete(props, "AppleCertificateExpirationDate")
	switch platform {
	case "APNS", "APNS_SANDBOX":
		if props["ApplePlatformTeamID"] != nil {
			props["AuthenticationMethod"] = "Token"
			return
		}
		props["AuthenticationMethod"] = "Certificate"
		props["AppleCertificateExpirationDate"] = certificateExpiration
	case "GCM":
		props["AuthenticationMethod"] = "Key"
		if strings.HasPrefix(fmt.Sprint(props["PlatformCredential"]), "{") {
			props["AuthenticationMethod"] = "Token"
		}
	}
}

func (f *fakeAPI) CreatePlatformApplication(ctx context.Context, in *sns.CreatePlatformApplicationInput, _ ...func(*sns.Options)) (*sns.CreatePlatformApplicationOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("CreatePlatformApplication"); err != nil {
		return nil, err
	}
	switch {
	case in.Name == nil:
		return nil, missing("CreatePlatformApplicationInput", "Name")
	case in.Platform == nil:
		return nil, missing("CreatePlatformApplicationInput", "Platform")
	case in.Attributes == nil:
		return nil, missing("CreatePlatformApplicationInput", "Attributes")
	case !namePattern.MatchString(*in.Name):
		return nil, invalid("Name Reason: must contain only characters 'a'-'z', 'A'-'Z', '0'-'9', '_', '-', and '.'")
	case !slices.Contains(platforms, *in.Platform):
		return nil, invalid("Platform Reason: %s is not supported", *in.Platform)
	}
	if err := checkAttributes(in.Attributes); err != nil {
		return nil, err
	}
	if err := checkCredentials(*in.Platform, in.Attributes); err != nil {
		return nil, err
	}
	identifier := *in.Platform + "|" + *in.Name
	arn := fmt.Sprintf("arn:aws:sns:%s:%s:app/%s/%s", region, account, *in.Platform, *in.Name)
	props := map[string]any{platformProperty: *in.Platform, nameProperty: *in.Name, arnProperty: arn, "Enabled": "true"}
	for key, value := range in.Attributes {
		// AWS ignores Enabled in a create.
		if key != "Enabled" && value != "" {
			props[key] = value
		}
	}
	authenticate(*in.Platform, props)
	// A create of an application AWS has returns it when every attribute is the same, as AWS does.
	if existing, ok := f.store.Read(platformApplicationType, identifier); ok {
		if !reflect.DeepEqual(existing, props) {
			return nil, invalid("Name Reason: An application with the same name but different properties already exists")
		}
		return &sns.CreatePlatformApplicationOutput{PlatformApplicationArn: aws.String(arn)}, nil
	}
	f.write(identifier, props)
	return &sns.CreatePlatformApplicationOutput{PlatformApplicationArn: aws.String(arn)}, nil
}

func (f *fakeAPI) SetPlatformApplicationAttributes(ctx context.Context, in *sns.SetPlatformApplicationAttributesInput, _ ...func(*sns.Options)) (*sns.SetPlatformApplicationAttributesOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("SetPlatformApplicationAttributes"); err != nil {
		return nil, err
	}
	if in.Attributes == nil {
		return nil, missing("SetPlatformApplicationAttributesInput", "Attributes")
	}
	identifier, err := identifierOf(in.PlatformApplicationArn)
	if err != nil {
		return nil, err
	}
	props, ok := f.store.Read(platformApplicationType, identifier)
	if !ok {
		return nil, notFound()
	}
	if err := checkAttributes(in.Attributes); err != nil {
		return nil, err
	}
	platform := props[platformProperty].(string)
	_, credential := in.Attributes["PlatformCredential"]
	_, principal := in.Attributes["PlatformPrincipal"]
	_, team := in.Attributes["ApplePlatformTeamID"]
	_, bundle := in.Attributes["ApplePlatformBundleID"]
	switch {
	case credential && !principal && platform != "GCM":
		return nil, invalid("Attributes Reason: PlatformCredential attribute provided without PlatformPrincipal")
	case principal && !credential:
		return nil, invalid("Attributes Reason: PlatformPrincipal attribute provided without PlatformCredential")
	case (team || bundle) && !credential:
		return nil, invalid("Attributes Reason: Missing one or more of the required attributes: PlatformPrincipal, PlatformCredential, ApplePlatformTeamID, ApplePlatformBundleID.")
	}
	if credential {
		// New credentials replace the old ones, and the values AWS checks them with.
		sent := map[string]string{}
		for _, key := range credentialAttributes {
			delete(props, key)
			if value := in.Attributes[key]; value != "" {
				sent[key] = value
			}
		}
		if err := checkCredentials(platform, sent); err != nil {
			return nil, err
		}
	}
	for key, value := range in.Attributes {
		switch {
		case value != "":
			props[key] = value
		case strings.HasSuffix(key, "FeedbackRoleArn"):
			props[key] = ""
		default:
			delete(props, key)
		}
	}
	if credential {
		authenticate(platform, props)
	}
	f.write(identifier, props)
	return &sns.SetPlatformApplicationAttributesOutput{}, nil
}

func (f *fakeAPI) DeletePlatformApplication(ctx context.Context, in *sns.DeletePlatformApplicationInput, _ ...func(*sns.Options)) (*sns.DeletePlatformApplicationOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DeletePlatformApplication"); err != nil {
		return nil, err
	}
	identifier, err := identifierOf(in.PlatformApplicationArn)
	if err != nil {
		return nil, err
	}
	// AWS deletes an application it doesn't have without an error.
	if _, ok := f.store.Read(platformApplicationType, identifier); ok {
		f.write(identifier, nil)
	}
	return &sns.DeletePlatformApplicationOutput{}, nil
}

func (f *fakeAPI) ListPlatformApplications(ctx context.Context, in *sns.ListPlatformApplicationsInput, _ ...func(*sns.Options)) (*sns.ListPlatformApplicationsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("ListPlatformApplications"); err != nil {
		return nil, err
	}
	// An application deleted moments ago is still listed.
	identifiers := f.store.List(platformApplicationType)
	for identifier, s := range f.stale {
		if s.before != nil && !slices.Contains(identifiers, identifier) {
			identifiers = append(identifiers, identifier)
		}
	}
	slices.Sort(identifiers)
	start := 0
	if in.NextToken != nil {
		var err error
		if start, err = strconv.Atoi(*in.NextToken); err != nil || start > len(identifiers) {
			return nil, invalid("NextToken Reason: Invalid token")
		}
	}
	out := &sns.ListPlatformApplicationsOutput{}
	for _, identifier := range identifiers[start:min(start+pageSize, len(identifiers))] {
		props, ok := f.read(identifier)
		if !ok {
			continue
		}
		out.PlatformApplications = append(out.PlatformApplications, types.PlatformApplication{
			PlatformApplicationArn: aws.String(fmt.Sprint(props[arnProperty])),
			Attributes:             attributesOf(props),
		})
	}
	if start+pageSize < len(identifiers) {
		out.NextToken = aws.String(strconv.Itoa(start + pageSize))
	}
	return out, nil
}
