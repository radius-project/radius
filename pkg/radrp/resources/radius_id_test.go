// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resources

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func parseOrPanic(id string) ResourceID {
	p, err := Parse(id)
	if err != nil {
		panic(err)
	}

	return p
}

func Test_Application_Invalid(t *testing.T) {
	values := []ResourceID{
		parseOrPanic("/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders"),
		parseOrPanic("/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/foo"),
		parseOrPanic("/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/foo/Applications"),
		parseOrPanic("/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/foo/Components"),
		parseOrPanic("/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/foo/Components/baz"),
		parseOrPanic("/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/bar/foo/Applications/baz"),
	}

	for i, v := range values {
		t.Run(fmt.Sprintf("%d: %v", i, v.ID), func(t *testing.T) {
			app, err := v.Application()
			require.Error(t, err, "should not have succeeded for %v: got %v", v.ID, app)
		})
	}
}

func Test_Application_Valid(t *testing.T) {
	values := []struct {
		ID       ResourceID
		Expected string
	}{
		{
			ID:       parseOrPanic("/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/a1"),
			Expected: "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/a1",
		},
		{
			ID:       parseOrPanic("/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.customProviders/resourceProviders/radius/applications/a1"),
			Expected: "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.customProviders/resourceProviders/radius/applications/a1",
		},
		{
			ID:       parseOrPanic("/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.customProviders/resourceProviders/radius/applications/a1/Components"),
			Expected: "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.customProviders/resourceProviders/radius/applications/a1",
		},
		{
			ID:       parseOrPanic("/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.customProviders/resourceProviders/radius/applications/a1/Components/c1"),
			Expected: "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.customProviders/resourceProviders/radius/applications/a1",
		},
	}

	for i, v := range values {
		t.Run(fmt.Sprintf("%d: %v", i, v.ID.ID), func(t *testing.T) {
			app, err := v.ID.Application()
			require.NoError(t, err, "should not have failed for %v", v.ID.ID)

			require.Equal(t, v.Expected, app.ID)
			require.Equal(t, "s1", app.SubscriptionID)
			require.Equal(t, "r1", app.ResourceGroup)

			require.Len(t, app.Types, 2)

			require.Equal(t, strings.ToLower(baseResourceType), strings.ToLower(app.Types[0].Type))
			require.Equal(t, "radius", app.Types[0].Name)

			require.Equal(t, strings.ToLower(applicationResourceType), strings.ToLower(app.Types[1].Type))
			require.Equal(t, "a1", app.Types[1].Name)
		})
	}
}
