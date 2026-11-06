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

package awsreplay

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/stretchr/testify/require"
)

const identityResponse = `<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <GetCallerIdentityResult>
    <Arn>arn:aws:iam::123456789012:user/recorder</Arn>
    <UserId>AIDAEXAMPLE</UserId>
    <Account>123456789012</Account>
  </GetCallerIdentityResult>
</GetCallerIdentityResponse>`

func writeCassette(t *testing.T, c Cassette) string {
	file := filepath.Join(t.TempDir(), "cassette.json")
	b, err := json.Marshal(c)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(file, b, 0o644))
	return file
}

func Test_Replay(t *testing.T) {
	t.Setenv(RecordEnv, "")
	file := writeCassette(t, Cassette{
		Source: SourceAPIReference,
		Inputs: map[string]string{"SUBNET": "subnet-1"},
		Interactions: []Interaction{
			{Service: "STS", Operation: "GetCallerIdentity", Status: 200, Headers: map[string]string{"Content-Type": "text/xml"}, Body: identityResponse},
			{Service: "STS", Operation: "GetCallerIdentity", Status: 200, Headers: map[string]string{"Content-Type": "text/xml"}, Body: base64.StdEncoding.EncodeToString([]byte(identityResponse)), Binary: true},
		},
	})
	s := New(t, file)
	require.False(t, s.Recording())
	require.Equal(t, "subnet-1", s.Input("SUBNET"))
	start := time.Now()
	s.Wait(time.Hour)
	require.Less(t, time.Since(start), time.Second, "a replay doesn't wait")

	client := sts.NewFromConfig(s.Config(t.Context()))
	out, err := client.GetCallerIdentity(t.Context(), &sts.GetCallerIdentityInput{})
	require.NoError(t, err)
	require.Equal(t, "123456789012", aws.ToString(out.Account))
	_, err = client.GetCallerIdentity(t.Context(), &sts.GetCallerIdentityInput{})
	require.NoError(t, err)

	_, err = client.GetCallerIdentity(t.Context(), &sts.GetCallerIdentityInput{})
	require.ErrorContains(t, err, "was called after the last call")
}

func Test_Replay_WrongOperation(t *testing.T) {
	t.Setenv(RecordEnv, "")
	file := writeCassette(t, Cassette{Interactions: []Interaction{{Service: "STS", Operation: "AssumeRole", Status: 200}}})
	s := New(t, file)
	_, err := sts.NewFromConfig(s.Config(t.Context())).GetCallerIdentity(t.Context(), &sts.GetCallerIdentityInput{})
	require.ErrorContains(t, err, "call 1 in "+file+" is STS AssumeRole, but the handler called STS GetCallerIdentity")
	s.next = 1
}

// Test_Record records a call to a fake AWS, and checks that the cassette keeps no account ID, no scrubbed value, and
// only the response headers the SDK needs.
func Test_Record(t *testing.T) {
	const account = "999988887777"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/xml")
		w.Header().Set("X-Amzn-Requestid", "kept-out")
		_, _ = w.Write([]byte(strings.ReplaceAll(identityResponse, "123456789012", account) + "<!-- p@ssw0rd -->"))
	}))
	defer server.Close()

	file := filepath.Join(t.TempDir(), "recorded", "cassette.json")
	s := &Session{t: t, file: file, recording: true, identity: &sts.GetCallerIdentityOutput{Account: aws.String(account)}, cassette: Cassette{Source: SourceRecorded, Inputs: map[string]string{}}}
	t.Setenv("ROLE", "arn:aws:iam::"+account+":role/r")
	require.Equal(t, "arn:aws:iam::"+account+":role/r", s.Input("ROLE"), "a recording sends the real value")
	s.Scrub("p@ssw0rd", "")

	cfg := aws.Config{
		Region:       Region,
		Credentials:  credentials.NewStaticCredentialsProvider("AKIA", "secret", ""),
		BaseEndpoint: aws.String(server.URL),
		HTTPClient:   &http.Client{Transport: recorder{s: s, next: http.DefaultTransport}},
	}
	out, err := sts.NewFromConfig(cfg).GetCallerIdentity(t.Context(), &sts.GetCallerIdentityInput{})
	require.NoError(t, err)
	require.Equal(t, account, aws.ToString(out.Account), "the test sees AWS's real response")
	require.NoError(t, s.save())

	b, err := os.ReadFile(file)
	require.NoError(t, err)
	saved := string(b)
	require.NotContains(t, saved, account)
	require.NotContains(t, saved, "p@ssw0rd")
	require.NotContains(t, saved, "kept-out")
	require.NotContains(t, saved, "AKIA")
	require.Contains(t, saved, "xxxxxxxx")

	var c Cassette
	require.NoError(t, json.Unmarshal(b, &c))
	require.Equal(t, "arn:aws:iam::123456789012:role/r", c.Inputs["ROLE"])
	require.Len(t, c.Interactions, 1)
	i := c.Interactions[0]
	require.Equal(t, "STS", i.Service)
	require.Equal(t, "GetCallerIdentity", i.Operation)
	require.Equal(t, map[string]string{"Content-Type": "text/xml"}, i.Headers)
	require.Contains(t, i.Request, "Action=GetCallerIdentity")
	require.False(t, i.Binary)
}

