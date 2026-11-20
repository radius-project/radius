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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/redshift/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const profileType = "AWS::Redshift::AuthenticationProfile"

// ProfileAPI is the part of the Redshift client AuthenticationProfile uses.
type ProfileAPI interface {
	CreateAuthenticationProfile(ctx context.Context, params *redshift.CreateAuthenticationProfileInput, optFns ...func(*redshift.Options)) (*redshift.CreateAuthenticationProfileOutput, error)
	ModifyAuthenticationProfile(ctx context.Context, params *redshift.ModifyAuthenticationProfileInput, optFns ...func(*redshift.Options)) (*redshift.ModifyAuthenticationProfileOutput, error)
	DeleteAuthenticationProfile(ctx context.Context, params *redshift.DeleteAuthenticationProfileInput, optFns ...func(*redshift.Options)) (*redshift.DeleteAuthenticationProfileOutput, error)
	DescribeAuthenticationProfiles(ctx context.Context, params *redshift.DescribeAuthenticationProfilesInput, optFns ...func(*redshift.Options)) (*redshift.DescribeAuthenticationProfilesOutput, error)
}

var (
	_ native.Handler           = (*AuthenticationProfile)(nil)
	_ native.IdentifierEncoder = (*AuthenticationProfile)(nil)

	profileRegion = awsapi.InRegion[redshift.Options]

	// profileName is the form of a profile name, from the AWS API reference. Names are case sensitive.
	profileName = regexp.MustCompile(`^[a-zA-Z0-9-]{1,63}$`)
)

// profileOnce makes the SDK send a call once. A create whose response is lost may have made the profile, and the
// SDK's retry would get AuthenticationProfileAlreadyExistsFault, which reads as a create AWS rejected.
func profileOnce(o *redshift.Options) {
	o.RetryMaxAttempts = 1
}

// profileNotFound is the code of a profile AWS doesn't have.
const profileNotFound = "AuthenticationProfileNotFoundFault"

// AuthenticationProfile serves AWS::Redshift::AuthenticationProfile: connection options that JDBC and ODBC clients
// read by the profile's name, made with CreateAuthenticationProfile. Its identifier is AuthenticationProfileName.
// AWS stores AuthenticationProfileContent in its own form, so the handler compares it as JSON. AWS rejects content
// that holds secrets, such as SecretAccessKey or Password, so no property is write-only.
type AuthenticationProfile struct {
	api ProfileAPI
}

// NewAuthenticationProfile returns the handler for Redshift authentication profiles.
func NewAuthenticationProfile(api ProfileAPI) *AuthenticationProfile {
	return &AuthenticationProfile{api: api}
}

// TypeName returns AWS::Redshift::AuthenticationProfile.
func (h *AuthenticationProfile) TypeName() string {
	return profileType
}

// Options returns how UCP uses the handler.
func (h *AuthenticationProfile) Options() native.Options {
	return native.Options{
		List:         native.ListRegion,
		Recovery:     native.RecoverByIdentifier,
		NameProperty: "AuthenticationProfileName",
		Equivalent:   profileEquivalent,
	}
}

// invalidProfile is the failure of a template value the handler can't send. It names the property and what it must
// be, never the value.
func invalidProfile(name string, want string) error {
	return &native.Failure{
		Message:     fmt.Sprintf("%s of an %s must be %s", name, profileType, want),
		ClientError: true,
		Outcome:     native.OutcomeRejected,
	}
}

// Identifier checks the template's values, so UCP rejects one AWS wouldn't take before any AWS call, and returns
// AuthenticationProfileName.
func (h *AuthenticationProfile) Identifier(properties map[string]any) (string, error) {
	if value, ok := properties["AuthenticationProfileContent"]; ok && value != nil {
		content, ok := value.(string)
		if !ok {
			return "", invalidProfile("AuthenticationProfileContent", "a string")
		}
		if _, ok := profileContent(content); !ok {
			return "", invalidProfile("AuthenticationProfileContent", "a JSON object whose values are strings, numbers, or booleans")
		}
	}
	value, ok := properties["AuthenticationProfileName"]
	if !ok || value == nil {
		return "", nil
	}
	name, ok := value.(string)
	if !ok || !profileName.MatchString(name) {
		return "", invalidProfile("AuthenticationProfileName", "1 to 63 letters, digits, and hyphens")
	}
	return name, nil
}

// profileContent returns content as AWS stores it: each value as a string, with a number in the form the JSON has
// it, such as 1.50, and without null values. It returns false when content isn't a JSON object whose values are
// strings, numbers, booleans, or null, which AWS doesn't take.
func profileContent(content string) (map[string]string, bool) {
	decoder := json.NewDecoder(bytes.NewReader([]byte(content)))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil || object == nil {
		return nil, false
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, false
	}
	out := map[string]string{}
	for key, value := range object {
		switch v := value.(type) {
		case nil:
		case string:
			out[key] = v
		case json.Number:
			out[key] = v.String()
		case bool:
			out[key] = fmt.Sprint(v)
		default:
			return nil, false
		}
	}
	return out, true
}

