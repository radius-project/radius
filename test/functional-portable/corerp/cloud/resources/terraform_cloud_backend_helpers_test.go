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

package resource_test

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/stretchr/testify/require"
)

func TestTerraformCloudAzureReadinessErrors(t *testing.T) {
	type readinessCase struct {
		name       string
		err        error
		retry      bool
		credential bool
	}
	tests := []readinessCase{
		{"propagating permission", &azcore.ResponseError{StatusCode: 403, ErrorCode: "AuthorizationPermissionMismatch"}, true, false},
		{"propagating authorization", &azcore.ResponseError{StatusCode: 403, ErrorCode: "AuthorizationFailure"}, true, false},
		{"invalid authentication", &azcore.ResponseError{StatusCode: 403, ErrorCode: "AuthenticationFailed"}, false, false},
		{"unknown forbidden", &azcore.ResponseError{StatusCode: 403, ErrorCode: "do-not-log"}, false, false},
		{"not found", &azcore.ResponseError{StatusCode: 404}, false, false},
		{"invalid request", &azcore.ResponseError{StatusCode: 400}, false, false},
		{"not authenticated", &azcore.ResponseError{StatusCode: 401}, false, false},
		{"non-retryable server error", &azcore.ResponseError{StatusCode: 501}, false, false},
		{"new account DNS", &net.DNSError{IsNotFound: true, Name: "do-not-log"}, true, false},
		{"temporary DNS", &net.DNSError{IsTemporary: true}, true, false},
		{"DNS timeout", &net.DNSError{IsTimeout: true}, true, false},
		{"permanent DNS error", &net.DNSError{Err: "do-not-log"}, false, false},
		{"wrapped connection refused", &url.Error{Op: "Get", URL: "do-not-log", Err: &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}}, true, false},
		{"connection reset", &net.OpError{Op: "read", Err: syscall.ECONNRESET}, true, false},
		{"network timeout", &net.OpError{Op: "read", Err: syscall.ETIMEDOUT}, true, false},
		{"EOF", io.EOF, true, false},
		{"certificate failure", x509.UnknownAuthorityError{}, false, false},
		{"other failure", errors.New("do-not-log"), false, false},
		{"credential unavailable", azidentity.NewCredentialUnavailableError("do-not-log AADSTS700016"), true, true},
		{"wrapped credential unavailable", fmt.Errorf("probe: %w", azidentity.NewCredentialUnavailableError("do-not-log")), true, true},
		{"authentication failed", &azidentity.AuthenticationFailedError{}, true, true},
		{"canceled", fmt.Errorf("do-not-log: %w", context.Canceled), false, false},
		{"deadline", fmt.Errorf("do-not-log: %w", context.DeadlineExceeded), false, false},
	}
	for _, status := range []int{408, 429, 500, 502, 503, 504} {
		tests = append(tests, readinessCase{
			name:  fmt.Sprintf("retryable HTTP %d", status),
			err:   &azcore.ResponseError{StatusCode: status},
			retry: true,
		})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			classification := azureBlobReadinessError(tt.err)
			require.Equal(t, tt.retry, classification.retry)
			require.Equal(t, tt.credential, classification.credential)
			require.NotEmpty(t, classification.observation)
			require.NotContains(t, classification.observation, "do-not-log")
		})
	}
}