// Test_Record_Identity records the caller identity from a fake AWS, and checks that the cassette keeps the fixed
// account ID, user ID, and principal name in place of the identity that recorded it.
func Test_Record_Identity(t *testing.T) {
	const account = "999988887777"
	tests := []struct {
		name   string
		arn    string
		userID string
		names  []string
		want   string
	}{
		{
			name:   "user",
			arn:    "arn:aws:iam::" + account + ":user/team/alice",
			userID: "AIDA5EXAMPLEUSERID123",
			names:  []string{"alice"},
			want:   "arn:aws:iam::123456789012:user/team/radius-recording",
		},
		{
			name:   "assumed role",
			arn:    "arn:aws:sts::" + account + ":assumed-role/RecorderRole/alice@example.com",
			userID: "AROA5EXAMPLEROLEID123:alice@example.com",
			names:  []string{"RecorderRole", "alice@example.com"},
			want:   "arn:aws:sts::123456789012:assumed-role/radius-recording/radius-recording",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := strings.NewReplacer("arn:aws:iam::123456789012:user/recorder", tt.arn, "AIDAEXAMPLE", tt.userID, "123456789012", account).Replace(identityResponse)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.ReadAll(r.Body)
				w.Header().Set("Content-Type", "text/xml")
				_, _ = w.Write([]byte(response))
			}))
			defer server.Close()

			identity := &sts.GetCallerIdentityOutput{Account: aws.String(account), Arn: aws.String(tt.arn), UserId: aws.String(tt.userID)}
			file := filepath.Join(t.TempDir(), "cassette.json")
			s := &Session{t: t, file: file, recording: true, identity: identity, cassette: Cassette{Source: SourceRecorded, Inputs: map[string]string{}}}
			cfg := aws.Config{
				Region:       Region,
				Credentials:  credentials.NewStaticCredentialsProvider("AKIA", "secret", ""),
				BaseEndpoint: aws.String(server.URL),
				HTTPClient:   &http.Client{Transport: recorder{s: s, next: http.DefaultTransport}},
			}
			out, err := sts.NewFromConfig(cfg).GetCallerIdentity(t.Context(), &sts.GetCallerIdentityInput{})
			require.NoError(t, err)
			require.Equal(t, tt.arn, aws.ToString(out.Arn), "the test sees AWS's real response")
			require.NoError(t, s.save())

			b, err := os.ReadFile(file)
			require.NoError(t, err)
			saved := string(b)
			require.NotContains(t, saved, tt.userID)
			for _, name := range tt.names {
				require.NotContains(t, saved, name)
			}

			var c Cassette
			require.NoError(t, json.Unmarshal(b, &c))
			require.Len(t, c.Interactions, 1)
			require.Contains(t, c.Interactions[0].Body, "<Arn>"+tt.want+"</Arn>")
			require.Contains(t, c.Interactions[0].Body, "<UserId>AIDAREPLAY</UserId>")
			require.Contains(t, c.Interactions[0].Body, "<Account>123456789012</Account>")
		})
	}
}

