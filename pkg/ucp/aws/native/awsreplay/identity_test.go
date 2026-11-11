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
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/appsync"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go/encoding/cbor"
	"github.com/stretchr/testify/require"
)

const (
	recordingAccount = "999988887777"
	recordingUserID  = "AIDA5EXAMPLEUSERID123"
)

// identityNames are IAM user names to record with: a one-character name, names that are part of or equal to a key in
// the bodies below, names with characters a form body escapes, and a long name.
var identityNames = []string{"a", "Name", "Account", "alice@example.com", "a+b", "radius-recorder-with-a-long-name-0123456789"}

// protocolCase is a call in one AWS protocol. call makes the call with the user name and returns the output values
// that hold the name, and the other output values, which a replay must decode unchanged.
type protocolCase struct {
	name     string
	headers  map[string]string
	response func(name string) []byte
	call     func(ctx context.Context, cfg aws.Config, name string) (identity []string, other string, err error)
	// want is identity after the recording identity is replaced.
	want []string
	// request checks the request the cassette keeps.
	request func(t *testing.T, request string)
}

// template returns a response that is body with NAME, ACCOUNT, and USERID replaced by the recording identity.
func template(body string) func(string) []byte {
	return func(name string) []byte {
		return []byte(strings.NewReplacer("NAME", name, "ACCOUNT", recordingAccount, "USERID", recordingUserID).Replace(body))
	}
}