func TestTerraformCloudAzureReadinessPolling(t *testing.T) {
	t.Run("transient errors then ready", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			start := time.Now()
			attempts := 0
			err := waitForAzureBlobAccess(ctx, func(context.Context) error {
				attempts++
				switch attempts {
				case 1:
					return &net.DNSError{IsNotFound: true}
				case 2:
					return &azcore.ResponseError{StatusCode: 403, ErrorCode: "AuthorizationPermissionMismatch"}
				default:
					return nil
				}
			})
			require.NoError(t, err)
			require.Equal(t, 3, attempts)
			require.Equal(t, 10*time.Second, time.Since(start))
		})
	})
	t.Run("permanent failure is immediate and redacted", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			start := time.Now()
			attempts := 0
			err := waitForAzureBlobAccess(t.Context(), func(context.Context) error {
				attempts++
				return &azcore.ResponseError{StatusCode: 400, ErrorCode: "do-not-log"}
			})
			require.ErrorContains(t, err, "HTTP 400")
			require.NotContains(t, err.Error(), "do-not-log")
			require.Equal(t, 1, attempts)
			require.Zero(t, time.Since(start))
		})
	})
	t.Run("credential failures are retried then reported with the Entra code", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			attempts := 0
			err := waitForAzureBlobAccess(ctx, func(context.Context) error {
				attempts++
				return azidentity.NewCredentialUnavailableError("do-not-log AADSTS700016")
			})
			require.ErrorContains(t, err, "credential acquisition failed (AADSTS700016)")
			require.NotContains(t, err.Error(), "do-not-log")
			// Bounded, so a real misconfiguration does not consume the whole propagation budget.
			require.Equal(t, azureCredentialRetries+1, attempts)
		})
	})
	t.Run("a transient credential failure still succeeds", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			attempts := 0
			err := waitForAzureBlobAccess(ctx, func(context.Context) error {
				attempts++
				if attempts == 1 {
					return azidentity.NewCredentialUnavailableError("do-not-log")
				}
				return nil
			})
			require.NoError(t, err)
			require.Equal(t, 2, attempts)
		})
	})
	t.Run("deadline preserves last observation", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
			defer cancel()
			start := time.Now()
			attempts := 0
			err := waitForAzureBlobAccess(ctx, func(context.Context) error {
				attempts++
				return &azcore.ResponseError{StatusCode: 429}
			})
			require.ErrorIs(t, err, context.DeadlineExceeded)
			require.ErrorContains(t, err, "HTTP 429")
			require.Equal(t, 3, attempts)
			require.Equal(t, 12*time.Second, time.Since(start))
		})
	})
	for _, phase := range []string{"before probe", "between probes", "in flight"} {
		t.Run("cancellation "+phase, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				if phase == "before probe" {
					cancel()
				} else {
					go func() {
						time.Sleep(2 * time.Second)
						cancel()
					}()
				}
				attempts := 0
				err := waitForAzureBlobAccess(ctx, func(ctx context.Context) error {
					attempts++
					if phase == "in flight" {
						<-ctx.Done()
						return fmt.Errorf("do-not-log: %w", ctx.Err())
					}
					return &azcore.ResponseError{StatusCode: 503}
				})
				require.ErrorIs(t, err, context.Canceled)
				require.NotContains(t, err.Error(), "do-not-log")
				if phase == "before probe" {
					require.Zero(t, attempts)
				} else {
					require.Equal(t, 1, attempts)
				}
			})
		})
	}
	t.Run("already expired deadline does not probe", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
			defer cancel()
			err := waitForAzureBlobAccess(ctx, func(context.Context) error {
				t.Fatal("probe must not run after deadline")
				return nil
			})
			require.ErrorIs(t, err, context.DeadlineExceeded)
		})
	})
}

