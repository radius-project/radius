// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package outputresource

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Placeholder_GetValue(t *testing.T) {
	resource := map[string]interface{}{
		"foo": map[string]interface{}{
			"bar": "good",
		},
		"baz": "bad",
	}

	t.Run("invalid", func(t *testing.T) {
		placeholder := Placeholder{
			SourcePointer: "invalid//",
		}

		_, err := placeholder.GetValue(resource)
		require.Error(t, err)
	})

	t.Run("found", func(t *testing.T) {
		placeholder := Placeholder{
			SourcePointer: "/foo/bar",
		}

		value, err := placeholder.GetValue(resource)
		require.NoError(t, err)
		require.Equal(t, "good", value)
	})

	t.Run("missing", func(t *testing.T) {
		placeholder := Placeholder{
			SourcePointer: "/foo/baz",
		}

		_, err := placeholder.GetValue(resource)
		require.Error(t, err)
	})
}

func Test_Placeholder_SetValue(t *testing.T) {
	resource := map[string]interface{}{
		"foo": map[string]interface{}{
			"bar": "good",
		},
		"baz": "bad",
	}

	t.Run("invalid", func(t *testing.T) {
		placeholder := Placeholder{
			DestinationPointer: "invalid//",
		}

		err := placeholder.ApplyValue(resource, 3)
		require.Error(t, err)
	})

	t.Run("set-value", func(t *testing.T) {
		placeholder := Placeholder{
			DestinationPointer: "/foo/bar",
		}

		err := placeholder.ApplyValue(resource, "very-good")
		require.NoError(t, err)
		require.Equal(t, "very-good", resource["foo"].(map[string]interface{})["bar"])
	})

	t.Run("add-value", func(t *testing.T) {
		placeholder := Placeholder{
			DestinationPointer: "/foo/another",
		}

		err := placeholder.ApplyValue(resource, "very-good")
		require.NoError(t, err)
		require.Equal(t, "very-good", resource["foo"].(map[string]interface{})["another"])
	})

	t.Run("missing-intermediate", func(t *testing.T) {
		placeholder := Placeholder{
			DestinationPointer: "/foo/missing/another",
		}

		err := placeholder.ApplyValue(resource, "very-good")
		require.Error(t, err)
	})
}
