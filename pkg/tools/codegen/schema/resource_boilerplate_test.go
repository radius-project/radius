// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package schema

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
)

func TestLoadResourcePathSchemaForType(t *testing.T) {
	expected, err := Load("testdata/resource_boilerplate.json")
	require.Nil(t, err)

	s, err := LoadResourceBoilerplateSchemaForType(newResourceInfo("radius.com.AwesomeThing", "#/definitions/AwesomeThingResource"))
	require.Nil(t, err)

	if diff := cmp.Diff(expected, s); diff != "" {
		t.Errorf("Unexpected diff (-want,+got): %s", diff)
	}
}
