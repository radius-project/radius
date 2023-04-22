// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radinit

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseRepoPathForMetadata(t *testing.T) {
	t.Run("Successfully returns metadata", func(t *testing.T) {
		link, provider := parseRepoPathForMetadata("recipes/linkName/providerName")
		require.Equal(t, "linkName", link)
		require.Equal(t, "providerName", provider)
	})

	tests := []struct {
		name             string
		repo             string
		expectedLink     string
		expectedProvider string
	}{
		{
			"Repo isn't related to recipes",
			"randomRepo",
			"",
			"",
		},
		{
			"Repo for recipes doesn't have link and provider names",
			"recipes/noLinkAndProvider",
			"",
			"",
		},
		{
			"Repo for recipes has extra path component",
			"recipes/link/provider/randomValue",
			"",
			"",
		},
		{
			"Repo name has a link and no provider",
			"recipes/linkName/",
			"linkName",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			link, provider := parseRepoPathForMetadata(tt.repo)
			require.Equal(t, tt.expectedLink, link)
			require.Equal(t, tt.expectedProvider, provider)
		})
	}
}

func TestFindHighestVersion(t *testing.T) {
	t.Run("Max version is returned when tags are int/float values with float max", func(t *testing.T) {
		versions := []string{"1", "2", "3", "4.0"}
		max, err := findHighestVersion(versions)
		require.NoError(t, err)
		require.Equal(t, max, 4.0)
	})
	t.Run("Max version is returned when tags are int/float values with int max", func(t *testing.T) {
		versions := []string{"1.0", "2.0", "3.0", "4"}
		max, err := findHighestVersion(versions)
		require.NoError(t, err)
		require.Equal(t, max, 4.0)
	})
	t.Run("Version tags are not all float values", func(t *testing.T) {
		versions := []string{"1.0", "otherTag", "3.0", "4.0"}
		_, err := findHighestVersion(versions)
		require.ErrorContains(t, err, "unable to convert tag otherTag into valid version")
	})
}
