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

// Package awsreplay records the HTTP calls a native AWS handler makes to AWS, and replays them in tests. A cassette
// is a JSON file of calls in order, each with the operation and AWS's response. Replay serves the responses in that
// order and fails a test whose handler calls a different operation, so a test checks the calls a handler makes and
// how it reads AWS's real responses, without AWS.
//
// Set RADIUS_AWS_RECORD=1, with AWS credentials, to call AWS and save the cassette instead. Recording keeps no request
// or response headers except the few the SDK needs to read a response. In every value it keeps, it replaces the account
// ID with 123456789012, and each value registered with Scrub, such as a password, with x characters of the same length.
// It also replaces the recording identity's user ID with AIDAREPLAY, and each name in its ARN (the user name, or the
// role and session names of an assumed role) with radius-recording. It replaces those only in the string values of a
// JSON, XML, form, or CBOR body, never in a key or the syntax, and only where the whole name appears, so a short name
// such as "a" doesn't change other words. A recording fails when it can't replace them, such as in an XML attribute.
package awsreplay

import (
	"bytes"
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

const (
	// RecordEnv turns recording on when set to 1.
	RecordEnv = "RADIUS_AWS_RECORD"

	// Account, UserID, and Principal are the account ID, user ID, and principal name cassettes use in place of the
	// identity that recorded them.
	Account   = "123456789012"
	UserID    = "AIDAREPLAY"
	Principal = "radius-recording"

	// Region is the region every cassette is recorded and replayed in.
	Region = "us-west-2"

	// SourceRecorded marks a cassette AWS's responses were recorded into, and SourceAPIReference one written by hand
	// from the AWS API reference.
	SourceRecorded     = "recorded"
	SourceAPIReference = "api-reference"
)

// keptHeaders are the response headers a cassette keeps, which the SDK reads to tell the protocol, the content, and
// errors apart.
var keptHeaders = []string{"Content-Type", "Smithy-Protocol", "X-Amzn-Errortype", "X-Amzn-Query-Error"}

// Cassette is the calls of one test, in order.
type Cassette struct {
	// Source is SourceRecorded or SourceAPIReference.
	Source string `json:"source"`

	// Inputs are values the test reads from the environment when it records, such as the ID of a subnet the
	// recording created, so a replay sends the same requests.
	Inputs map[string]string `json:"inputs,omitempty"`

	Interactions []Interaction `json:"interactions"`
}

// Interaction is one call and AWS's response.
type Interaction struct {
	// Service and Operation are the SDK's service ID and operation name, such as DocDB and CreateDBCluster.
	Service   string `json:"service"`
	Operation string `json:"operation"`

	// Request is the request body, which replay doesn't match, kept so a reader can see what the handler sent.
	Request string `json:"request,omitempty"`

	Status  int               `json:"status"`
	Headers map[string]string `json:"headers,omitempty"`

	// Body is the response body, base64 encoded when Binary is true, such as for CBOR.
	Body   string `json:"body"`
	Binary bool   `json:"binary,omitempty"`
}

// Session records or replays the cassette of one test.
type Session struct {
	t         *testing.T
	file      string
	recording bool

	mu       sync.Mutex
	cassette Cassette
	next     int
	identity *sts.GetCallerIdentityOutput
	scrubbed []string
}

// Recording reports whether RADIUS_AWS_RECORD is 1, so tests call AWS and save cassettes.
func Recording() bool {
	return os.Getenv(RecordEnv) == "1"
}

// New returns the session for the cassette file. When replaying, the file must exist. When recording, the session
// saves the file when the test ends, unless it failed.
func New(t *testing.T, file string) *Session {
	t.Helper()
	s := &Session{t: t, file: file, recording: Recording()}
	if !s.recording {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("failed to read the cassette %s: %v. Record it with %s=1", file, err, RecordEnv)
		}
		if err := json.Unmarshal(b, &s.cassette); err != nil {
			t.Fatalf("failed to read the cassette %s: %v", file, err)
		}
		t.Cleanup(func() {
			if !t.Failed() && s.next != len(s.cassette.Interactions) {
				t.Errorf("the handler made %d of the %d calls in %s", s.next, len(s.cassette.Interactions), file)
			}
		})
		return s
	}
	s.cassette = Cassette{Source: SourceRecorded, Inputs: map[string]string{}}
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("not saving the cassette %s, because the test failed", file)
			return
		}
		if err := s.save(); err != nil {
			t.Errorf("failed to save the cassette %s: %v", file, err)
		}
	})
	return s
}

