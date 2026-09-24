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
	"net/url"
	"strings"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/stretchr/testify/require"
)

func TestTerraformCloudAzureReadinessErrors(t *testing.T) {
	tests := []struct {
		name  string
		err   error
		retry bool
	}{
		{"propagating permission", &azcore.ResponseError{StatusCode: 403, ErrorCode: "AuthorizationPermissionMismatch"}, true},
		{"propagating authorization", &azcore.ResponseError{StatusCode: 403, ErrorCode: "AuthorizationFailure"}, true},
		{"invalid authentication", &azcore.ResponseError{StatusCode: 403, ErrorCode: "AuthenticationFailed"}, false},
		{"unknown forbidden", &azcore.ResponseError{StatusCode: 403, ErrorCode: "do-not-log"}, false},
		{"not found", &azcore.ResponseError{StatusCode: 404}, false},
		{"invalid request", &azcore.ResponseError{StatusCode: 400}, false},
		{"not authenticated", &azcore.ResponseError{StatusCode: 401}, false},
		{"non-retryable server error", &azcore.ResponseError{StatusCode: 501}, false},
		{"new account DNS", &net.DNSError{IsNotFound: true, Name: "do-not-log"}, true},
		{"temporary DNS", &net.DNSError{IsTemporary: true}, true},
		{"DNS timeout", &net.DNSError{IsTimeout: true}, true},
		{"permanent DNS error", &net.DNSError{Err: "do-not-log"}, false},
		{"wrapped connection refused", &url.Error{Op: "Get", URL: "do-not-log", Err: &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}}, true},
		{"connection reset", &net.OpError{Op: "read", Err: syscall.ECONNRESET}, true},
		{"network timeout", &net.OpError{Op: "read", Err: syscall.ETIMEDOUT}, true},
		{"EOF", io.EOF, true},
		{"certificate failure", x509.UnknownAuthorityError{}, false},
		{"other failure", errors.New("do-not-log"), false},
		{"canceled", fmt.Errorf("do-not-log: %w", context.Canceled), false},
		{"deadline", fmt.Errorf("do-not-log: %w", context.DeadlineExceeded), false},
	}
	for _, status := range []int{408, 429, 500, 502, 503, 504} {
		tests = append(tests, struct {
			name  string
			err   error
			retry bool
		}{fmt.Sprintf("retryable HTTP %d", status), &azcore.ResponseError{StatusCode: status}, true})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retry, description := azureBlobReadinessError(tt.err)
			require.Equal(t, tt.retry, retry)
			require.NotEmpty(t, description)
			require.NotContains(t, description, "do-not-log")
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
		resource := "/planes/radius/local/resourceGroups/test/providers/Applications.Core/extenders/a"
		key := expectedCloudStateKey("radius", "env", "app", resource)
		require.Regexp(t, `^radius/[0-9a-f]{40}\.tfstate$`, key)
		require.Equal(t, key, expectedCloudStateKey("radius", "ENV", "APP", strings.ToUpper(resource)))
		require.NotEqual(t, key, expectedCloudStateKey("radius", "env", "app", resource+"b"))
		require.NotEqual(t, key, expectedCloudStateKey("radius", "other-env", "app", resource))
		require.NotEqual(t, key, expectedCloudStateKey("radius", "env", "other-app", resource))
		require.Equal(t, "custom/nested/"+strings.TrimPrefix(key, "radius/"), expectedCloudStateKey("custom/nested", "env", "app", resource))
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
