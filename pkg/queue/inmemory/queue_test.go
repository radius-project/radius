// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package inmemory

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnqueue(t *testing.T) {
	q := newInMemQueue()
	require.NotNil(t, q)
}