var protocolCases = []protocolCase{
	{
		name:     "query XML",
		headers:  map[string]string{"Content-Type": "text/xml"},
		response: template(`<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><GetCallerIdentityResult><Arn>arn:aws:iam::ACCOUNT:user/NAME</Arn><UserId>USERID</UserId><Account>ACCOUNT</Account></GetCallerIdentityResult></GetCallerIdentityResponse>`),
		call: func(ctx context.Context, cfg aws.Config, name string) ([]string, string, error) {
			out, err := sts.NewFromConfig(cfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
			if err != nil {
				return nil, "", err
			}
			return []string{aws.ToString(out.Arn), aws.ToString(out.UserId), aws.ToString(out.Account)}, "", nil
		},
		want: []string{"arn:aws:iam::123456789012:user/radius-recording", "AIDAREPLAY", "123456789012"},
	},
	{
		name:     "query form request",
		headers:  map[string]string{"Content-Type": "text/xml"},
		response: template(`<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><AssumeRoleResult><AssumedRoleUser><Arn>arn:aws:sts::ACCOUNT:assumed-role/NAME/NAME</Arn><AssumedRoleId>AROA5EXAMPLE:NAME</AssumedRoleId></AssumedRoleUser><SourceIdentity>available</SourceIdentity></AssumeRoleResult></AssumeRoleResponse>`),
		call: func(ctx context.Context, cfg aws.Config, name string) ([]string, string, error) {
			out, err := sts.NewFromConfig(cfg).AssumeRole(ctx, &sts.AssumeRoleInput{RoleArn: aws.String("arn:aws:iam::" + recordingAccount + ":role/" + name), RoleSessionName: aws.String(name)})
			if err != nil {
				return nil, "", err
			}
			return []string{aws.ToString(out.AssumedRoleUser.Arn), aws.ToString(out.AssumedRoleUser.AssumedRoleId)}, aws.ToString(out.SourceIdentity), nil
		},
		want: []string{"arn:aws:sts::123456789012:assumed-role/radius-recording/radius-recording", "AROA5EXAMPLE:radius-recording"},
		request: func(t *testing.T, request string) {
			values, err := url.ParseQuery(request)
			require.NoError(t, err)
			require.Equal(t, "AssumeRole", values.Get("Action"))
			require.Equal(t, "radius-recording", values.Get("RoleSessionName"))
			require.Equal(t, "arn:aws:iam::123456789012:role/radius-recording", values.Get("RoleArn"))
		},
	},
	{
		name:     "AWS JSON",
		headers:  map[string]string{"Content-Type": "application/x-amz-json-1.1"},
		response: template(`{"repositories":[{"repositoryArn":"arn:aws:ecr:us-west-2:ACCOUNT:repository/NAME","registryId":"ACCOUNT","repositoryName":"NAME","imageTagMutability":"MUTABLE","imageScanningConfiguration":{"scanOnPush":false}}]}`),
		call: func(ctx context.Context, cfg aws.Config, name string) ([]string, string, error) {
			out, err := ecr.NewFromConfig(cfg).DescribeRepositories(ctx, &ecr.DescribeRepositoriesInput{RepositoryNames: []string{name}})
			if err != nil {
				return nil, "", err
			}
			r := out.Repositories[0]
			return []string{aws.ToString(r.RepositoryArn), aws.ToString(r.RepositoryName)}, fmt.Sprint(r.ImageTagMutability, r.ImageScanningConfiguration.ScanOnPush), nil
		},
		want: []string{"arn:aws:ecr:us-west-2:123456789012:repository/radius-recording", "radius-recording"},
		request: func(t *testing.T, request string) {
			var in map[string][]string
			require.NoError(t, json.Unmarshal([]byte(request), &in))
			require.Equal(t, map[string][]string{"repositoryNames": {"radius-recording"}}, in)
		},
	},
	{
		name:     "REST JSON",
		headers:  map[string]string{"Content-Type": "application/json"},
		response: template(`{"graphqlApi":{"name":"NAME","apiId":"abc","owner":"ACCOUNT","xrayEnabled":false,"tags":{"owner":"NAME","Enabled":"available"}}}`),
		call: func(ctx context.Context, cfg aws.Config, name string) ([]string, string, error) {
			out, err := appsync.NewFromConfig(cfg).GetGraphqlApi(ctx, &appsync.GetGraphqlApiInput{ApiId: aws.String("abc")})
			if err != nil {
				return nil, "", err
			}
			api := out.GraphqlApi
			return []string{aws.ToString(api.Name), api.Tags["owner"]}, fmt.Sprint(aws.ToString(api.ApiId), api.XrayEnabled, api.Tags["Enabled"]), nil
		},
		want: []string{"radius-recording", "radius-recording"},
	},
	{
		name:    "CBOR",
		headers: map[string]string{"Content-Type": "application/cbor", "Smithy-Protocol": "rpc-v2-cbor"},
		response: func(name string) []byte {
			return cbor.Encode(cbor.Map{"MetricAlarms": cbor.List{cbor.Map{
				"AlarmName":        cbor.String(name),
				"AlarmArn":         cbor.String("arn:aws:cloudwatch:us-west-2:" + recordingAccount + ":alarm:" + name),
				"AlarmDescription": cbor.String("owner " + name),
				"ActionsEnabled":   cbor.Bool(false),
				"StateValue":       cbor.String("OK"),
				"Threshold":        cbor.Float64(1.5),
			}}})
		},
		call: func(ctx context.Context, cfg aws.Config, name string) ([]string, string, error) {
			out, err := cloudwatch.NewFromConfig(cfg).DescribeAlarms(ctx, &cloudwatch.DescribeAlarmsInput{AlarmNames: []string{name}})
			if err != nil {
				return nil, "", err
			}
			a := out.MetricAlarms[0]
			return []string{aws.ToString(a.AlarmName), aws.ToString(a.AlarmArn), aws.ToString(a.AlarmDescription)}, fmt.Sprint(aws.ToBool(a.ActionsEnabled), a.StateValue, aws.ToFloat64(a.Threshold)), nil
		},
		want: []string{"radius-recording", "arn:aws:cloudwatch:us-west-2:123456789012:alarm:radius-recording", "owner radius-recording"},
	},
}

// Test_RecordAndReplay_Identity records a call in each protocol for each user name, and checks that the cassette
// replays through the SDK, with the recording identity replaced and every other value unchanged.
func Test_RecordAndReplay_Identity(t *testing.T) {
	for _, c := range protocolCases {
		for _, name := range identityNames {
			t.Run(c.name+"/"+name, func(t *testing.T) {
				recorded, recordedOther, replayed, replayedOther, cassette := recordAndReplay(t, name, c)

				require.Contains(t, strings.Join(recorded, " "), name, "the test sees AWS's real response")
				require.Equal(t, c.want, replayed[:len(c.want)])
				require.Equal(t, recordedOther, replayedOther)
				require.Len(t, cassette.Interactions, 1)
				if c.request != nil {
					c.request(t, cassette.Interactions[0].Request)
				}

				kept := cassette.Interactions[0].Request + cassette.Interactions[0].Body
				if cassette.Interactions[0].Binary {
					body, err := base64.StdEncoding.DecodeString(cassette.Interactions[0].Body)
					require.NoError(t, err)
					kept = cassette.Interactions[0].Request + string(body)
				}
				require.NotContains(t, kept, recordingAccount)
				require.NotContains(t, kept, recordingUserID)
				if len(name) > len("Account") {
					for _, form := range escapes(name) {
						require.NotContains(t, kept, form)
					}
				}
			})
		}
	}
}

// recordAndReplay records the call c makes, with the IAM user name as the recording identity, against a fake AWS
// that returns c's response, and then replays the cassette.
func recordAndReplay(t *testing.T, name string, c protocolCase) (recorded []string, recordedOther string, replayed []string, replayedOther string, cassette Cassette) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		for k, v := range c.headers {
			w.Header().Set(k, v)
		}
		_, _ = w.Write(c.response(name))
	}))
	defer server.Close()

	file := filepath.Join(t.TempDir(), "cassette.json")
	identity := &sts.GetCallerIdentityOutput{
		Account: aws.String(recordingAccount),
		Arn:     aws.String("arn:aws:iam::" + recordingAccount + ":user/" + name),
		UserId:  aws.String(recordingUserID),
	}
	s := &Session{t: t, file: file, recording: true, identity: identity, cassette: Cassette{Source: SourceRecorded, Inputs: map[string]string{}}}
	cfg := aws.Config{
		Region:       Region,
		Credentials:  credentials.NewStaticCredentialsProvider("AKIA", "secret", ""),
		BaseEndpoint: aws.String(server.URL),
		HTTPClient:   &http.Client{Transport: recorder{s: s, next: http.DefaultTransport}},
	}
	recorded, recordedOther, err := c.call(t.Context(), cfg, name)
	require.NoError(t, err)
	require.NoError(t, s.save())
	cassette = s.cassette

	t.Setenv(RecordEnv, "")
	replay := New(t, file)
	replayed, replayedOther, err = c.call(t.Context(), replay.Config(t.Context()), name)
	require.NoError(t, err, "the cassette replays")
	return recorded, recordedOther, replayed, replayedOther, cassette
}

