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
	"maps"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	region = "us-west-2"

	// testIdentity is an identity the fake has, missingIdentity one it doesn't have, and noForwardingIdentity one
	// whose email feedback forwarding is off.
	testIdentity         = "sender@example.com"
	missingIdentity      = missingPrefix + "@example.com"
	noForwardingIdentity = noForwardingPrefix + "@example.com"

	// testIP and otherIP are dedicated IP addresses the fake account leases, and unleasedIP one it doesn't.
	testIP     = "198.51.100.7"
	otherIP    = "198.51.100.8"
	unleasedIP = "198.51.100.9"
)

var (
	topicA = snsTopic("bounces")
	topicB = snsTopic("other-bounces")
)

// snsTopic returns the ARN of the SNS topic name in the fake's account and region.
func snsTopic(name string) string {
	return "arn:aws:sns:" + region + ":" + account + ":" + name
}

func readFile(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/handwritten/" + file)
	require.NoError(t, err)
	return string(b)
}

func with(properties map[string]any, changes map[string]any) map[string]any {
	out := maps.Clone(properties)
	for k, v := range changes {
		if v == nil {
			delete(out, k)
		} else {
			out[k] = v
		}
	}
	return out
}

// fakes maps each handler a suite made to its fake, for Suite.FailCall.
type fakes struct {
	mu  sync.Mutex
	api map[native.Handler]*fakeAPI
}

func (f *fakes) add(h native.Handler, api *fakeAPI) native.Handler {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.api == nil {
		f.api = map[native.Handler]*fakeAPI{}
	}
	f.api[h] = api
	return h
}

func (f *fakes) failer(h native.Handler) *nativetest.Failer {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &f.api[h].failer
}

// splitIdentifier returns the properties of an identifier that joins first and second with "|".
func splitIdentifier(first string, second string) func(identifier string) map[string]any {
	return func(identifier string) map[string]any {
		a, b, _ := strings.Cut(identifier, "|")
		return map[string]any{first: a, second: b}
	}
}

func identityPolicy(name string, policy string) map[string]any {
	return map[string]any{"EmailIdentity": testIdentity, "PolicyName": name, "Policy": policy}
}

func Test_Suite_EmailIdentityPolicy(t *testing.T) {
	// Two principals, in the order SES returns them.
	user := "arn:aws:iam::" + account + ":user/sender"
	properties := identityPolicy("p1", strings.Replace(testPolicy(testIdentity), `"AWS":"arn:aws:iam::`+account+`:root"`, `"AWS":["`+user+`","arn:aws:iam::`+account+`:root"]`, 1))
	nativetest.Run(t, nativetest.Suite{
		TypeName:    identityPolicyType,
		Schema:      readFile(t, "aws-ses-emailidentitypolicy.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			h := NewEmailIdentityPolicy(newFakeAPI(store))
			h.wait = noWait
			return h
		},
		IdentifierProperty:   "PolicyName",
		IdentifierProperties: splitIdentifier("EmailIdentity", "PolicyName"),
		Properties:           properties,
		Identifier:           testIdentity + "|p1",
		Changed:              identityPolicy("p1", testPolicy(testIdentity, "ses:SendEmail", "ses:SendRawEmail")),
		CreateOnlyChange:     identityPolicy("p2", testPolicy(testIdentity)),
		// The template's policy in another form, as SES may return it: with whitespace, the account ID for its root
		// ARN, the principals in another order, a list of one action, and keys in another order.
		OtherForm: map[string]any{"Policy": `{
  "Statement": [{"Resource": "` + identityARN(testIdentity) + `", "Action": ["ses:SendEmail"], "Effect": "Allow",
    "Principal": {"AWS": ["` + account + `", "` + user + `"]}}],
  "Version": "2012-10-17"
}`},
		Invalid: []map[string]any{
			identityPolicy("bad.name", testPolicy(testIdentity)),
			identityPolicy(strings.Repeat("a", 65), testPolicy(testIdentity)),
			with(properties, map[string]any{"EmailIdentity": identityARN(testIdentity)}),
		},
		API: identityPolicyAPI,
	})
}

func notificationTopic(kind string, topic string, headers bool) map[string]any {
	props := map[string]any{"Identity": testIdentity, "NotificationType": kind, "SnsTopic": topic}
	if headers {
		props[headersProperty] = true
	}
	return props
}

func Test_Suite_IdentityNotificationTopic(t *testing.T) {
	fs := &fakes{}
	nativetest.Run(t, nativetest.Suite{
		TypeName:    notificationTopicType,
		Schema:      readFile(t, "aws-ses-identitynotificationtopic.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			api := newFakeAPI(store)
			h := NewIdentityNotificationTopic(api)
			h.wait = noWait
			return fs.add(h, api)
		},
		IdentifierProperty:   "NotificationType",
		IdentifierProperties: splitIdentifier("Identity", "NotificationType"),
		Properties:           notificationTopic("Bounce", topicA, true),
		Identifier:           testIdentity + "|Bounce",
		Changed:              notificationTopic("Bounce", topicB, true),
		Removable:            headersProperty,
		CreateOnlyChange:     notificationTopic("Complaint", topicA, true),
		// A new topic, and headers turned off.
		MultiCall: notificationTopic("Bounce", topicB, false),
		Calls:     2,
		FailCall:  nativetest.FailCall(2, fs.failer),
		Invalid: []map[string]any{
			notificationTopic("Open", topicA, false),
			notificationTopic("Bounce", "not-an-arn", false),
			with(notificationTopic("Bounce", topicA, false), map[string]any{"Identity": identityARN(testIdentity)}),
		},
		API: notificationTopicAPI,
	})
}

func Test_Suite_DedicatedIPAssignment(t *testing.T) {
	assignment := func(ip string, pool string) map[string]any {
		return map[string]any{"Ip": ip, poolProperty: pool}
	}
	nativetest.Run(t, nativetest.Suite{
		TypeName:    dedicatedIPType,
		Schema:      readFile(t, "aws-ses-dedicatedipassignment.json"),
		Metadata:    readFile(t, "aws-ses-dedicatedipassignment.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			h := NewDedicatedIPAssignment(newFakeAPI(store))
			h.wait = noWait
			return h
		},
		IdentifierProperty: "Ip",
		Properties:         assignment(testIP, "pool-a"),
		Identifier:         testIP,
		Changed:            assignment(testIP, "pool-b"),
		CreateOnlyChange:   assignment(otherIP, "pool-a"),
		Invalid: []map[string]any{
			assignment("198.51.100.07", "pool-a"),
			assignment("2001:db8::1", "pool-a"),
		},
		AlwaysPresent: true,
		API:           dedicatedIPAPI,
	})
}
