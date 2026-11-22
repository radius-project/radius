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

package kms

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// Test_Replay_Grant creates a grant on a key the template names by key ID, reads it, and retires it, against AWS's
// responses. Every property is create-only, so there is no update step.
func Test_Replay_Grant(t *testing.T) {
	s := awsreplay.New(t, "testdata/grant.json")
	api := kms.NewFromConfig(s.Config(t.Context()))

	key := s.Input("RADIUS_REC_KMS_KEY_ID")
	properties := map[string]any{
		"KeyId":            key,
		"GranteePrincipal": s.Input("RADIUS_REC_GRANTEE_ROLE_ARN"),
		"Operations":       []any{"Decrypt", "GenerateDataKey"},
		"Constraints":      map[string]any{"EncryptionContextSubset": map[string]any{"app": "radius-rec"}},
	}
	account := s.Input("RADIUS_REC_ACCOUNT_ID")
	nativetest.Lifecycle{
		Handler:    NewGrant(api),
		Session:    s,
		Properties: properties,
		Want: with(properties, map[string]any{
			"KeyId":          "arn:aws:kms:" + awsreplay.Region + ":" + account + ":key/" + key,
			"IssuingAccount": "arn:aws:iam::" + account + ":root",
		}),
		Poll: 5 * time.Second,
	}.Run(t)
}

// lostAnswers answers ListGrants with no grants, and every other KMS call with 500, as AWS does when it may have
// applied a call but its answer is lost. It counts the calls by operation.
type lostAnswers struct {
	mu    sync.Mutex
	calls map[string]int
}

func (l *lostAnswers) Do(req *http.Request) (*http.Response, error) {
	op := strings.TrimPrefix(req.Header.Get("X-Amz-Target"), "TrentService.")
	l.mu.Lock()
	l.calls[op]++
	l.mu.Unlock()
	status, body := http.StatusInternalServerError, `{"__type":"KMSInternalException","message":"lost"}`
	if op == "ListGrants" {
		status, body = http.StatusOK, `{"Grants":[],"Truncated":false}`
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/x-amz-json-1.1"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}

// Test_CreateGrantSentOnce checks that the SDK doesn't send CreateGrant again after an answer it would retry, from
// Create or Find, and that the create's outcome stays unknown, so UCP keeps the record of a grant AWS may have made.
func Test_CreateGrantSentOnce(t *testing.T) {
	properties := map[string]any{"KeyId": keyARN(keyID), "GranteePrincipal": grantee, "Operations": []any{"Decrypt"}}
	for name, send := range map[string]func(h *Grant, request native.CreateRequest) error{
		"Create": func(h *Grant, request native.CreateRequest) error {
			_, err := h.Create(t.Context(), request)
			return err
		},
		"Find": func(h *Grant, request native.CreateRequest) error {
			_, err := h.Find(t.Context(), request)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			transport := &lostAnswers{calls: map[string]int{}}
			api := kms.New(kms.Options{
				Region:      region,
				Credentials: credentials.NewStaticCredentialsProvider("id", "secret", ""),
				HTTPClient:  transport,
			})
			err := send(NewGrant(api), native.CreateRequest{Region: region, Properties: properties, ClientToken: "token"})
			require.Error(t, err)
			var failure *native.Failure
			require.False(t, errors.As(err, &failure), "AWS may have made the grant")
			require.Equal(t, 1, transport.calls["CreateGrant"])

			_, err = NewGrant(api).ResolveIdentifier(t.Context(), region, keyID+"|"+firstGrant)
			require.Error(t, err)
			require.Greater(t, transport.calls["DescribeKey"], 1, "other calls keep the SDK's retries")
		})
	}
}