// Recording reports whether the session calls AWS.
func (s *Session) Recording() bool {
	return s.recording
}

// Config returns the AWS config for the handler under test. When recording, it uses the default credentials and
// region us-west-2. When replaying, it uses fake credentials, and the SDK makes no retries.
func (s *Session) Config(ctx context.Context) aws.Config {
	s.t.Helper()
	if !s.recording {
		return aws.Config{
			Region:           Region,
			Credentials:      credentials.NewStaticCredentialsProvider("AKIAREPLAY", "replay", ""),
			HTTPClient:       &http.Client{Transport: replayer{s}},
			RetryMaxAttempts: 1,
		}
	}
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(Region))
	if err != nil {
		s.t.Fatalf("failed to load AWS credentials to record: %v", err)
	}
	identity, err := sts.NewFromConfig(cfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		s.t.Fatalf("failed to read the AWS account to record in: %v", err)
	}
	s.identity = identity
	cfg.HTTPClient = &http.Client{Transport: recorder{s: s, next: http.DefaultTransport}}
	return cfg
}

// Input returns the value of the environment variable name when recording, and saves it in the cassette, and the
// saved value when replaying. A recording fails when the variable isn't set.
func (s *Session) Input(name string) string {
	s.t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.recording {
		v, ok := s.cassette.Inputs[name]
		if !ok {
			s.t.Fatalf("the cassette %s has no input %s", s.file, name)
		}
		return v
	}
	v := os.Getenv(name)
	if v == "" {
		s.t.Fatalf("set %s to record %s", name, s.file)
	}
	s.cassette.Inputs[name] = v
	return v
}

// Scrub registers values, such as passwords, that the cassette must never contain.
func (s *Session) Scrub(values ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range values {
		if v != "" {
			s.scrubbed = append(s.scrubbed, v)
		}
	}
}

// Wait sleeps for d when recording, such as between reads of a resource AWS is still creating, and returns at once
// when replaying.
func (s *Session) Wait(d time.Duration) {
	if s.recording {
		time.Sleep(d)
	}
}

// scrub returns b with the account ID replaced with Account, and every scrubbed value replaced with x characters of
// the same length in each form a request or response can carry it in: as is, percent-encoded in a form body, escaped
// in JSON, and base64 encoded. An account ID is always 12 digits, so every replacement keeps the length and the kind
// of character, and a body such as CBOR stays valid. All values are replaced in one pass, longest first, so a value
// that contains another, such as a scrubbed value that contains the account ID, is replaced whole.
func (s *Session) scrub(b []byte) []byte {
	var pairs []replacement
	if s.identity != nil && aws.ToString(s.identity.Account) != "" {
		pairs = append(pairs, replacement{aws.ToString(s.identity.Account), Account})
	}
	for _, v := range s.scrubbed {
		for _, form := range encodings(v) {
			pairs = append(pairs, replacement{form, strings.Repeat("x", len(form))})
		}
	}
	if len(pairs) == 0 {
		return b
	}
	slices.SortStableFunc(pairs, func(a, b replacement) int { return len(b.old) - len(a.old) })
	args := make([]string, 0, 2*len(pairs))
	for _, r := range pairs {
		args = append(args, r.old, r.new)
	}
	return []byte(strings.NewReplacer(args...).Replace(string(b)))
}

// escapes returns v and the forms text can carry it in: percent-encoded in a form body or path, and escaped in JSON.
func escapes(v string) []string {
	escaped, _ := json.Marshal(v)
	return unique([]string{v, url.QueryEscape(v), url.PathEscape(v), string(escaped[1 : len(escaped)-1])})
}