// Test_ScrubIdentityBinary checks that the identity is replaced with values of the same length in a binary body, so a
// body such as CBOR stays valid.
func Test_ScrubIdentityBinary(t *testing.T) {
	s := &Session{identity: &sts.GetCallerIdentityOutput{
		Account: aws.String("999988887777"),
		Arn:     aws.String("arn:aws:iam::999988887777:user/alice"),
		UserId:  aws.String("AIDA5EXAMPLEUSERID123"),
	}}
	body := "arn:aws:iam::999988887777:user/alice AIDA5EXAMPLEUSERID123"
	require.Equal(t, "arn:aws:iam::123456789012:user/xxxxx xxxxxxxxxxxxxxxxxxxxx", string(s.scrub([]byte(body), true)))
	require.Equal(t, "arn:aws:iam::123456789012:user/radius-recording AIDAREPLAY", string(s.scrub([]byte(body), false)))
}

func Test_PrincipalNames(t *testing.T) {
	tests := []struct {
		arn  string
		want []string
	}{
		{"arn:aws:iam::123456789012:user/alice", []string{"alice"}},
		{"arn:aws:iam::123456789012:user/team/alice", []string{"alice"}},
		{"arn:aws:sts::123456789012:assumed-role/Role/session", []string{"Role", "session"}},
		{"arn:aws:sts::123456789012:federated-user/bob", []string{"bob"}},
		{"arn:aws:iam::123456789012:root", nil},
		{"not-an-arn", nil},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, principalNames(tt.arn), tt.arn)
	}
}

func Test_Text(t *testing.T) {
	require.Equal(t, "", text([]byte{0xff, 0xfe}))
	require.Equal(t, `{"a":1}`, text([]byte(`{"a":1}`)))
	require.Equal(t, "Action=Describe&Name=x", text([]byte("Name=x&Action=Describe")))
}

// Test_New_Recording checks that a recording session saves its cassette when the test ends, and that Wait sleeps.
func Test_New_Recording(t *testing.T) {
	t.Setenv(RecordEnv, "1")
	t.Setenv("RADIUS_REC_SUBNET", "subnet-1")
	file := filepath.Join(t.TempDir(), "saved", "cassette.json")
	t.Run("record", func(t *testing.T) {
		s := New(t, file)
		require.True(t, s.Recording())
		require.Equal(t, "subnet-1", s.Input("RADIUS_REC_SUBNET"))
		start := time.Now()
		s.Wait(time.Millisecond)
		require.GreaterOrEqual(t, time.Since(start), time.Millisecond)
	})

	b, err := os.ReadFile(file)
	require.NoError(t, err)
	var c Cassette
	require.NoError(t, json.Unmarshal(b, &c))
	require.Equal(t, SourceRecorded, c.Source)
	require.Equal(t, map[string]string{"RADIUS_REC_SUBNET": "subnet-1"}, c.Inputs)
}

// Test_Scrub checks that a scrubbed value is removed in every form a body can carry it in.
func Test_Scrub(t *testing.T) {
	s := &Session{identity: &sts.GetCallerIdentityOutput{Account: aws.String("999988887777")}}
	secret := `p@ss "w0rd"+&$!/`
	s.Scrub(secret)
	escaped, err := json.Marshal(secret)
	require.NoError(t, err)
	for _, body := range []string{
		secret,
		"Action=Modify&MasterUserPassword=" + url.QueryEscape(secret),
		`{"MasterUserPassword":` + string(escaped) + `}`,
		`{"definition":"` + base64.StdEncoding.EncodeToString([]byte(secret)) + `"}`,
		"arn:aws:iam::999988887777:role/r",
	} {
		scrubbed := s.scrub([]byte(body), false)
		require.Len(t, scrubbed, len(body), "the length stays the same")
		require.NotContains(t, text(scrubbed), "w0rd")
		require.NotContains(t, string(scrubbed), "999988887777")
	}
}

// Test_ScrubKeepsJSONValid checks that scrubbing a value that starts or ends with a quote, which JSON escapes, leaves
// the body valid JSON with the escape intact.
func Test_ScrubKeepsJSONValid(t *testing.T) {
	for _, secret := range []string{`pa"`, `"pa`, `"pa"`, `pa\`} {
		s := &Session{}
		s.Scrub(secret)
		body, err := json.Marshal(map[string]string{"Password": secret})
		require.NoError(t, err)
		scrubbed := s.scrub(body, false)
		var parsed map[string]string
		require.NoError(t, json.Unmarshal(scrubbed, &parsed), "%q scrubs to %s", secret, scrubbed)
		require.NotEqual(t, secret, parsed["Password"])
		require.NotContains(t, parsed["Password"], "pa")
	}
}
