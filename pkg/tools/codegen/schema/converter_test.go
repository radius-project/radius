// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package schema

import (
	"encoding/json"
	"io/fs"
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConverter(t *testing.T) {
	/* Load the input files */
	inputSchemas := make(map[string]Schema)
	err := filepath.Walk("testdata/input", func(path string, info fs.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		s, err := Load(path)
		if err != nil {
			log.Fatalf("Error: cannot read file %q: %v", path, err)
		}
		inputSchemas[path] = *s
		return nil
	})
	require.Nil(t, err)

	/* Load the expected output file */
	expectedOut, err := ioutil.ReadFile("testdata/output.json")
	require.Nil(t, err)

	outputSchema, err := NewAutorestConverter().Convert(inputSchemas)
	require.Nil(t, err)

	out, err := json.MarshalIndent(outputSchema, "", "  ")
	require.Nil(t, err)

	/* Compare the expected vs actual */
	expected := strings.ReplaceAll(strings.TrimSpace(string(expectedOut)), "\r", "")
	require.Equal(t, expected, string(out))
}
