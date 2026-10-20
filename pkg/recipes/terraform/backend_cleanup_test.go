/*
Copyright 2026 The Radius Authors.

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

package terraform

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/backends"
	"github.com/radius-project/radius/pkg/ucp/credentials"
)

const (
	cleanupTestKey       = "prefix/0123456789abcdef0123456789abcdef01234567.tfstate"
	cleanupTestBucket    = "radius-states"
	cleanupTestAccount   = "radiusstates"
	cleanupTestContainer = "tfstate"
	cleanupTestETag      = "\"etag-v1\""
	cleanupTestETagV2    = "\"etag-v2\""
)

// emptyStateJSON is what Terraform leaves at the state key once destroy has removed everything it
// tracked. Cleanup may delete this.
const emptyStateJSON = `{"version":4,"terraform_version":"1.15.8","serial":3,"lineage":"a-lineage","outputs":{},"resources":[],"check_results":null}`

// liveStateJSON stands for state a concurrent writer committed after destroy released the Terraform
// lock. Cleanup must never delete this, even though the entity tag it reads back is perfectly valid.
const liveStateJSON = `{"version":4,"terraform_version":"1.15.8","serial":4,"lineage":"a-lineage","outputs":{},"resources":[{"mode":"managed","type":"random_id","name":"id","instances":[{"attributes":{"id":"abc"}}]}],"check_results":null}`

// outputOnlyStateJSON has no resources but still records an output, so it is not the state destroy
// left behind and must survive cleanup.
const outputOnlyStateJSON = `{"version":4,"terraform_version":"1.15.8","serial":4,"lineage":"a-lineage","outputs":{"endpoint":{"value":"https://example.invalid","type":"string"}},"resources":[],"check_results":null}`

// legacyStateJSON is a pre-v4 state tracking live infrastructure. Its resources live under a
// top-level "modules" array and it has no top-level "resources" or "outputs" key at all, so a check
// that treats missing keys as emptiness would delete the state of running infrastructure.
const legacyStateJSON = `{"version":3,"terraform_version":"0.11.14","serial":7,"lineage":"a-lineage","modules":[{"path":["root"],"resources":{"aws_s3_bucket.live":{"type":"aws_s3_bucket","primary":{"id":"live-bucket"}}}}]}`

// stateStore is the stub storage backing both cloud cleanup tests. It evaluates conditional deletes
// the way the real services do and records every request cleanup makes, so a test can assert that an
// unsafe delete was never attempted, not merely that one failed.
//
// All fields are guarded, because a read may mutate the stored object to simulate an interleaved
// writer while the test goroutine reads the recorded requests.
type stateStore struct {
	mu sync.Mutex

	// body is the state served on read, or empty to serve a missing object.
	body string

	// etag is returned with the body and is what a conditional delete must match.
	etag string

	// rotateBody and rotateETag replace the stored object immediately after a read is served, which
	// is how a writer committing between cleanup's read and its conditional delete is simulated.
	// The delete then arrives carrying the entity tag of the version that was read, and the handler's
	// own If-Match comparison rejects it, rather than the test asserting a canned precondition error.
	rotateBody string
	rotateETag string

	// getStatus and getErrorCode fail the read instead of serving the object.
	getStatus    int
	getErrorCode string

	// omitETag serves the body without an entity tag.
	omitETag bool

	// deleteConditions records the If-Match value of every delete the client issued.
	deleteConditions []string
}

// serveRead returns the object to serve and applies any rotation, under the lock.
func (s *stateStore) serveRead() (body string, etag string, found bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.body == "" {
		return "", "", false
	}
	body, etag = s.body, s.etag
	if s.rotateETag != "" {
		s.body, s.etag = s.rotateBody, s.rotateETag
	}
	return body, etag, true
}

// tryDelete records the condition the client sent and evaluates it against the current object the
// way the storage service would, so the precondition is genuinely exercised end to end.
func (s *stateStore) tryDelete(ifMatch string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteConditions = append(s.deleteConditions, ifMatch)
	if s.body == "" {
		return http.StatusNotFound
	}
	// An unconditional delete succeeds, as it would against the real service. Rejecting it here
	// would let a cleanup that dropped the precondition still look correct to these tests.
	if ifMatch != "" && ifMatch != s.etag {
		return http.StatusPreconditionFailed
	}
	s.body, s.etag = "", ""
	return http.StatusOK
}

func (s *stateStore) deletes() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.deleteConditions...)
}

// s3StubHandler serves the minimal S3 REST surface cleanup uses, so the AWS SDK builds, signs, and
// parses real requests and responses rather than a hand-written fake standing in for the client.
func (s *stateStore) s3StubHandler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Report a mismatch with Errorf rather than require: this runs on the server's goroutine,
		// where FailNow would Goexit the handler and surface as a connection error instead.
		if r.URL.Path != "/"+cleanupTestBucket+"/"+cleanupTestKey {
			t.Errorf("unexpected S3 request path %q", r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		switch r.Method {
		case http.MethodGet:
			if s.getStatus != 0 {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(s.getStatus)
				_, _ = w.Write([]byte(`<Error><Code>` + s.getErrorCode + `</Code><Message>stub</Message></Error>`))
				return
			}
			body, etag, found := s.serveRead()
			if !found {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`<Error><Code>NoSuchKey</Code><Message>missing</Message></Error>`))
				return
			}
			if !s.omitETag {
				w.Header().Set("ETag", etag)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(body))
		case http.MethodDelete:
			switch s.tryDelete(r.Header.Get("If-Match")) {
			case http.StatusNotFound:
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`<Error><Code>NoSuchKey</Code><Message>missing</Message></Error>`))
			case http.StatusPreconditionFailed:
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusPreconditionFailed)
				_, _ = w.Write([]byte(`<Error><Code>PreconditionFailed</Code><Message>etag mismatch</Message></Error>`))
			default:
				w.WriteHeader(http.StatusNoContent)
			}
		default:
			t.Errorf("unexpected S3 request method %q", r.Method)
			w.WriteHeader(http.StatusBadRequest)
		}
	})
}

// azureStubHandler serves the blob surface cleanup uses plus the Entra token endpoint, because the
// transport below keeps the credential's own traffic inside this server too.
func (s *stateStore) azureStubHandler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		blobPath := "/" + cleanupTestAccount + "/" + cleanupTestContainer + "/" + cleanupTestKey
		if r.URL.Path != blobPath {
			// Everything that is not the blob itself is the credential discovering its endpoints and
			// fetching a token. Serving it here is what keeps the credential off the real network.
			w.Header().Set("Content-Type", "application/json")
			if strings.Contains(r.URL.Path, "openid-configuration") {
				// MSAL checks that the issuer matches the configured authority, so the document has
				// to name the real public-cloud authority. The transport still routes every request
				// built from it back to this server.
				issuer := "https://login.microsoftonline.com/registered-tenant/v2.0"
				_, _ = w.Write([]byte(fmt.Sprintf(
					`{"issuer":%q,"authorization_endpoint":%q,"token_endpoint":%q}`,
					issuer, issuer+"/authorize", issuer+"/token")))
				return
			}
			_, _ = w.Write([]byte(`{"token_type":"Bearer","expires_in":3599,"ext_expires_in":3599,"access_token":"stub-token"}`))
			return
		}

		switch r.Method {
		case http.MethodGet:
			if s.getStatus != 0 {
				w.Header().Set("x-ms-error-code", s.getErrorCode)
				w.WriteHeader(s.getStatus)
				return
			}
			body, etag, found := s.serveRead()
			if !found {
				w.Header().Set("x-ms-error-code", "BlobNotFound")
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if !s.omitETag {
				w.Header().Set("ETag", etag)
			}
			w.Header().Set("Content-Length", fmt.Sprint(len(body)))
			w.Header().Set("x-ms-blob-type", "BlockBlob")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(body))
		case http.MethodDelete:
			switch s.tryDelete(r.Header.Get("If-Match")) {
			case http.StatusNotFound:
				w.Header().Set("x-ms-error-code", "BlobNotFound")
				w.WriteHeader(http.StatusNotFound)
			case http.StatusPreconditionFailed:
				w.Header().Set("x-ms-error-code", "ConditionNotMet")
				w.WriteHeader(http.StatusPreconditionFailed)
			default:
				w.WriteHeader(http.StatusAccepted)
			}
		default:
			t.Errorf("unexpected blob request method %q", r.Method)
			w.WriteHeader(http.StatusBadRequest)
		}
	})
}

// stubTransport sends every azcore request to the stub server, including the credential's token
// request, which would otherwise leave the test and reach the real Entra endpoint.
type stubTransport struct {
	client *http.Client
	target *url.URL
}

func (t *stubTransport) Do(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = t.target.Scheme
	req.URL.Host = t.target.Host
	req.Host = t.target.Host
	return t.client.Do(req)
}

func s3CleanupExecutor(t *testing.T, store *stateStore) (executor, *datamodel.TerraformBackend) {
	t.Helper()
	server := httptest.NewServer(store.s3StubHandler(t))
	t.Cleanup(server.Close)

	e := executor{
		awsCredentials:        &backendCredentialStub[credentials.AWSCredential]{value: backendTestAWSCredential(false)},
		stateEndpointOverride: server.URL,
	}
	return e, &datamodel.TerraformBackend{Type: backends.BackendS3, Bucket: cleanupTestBucket, Region: "us-west-2", KeyPrefix: "prefix"}
}

func azureCleanupExecutor(t *testing.T, store *stateStore) (executor, *datamodel.TerraformBackend) {
	t.Helper()
	// TLS, because azcore refuses to attach a credential to a plain HTTP request.
	server := httptest.NewTLSServer(store.azureStubHandler(t))
	t.Cleanup(server.Close)

	target, err := url.Parse(server.URL)
	require.NoError(t, err)

	e := executor{
		azureCredentials:      &backendCredentialStub[credentials.AzureCredential]{value: backendTestAzureCredential(false)},
		stateEndpointOverride: server.URL,
		stateTransport:        &stubTransport{client: server.Client(), target: target},
	}
	return e, &datamodel.TerraformBackend{Type: backends.BackendAzureRM, StorageAccountName: cleanupTestAccount, ContainerName: cleanupTestContainer, KeyPrefix: "prefix"}
}

// TestCloudStateCleanupAgainstStorageAPIs drives the real S3 and Azure Blob SDKs so the read,
// emptiness check, and conditional delete are exercised as actual HTTP exchanges. The stub evaluates
// If-Match itself, so the precondition is genuinely tested rather than asserted from a canned error.
//
// The "written before the read" cases are the reason cleanup reads the object body: an entity tag
// read alone cannot tell those versions from the empty one destroy wrote, so it would capture them
// and then delete them successfully. Each asserts that no delete was issued at all, not just that
// one failed.
func TestCloudStateCleanupAgainstStorageAPIs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string

		// body and etag are what the stub serves on read. rotateBody and rotateETag replace them
		// immediately after that read, simulating a writer that commits before the delete arrives.
		// getStatus/getErrorCode fail the read instead, and omitETag drops the entity tag. They are
		// plain fields rather than a stateStore so the table carries no mutex.
		body         string
		etag         string
		rotateBody   string
		rotateETag   string
		getStatus    int
		getErrorCode string
		omitETag     bool

		// expectedErr is the sentinel cleanup must return, or nil when the delete should succeed.
		expectedErr error

		// expectUnclassified marks outcomes that must surface as a plain cleanup failure, belonging
		// to none of the sentinels, so a real error is never silently treated as a safe outcome.
		expectUnclassified bool

		// expectDelete says whether cleanup may issue a delete at all.
		expectDelete bool
	}{
		{
			name:         "deletes the empty state destroy left",
			body:         emptyStateJSON,
			etag:         cleanupTestETag,
			expectDelete: true,
		},
		{
			name:         "keeps live state written before the read",
			body:         liveStateJSON,
			etag:         cleanupTestETag,
			expectedErr:  errStateNotEmptyDuringCleanup,
			expectDelete: false,
		},
		{
			name:         "keeps output-only state written before the read",
			body:         outputOnlyStateJSON,
			etag:         cleanupTestETag,
			expectedErr:  errStateNotEmptyDuringCleanup,
			expectDelete: false,
		},
		{
			// A pre-v4 state has no top-level resources or outputs key, so deciding emptiness from
			// the absence of those keys would delete the state of running infrastructure.
			name:         "keeps an unrecognized state format tracking live resources",
			body:         legacyStateJSON,
			etag:         cleanupTestETag,
			expectedErr:  errStateUnverifiableDuringCleanup,
			expectDelete: false,
		},
		{
			name:         "keeps an unrelated JSON document",
			body:         `{"not":"a terraform state"}`,
			etag:         cleanupTestETag,
			expectedErr:  errStateUnverifiableDuringCleanup,
			expectDelete: false,
		},
		{
			name:         "keeps an oversized state object",
			body:         `{"version":4,"padding":"` + strings.Repeat("x", maxEmptyStateBytes) + `"}`,
			etag:         cleanupTestETag,
			expectedErr:  errStateUnverifiableDuringCleanup,
			expectDelete: false,
		},
		{
			name:         "keeps a state object served without an entity tag",
			body:         emptyStateJSON,
			etag:         cleanupTestETag,
			omitETag:     true,
			expectedErr:  errStateUnverifiableDuringCleanup,
			expectDelete: false,
		},
		{
			// The stub rotates the object after serving the read, so the delete arrives carrying the
			// entity tag of the version cleanup verified and the stub's own If-Match comparison
			// rejects it. The object left behind is the live state the other writer committed.
			name:         "keeps state rewritten between the read and the delete",
			body:         emptyStateJSON,
			etag:         cleanupTestETag,
			rotateBody:   liveStateJSON,
			rotateETag:   cleanupTestETagV2,
			expectedErr:  errStateModifiedDuringCleanup,
			expectDelete: true,
		},
		{
			name:         "treats a missing state object as already absent",
			expectedErr:  errStateAlreadyAbsent,
			expectDelete: false,
		},
		{
			name:               "reports a denied read as a cleanup failure",
			getStatus:          http.StatusForbidden,
			getErrorCode:       "AuthorizationPermissionMismatch",
			expectUnclassified: true,
			expectDelete:       false,
		},
	}

	for _, tc := range cases {
		for _, cloud := range []string{backends.BackendS3, backends.BackendAzureRM} {
			t.Run(cloud+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				store := &stateStore{
					body: tc.body, etag: tc.etag,
					rotateBody: tc.rotateBody, rotateETag: tc.rotateETag,
					getStatus: tc.getStatus, getErrorCode: tc.getErrorCode,
					omitETag: tc.omitETag,
				}

				var e executor
				var settings *datamodel.TerraformBackend
				if cloud == backends.BackendS3 {
					e, settings = s3CleanupExecutor(t, store)
				} else {
					e, settings = azureCleanupExecutor(t, store)
				}

				err := e.deleteCloudStateObject(t.Context(), settings, backends.CloudBackendAuth{}, cleanupTestKey)
				switch {
				case tc.expectUnclassified:
					require.Error(t, err)
					for _, sentinel := range []error{errStateAlreadyAbsent, errStateNotEmptyDuringCleanup, errStateModifiedDuringCleanup, errStateUnverifiableDuringCleanup} {
						require.NotErrorIs(t, err, sentinel, "a real failure must not be classified as a safe outcome")
					}
				case tc.expectedErr != nil:
					require.ErrorIs(t, err, tc.expectedErr)
				default:
					require.NoError(t, err)
				}

				deletes := store.deletes()
				if tc.expectDelete {
					require.Len(t, deletes, 1, "cleanup should have issued exactly one conditional delete")
					require.Equal(t, cleanupTestETag, deletes[0], "the delete must be conditional on the entity tag of the version that was read")
				} else {
					require.Empty(t, deletes, "cleanup must not attempt a delete it cannot prove is safe")
				}
			})
		}
	}
}

// TestVerifyStateIsEmpty pins the decision that makes cleanup safe against a writer that committed
// before the read. Anything not provably the empty state destroy wrote must be kept.
func TestVerifyStateIsEmpty(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string

		// expectedErr is nil when the state is provably the empty state destroy wrote, the not-empty
		// sentinel when it provably still tracks something, and the unverifiable sentinel when
		// cleanup cannot tell. Only the first of those permits a delete.
		expectedErr error
	}{
		{name: "empty state", body: emptyStateJSON},
		{name: "v4 state with no resources or outputs key", body: `{"version":4,"serial":1}`},
		{name: "state with resources", body: liveStateJSON, expectedErr: errStateNotEmptyDuringCleanup},
		{name: "state with outputs only", body: outputOnlyStateJSON, expectedErr: errStateNotEmptyDuringCleanup},
		// Each of these parses cleanly with resources and outputs both zero, so an emptiness check
		// shaped only like the v4 schema would call every one of them empty and delete it.
		{name: "pre-v4 state tracking live resources", body: legacyStateJSON, expectedErr: errStateUnverifiableDuringCleanup},
		{name: "future state version", body: `{"version":5,"serial":1}`, expectedErr: errStateUnverifiableDuringCleanup},
		{name: "json null", body: "null", expectedErr: errStateUnverifiableDuringCleanup},
		{name: "empty json object", body: "{}", expectedErr: errStateUnverifiableDuringCleanup},
		{name: "unrelated json document", body: `{"foo":1}`, expectedErr: errStateUnverifiableDuringCleanup},
		{name: "oversized body", body: strings.Repeat("x", maxEmptyStateBytes+1), expectedErr: errStateUnverifiableDuringCleanup},
		{name: "unparseable body", body: "not json", expectedErr: errStateUnverifiableDuringCleanup},
		{name: "empty body", body: "", expectedErr: errStateUnverifiableDuringCleanup},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := verifyStateIsEmpty(strings.NewReader(tc.body))
			if tc.expectedErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.expectedErr)
			// The sentinels drive different operator guidance, so one must never stand in for another.
			for _, other := range []error{errStateNotEmptyDuringCleanup, errStateUnverifiableDuringCleanup, errStateModifiedDuringCleanup, errStateAlreadyAbsent} {
				if other != tc.expectedErr {
					require.NotErrorIs(t, err, other)
				}
			}
		})
	}
}

// TestCloudStateCleanupLogsRetainedState checks the message an operator actually sees for each
// outcome that leaves the object in place. Cleanup is best effort, so the log is the only signal,
// and it has to say which outcome occurred: a state deliberately retained for safety must not be
// reported as a leftover the operator should go and delete by hand.
func TestCloudStateCleanupLogsRetainedState(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string

		// contains is the phrase identifying this outcome in the log.
		contains string

		// allowManualRemoval says whether advising a manual delete is correct for this outcome.
		allowManualRemoval bool
	}{
		{
			name:     "live state is retained deliberately",
			body:     liveStateJSON,
			contains: "still tracks resources or outputs",
		},
		{
			name:     "unverifiable state is retained deliberately",
			body:     legacyStateJSON,
			contains: "could not be verified as empty",
		},
		{
			name:               "a missing object is reported as nothing deleted",
			body:               "",
			contains:           "it was already absent",
			allowManualRemoval: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := &stateStore{body: tc.body, etag: cleanupTestETag}
			e, settings := s3CleanupExecutor(t, store)

			var logs strings.Builder
			ctx := logr.NewContext(t.Context(), funcr.New(func(prefix, args string) {
				logs.WriteString(prefix + args)
			}, funcr.Options{}))

			e.deleteCloudState(ctx, settings, backends.CloudBackendAuth{}, cleanupTestKey)

			require.Contains(t, logs.String(), tc.contains)
			// The state key is the only way an operator can map the message back to a resource.
			require.Contains(t, logs.String(), cleanupTestKey)
			if !tc.allowManualRemoval {
				require.NotContains(t, logs.String(), "can be removed manually",
					"cleanup must not advise deleting an object it just refused to delete")
			}
			require.Empty(t, store.deletes())
		})
	}
}
