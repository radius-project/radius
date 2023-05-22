/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package prompt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ValidateKubernetesNamespace(t *testing.T) {
	t.Run("valid", validatorPositiveTest(t, "my-namespace0", ValidateKubernetesNamespace))
	t.Run("capital", validatorNegativeTest(t, "A", ValidateKubernetesNamespace))
	t.Run("starts with number", validatorNegativeTest(t, "000", ValidateKubernetesNamespace))
	t.Run("empty", validatorNegativeTest(t, "", ValidateKubernetesNamespace))
	t.Run("too long", validatorNegativeTest(t, strings.Repeat("a", 64), ValidateKubernetesNamespace))
	t.Run("ends with dash", validatorNegativeTest(t, "a-", ValidateKubernetesNamespace))
	t.Run("invalid character", validatorNegativeTest(t, "a#", ValidateKubernetesNamespace))
}

func Test_ValidateKubernetesNamespaceOrDefault(t *testing.T) {
	t.Run("valid", validatorPositiveTest(t, "my-namespace0", ValidateKubernetesNamespaceOrDefault))
	t.Run("capital", validatorNegativeTest(t, "A", ValidateKubernetesNamespaceOrDefault))
	t.Run("starts with number", validatorNegativeTest(t, "000", ValidateKubernetesNamespaceOrDefault))
	t.Run("empty", validatorPositiveTest(t, "", ValidateKubernetesNamespaceOrDefault))
	t.Run("too long", validatorNegativeTest(t, strings.Repeat("a", 64), ValidateKubernetesNamespaceOrDefault))
	t.Run("ends with dash", validatorNegativeTest(t, "a-", ValidateKubernetesNamespaceOrDefault))
	t.Run("invalid character", validatorNegativeTest(t, "a#", ValidateKubernetesNamespaceOrDefault))
}

func Test_ValidateResourceName(t *testing.T) {
	t.Run("valid", validatorPositiveTest(t, "my-resource0", ValidateResourceName))
	t.Run("capital", validatorPositiveTest(t, "A", ValidateResourceName))
	t.Run("starts with number", validatorNegativeTest(t, "000", ValidateResourceName))
	t.Run("empty", validatorNegativeTest(t, "", ValidateResourceName))
	t.Run("ends with dash", validatorNegativeTest(t, "a-", ValidateResourceName))
	t.Run("invalid character", validatorNegativeTest(t, "a#", ValidateResourceName))
}

func Test_ValidateResourceNameOrDefault(t *testing.T) {
	t.Run("valid", validatorPositiveTest(t, "my-resource0", ValidateResourceNameOrDefault))
	t.Run("capital", validatorPositiveTest(t, "A", ValidateResourceNameOrDefault))
	t.Run("starts with number", validatorNegativeTest(t, "000", ValidateResourceNameOrDefault))
	t.Run("empty", validatorPositiveTest(t, "", ValidateResourceNameOrDefault))
	t.Run("ends with dash", validatorNegativeTest(t, "a-", ValidateResourceNameOrDefault))
	t.Run("invalid character", validatorNegativeTest(t, "a#", ValidateResourceNameOrDefault))
}

func Test_ValidateUUIDv4(t *testing.T) {
	// UUIDv4 is a well-known format with a documented regex, so we're just doing basic coverage here.
	t.Run("valid", validatorPositiveTest(t, "957a2fd1-ba34-4d02-ab11-4d046568661c", ValidateUUIDv4))
	t.Run("invalid", validatorNegativeTest(t, "957a2fd1-ba34-4d02-ab11-4d046568661z", ValidateUUIDv4))
	t.Run("empty", validatorNegativeTest(t, "", ValidateUUIDv4))
}

func validatorPositiveTest(t *testing.T, input string, validator func(string) error) func(*testing.T) {
	return func(t *testing.T) {
		err := validator(input)
		require.NoError(t, err)
	}
}

func validatorNegativeTest(t *testing.T, input string, validator func(string) error) func(*testing.T) {
	return func(t *testing.T) {
		err := validator(input)
		require.Error(t, err)
	}
}