func TestTerraformCloudRadiusCleanup(t *testing.T) {
	for _, status := range []int{0, 404, 403, 409} {
		t.Run(fmt.Sprintf("HTTP %d", status), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
				defer cancel()
				attempts := 0
				err := deleteRadiusAfterUpdate(ctx, func(context.Context) error {
					attempts++
					if status == 0 {
						return nil
					}
					return &azcore.ResponseError{StatusCode: status}
				})
				switch status {
				case 0, 404:
					require.NoError(t, err)
					require.Equal(t, 1, attempts)
				case 403:
					require.Error(t, err)
					require.Equal(t, 1, attempts)
				case 409:
					require.ErrorIs(t, err, context.DeadlineExceeded)
					require.ErrorContains(t, err, "409")
					require.Equal(t, 3, attempts)
				}
			})
		})
	}
	t.Run("Updating finishes before cleanup deadline", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			attempts := 0
			err := deleteRadiusAfterUpdate(ctx, func(context.Context) error {
				attempts++
				if attempts < 3 {
					return &azcore.ResponseError{StatusCode: 409}
				}
				return nil
			})
			require.NoError(t, err)
			require.Equal(t, 3, attempts)
		})
	})
	t.Run("canceled cleanup does not delete", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		err := deleteRadiusAfterUpdate(ctx, func(context.Context) error {
			t.Fatal("delete must not run after cancellation")
			return nil
		})
		require.ErrorIs(t, err, context.Canceled)
	})
}

// These helper tests need neither TestOptions nor cloud/Kubernetes credentials.
func TestTerraformCloudStateHelpers(t *testing.T) {
	t.Run("key identity and prefix", func(t *testing.T) {
		resource := "/planes/radius/local/resourceGroups/test/providers/Test.CloudBackend/stateResource/a"
		key := expectedCloudStateKey("e2e", "env", "app", resource)
		require.Regexp(t, `^e2e/[0-9a-f]{40}\.tfstate$`, key)
		require.Equal(t, key, expectedCloudStateKey("e2e", "ENV", "APP", strings.ToUpper(resource)))
		require.NotEqual(t, key, expectedCloudStateKey("e2e", "env", "app", resource+"b"))
		require.NotEqual(t, key, expectedCloudStateKey("e2e", "other-env", "app", resource))
		require.NotEqual(t, key, expectedCloudStateKey("e2e", "env", "other-app", resource))
		require.Equal(t, "custom/nested/"+strings.TrimPrefix(key, "e2e/"), expectedCloudStateKey("custom/nested", "env", "app", resource))
	})
	t.Run("parse without exposing state", func(t *testing.T) {
		body := []byte(`{"version":4,"lineage":"lineage-a","serial":2,"resources":[{"mode":"managed","type":"aws_s3_bucket","instances":[{"attributes":{"id":"bucket-a","tags":{"revision":"two"},"secret":"do-not-log"}}]}]}`)
		state, err := decodeCloudState(body)
		require.NoError(t, err)
		require.Equal(t, 4, state.Version)
		require.Equal(t, uint64(2), state.Serial)
		require.Equal(t, "bucket-a", state.Resources[0].Instances[0].Attributes.ID)
		require.Equal(t, "two", state.Resources[0].Instances[0].Attributes.Tags["revision"])
		_, err = decodeCloudState([]byte(`{"serial":"do-not-log"}`))
		require.EqualError(t, err, "invalid Terraform state JSON (content redacted)")
		empty, err := decodeCloudState([]byte(`{"version":4,"lineage":"lineage-a","serial":3,"resources":[]}`))
		require.NoError(t, err)
		require.Zero(t, len(empty.Resources))
		require.NotEqual(t, state.digest, empty.digest)
	})
	t.Run("bounded read", func(t *testing.T) {
		_, err := readCloudStateBody(io.NopCloser(strings.NewReader(strings.Repeat("x", 1024*1024+1))))
		require.ErrorContains(t, err, "exceeds")
	})
	t.Run("principal token errors are redacted", func(t *testing.T) {
		for _, token := range []string{"do-not-log", "header.!.signature", "header." + base64.RawURLEncoding.EncodeToString([]byte(`{"oid":"do-not-log"}`)) + ".signature"} {
			_, err := azureTokenPrincipal(token)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "do-not-log")
		}
		objectID := "9e5c389d-996e-4724-a3d6-c7a9bcd2f1a8"
		payload := base64.RawURLEncoding.EncodeToString([]byte(`{"oid":"` + objectID + `"}`))
		principal, err := azureTokenPrincipal("header." + payload + ".signature")
		require.NoError(t, err)
		require.Equal(t, objectID, principal)
	})
}

