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
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func Test_waitForPostgreSQL(t *testing.T) {
	t.Parallel()

	connRefused := errors.New("dial tcp: connect: connection refused")

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

			err := waitForPostgreSQL(context.Background(), ping, tt.timeout, time.Millisecond, 5*time.Millisecond)
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

		err := waitForPostgreSQL(context.Background(), func(context.Context) error { return connRefused }, 50*time.Millisecond, time.Millisecond, 5*time.Millisecond)
		require.ErrorContains(t, err, "timed out")
	})

	t.Run("cancelled context", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := waitForPostgreSQL(ctx, func(context.Context) error { return connRefused }, time.Minute, time.Millisecond, time.Millisecond)
		require.Error(t, err)
	})
}
