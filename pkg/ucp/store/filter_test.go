// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package store

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_MatchesFilters(t *testing.T) {
	type testcase struct {
		Description   string
		Obj           *Object
		Filters       []QueryFilter
		ExpectedMatch bool
	}

	type coolstruct struct {
		Value string `json:"value"`
	}

	cases := []testcase{
		// Any object matches no filters
		{
			Description:   "empty",
			Obj:           &Object{},
			Filters:       []QueryFilter{},
			ExpectedMatch: true,
		},

		// We can work with structs
		{
			Description:   "struct_match",
			Obj:           &Object{Data: &coolstruct{Value: "cool"}},
			Filters:       []QueryFilter{{Field: "value", Value: "cool"}},
			ExpectedMatch: true,
		},
		{
			Description:   "struct_not_match",
			Obj:           &Object{Data: &coolstruct{Value: "cool"}},
			Filters:       []QueryFilter{{Field: "value", Value: "uncool"}},
			ExpectedMatch: false,
		},

		// We can work with maps of different types
		{
			Description:   "map_string_interface_match",
			Obj:           &Object{Data: map[string]any{"value": "cool"}},
			Filters:       []QueryFilter{{Field: "value", Value: "cool"}},
			ExpectedMatch: true,
		},
		{
			Description:   "map_string_interface_match_not_match",
			Obj:           &Object{Data: map[string]any{"value": "cool"}},
			Filters:       []QueryFilter{{Field: "value", Value: "uncool"}},
			ExpectedMatch: false,
		},
		{
			Description:   "map_string_interface_match_not_match_wrong_type",
			Obj:           &Object{Data: map[string]any{"value": 3}},
			Filters:       []QueryFilter{{Field: "value", Value: "uncool"}},
			ExpectedMatch: false,
		},
		{
			Description:   "map_string_string_match",
			Obj:           &Object{Data: map[string]string{"value": "cool"}},
			Filters:       []QueryFilter{{Field: "value", Value: "cool"}},
			ExpectedMatch: true,
		},
		{
			Description:   "map_string_string_match_not_match",
			Obj:           &Object{Data: map[string]string{"value": "cool"}},
			Filters:       []QueryFilter{{Field: "value", Value: "uncool"}},
			ExpectedMatch: false,
		},

		{
			Description:   "multi_match",
			Obj:           &Object{Data: map[string]any{"value": "cool", "another": "very-cool"}},
			Filters:       []QueryFilter{{Field: "value", Value: "cool"}, {Field: "another", Value: "very-cool"}},
			ExpectedMatch: true,
		},
		{
			Description:   "multi_not_match",
			Obj:           &Object{Data: map[string]any{"value": "cool", "another": "sub-zero"}},
			Filters:       []QueryFilter{{Field: "value", Value: "cool"}, {Field: "another", Value: "very-cool"}},
			ExpectedMatch: false,
		},
		{
			Description:   "nested_match",
			Obj:           &Object{Data: map[string]any{"properties": map[string]any{"value": "freezing"}}},
			Filters:       []QueryFilter{{Field: "properties.value", Value: "freezing"}},
			ExpectedMatch: true,
		},
		{
			Description:   "nested_match",
			Obj:           &Object{Data: map[string]any{"properties": map[string]any{"value": "freezing"}}},
			Filters:       []QueryFilter{{Field: "properties.value", Value: "warm"}},
			ExpectedMatch: false,
		},
	}

	for _, testcase := range cases {
		t.Run(testcase.Description, func(t *testing.T) {
			match, err := testcase.Obj.MatchesFilters(testcase.Filters)
			require.NoError(t, err)
			require.Equal(t, testcase.ExpectedMatch, match)
		})
	}
}
