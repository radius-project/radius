// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type tableInput struct {
	Size   string
	IsCool bool
}

var tableInputOptions = FormatterOptions{
	// Note: We're not testing the behavior of JSONPath in detail since we don't implement that, just the E2E.
	Columns: []Column{
		{
			Heading:  "Size",
			JSONPath: "{ .Size }",
		},
		{
			Heading:  "Coolness",
			JSONPath: "{ .IsCool }",
		},
		{
			Heading:  "Unknown",
			JSONPath: "{ .FieldDoesNotExist }",
		},
		{
			Heading:  "Static",
			JSONPath: "Some-Value",
		},
		{
			Heading:     "Lowered",
			JSONPath:    "Some-Value",
			Transformer: strings.ToLower,
		},
	},
}

func Test_Table_NoColumns(t *testing.T) {
	obj := struct{}{}

	formatter := &TableFormatter{}

	buffer := &bytes.Buffer{}
	err := formatter.Format(obj, buffer, FormatterOptions{})
	require.Error(t, err)
	require.Equal(t, "no columns were defined, table format is not supported for this command", err.Error())
}

func Test_Table_Scalar(t *testing.T) {
	obj := tableInput{
		Size:   "mega",
		IsCool: true,
	}

	formatter := &TableFormatter{}

	buffer := &bytes.Buffer{}
	err := formatter.Format(obj, buffer, tableInputOptions)
	require.NoError(t, err)

	expected := `Size      Coolness  Unknown   Static      Lowered
mega      true                Some-Value  some-value
`
	require.Equal(t, expected, buffer.String())
}

func Test_Table_Slice(t *testing.T) {
	obj := []any{
		tableInput{
			Size:   "mega",
			IsCool: true,
		},
		tableInput{
			Size:   "medium",
			IsCool: false,
		},
	}

	formatter := &TableFormatter{}

	buffer := &bytes.Buffer{}
	err := formatter.Format(obj, buffer, tableInputOptions)
	require.NoError(t, err)

	expected := `Size      Coolness  Unknown   Static      Lowered
mega      true                Some-Value  some-value
medium    false               Some-Value  some-value
`
	require.Equal(t, expected, buffer.String())
}

func Test_convertToStruct(t *testing.T) {
	aStruct := tableInput{
		Size: "medium",
	}
	inputs := []convertInput{
		{
			Name:    "string",
			Input:   "test",
			Success: false,
		},
		{
			Name:    "nil",
			Input:   nil,
			Success: false,
		},
		{
			Name:    "nil pointer",
			Input:   (*tableInput)(nil),
			Success: false,
		},
		{
			Name:    "struct",
			Input:   aStruct,
			Success: true,
			Expected: []any{
				aStruct,
			},
		},
		{
			Name:    "struct pointer",
			Input:   &aStruct,
			Success: true,
			Expected: []any{
				aStruct,
			},
		},
		{
			Name: "slice",
			Input: []any{
				aStruct, &aStruct, "test", []any{},
			},
			Success: true,
			Expected: []any{
				aStruct, &aStruct, "test", []any{},
			},
		},
	}

	for _, input := range inputs {
		t.Run(input.Name, func(t *testing.T) {
			actual, err := convertToSlice(input.Input)
			if input.Success {
				require.NoError(t, err)
				require.Equal(t, input.Expected, actual)
			} else {
				require.Error(t, err)
			}
		})
	}
}

type convertInput struct {
	Name     string
	Input    any
	Success  bool
	Expected []any
}
