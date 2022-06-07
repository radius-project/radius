// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package asyncoperation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTimeout(t *testing.T) {
	r := Request{}
	require.Equal(t, DefaultAsyncOperationTimeout, r.Timeout())

	testTimeout := time.Duration(200) * time.Minute
	r = Request{OperationTimeout: &testTimeout}
	require.Equal(t, testTimeout, r.Timeout())
}
