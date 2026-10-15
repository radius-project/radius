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

package databaseprovider

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func Test_waitForPostgreSQL(t *testing.T) {
	t.Parallel()

	connRefused := &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}

	tests := []struct {
		name         string
		errs         []error
		timeout      time.Duration
		wantErr      bool
		wantAttempts int
	}{
		{name: "ready immediately", errs: nil, timeout: time.Second, wantAttempts: 1},
		{name: "ready after connection errors", errs: []error{connRefused, connRefused, connRefused}, timeout: 5 * time.Second, wantAttempts: 4},
		{name: "ready after server starting up", errs: []error{&pgconn.PgError{Code: postgresCannotConnectNow}}, timeout: 5 * time.Second, wantAttempts: 2},
		{name: "auth failure is not retried", errs: []error{&pgconn.PgError{Code: "28P01"}}, timeout: 5 * time.Second, wantErr: true, wantAttempts: 1},
		{name: "certificate failure is not retried", errs: []error{x509.UnknownAuthorityError{}}, timeout: 5 * time.Second, wantErr: true, wantAttempts: 1},
		{name: "unknown error is not retried", errs: []error{errors.New("boom")}, timeout: 5 * time.Second, wantErr: true, wantAttempts: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			attempts := 0
			ping := func(ctx context.Context) error {
				attempts++
				if attempts <= len(tt.errs) {
					return tt.errs[attempts-1]
				}
				return nil
			}

			err := waitForPostgreSQL(context.Background(), ping, testWaitOptions(tt.timeout))
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			if tt.wantAttempts > 0 {
				require.Equal(t, tt.wantAttempts, attempts)
			}
		})
	}

	t.Run("times out", func(t *testing.T) {
		t.Parallel()

		err := waitForPostgreSQL(context.Background(), func(context.Context) error { return connRefused }, testWaitOptions(50*time.Millisecond))
		require.ErrorContains(t, err, "timed out")
	})

	t.Run("cancelled context", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := waitForPostgreSQL(ctx, func(context.Context) error { return connRefused }, testWaitOptions(time.Minute))
		require.ErrorIs(t, err, context.Canceled)
		require.NotContains(t, err.Error(), "timed out")
	})

	t.Run("hung attempt is bounded and retried", func(t *testing.T) {
		t.Parallel()

		attempts := 0
		ping := func(ctx context.Context) error {
			attempts++
			if attempts == 1 {
				<-ctx.Done()
				return ctx.Err()
			}
			return nil
		}
		opts := testWaitOptions(5 * time.Second)
		opts.attemptTimeout = 10 * time.Millisecond
		require.NoError(t, waitForPostgreSQL(context.Background(), ping, opts))
		require.Equal(t, 2, attempts)
	})
}

func Test_isRetryablePostgreSQLError(t *testing.T) {
	t.Parallel()

	dial := func(errno syscall.Errno) error {
		return fmt.Errorf("failed to connect: %w", &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", errno)})
	}

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "connection refused", err: dial(syscall.ECONNREFUSED), want: true},
		{name: "connection reset", err: dial(syscall.ECONNRESET), want: true},
		{name: "host unreachable", err: dial(syscall.EHOSTUNREACH), want: true},
		{name: "network unreachable", err: dial(syscall.ENETUNREACH), want: true},
		{name: "dns not found", err: &net.DNSError{Err: "no such host", Name: "database", IsNotFound: true}, want: true},
		{name: "attempt deadline", err: fmt.Errorf("ping: %w", context.DeadlineExceeded), want: true},
		{name: "unexpected eof", err: fmt.Errorf("read: %w", io.ErrUnexpectedEOF), want: true},
		{name: "server starting up", err: &pgconn.PgError{Code: postgresCannotConnectNow}, want: true},
		{name: "password authentication failed", err: &pgconn.PgError{Code: "28P01"}, want: false},
		{name: "permission denied", err: dial(syscall.EACCES), want: false},
		{name: "remote tls alert", err: &net.OpError{Op: "remote error", Err: errors.New("tls: bad certificate")}, want: false},
		{name: "unknown certificate authority", err: fmt.Errorf("tls: %w", x509.UnknownAuthorityError{}), want: false},
		{name: "cancelled", err: context.Canceled, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, isRetryablePostgreSQLError(tt.err))
		})
	}
}

func testWaitOptions(timeout time.Duration) postgresWaitOptions {
	return postgresWaitOptions{
		timeout:        timeout,
		attemptTimeout: time.Second,
		initialDelay:   time.Millisecond,
		maxDelay:       5 * time.Millisecond,
	}
}
