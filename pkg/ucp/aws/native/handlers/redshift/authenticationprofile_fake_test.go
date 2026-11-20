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
	"slices"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/redshift/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	// profileInvalid is the code of a profile request AWS rejects, profileExists of a create for a name AWS has, and
	// profileValidation of a value outside the limits of the API reference.
	profileInvalid    = "InvalidAuthenticationProfileRequestFault"
	profileExists     = "AuthenticationProfileAlreadyExistsFault"
	profileValidation = "ValidationError"
)

// profileSecrets are content keys AWS rejects as sensitive: AWS's guide names AccessKeyID and SecretAccessKey, and
// AWS rejected SecretAccessKey and Password in a run against real AWS.
var profileSecrets = []string{"AccessKeyID", "SecretAccessKey", "Password"}

// profileFault returns the client fault AWS returns with the code.
func profileFault(code string, message string) error {
	return &smithy.GenericAPIError{Code: code, Message: message, Fault: smithy.FaultClient}
}

// profileFake is an in-memory Redshift API for authentication profiles. It keeps them in an awsfake store, in the
// type's property format with the content as it was sent, so the suite's Cloud Control sees the same resources. It follows the rules in profileAPI,
// and where AWS's docs are silent it takes the side that is harder for the handler: a deleted profile is still listed
// for one read.
type profileFake struct {
	store *awsfake.CloudControl

	mu sync.Mutex

	// deleted holds deleted profiles still listed for one read, by name.
	deleted map[string]map[string]any

	// awsForm makes reads return the content in AWS's own form, as AWS does. The suite leaves it unset, because its
	// checks compare reads with the template, and sets AWS's form with Suite.OtherForm instead.
	awsForm bool

	// errs maps a method name to an error the call returns instead of acting, and calls counts the calls of each.
	errs  map[string]error
	calls map[string]int
}

var _ ProfileAPI = (*profileFake)(nil)

func newProfileFake(store *awsfake.CloudControl) *profileFake {
	return &profileFake{
		store:   store,
		awsForm: true,
		deleted: map[string]map[string]any{},
		errs:    map[string]error{},
		calls:   map[string]int{},
	}
}

func (f *profileFake) begin(method string) error {
	f.calls[method]++
	return f.errs[method]
}

// checkProfileName checks a profile name against the API reference's limits.
func checkProfileName(name *string) error {
	if name == nil || !profileName.MatchString(*name) {
		return profileFault(profileValidation, "Value at 'authenticationProfileName' failed to satisfy constraint")
	}
	return nil
}

// storedProfileContent returns content as AWS stores it, in the order of its keys, which AWS may change, or a fault
// when AWS rejects it: content that isn't a JSON object of strings, numbers, booleans, and nulls, an empty object, or
// one with a sensitive key.
func storedProfileContent(content *string) (string, error) {
	invalid := profileFault(profileInvalid, "The profile map file doesn't contain valid content in JSON format.")
	var object map[string]any
	if content == nil || json.Unmarshal([]byte(*content), &object) != nil || len(object) == 0 {
		return "", invalid
	}
	values, ok := profileContent(*content)
	if !ok {
		return "", invalid
	}
	for key := range values {
		if slices.Contains(profileSecrets, key) {
			return "", profileFault(profileInvalid, "This authentication profile contains sensitive data fields: ["+key+"].")
		}
	}
	decoder := json.NewDecoder(bytes.NewReader([]byte(*content)))
	decoder.UseNumber()
	if _, err := decoder.Token(); err != nil {
		return "", invalid
	}
	out := &bytes.Buffer{}
	out.WriteString("{")
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return "", invalid
		}
		key, _ := token.(string)
		if _, err := decoder.Token(); err != nil {
			return "", invalid
		}
		value, ok := values[key]
		if !ok {
			continue
		}
		if out.Len() > 1 {
			out.WriteString(",")
		}
		k, _ := json.Marshal(key)
		v, _ := json.Marshal(value)
		out.Write(k)
		out.WriteString(":")
		out.Write(v)
	}
	out.WriteString("}")
	return out.String(), nil
}

