// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package tooling

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

type bicepConsumerConfig struct {
	ExperimentalFeaturesEnabled map[string]bool   `json:"experimentalFeaturesEnabled"`
	Extensions                  map[string]string `json:"extensions"`
}

func readConsumerConfig(t *testing.T, path string) bicepConsumerConfig {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var config bicepConsumerConfig
	require.NoError(t, json.Unmarshal(data, &config))
	require.True(t, config.ExperimentalFeaturesEnabled["ociEnabled"], path)
	return config
}

func TestGenerateBicepConfig(t *testing.T) {
	t.Parallel()
	script, err := os.ReadFile("../../build/scripts/generate-bicepconfig.sh")
	require.NoError(t, err)
	for _, channel := range []string{"edge", "0.61", "0.61.1", "0.61.0-rc.1", "0.60.0-rc3", "latest"} {
		t.Run(channel, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(t.TempDir(), "output with spaces")
			cmd := exec.Command("bash", "-s", "--", channel, dir)
			cmd.Stdin = strings.NewReader(string(script))
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", output)
			config := readConsumerConfig(t, filepath.Join(dir, "bicepconfig.json"))
			require.Equal(t, map[string]string{
				"radius": "br:ghcr.io/radius-project/bicep-types-radius:" + channel,
				"aws":    "br:ghcr.io/radius-project/bicep-types-aws:" + channel,
			}, config.Extensions)
		})
	}
	for _, args := range [][]string{nil, {"edge"}, {"", t.TempDir()}} {
		cmd := exec.Command("bash", append([]string{"-s", "--"}, args...)...)
		cmd.Stdin = strings.NewReader(string(script))
		output, err := cmd.CombinedOutput()
		require.Error(t, err)
		require.Contains(t, string(output), "is required")
	}
}

func TestCheckedInBicepConfigs(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"bicepconfig.json",
		".radius/bicepconfig.json",
		"test/functional-portable/dynamicrp/noncloud/resources/bicepconfig.json",
	} {
		config := readConsumerConfig(t, filepath.Join("../..", path))
		require.Equal(t, "br:ghcr.io/radius-project/bicep-types-radius:edge", config.Extensions["radius"])
		if path != ".radius/bicepconfig.json" {
			require.Equal(t, "br:ghcr.io/radius-project/bicep-types-aws:edge", config.Extensions["aws"])
		}
		if strings.HasPrefix(path, "test/") {
			require.Equal(t, "br:testuserdefinedbiceptypes.azurecr.io/testresources:latest", config.Extensions["testresources"])
		}
	}
}

func TestBicepConsumerDefaultsAvoidProductionACR(t *testing.T) {
	t.Parallel()
	// Scope this guard to active consumer/config surfaces, not historical
	// release evidence or compatibility publishers and their tests.
	paths := []string{
		"bicepconfig.json", ".radius/bicepconfig.json",
		"test/functional-portable/dynamicrp/noncloud/resources/bicepconfig.json",
		"build/generate.mk", "build/scripts/generate-bicepconfig.sh",
		".github/scripts/test-repo-radius-state-e2e.sh",
		"docs/contributing/contributing-code/contributing-code-schema-changes/README.md",
	}
	for _, pattern := range []string{
		".github/workflows/*.yaml", ".github/workflows/*.yml",
		".github/agents/*.md", ".github/prompts/*.md", "pkg/cli/setup/*.go",
	} {
		matches, err := filepath.Glob(filepath.Join("../..", pattern))
		require.NoError(t, err)
		for _, match := range matches {
			path, err := filepath.Rel("../..", match)
			require.NoError(t, err)
			paths = append(paths, path)
		}
	}
	for _, path := range paths {
		data, err := os.ReadFile(filepath.Join("../..", path))
		require.NoError(t, err)
		require.NotContains(t, string(data), "br:biceptypes.azurecr.io/", path)
		require.NotContains(t, string(data), "BICEP_TYPES_REGISTRY: biceptypes.azurecr.io", path)
	}
}

