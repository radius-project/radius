// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package schemav3

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_GetValidator_AllResourceTypesAreLoadable(t *testing.T) {
	manifest := readManifestOrPanic()

	for resourceType := range manifest.Resources {
		validator, ok := GetValidator(resourceType)
		require.Truef(t, ok, "missing validator for %s", resourceType)
		require.NotNil(t, validator)
	}
}

func Test_GetValidator_UnknownTypeReturnsFalse(t *testing.T) {
	validator, ok := GetValidator("FakeType")
	require.False(t, ok)
	require.Nil(t, validator)
}

type testcase struct {
	InputFullPath  string
	ErrorsFullPath string
}

func (t testcase) IsValidTest() bool {
	return t.ErrorsFullPath == ""
}

func Test_Validation(t *testing.T) {
	tests := findTests(t)

	manifest := readManifestOrPanic()
	for resourceType := range manifest.Resources {
		t.Run(resourceType, func(t *testing.T) {
			// Each resource type should define *some* tests...
			cases, ok := tests[resourceType]
			if !ok || len(cases) == 0 {
				require.Failf(t, "tests are missing", "tests are missing for schema type %s", resourceType)
			}

			for _, tc := range cases {
				t.Run(path.Base(tc.InputFullPath), func(t *testing.T) {
					validator, ok := GetValidator(resourceType)
					require.Truef(t, ok, "missing validator for %s", resourceType)
					require.NotNil(t, validator)

					input, err := ioutil.ReadFile(tc.InputFullPath)
					require.NoError(t, err)

					validationErrs := validator.ValidateJSON(input)
					if tc.IsValidTest() {
						require.Empty(t, validationErrs, "valid case returned validation errors")
						return
					} else {
						require.Greater(t, len(validationErrs), 0, "invalid case returned no errors")
					}

					// OK errors are expected ... build a baseline to compare with the expected errors in the file.
					serialized := []string{}
					for _, validationErr := range validationErrs {
						if validationErr.JSONError == nil {
							serialized = append(serialized, fmt.Sprintf("%s: %s", validationErr.Position, validationErr.Message))
						} else {
							serialized = append(serialized, validationErr.JSONError.Error())
						}
					}

					sort.Strings(serialized)
					expectedText, err := ioutil.ReadFile(tc.ErrorsFullPath)
					require.NoError(t, err)
					expectedText = []byte(strings.TrimSpace(string(expectedText)))
					expected := strings.Split(strings.ReplaceAll(string(expectedText), "\r\n", "\n"), "\n")
					require.ElementsMatch(t, expected, serialized)
				})
			}
		})
	}
}

func findTests(t *testing.T) map[string][]testcase {
	tests := map[string][]testcase{}

	// Walk test directory and find test files that match one of the following two patterns:
	//  .+-valid.json
	//  .+-invalid.jsont
	//
	// And invalid test should have a matching .*-invalid.txt
	validTestRegex := regexp.MustCompile(".+-valid.json$")
	invalidTestRegex := regexp.MustCompile(".+-invalid.json$")

	directories, err := ioutil.ReadDir("testdata")
	require.NoError(t, err)

	for _, directory := range directories {
		if !directory.IsDir() {
			// Skip files, just process directories directly.
			continue
		}

		cases := []testcase{}
		directoryPath := path.Join("testdata", directory.Name())
		files, err := ioutil.ReadDir(directoryPath)
		require.NoError(t, err)

		for _, file := range files {
			if validTestRegex.Match([]byte(file.Name())) {
				cases = append(cases, testcase{InputFullPath: path.Join(directoryPath, file.Name())})
				continue
			}

			if invalidTestRegex.Match([]byte(file.Name())) {
				errorsFullPath := path.Join(directoryPath, strings.TrimSuffix(file.Name(), ".json")+".txt")
				_, err := os.Stat(errorsFullPath)
				if err == os.ErrExist {
					err = fmt.Errorf("expected to find a file at %q. Invalid tests must provide a list of errors", errorsFullPath)
				}
				require.NoError(t, err)

				cases = append(cases, testcase{
					InputFullPath:  path.Join(directoryPath, file.Name()),
					ErrorsFullPath: errorsFullPath,
				})
				continue
			}
		}

		tests[directory.Name()] = cases
	}

	return tests
}