// profileEquivalent reports whether AWS's AuthenticationProfileContent holds the same settings as the template's,
// in AWS's form: AWS removes the spaces, turns numbers and booleans into strings, and drops null values.
func profileEquivalent(path string, template any, stored any) bool {
	if path != "AuthenticationProfileContent" {
		return false
	}
	t, ok := template.(string)
	if !ok {
		return false
	}
	s, ok := stored.(string)
	if !ok {
		return false
	}
	want, ok := profileContent(t)
	if !ok {
		return false
	}
	got, ok := profileContent(s)
	return ok && maps.Equal(want, got)
}

// profileProperties returns the properties of a profile as AWS returns it.
func profileProperties(p *types.AuthenticationProfile) map[string]any {
	properties := map[string]any{"AuthenticationProfileName": aws.ToString(p.AuthenticationProfileName)}
	if p.AuthenticationProfileContent != nil {
		properties["AuthenticationProfileContent"] = *p.AuthenticationProfileContent
	}
	return properties
}

// get returns the profile with the name, or ErrNotFound.
func (h *AuthenticationProfile) get(ctx context.Context, region string, name string) (*types.AuthenticationProfile, error) {
	out, err := h.api.DescribeAuthenticationProfiles(ctx, &redshift.DescribeAuthenticationProfilesInput{AuthenticationProfileName: aws.String(name)}, profileRegion(region))
	if err != nil {
		return nil, awsapi.NotFound(err, profileNotFound)
	}
	for i := range out.AuthenticationProfiles {
		if aws.ToString(out.AuthenticationProfiles[i].AuthenticationProfileName) == name {
			return &out.AuthenticationProfiles[i], nil
		}
	}
	return nil, native.ErrNotFound
}

// Read returns the profile's properties.
func (h *AuthenticationProfile) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	profile, err := h.get(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	return profileProperties(profile), nil
}

// profileContentOf returns the template's AuthenticationProfileContent, which AWS requires.
func profileContentOf(properties map[string]any) (string, error) {
	content, ok := properties["AuthenticationProfileContent"].(string)
	if !ok {
		return "", invalidProfile("AuthenticationProfileContent", "set")
	}
	return content, nil
}

// Create creates the profile.
func (h *AuthenticationProfile) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	name, err := h.Identifier(request.Properties)
	if err != nil {
		return "", err
	}
	if name == "" {
		return "", invalidProfile("AuthenticationProfileName", "set")
	}
	content, err := profileContentOf(request.Properties)
	if err != nil {
		return "", err
	}
	_, err = h.api.CreateAuthenticationProfile(ctx, &redshift.CreateAuthenticationProfileInput{
		AuthenticationProfileName:    aws.String(name),
		AuthenticationProfileContent: aws.String(content),
	}, profileRegion(request.Region), profileOnce)
	if err != nil {
		return "", awsapi.Rejected(err)
	}
	return name, nil
}

// Update replaces the profile's content when the template changed it. ModifyAuthenticationProfile replaces the whole
// content, so the template's content is sent as it is.
func (h *AuthenticationProfile) Update(ctx context.Context, request native.UpdateRequest) error {
	if _, err := h.Identifier(request.Desired); err != nil {
		return err
	}
	if !slices.Contains(awsapi.Changed(request), "AuthenticationProfileContent") {
		return nil
	}
	content, err := profileContentOf(request.Desired)
	if err != nil {
		return err
	}
	_, err = h.api.ModifyAuthenticationProfile(ctx, &redshift.ModifyAuthenticationProfileInput{
		AuthenticationProfileName:    aws.String(request.Identifier),
		AuthenticationProfileContent: aws.String(content),
	}, profileRegion(request.Region))
	return awsapi.Rejected(awsapi.NotFound(err, profileNotFound))
}

// Delete deletes the profile.
func (h *AuthenticationProfile) Delete(ctx context.Context, region string, identifier string) error {
	_, err := h.api.DeleteAuthenticationProfile(ctx, &redshift.DeleteAuthenticationProfileInput{AuthenticationProfileName: aws.String(identifier)}, profileRegion(region))
	return awsapi.Rejected(awsapi.NotFound(err, profileNotFound))
}

// List returns every profile in the region.
func (h *AuthenticationProfile) List(ctx context.Context, region string) ([]native.Resource, error) {
	out, err := h.api.DescribeAuthenticationProfiles(ctx, &redshift.DescribeAuthenticationProfilesInput{}, profileRegion(region))
	if err != nil {
		return nil, err
	}
	resources := []native.Resource{}
	for i := range out.AuthenticationProfiles {
		p := &out.AuthenticationProfiles[i]
		resources = append(resources, native.Resource{Identifier: aws.ToString(p.AuthenticationProfileName), Properties: profileProperties(p)})
	}
	return resources, nil
}

// Progress reads the profile. A create or an update is done once AWS lists the profile, and a delete waits until AWS
// no longer does. AWS lists a new profile at once, so one it doesn't list is gone, and the next deploy creates it.
func (h *AuthenticationProfile) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	if _, err := h.get(ctx, region, identifier); err != nil {
		return native.Progress{}, err
	}
	if op == native.OperationDelete {
		return native.Progress{State: "listed", Waiting: "deleted"}, nil
	}
	return native.Progress{Done: true, State: "listed"}, nil
}