func TestWorkflowBicepConsumerConfigs(t *testing.T) {
	t.Parallel()
	for _, workflow := range []string{"functional-test-cloud", "functional-test-noncloud", "long-running-azure"} {
		t.Run(workflow, func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile("../../.github/workflows/" + workflow + ".yaml")
			require.NoError(t, err)
			var document struct {
				Env  map[string]string `yaml:"env"`
				Jobs map[string]struct {
					Steps []struct {
						Name string            `yaml:"name"`
						Run  string            `yaml:"run"`
						Env  map[string]string `yaml:"env"`
					} `yaml:"steps"`
				} `yaml:"jobs"`
			}
			require.NoError(t, yaml.Unmarshal(data, &document))
			require.Equal(t, "ghcr.io/radius-project", document.Env["BICEP_TYPES_REGISTRY"])
			count := 0
			for jobName, job := range document.Jobs {
				for _, step := range job.Steps {
					if strings.Contains(step.Run, "bicep publish-extension") {
						require.NotContains(t, step.Run, "br:localhost:", step.Name)
						require.NotContains(t, step.Run, "br:${LOCAL_REGISTRY_SERVER}:", step.Name)
					}
					if registry := step.Env["BICEP_RECIPE_REGISTRY"]; registry != "" {
						require.NotContains(t, registry, "localhost", step.Name)
						require.NotContains(t, registry, "LOCAL_REGISTRY_SERVER", step.Name)
					}
					if step.Name != "Generate test bicepconfig.json" && step.Name != "Point dynamicrp testresources extension to test ACR" {
						continue
					}
					count++
					for _, channel := range []string{"edge", "test-ci-123"} {
						t.Run(jobName+"/"+channel+"/"+step.Name, func(t *testing.T) {
							dir := t.TempDir()
							const resources = "test/functional-portable/dynamicrp/noncloud/resources"
							require.NoError(t, os.MkdirAll(filepath.Join(dir, resources), 0o755))
							path := filepath.Join(dir, "test/bicepconfig.json")
							if workflow == "long-running-azure" {
								path = filepath.Join(dir, resources, "bicepconfig.json")
								require.NoError(t, os.WriteFile(path, []byte(`{
									"experimentalFeaturesEnabled": {"symbolicNameCodegen": true},
									"extensions": {"custom": "./custom.tgz"},
									"formatting": {"indentSize": 4}
								}`), 0o600))
							}
							cmd := exec.Command("bash", "-euo", "pipefail", "-c", step.Run)
							cmd.Dir = dir
							cmd.Env = append(os.Environ(),
								"REL_VERSION="+channel, "RELEASE_DIR=.",
								"BICEP_TYPES_REGISTRY=ghcr.io/radius-project",
								"TEST_BICEP_TYPES_REGISTRY=private-test.example",
								"LOCAL_REGISTRY_SERVER=localhost",
								"LOCAL_REGISTRY_NAME=radius-registry", "LOCAL_REGISTRY_PORT=5000",
							)
							output, err := cmd.CombinedOutput()
							require.NoError(t, err, "%s", output)
							config := readConsumerConfig(t, path)
							require.Equal(t, "br:ghcr.io/radius-project/bicep-types-aws:edge", config.Extensions["aws"])
							switch workflow {
							case "functional-test-noncloud":
								tag := channel
								if tag == "edge" {
									tag = "latest" // Existing per-job test tag, not a production default.
								}
								require.Equal(t, "br:radius-registry:5000/radius:"+tag, config.Extensions["radius"])
								if config.Extensions["testresources"] != "" {
									require.Equal(t, "br:radius-registry:5000/testresources:"+tag, config.Extensions["testresources"])
								}
							case "functional-test-cloud":
								if jobName == "build" {
									require.Equal(t, "br:radius-registry:5000/test/radius:"+channel, config.Extensions["radius"])
								} else {
									tag := channel
									if tag == "edge" {
										tag = "latest"
									}
									require.Equal(t, "br:private-test.example/test/radius:"+tag, config.Extensions["radius"])
								}
							case "long-running-azure":
								require.Equal(t, "br:ghcr.io/radius-project/bicep-types-radius:edge", config.Extensions["radius"])
								require.Equal(t, "br:private-test.example/testresources:latest", config.Extensions["testresources"])
								require.Equal(t, "./custom.tgz", config.Extensions["custom"])
								require.True(t, config.ExperimentalFeaturesEnabled["symbolicNameCodegen"])
								data, err := os.ReadFile(path)
								require.NoError(t, err)
								var preserved struct {
									Formatting struct{ IndentSize int }
									Cloud      struct{ CredentialPrecedence []string }
								}
								require.NoError(t, json.Unmarshal(data, &preserved))
								require.Equal(t, 4, preserved.Formatting.IndentSize)
								require.Equal(t, []string{"AzureCLI", "Environment"}, preserved.Cloud.CredentialPrecedence)
							}
						})
					}
				}
			}
			expected := 2
			if workflow == "long-running-azure" {
				expected = 1
			}
			require.Equal(t, expected, count, "all effective configs must be exercised")
		})
	}
}
