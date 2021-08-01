// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package workloads

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_FindByLocalID(t *testing.T) {
	t.Run("Match found", func(t *testing.T) {
		id := "test-id"
		resources := []WorkloadResourceProperties{
			{
				LocalID: "A",
			},
			{
				Type:    "test-type",
				LocalID: "test-id",
			},
		}

		match, err := FindByLocalID(resources, id)
		require.NoError(t, err)
		require.Equal(t, "test-type", match.Type)
	})

	t.Run("Match not found", func(t *testing.T) {
		id := "test-id"
		resources := []WorkloadResourceProperties{
			{
				LocalID: "A",
			},
			{
				LocalID: "B",
			},
		}

		match, err := FindByLocalID(resources, id)
		require.Error(t, err)
		require.Nil(t, match)
		require.Equal(t, "cannot find a resource matching id test-id searched 2 resources: A B", err.Error())
	})
}
