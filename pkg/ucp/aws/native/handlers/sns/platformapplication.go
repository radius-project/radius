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

// Package sns has the native handler for AWS::SNS::PlatformApplication, an Amazon SNS platform application, which
// has no CloudFormation type.
package sns

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	platformApplicationType = "AWS::SNS::PlatformApplication"

	nameProperty     = "Name"
	platformProperty = "Platform"
	arnProperty      = "PlatformApplicationArn"

	// sampleRateAttribute is the one setting AWS can't clear: it takes only an integer from 0 to 100.
	sampleRateAttribute = "SuccessFeedbackSampleRate"

	// notFoundCode is the error Amazon SNS returns for a platform application it doesn't have.
	notFoundCode = "NotFound"

	// confirmReads is how many times a create or update reads the application before it stops waiting for a read to
	// show the change, and confirmPause is how long it waits between reads.
	confirmReads = 20
	confirmPause = 500 * time.Millisecond

	// timeout is how long UCP waits for any operation. AWS showed every change at once in live calls.
	timeout = 10 * time.Minute
)

var (
	// credentialAttributes are the push credentials and the values AWS checks them with. AWS takes a change to any of
	// them only with the others the platform needs, so an update sends them all. PlatformPrincipal and
	// PlatformCredential are secrets, which AWS never returns.
	credentialAttributes = []string{"PlatformPrincipal", "PlatformCredential", "ApplePlatformTeamID", "ApplePlatformBundleID"}

	// settingAttributes are the other attributes a template sets. AWS clears each one sent as "", except
	// SuccessFeedbackSampleRate.
	settingAttributes = []string{
		"EventEndpointCreated", "EventEndpointDeleted", "EventEndpointUpdated", "EventDeliveryFailure",
		"SuccessFeedbackRoleArn", "FailureFeedbackRoleArn", sampleRateAttribute,
	}

	// readableAttributes are the attributes Read returns. Only these are read, so a secret AWS might add to the
	// response never reaches UCP.
	readableAttributes = append([]string{
		"ApplePlatformTeamID", "ApplePlatformBundleID", "Enabled", "AuthenticationMethod", "AppleCertificateExpirationDate",
	}, settingAttributes...)

	// namePattern and platformPattern are the forms of the identifier's parts. The name's is AWS's, and both keep an
	// identifier from naming anything but one application's ARN.
	namePattern     = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,256}$`)
	platformPattern = regexp.MustCompile(`^[A-Z_]+$`)
)

// API is the part of the Amazon SNS client the handler uses.
type API interface {
	CreatePlatformApplication(ctx context.Context, params *sns.CreatePlatformApplicationInput, optFns ...func(*sns.Options)) (*sns.CreatePlatformApplicationOutput, error)
	SetPlatformApplicationAttributes(ctx context.Context, params *sns.SetPlatformApplicationAttributesInput, optFns ...func(*sns.Options)) (*sns.SetPlatformApplicationAttributesOutput, error)
	DeletePlatformApplication(ctx context.Context, params *sns.DeletePlatformApplicationInput, optFns ...func(*sns.Options)) (*sns.DeletePlatformApplicationOutput, error)
	ListPlatformApplications(ctx context.Context, params *sns.ListPlatformApplicationsInput, optFns ...func(*sns.Options)) (*sns.ListPlatformApplicationsOutput, error)
}

var inRegion = awsapi.InRegion[sns.Options]

// pause waits for d, or until ctx ends. Tests replace it, so they don't wait.
type pause func(ctx context.Context, d time.Duration)

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

var (
	_ native.Handler           = (*PlatformApplication)(nil)
	_ native.IdentifierEncoder = (*PlatformApplication)(nil)
)

// PlatformApplication serves AWS::SNS::PlatformApplication, the push credentials Amazon SNS uses to send to an app's
// devices through a push notification service such as APNs or Firebase Cloud Messaging.
//
// Its identifier is the platform and the name, joined with "|", such as GCM|my-app. Both are known before the create,
// and AWS keeps them unique within a region, so UCP finds a create it lost by reading the application, and two aliases
// can't hold one application. Sending the create again can't find it: AWS returns the first application only for a
// create with the same credentials, which UCP doesn't have before a delete. The application's ARN, which AWS's calls
// take, is a read-only property.
//
// The handler reads an application with ListPlatformApplications, which also gives its ARN. In live calls
// GetPlatformApplicationAttributes returned the attributes from before a change for about 20 seconds, while
// ListPlatformApplications returned the change at once.
//
// AWS checks the credentials with the push notification service on every create and update, and never returns them,
// so the schema marks them write-only. A create or update waits, for up to confirmReads reads, until a read shows it,
// and fails with an unknown outcome when none does.
type PlatformApplication struct {
	api  API
	wait pause
}

// NewPlatformApplication returns the handler for Amazon SNS platform applications.
func NewPlatformApplication(api API) *PlatformApplication {
	return &PlatformApplication{api: api, wait: sleep}
}

// TypeName returns AWS::SNS::PlatformApplication.
func (h *PlatformApplication) TypeName() string {
	return platformApplicationType
}

// Options returns how UCP uses the handler.
func (h *PlatformApplication) Options() native.Options {
	return native.Options{
		List:          native.ListRegion,
		Recovery:      native.RecoverByIdentifier,
		NameProperty:  nameProperty,
		CreateTimeout: timeout,
		UpdateTimeout: timeout,
		DeleteTimeout: timeout,
	}
}

// rejected returns a client failure AWS applied none of, with message.
func rejected(format string, args ...any) error {
	return &native.Failure{Message: fmt.Sprintf(format, args...), ClientError: true, Outcome: native.OutcomeRejected}
}

// notFoundOrRejected returns err as ErrNotFound for an application AWS doesn't have, and otherwise as awsapi.Rejected
// does.
func notFoundOrRejected(err error) error {
	return awsapi.Rejected(awsapi.NotFound(err, notFoundCode))
}

// parseIdentifier returns the platform and the name an identifier gives.
func parseIdentifier(identifier string) (platform string, name string, err error) {
	platform, name, ok := strings.Cut(identifier, "|")
	if !ok || !platformPattern.MatchString(platform) || !namePattern.MatchString(name) {
		return "", "", rejected("the identifier of an %s is its Platform and Name joined with |, such as GCM|my-app", platformApplicationType)
	}
	return platform, name, nil
}

// find returns the ARN and the attributes of the application with the identifier, or ErrNotFound. It returns any other
// error as it is, never as a rejected failure, because Progress reads with it after AWS applied an operation.
func (h *PlatformApplication) find(ctx context.Context, region string, identifier string) (string, map[string]string, error) {
	platform, name, err := parseIdentifier(identifier)
	if err != nil {
		return "", nil, err
	}
	pages := sns.NewListPlatformApplicationsPaginator(h.api, &sns.ListPlatformApplicationsInput{}, func(o *sns.ListPlatformApplicationsPaginatorOptions) {
		o.StopOnDuplicateToken = true
	})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx, inRegion(region))
		if err != nil {
			return "", nil, err
		}
		for _, app := range page.PlatformApplications {
			arn := aws.ToString(app.PlatformApplicationArn)
			if p, n, ok := platformAndName(arn); ok && p == platform && n == name {
				return arn, app.Attributes, nil
			}
		}
	}
	return "", nil, native.ErrNotFound
}

// properties returns the properties of the application with arn and attributes. AWS returns some attributes it
// cleared as "", which are left out, and so is every attribute the type doesn't have.
func properties(platform string, name string, arn string, attributes map[string]string) map[string]any {
	out := map[string]any{platformProperty: platform, nameProperty: name, arnProperty: arn}
	for _, attribute := range readableAttributes {
		if value := attributes[attribute]; value != "" {
			out[attribute] = value
		}
	}
	return out
}

// attributes returns the string properties of properties named names that aren't "".
func attributes(properties map[string]any, names ...string) map[string]string {
	out := map[string]string{}
	for _, name := range names {
		if value := awsapi.String(properties, name); value != "" {
			out[name] = value
		}
	}
	return out
}

// Identifier returns the identifier the properties give, their Platform and Name joined with "|", or "" when they
// don't give both. A platform or name AWS can't take is a client failure.
func (h *PlatformApplication) Identifier(properties map[string]any) (string, error) {
	platform, name := awsapi.String(properties, platformProperty), awsapi.String(properties, nameProperty)
	if platform == "" || name == "" {
		return "", nil
	}
	identifier := platform + "|" + name
	if _, _, err := parseIdentifier(identifier); err != nil {
		return "", err
	}
	return identifier, nil
}

// Read returns the application with the identifier, or ErrNotFound.
func (h *PlatformApplication) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	arn, attributes, err := h.find(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	platform, name, _ := parseIdentifier(identifier)
	return properties(platform, name, arn, attributes), nil
}

// Create creates the application with the template's attributes and returns its identifier. AWS checks the
// credentials with the push notification service before it creates anything.
func (h *PlatformApplication) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	identifier, err := h.Identifier(request.Properties)
	if err != nil {
		return "", err
	}
	platform, name, err := parseIdentifier(identifier)
	if err != nil {
		return "", err
	}
	sent := attributes(request.Properties, append(slices.Clone(credentialAttributes), settingAttributes...)...)
	_, err = h.api.CreatePlatformApplication(ctx, &sns.CreatePlatformApplicationInput{
		Name:       aws.String(name),
		Platform:   aws.String(platform),
		Attributes: sent,
	}, inRegion(request.Region))
	if err != nil {
		return "", awsapi.Rejected(err)
	}
	return identifier, h.confirm(ctx, request.Region, identifier, sent)
}

// Update sets the attributes the update changes. A setting the template no longer sets is cleared, which AWS does for
// an attribute sent as "", except SuccessFeedbackSampleRate, which AWS can't clear, so removing it is rejected before
// any call. AWS takes a change to the credentials only with every credential the platform needs, so a change to any of
// them sends all the template sets, including the secrets whose fingerprints show they are unchanged. An attribute the
// template sets to "" that AWS doesn't have is no change: Read leaves out an empty attribute, so every deploy of such a
// template has it in its patch.
func (h *PlatformApplication) Update(ctx context.Context, request native.UpdateRequest) error {
	changed := slices.DeleteFunc(awsapi.Changed(request), func(attribute string) bool {
		return awsapi.String(request.Desired, attribute) == "" && awsapi.String(request.Current, attribute) == ""
	})
	sent := map[string]string{}
	for _, attribute := range settingAttributes {
		if !slices.Contains(changed, attribute) {
			continue
		}
		value := awsapi.String(request.Desired, attribute)
		if value == "" && attribute == sampleRateAttribute {
			return rejected("AWS can't clear %s once it is set. Set it to a value from 0 to 100 instead.", sampleRateAttribute)
		}
		sent[attribute] = value
	}
	if slices.ContainsFunc(credentialAttributes, func(attribute string) bool { return slices.Contains(changed, attribute) }) {
		// The patch leaves out secrets whose fingerprints are unchanged, so they come from the template.
		maps.Copy(sent, attributes(request.Desired, credentialAttributes...))
	}
	if len(sent) == 0 {
		return nil
	}
	arn, _, err := h.find(ctx, request.Region, request.Identifier)
	if err != nil {
		return err
	}
	_, err = h.api.SetPlatformApplicationAttributes(ctx, &sns.SetPlatformApplicationAttributesInput{
		PlatformApplicationArn: aws.String(arn),
		Attributes:             sent,
	}, inRegion(request.Region))
	if err != nil {
		return notFoundOrRejected(err)
	}
	return h.confirm(ctx, request.Region, request.Identifier, sent)
}

// confirm reads the application until a read shows each attribute in sent that AWS returns, for up to confirmReads
// reads. New credentials replace the team and bundle IDs, so then a read must show only those sent. AWS accepted the
// change, but until a read shows it, a later deploy could read the earlier attributes and keep a value the template
// removed, and the worker could take a create that no read shows yet as gone. So when no read shows it, confirm returns
// a failure whose outcome is unknown, which keeps the template's values as possibly applied, and the next deploy reads
// the application again.
func (h *PlatformApplication) confirm(ctx context.Context, region string, identifier string, sent map[string]string) error {
	want := map[string]string{}
	for attribute, value := range sent {
		if slices.Contains(readableAttributes, attribute) {
			want[attribute] = value
		}
	}
	if _, ok := sent["PlatformCredential"]; ok {
		for _, attribute := range []string{"ApplePlatformTeamID", "ApplePlatformBundleID"} {
			want[attribute] = sent[attribute]
		}
	}
	var err error
	for i := range confirmReads {
		if i > 0 {
			h.wait(ctx, confirmPause)
		}
		if ctx.Err() != nil {
			err = ctx.Err()
			break
		}
		var got map[string]any
		if got, err = h.Read(ctx, region, identifier); err == nil && shows(got, want) {
			return nil
		}
	}
	message := fmt.Sprintf("AWS accepted the change to the Amazon SNS platform application %q, but no read showed it within %s, so UCP can't tell what AWS has. Deploy again.", identifier, confirmReads*confirmPause)
	if err != nil {
		message += " The last read failed: " + err.Error()
	}
	return &native.Failure{Message: message, Outcome: native.OutcomeUnknown}
}

// shows reports whether the properties of a read have the attributes in want. AWS returns a cleared attribute as ""
// or not at all, which Read leaves out, so "" in want means the read leaves it out.
func shows(got map[string]any, want map[string]string) bool {
	for attribute, value := range want {
		if read, _ := got[attribute].(string); read != value {
			return false
		}
	}
	return true
}

// Delete deletes the application. It returns ErrNotFound when AWS has no such application.
func (h *PlatformApplication) Delete(ctx context.Context, region string, identifier string) error {
	arn, _, err := h.find(ctx, region, identifier)
	if err != nil {
		return err
	}
	_, err = h.api.DeletePlatformApplication(ctx, &sns.DeletePlatformApplicationInput{PlatformApplicationArn: aws.String(arn)}, inRegion(region))
	return notFoundOrRejected(err)
}

// List returns every application in the region.
func (h *PlatformApplication) List(ctx context.Context, region string) ([]native.Resource, error) {
	out := []native.Resource{}
	pages := sns.NewListPlatformApplicationsPaginator(h.api, &sns.ListPlatformApplicationsInput{}, func(o *sns.ListPlatformApplicationsPaginatorOptions) {
		o.StopOnDuplicateToken = true
	})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx, inRegion(region))
		if err != nil {
			return nil, awsapi.Rejected(err)
		}
		for _, app := range page.PlatformApplications {
			arn := aws.ToString(app.PlatformApplicationArn)
			platform, name, ok := platformAndName(arn)
			if !ok {
				continue
			}
			out = append(out, native.Resource{Identifier: platform + "|" + name, Properties: properties(platform, name, arn, app.Attributes)})
		}
	}
	return out, nil
}

// platformAndName returns the platform and the name an application's ARN gives, such as GCM and my-app for
// arn:aws:sns:us-west-2:123456789012:app/GCM/my-app.
func platformAndName(arn string) (string, string, bool) {
	parts := strings.Split(arn[strings.LastIndex(arn, ":")+1:], "/")
	if len(parts) != 3 || parts[0] != "app" || !platformPattern.MatchString(parts[1]) || !namePattern.MatchString(parts[2]) {
		return "", "", false
	}
	return parts[1], parts[2], true
}

// Progress reports a create as done once a read finds the application, and an update as done, because the update
// already waited until a read showed it. A create whose response UCP lost never waited, so a read that doesn't find
// the application gets up to confirmReads tries before Progress returns ErrNotFound, which makes UCP send the create
// again, or a DELETE remove only the record. A delete is done once a read no longer finds the application.
func (h *PlatformApplication) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	switch op {
	case native.OperationUpdate:
		return native.Progress{Done: true}, nil
	case native.OperationDelete:
		if _, err := h.Read(ctx, region, identifier); err != nil {
			return native.Progress{}, err
		}
		return native.Progress{State: "present", Waiting: "deleted"}, nil
	}
	var err error
	for i := range confirmReads {
		if i > 0 {
			h.wait(ctx, confirmPause)
		}
		if ctx.Err() != nil {
			return native.Progress{}, ctx.Err()
		}
		if _, err = h.Read(ctx, region, identifier); !errors.Is(err, native.ErrNotFound) {
			break
		}
	}
	if err != nil {
		return native.Progress{}, err
	}
	return native.Progress{Done: true}, nil
}
