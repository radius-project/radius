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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	// topicInput and roleInput name a topic the application's events go to, and an IAM role Amazon SNS can assume to
	// log deliveries, which the recording uses.
	topicInput = "RADIUS_REC_SNS_TOPIC_ARN"
	roleInput  = "RADIUS_REC_SNS_FEEDBACK_ROLE_ARN"

	// recordedName is the name of the application the recordings create and delete.
	recordedName = "radius-rec-platform-app"
)

// replay returns the session for the cassette file and a handler that uses it, which waits only when recording.
func replay(t *testing.T, file string) (*awsreplay.Session, *PlatformApplication) {
	s := awsreplay.New(t, file)
	cfg := s.Config(t.Context())
	h := NewPlatformApplication(sns.NewFromConfig(cfg))
	h.wait = func(_ context.Context, d time.Duration) { s.Wait(d) }
	return s, h
}

// apnsCredentials returns APNs credentials Amazon SNS takes without calling Apple: a new ES256 signing key for token
// credentials, and a new self-signed certificate and its key for certificate credentials. A replay sends none of
// them, so it gets placeholders. The session scrubs them from the cassette.
func apnsCredentials(t *testing.T, s *awsreplay.Session) (signingKey string, certificate string, privateKey string) {
	if !s.Recording() {
		return "replayed-signing-key", "replayed-certificate", "replayed-private-key"
	}
	ec, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(ec)
	require.NoError(t, err)
	signingKey = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "Apple Development IOS Push Services: com.example.radius", OrganizationalUnit: []string{"ABCDE12345"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	cert, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	certificate = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert}))
	privateKey = string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	s.Scrub(signingKey, certificate, privateKey)
	return signingKey, certificate, privateKey
}

// Test_Replay_PlatformApplication creates an APNs sandbox application with token credentials, a topic for new
// endpoints, and delivery logging, then switches it to certificate credentials and sends failures to the topic
// instead, and deletes it, against AWS's responses.
func Test_Replay_PlatformApplication(t *testing.T) {
	s, h := replay(t, "testdata/platformapplication.json")
	topic, role := s.Input(topicInput), s.Input(roleInput)
	signingKey, certificate, privateKey := apnsCredentials(t, s)
	want := map[string]any{
		"Platform":                  "APNS_SANDBOX",
		"Name":                      recordedName,
		"ApplePlatformTeamID":       "ABCDE12345",
		"ApplePlatformBundleID":     "com.example.radius",
		"EventEndpointCreated":      topic,
		"SuccessFeedbackRoleArn":    role,
		"SuccessFeedbackSampleRate": "50",
	}
	properties := map[string]any{"PlatformPrincipal": "ABC123DEFG", "PlatformCredential": signingKey}
	for k, v := range want {
		properties[k] = v
	}
	wantUpdated := map[string]any{
		"Platform":                  "APNS_SANDBOX",
		"Name":                      recordedName,
		"EventDeliveryFailure":      topic,
		"SuccessFeedbackRoleArn":    role,
		"SuccessFeedbackSampleRate": "50",
		"AuthenticationMethod":      "Certificate",
	}
	updated := map[string]any{"PlatformPrincipal": certificate, "PlatformCredential": privateKey}
	for k, v := range wantUpdated {
		if k != "AuthenticationMethod" {
			updated[k] = v
		}
	}
	want["AuthenticationMethod"] = "Token"
	want["Enabled"] = "true"

	nativetest.Lifecycle{
		Handler:     h,
		Session:     s,
		Properties:  properties,
		Want:        want,
		Update:      updated,
		WantUpdated: wantUpdated,
		Poll:        time.Second,
	}.Run(t)
}

// Test_Replay_Errors checks how the handler reports AWS's errors, recorded from real calls: credentials the push
// notification service rejects, a create of a name AWS has with other credentials, and a read, update, and delete of
// an application AWS doesn't have. Each rejected call applied nothing.
func Test_Replay_Errors(t *testing.T) {
	s, h := replay(t, "testdata/errors.json")
	ctx := t.Context()
	signingKey, _, _ := apnsCredentials(t, s)

	_, err := h.Create(ctx, native.CreateRequest{Region: awsreplay.Region, Properties: map[string]any{
		"Platform": "GCM", "Name": recordedName, "PlatformCredential": "not-a-firebase-key",
	}})
	requireRejected(t, err, "Platform credentials are invalid")

	token := map[string]any{
		"Platform": "APNS_SANDBOX", "Name": recordedName, "PlatformPrincipal": "ABC123DEFG", "PlatformCredential": signingKey,
		"ApplePlatformTeamID": "ABCDE12345", "ApplePlatformBundleID": "com.example.radius",
	}
	identifier, err := h.Create(ctx, native.CreateRequest{Region: awsreplay.Region, Properties: token})
	require.NoError(t, err)
	other := map[string]any{}
	for k, v := range token {
		other[k] = v
	}
	other["ApplePlatformBundleID"] = "com.example.other"
	_, err = h.Create(ctx, native.CreateRequest{Region: awsreplay.Region, Properties: other})
	requireRejected(t, err, "same name but different properties")
	require.NoError(t, h.Delete(ctx, awsreplay.Region, identifier))

	missing := "GCM|radius-rec-missing-app"
	_, err = h.Read(ctx, awsreplay.Region, missing)
	require.ErrorIs(t, err, native.ErrNotFound)
	current := map[string]any{"Platform": "GCM", "Name": "radius-rec-missing-app", "EventEndpointCreated": "arn:aws:sns:us-west-2:123456789012:topic"}
	desired := map[string]any{"Platform": "GCM", "Name": "radius-rec-missing-app"}
	patch, err := jsondiff.Compare(current, desired)
	require.NoError(t, err)
	err = h.Update(ctx, native.UpdateRequest{Region: awsreplay.Region, Identifier: missing, Current: current, Desired: desired, Patch: patch})
	require.ErrorIs(t, err, native.ErrNotFound)
	require.ErrorIs(t, h.Delete(ctx, awsreplay.Region, missing), native.ErrNotFound)
}