// encodings returns the forms of escapes and v base64 encoded.
func encodings(v string) []string {
	return unique(append(escapes(v), base64.StdEncoding.EncodeToString([]byte(v)), base64.RawStdEncoding.EncodeToString([]byte(v))))
}

// unique returns forms without duplicates, longest first.
func unique(forms []string) []string {
	slices.SortFunc(forms, func(a, b string) int { return cmp.Or(len(b)-len(a), strings.Compare(a, b)) })
	return slices.Compact(forms)
}

func (s *Session) save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rs := s.identityReplacements()
	for name, v := range s.cassette.Inputs {
		s.cassette.Inputs[name] = replaceNames(string(s.scrub([]byte(v))), rs)
	}
	b, err := json.MarshalIndent(s.cassette, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.file), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.file, append(b, '\n'), 0o644)
}

// operation returns the SDK's service ID and operation name for req.
func operation(req *http.Request) (string, string) {
	return awsmiddleware.GetServiceID(req.Context()), awsmiddleware.GetOperationName(req.Context())
}

type replayer struct {
	s *Session
}

// RoundTrip returns the next response in the cassette, or an error when the request is for another operation.
func (r replayer) RoundTrip(req *http.Request) (*http.Response, error) {
	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	service, op := operation(req)
	if s.next >= len(s.cassette.Interactions) {
		return nil, fmt.Errorf("awsreplay: %s %s was called after the last call in %s", service, op, s.file)
	}
	i := s.cassette.Interactions[s.next]
	if i.Service != service || i.Operation != op {
		return nil, fmt.Errorf("awsreplay: call %d in %s is %s %s, but the handler called %s %s", s.next+1, s.file, i.Service, i.Operation, service, op)
	}
	s.next++
	body := []byte(i.Body)
	if i.Binary {
		var err error
		if body, err = base64.StdEncoding.DecodeString(i.Body); err != nil {
			return nil, err
		}
	}
	header := http.Header{}
	for k, v := range i.Headers {
		header.Set(k, v)
	}
	return &http.Response{
		StatusCode:    i.Status,
		Status:        fmt.Sprintf("%d %s", i.Status, http.StatusText(i.Status)),
		Header:        header,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
	}, nil
}

type recorder struct {
	s    *Session
	next http.RoundTripper
}

// RoundTrip calls AWS and adds the call to the cassette.
func (r recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	var request []byte
	if req.Body != nil {
		var err error
		if request, err = io.ReadAll(req.Body); err != nil {
			return nil, err
		}
		req.Body = io.NopCloser(bytes.NewReader(request))
	}
	resp, err := r.next.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))

	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	service, op := operation(req)
	i := Interaction{Service: service, Operation: op, Status: resp.StatusCode, Headers: map[string]string{}}
	for _, h := range keptHeaders {
		if v := resp.Header.Get(h); v != "" {
			i.Headers[h] = v
		}
	}
	request, err = s.scrubIdentity(s.scrub(request), req.Header.Get("Content-Type"))
	if err != nil {
		s.t.Errorf("awsreplay: failed to scrub the request of %s %s: %v", service, op, err)
	}
	i.Request = text(request)
	scrubbed, err := s.scrubIdentity(s.scrub(body), resp.Header.Get("Content-Type"))
	if err != nil {
		s.t.Errorf("awsreplay: failed to scrub the response of %s %s: %v", service, op, err)
	}
	i.Binary = !utf8.Valid(scrubbed) || strings.Contains(resp.Header.Get("Content-Type"), "cbor")
	if i.Binary {
		i.Body = base64.StdEncoding.EncodeToString(scrubbed)
	} else {
		i.Body = string(scrubbed)
	}
	s.cassette.Interactions = append(s.cassette.Interactions, i)
	return resp, nil
}

// text returns a request body to keep: form bodies decoded for reading, other text as is, and binary bodies as "".
func text(b []byte) string {
	if !utf8.Valid(b) {
		return ""
	}
	if values, err := url.ParseQuery(string(b)); err == nil && strings.Contains(string(b), "Action=") {
		return values.Encode()
	}
	return string(b)
}
