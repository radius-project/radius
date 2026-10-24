/*
Copyright 2026 The Radius Authors.

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

package validation

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// New tests in suites omitted from PRs require an explicit coverage decision.
func TestLegacyFunctionalSuiteInventory(t *testing.T) {
	t.Parallel()
	for suite, expected := range map[string][]string{
		"daprrp": {
			"Test_DaprComponentNameConflict",
			"Test_ConfigurationStore_Manual_Secret",
			"Test_ConfigurationStore_Manual",
			"Test_ConfigurationStore_Recipe",
			"Test_DaprPubSubBroker_Manual_Secret",
			"Test_DaprPubSubBroker_Manual",
			"Test_DaprPubSubBroker_Recipe",
			"Test_DaprSecretStore_Manual",
			"Test_DaprSecretStore_Recipe",
			"Test_DaprServiceInvocation",
			"Test_DaprStateStore_Manual",
			"Test_DaprStateStore_Manual_Secret",
			"Test_DaprStateStore_Recipe",
			"TestWaitForRedis",
			"TestWaitForRedisPingDeadline",
			"TestWaitForRedisRetriesUntilPong",
			"TestWaitForRedisRejectsAmbiguousPods",
			"TestWaitForRedisListError",
			"TestWaitForRedisCanceled",
			"TestWaitForRedisCancelsInFlightPing",
		},
		"datastoresrp": {
			"Test_MongoDB_Manual",
			"Test_MongoDB_Recipe",
			"Test_Redis_Manual",
			"Test_Redis_Recipe",
			"Test_Redis_DefaultRecipe",
			"Test_Deployment_SimulatedEnv_BicepRecipe",
		},
		"messagingrp": {
			"Test_RabbitMQ_Manual",
			"Test_RabbitMQ_Recipe",
		},
	} {
		t.Run(suite, func(t *testing.T) {
			t.Parallel()
			root := filepath.Join("..", "functional-portable", suite, "noncloud")
			var expectedIDs []string
			for _, name := range expected {
				expectedIDs = append(expectedIDs, "resources/"+name)
			}
			var actual []string
			err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() || !strings.HasSuffix(path, "_test.go") {
					return nil
				}
				file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
				if err != nil {
					return err
				}
				relative, err := filepath.Rel(root, path)
				if err != nil {
					return err
				}
				for _, declaration := range file.Decls {
					function, ok := declaration.(*ast.FuncDecl)
					if ok && function.Recv == nil && strings.HasPrefix(function.Name.Name, "Test") {
						actual = append(actual, filepath.ToSlash(filepath.Join(filepath.Dir(relative), function.Name.Name)))
					}
				}
				return nil
			})
			require.NoError(t, err)
			require.ElementsMatch(t, expectedIDs, actual,
				"Review changed tests for PR coverage before updating this legacy suite inventory; TestWaitForRedis* runs per PR via make test-dapr-helpers")
		})
	}
}
