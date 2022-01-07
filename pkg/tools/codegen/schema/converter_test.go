// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package schema

import (
	"io/fs"
	"log"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/project-radius/radius/pkg/radrp/schema"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

func TestNewAutorestConverter(t *testing.T) {
	assert.Equal(t, len(NewAutorestConverter().resources), len(schema.ResourceManifest.Resources))
}

func TestConverter(t *testing.T) {
	underTest := converter{
		resources: []resourceInfo{
			newResourceInfo("Application", "#/definitions/ApplicationResource"),
			newResourceInfo("RadiusResource", "#/definitions/RadiusResource"),
			newResourceInfo("radius.com.AwesomeThing", "radius.json#/definitions/AwesomeThingResource"),
		},
	}
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
	expected, err := Load("testdata/output.json")
	require.Nil(t, err)

	actual, err := underTest.Convert(inputSchemas)
	require.Nil(t, err)

	/* Compare the expected vs actual */
	if diff := cmp.Diff(expected, actual); diff != "" {
		t.Errorf("Unexpected diff (-want,+got): %s", diff)
	}
}