func (f *profileFake) CreateAuthenticationProfile(ctx context.Context, in *redshift.CreateAuthenticationProfileInput, _ ...func(*redshift.Options)) (*redshift.CreateAuthenticationProfileOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("CreateAuthenticationProfile"); err != nil {
		return nil, err
	}
	if err := checkProfileName(in.AuthenticationProfileName); err != nil {
		return nil, err
	}
	content, err := storedProfileContent(in.AuthenticationProfileContent)
	if err != nil {
		return nil, err
	}
	name := aws.ToString(in.AuthenticationProfileName)
	if f.store.Resource(profileType, name) != nil {
		return nil, profileFault(profileExists, "The authentication profile already exists.")
	}
	f.store.Put(profileType, name, map[string]any{"AuthenticationProfileName": name, "AuthenticationProfileContent": aws.ToString(in.AuthenticationProfileContent)})
	delete(f.deleted, name)
	return &redshift.CreateAuthenticationProfileOutput{AuthenticationProfileName: aws.String(name), AuthenticationProfileContent: aws.String(content)}, nil
}

func (f *profileFake) ModifyAuthenticationProfile(ctx context.Context, in *redshift.ModifyAuthenticationProfileInput, _ ...func(*redshift.Options)) (*redshift.ModifyAuthenticationProfileOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("ModifyAuthenticationProfile"); err != nil {
		return nil, err
	}
	if err := checkProfileName(in.AuthenticationProfileName); err != nil {
		return nil, err
	}
	content, err := storedProfileContent(in.AuthenticationProfileContent)
	if err != nil {
		return nil, err
	}
	name := aws.ToString(in.AuthenticationProfileName)
	if f.store.Resource(profileType, name) == nil {
		return nil, profileFault(profileNotFound, "The authentication profile can't be found.")
	}
	f.store.Put(profileType, name, map[string]any{"AuthenticationProfileName": name, "AuthenticationProfileContent": aws.ToString(in.AuthenticationProfileContent)})
	return &redshift.ModifyAuthenticationProfileOutput{AuthenticationProfileName: aws.String(name), AuthenticationProfileContent: aws.String(content)}, nil
}

func (f *profileFake) DeleteAuthenticationProfile(ctx context.Context, in *redshift.DeleteAuthenticationProfileInput, _ ...func(*redshift.Options)) (*redshift.DeleteAuthenticationProfileOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DeleteAuthenticationProfile"); err != nil {
		return nil, err
	}
	if err := checkProfileName(in.AuthenticationProfileName); err != nil {
		return nil, err
	}
	name := aws.ToString(in.AuthenticationProfileName)
	props := f.store.Resource(profileType, name)
	if props == nil {
		return nil, profileFault(profileNotFound, "The authentication profile can't be found.")
	}
	f.store.Remove(profileType, name)
	f.deleted[name] = props
	return &redshift.DeleteAuthenticationProfileOutput{AuthenticationProfileName: aws.String(name)}, nil
}

// listed returns the profiles a read of name lists, or of every profile when name is nil, by name, counting the
// read.
func (f *profileFake) listed(name *string) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, n := range f.store.List(profileType) {
		if name != nil && *name != n {
			continue
		}
		out[n] = f.store.Resource(profileType, n)
	}
	for n, props := range f.deleted {
		if name != nil && *name != n {
			continue
		}
		out[n] = props
		delete(f.deleted, n)
	}
	return out
}

func (f *profileFake) DescribeAuthenticationProfiles(ctx context.Context, in *redshift.DescribeAuthenticationProfilesInput, _ ...func(*redshift.Options)) (*redshift.DescribeAuthenticationProfilesOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DescribeAuthenticationProfiles"); err != nil {
		return nil, err
	}
	if in.AuthenticationProfileName != nil {
		if err := checkProfileName(in.AuthenticationProfileName); err != nil {
			return nil, err
		}
	}
	out := []types.AuthenticationProfile{}
	for name, props := range f.listed(in.AuthenticationProfileName) {
		content := awsapi.String(props, "AuthenticationProfileContent")
		if f.awsForm {
			content, _ = storedProfileContent(aws.String(content))
		}
		out = append(out, types.AuthenticationProfile{
			AuthenticationProfileName:    aws.String(name),
			AuthenticationProfileContent: aws.String(content),
		})
	}
	if in.AuthenticationProfileName != nil && len(out) == 0 {
		return nil, profileFault(profileNotFound, "The authentication profile does not exist.")
	}
	slices.SortFunc(out, func(a, b types.AuthenticationProfile) int {
		return strings.Compare(aws.ToString(a.AuthenticationProfileName), aws.ToString(b.AuthenticationProfileName))
	})
	return &redshift.DescribeAuthenticationProfilesOutput{AuthenticationProfiles: out}, nil
}