// Test_ScrubIdentity checks that the identity is replaced only in string values, so each body still decodes, and
// that a body holding the identity somewhere it can't be replaced is an error.
func Test_ScrubIdentity(t *testing.T) {
	s := &Session{identity: &sts.GetCallerIdentityOutput{Arn: aws.String("arn:aws:iam::123456789012:user/a"), UserId: aws.String(recordingUserID)}}
	tests := []struct {
		name, contentType, body, want, err string
	}{
		{name: "JSON", body: `{"Enabled":false,"User":"a"}`, want: `{"Enabled":false,"User":"radius-recording"}`},
		{name: "JSON key", body: `{"a" : "a", "b": ["x/a", "available", null]}`, want: `{"a" : "radius-recording", "b": ["x/radius-recording", "available", null]}`},
		{name: "JSON escapes", body: `{"p":"user\/a \u0022a\u0022 <a>"}`, want: `{"p":"user/radius-recording \"radius-recording\" <radius-recording>"}`},
		{name: "XML", body: `<?xml version="1.0"?><a><Status>available</Status><User>a</User><Id>AIDA5EXAMPLEUSERID123</Id></a>`, want: `<?xml version="1.0"?><a><Status>available</Status><User>radius-recording</User><Id>AIDAREPLAY</Id></a>`},
		{name: "XML attribute", body: `<x name="a"></x>`, err: "attribute name"},
		{name: "XML namespace", body: `<x xmlns="https://a/" xmlns:a="urn:a"><a:y>a</a:y></x>`, want: `<x xmlns="https://a/" xmlns:a="urn:a"><a:y>radius-recording</a:y></x>`},
		{name: "XML CDATA and entities", body: `<x><![CDATA[a]]> &amp; <y>a &lt;a&gt;</y></x>`, want: `<x>radius-recording &amp; <y>radius-recording &lt;radius-recording&gt;</y></x>`},
		{name: "XML comment", body: `<x><!-- a --></x>`, err: "comment"},
		{name: "XML attribute with an entity", body: `<x name="&#97;"/>`, err: "attribute name"},
		{name: "XML text split by a comment", body: `<x>user/<!-- c -->a</x>`, want: `<x>user/radius-recording</x>`},
		{name: "XML text split by CDATA", body: `<x>user/<![CDATA[a]]></x><y>b<![CDATA[c]]></y>`, want: `<x>user/radius-recording</x><y>b<![CDATA[c]]></y>`},
		{name: "form", contentType: "application/x-www-form-urlencoded; charset=utf-8", body: "Action=Get&UserName=a&a=b", want: "Action=Get&UserName=radius-recording&a=b"},
		{name: "percent-encoded", body: "user%2Fa a%40b", want: "user%2Fradius-recording a%40b"},
		{name: "text", body: "a cat ate a-b", want: "radius-recording cat ate a-b"},
		{name: "binary", contentType: "application/octet-stream", body: "\xff a", err: "binary body"},
		{name: "binary without the identity", contentType: "application/octet-stream", body: "\xff\x00", want: "\xff\x00"},
		{name: "invalid XML without the identity", body: "<html><b>x</html>", want: "<html><b>x</html>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.scrubIdentity([]byte(tt.body), tt.contentType)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, string(got))
		})
	}
}

