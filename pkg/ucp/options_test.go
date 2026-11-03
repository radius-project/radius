// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package ucp

import (
	"fmt"
	"testing"

	"github.com/radius-project/radius/pkg/ucp/authorization"
	"github.com/stretchr/testify/require"
)

func TestNewOptionsUserAuthorization(t *testing.T) {
	t.Parallel()
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			config, err := LoadConfig([]byte(fmt.Sprintf(`
databaseProvider:
  provider: inmemory
queueProvider:
  provider: inmemory
  name: test
ucp:
  kind: direct
  direct:
    endpoint: http://localhost:9000
userAuthorization:
  enabled: %t
`, enabled)))
			require.NoError(t, err)
			options, err := NewOptions(t.Context(), config)
			require.NoError(t, err)
			if !enabled {
				require.Nil(t, options.UserAuthorization)
				require.Equal(t, authorization.RuntimeNotConfigured, options.UserAuthorization.State().Status)
				return
			}
			require.NotNil(t, options.UserAuthorization)
			state := options.UserAuthorization.State()
			require.Equal(t, authorization.RuntimeUnavailable, state.Status)
			require.False(t, state.Running)
			require.Empty(t, state.LastKnownMode)
		})
	}
}
