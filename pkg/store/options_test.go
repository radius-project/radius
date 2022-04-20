// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package store

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQueryOptions(t *testing.T) {
	opts := []QueryOptions{}
	opts = append(opts, WithPaginationToken("token"))
	cfg := NewQueryConfig(opts...)
	require.Equal(t, "token", cfg.PaginationToken)
	require.Equal(t, 0, cfg.QueryCount)

	opts = append(opts, WithQueryCount(20))
	cfg = NewQueryConfig(opts...)
	require.Equal(t, "token", cfg.PaginationToken)
	require.Equal(t, 20, cfg.QueryCount)
}

func TestSaveOptions(t *testing.T) {
	opts := []SaveOptions{}
	opts = append(opts, WithETag("tag"))
	cfg := NewSaveConfig(opts...)
	require.Equal(t, "tag", cfg.ETag)
}