// Test_ScrubIdentity_AccountInName checks that a name or user ID that holds the account ID is replaced whole, after
// scrub has replaced the account ID, as the recorder does.
func Test_ScrubIdentity_AccountInName(t *testing.T) {
	s := &Session{identity: &sts.GetCallerIdentityOutput{
		Account: aws.String(recordingAccount),
		Arn:     aws.String("arn:aws:sts::" + recordingAccount + ":assumed-role/r-" + recordingAccount + "/a"),
		UserId:  aws.String("AROA5:s-" + recordingAccount),
	}}
	body := `{"Arn":"arn:aws:sts::` + recordingAccount + `:assumed-role/r-` + recordingAccount + `/a","Id":"AROA5:s-` + recordingAccount + `"}`
	got, err := s.scrubIdentity(s.scrub([]byte(body)), "")
	require.NoError(t, err)
	require.Equal(t, `{"Arn":"arn:aws:sts::123456789012:assumed-role/radius-recording/radius-recording","Id":"AIDAREPLAY"}`, string(got))
}

// Test_ScrubIdentity_ScrubbedName checks that a user ID that holds a value registered with Scrub is replaced whole.
func Test_ScrubIdentity_ScrubbedName(t *testing.T) {
	s := &Session{identity: &sts.GetCallerIdentityOutput{Arn: aws.String("arn:aws:sts::1:assumed-role/r/alice"), UserId: aws.String("AROA5:alice")}}
	s.Scrub("alice")
	got, err := s.scrubIdentity(s.scrub([]byte(`{"UserId":"AROA5:alice"}`)), "")
	require.NoError(t, err)
	require.Equal(t, `{"UserId":"AIDAREPLAY"}`, string(got))
}

// Test_ScrubIdentity_Root checks that the root user's user ID, which is its account ID, is replaced with Account.
func Test_ScrubIdentity_Root(t *testing.T) {
	s := &Session{identity: &sts.GetCallerIdentityOutput{Account: aws.String(recordingAccount), Arn: aws.String("arn:aws:iam::" + recordingAccount + ":root"), UserId: aws.String(recordingAccount)}}
	body := `{"Account":"` + recordingAccount + `","UserId":"` + recordingAccount + `","Arn":"arn:aws:iam::` + recordingAccount + `:root"}`
	got, err := s.scrubIdentity(s.scrub([]byte(body)), "")
	require.NoError(t, err)
	require.Equal(t, `{"Account":"123456789012","UserId":"123456789012","Arn":"arn:aws:iam::123456789012:root"}`, string(got))
}

// Test_ScrubIdentity_CBOR checks that a CBOR body is kept as is when it doesn't hold the identity, and that map keys
// and other values are kept when it does.
func Test_ScrubIdentity_CBOR(t *testing.T) {
	s := &Session{identity: &sts.GetCallerIdentityOutput{Arn: aws.String("arn:aws:iam::123456789012:user/a")}}
	unchanged := cbor.Encode(cbor.Map{"Status": cbor.String("available")})
	got, err := s.scrubIdentity(unchanged, "application/cbor")
	require.NoError(t, err)
	require.Equal(t, unchanged, got)

	body := cbor.Encode(cbor.Map{"a": cbor.List{cbor.String("a"), &cbor.Tag{ID: 1, Value: cbor.String("x/a")}, cbor.Bool(false), cbor.Slice("a")}})
	got, err = s.scrubIdentity(body, "application/cbor")
	require.NoError(t, err)
	decoded, err := cbor.Decode(got)
	require.NoError(t, err)
	require.Equal(t, cbor.Map{"a": cbor.List{cbor.String("radius-recording"), &cbor.Tag{ID: 1, Value: cbor.String("x/radius-recording")}, cbor.Bool(false), cbor.Slice("a")}}, decoded)

	_, err = s.scrubIdentity([]byte{0xff, 'a'}, "application/cbor")
	require.Error(t, err, "a body that doesn't decode holds the identity")
}

func Test_ReplaceNames(t *testing.T) {
	rs := []replacement{{"alice@example.com", "P"}, {"alice%40example.com", "P"}, {"3A", "P"}, {"a", "P"}}
	tests := map[string]string{
		"a":                            "P",
		"available us-west-2a a.b a,b": "available us-west-2a a.b a,b",
		"arn:aws:iam::1:user/a":        "arn:aws:iam::1:user/P",
		"user%2Fa%3Ab":                 "user%2FP%3Ab",
		"x%40a a%2B":                   "x%40a a%2B",
		"%3A%3a%2F":                    "%3A%3a%2F",
		"user%2F3A":                    "user%2FP",
		"alice@example.com":            "P",
		"by alice%40example.com.":      "by alice%40example.com.",
		"by alice%40example.com":       "by P",
		"":                             "",
	}
	for v, want := range tests {
		require.Equal(t, want, replaceNames(v, rs), v)
	}
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