func TestTerraformCloudAzureFederatedAssertion(t *testing.T) {
	t.Parallel()

	t.Run("requests a fresh token with the exchange audience", func(t *testing.T) {
		t.Parallel()
		var gotAudience, gotAuthorization string
		var calls int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			gotAudience = r.URL.Query().Get("audience")
			gotAuthorization = r.Header.Get("Authorization")
			require.Equal(t, "keep", r.URL.Query().Get("existing"))
			_, _ = io.WriteString(w, `{"value":"fresh-token"}`)
		}))
		defer server.Close()

		assertion, err := azureFederatedAssertion(t.Context(), server.URL+"?existing=keep", "request-token")
		require.NoError(t, err)
		require.Equal(t, "fresh-token", assertion)
		require.Equal(t, azureEntraExchangeAudience, gotAudience)
		require.Equal(t, "Bearer request-token", gotAuthorization)
		require.Equal(t, 1, calls)
	})

	t.Run("each acquisition mints a new assertion", func(t *testing.T) {
		t.Parallel()
		var calls int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			_, _ = fmt.Fprintf(w, `{"value":"token-%d"}`, calls)
		}))
		defer server.Close()

		first, err := azureFederatedAssertion(t.Context(), server.URL, "request-token")
		require.NoError(t, err)
		second, err := azureFederatedAssertion(t.Context(), server.URL, "request-token")
		require.NoError(t, err)
		require.Equal(t, "token-1", first)
		require.Equal(t, "token-2", second)
	})

	t.Run("failures are reported without leaking the response", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, "do-not-log")
		}))
		defer server.Close()

		_, err := azureFederatedAssertion(t.Context(), server.URL, "do-not-log")
		require.Error(t, err)
		require.Contains(t, err.Error(), "HTTP 403")
		require.NotContains(t, err.Error(), "do-not-log")
	})

	t.Run("an empty token is rejected", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"value":""}`)
		}))
		defer server.Close()

		_, err := azureFederatedAssertion(t.Context(), server.URL, "request-token")
		require.ErrorContains(t, err, "contained no token")
	})

	t.Run("a malformed response is rejected without leaking the body", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, "do-not-log")
		}))
		defer server.Close()

		_, err := azureFederatedAssertion(t.Context(), server.URL, "request-token")
		require.Error(t, err)
		require.NotContains(t, err.Error(), "do-not-log")
	})
}

func TestTerraformCloudAzureCLIIdentity(t *testing.T) {
	t.Parallel()

	t.Run("reads the tenant and service principal", func(t *testing.T) {
		t.Parallel()
		tenant, client, err := azureCLIIdentity([]byte(`{
			"tenantId": "tenant-1",
			"user": {"name": "client-1", "type": "servicePrincipal"}
		}`))
		require.NoError(t, err)
		require.Equal(t, "tenant-1", tenant)
		require.Equal(t, "client-1", client)
	})

	t.Run("rejects a user login", func(t *testing.T) {
		t.Parallel()
		_, _, err := azureCLIIdentity([]byte(`{
			"tenantId": "tenant-1",
			"user": {"name": "someone@example.com", "type": "user"}
		}`))
		require.ErrorContains(t, err, "not a service principal")
	})

	t.Run("rejects an incomplete record", func(t *testing.T) {
		t.Parallel()
		_, _, err := azureCLIIdentity([]byte(`{"user": {"name": "", "type": "servicePrincipal"}}`))
		require.ErrorContains(t, err, "no tenant or service principal")
	})

	t.Run("a malformed record is rejected without leaking it", func(t *testing.T) {
		t.Parallel()
		_, _, err := azureCLIIdentity([]byte(`{"subscriptionId": "do-not-log"`))
		require.Error(t, err)
		require.NotContains(t, err.Error(), "do-not-log")
	})
}
