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

package bicep

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/driver"
)

const (
	containerImagesResourceType            = "Radius.Compute/containerImages"
	containerImagesBuildScriptVariableName = "radiusContainerImagesBuildScript"
	imageBuildOutputName                   = "imageBuild"
	imageReferenceValueName                = "imageReference"
	registryParameterName                  = "registry"
	registrySecretNameParameterName        = "registrySecretName"
	execOutputEnvName                      = "RADIUS_EXEC_OUTPUT"
	dockerConfigEnvName                    = "DOCKER_CONFIG"
	scriptShell                            = "/bin/sh"
	scriptName                             = "radius-container-images-build"
	stderrTailLimit                        = 4096
	scriptLogLineLimit                     = 4096
	scriptLogTruncationMarker              = " [truncated]"
)

func supportsImageBuildHook(resourceType string) bool {
	return strings.EqualFold(resourceType, containerImagesResourceType)
}

// extractImageBuild returns the optional imageBuild object without imposing a field schema.
func extractImageBuild(outputs any) (map[string]any, error) {
	out, ok := outputs.(map[string]any)
	if !ok {
		return nil, nil
	}

	raw, ok := out[imageBuildOutputName]
	if !ok {
		return nil, nil
	}

	output, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("output %q must be an object output", imageBuildOutputName)
	}
	value, ok := output["value"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("output %q must evaluate to an object", imageBuildOutputName)
	}

	return value, nil
}

// extractImageBuildScript accepts only static content from the compiled-template variable.
func extractImageBuildScript(template map[string]any) (string, error) {
	variables, ok := template["variables"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("recipe template has no variables; cannot find %q", containerImagesBuildScriptVariableName)
	}
	script, ok := variables[containerImagesBuildScriptVariableName].(string)
	if !ok || script == "" {
		return "", fmt.Errorf("recipe template variable %q must be a non-empty string", containerImagesBuildScriptVariableName)
	}
	if strings.HasPrefix(script, "[") {
		return "", fmt.Errorf("recipe template variable %q must be static script content", containerImagesBuildScriptVariableName)
	}
	return script, nil
}

// executeImageBuildHook runs only for Radius.Compute/containerImages.
func (d *bicepDriver) executeImageBuildHook(ctx context.Context, recipeData map[string]any, outputs any, recipeResponse *recipes.RecipeOutput, opts driver.ExecuteOptions) error {
	if !supportsImageBuildHook(opts.BaseOptions.Definition.ResourceType) {
		return nil
	}

	imageBuild, err := extractImageBuild(outputs)
	if err != nil || imageBuild == nil {
		return err
	}
	registry, registrySecretName, err := operatorRegistryParameters(opts.Definition.Parameters)
	if err != nil {
		return err
	}
	buildInputs := maps.Clone(imageBuild)
	// Preserve the deployment output while enforcing the operator-owned registry.
	buildInputs[registryParameterName] = registry

	script, err := extractImageBuildScript(recipeData)
	if err != nil {
		return err
	}

	imageReference, err := d.executeImageBuild(ctx, script, buildInputs, registry, registrySecretName, opts)
	if err != nil {
		return err
	}
	if recipeResponse.Values == nil {
		recipeResponse.Values = map[string]any{}
	}
	recipeResponse.Values[imageReferenceValueName] = imageReference
	delete(recipeResponse.Values, imageBuildOutputName)
	return nil
}

// operatorRegistryParameters avoids developer overrides by reading the registered definition.
func operatorRegistryParameters(parameters map[string]any) (registry string, registrySecretName string, err error) {
	value, ok := parameters[registryParameterName]
	if !ok {
		return "", "", fmt.Errorf("containerImages requires the recipe registration to set a non-empty %q parameter; developer resource parameters are intentionally not used for registry settings", registryParameterName)
	}
	registry, ok = value.(string)
	if !ok || registry == "" {
		return "", "", fmt.Errorf("containerImages requires the recipe registration to set a non-empty %q parameter; developer resource parameters are intentionally not used for registry settings", registryParameterName)
	}

	if secretName, ok := parameters[registrySecretNameParameterName]; ok && secretName != nil {
		registrySecretName, ok = secretName.(string)
		if !ok {
			return "", "", fmt.Errorf("containerImages recipe definition parameter %q must be a string", registrySecretNameParameterName)
		}
	}

	return registry, registrySecretName, nil
}

// imageBuildArguments converts supported JSON values into deterministic field-derived flags.
// The script owns field semantics; false booleans and empty collections emit no arguments.
func imageBuildArguments(buildInputs map[string]any) ([]string, error) {
	keys := make([]string, 0, len(buildInputs))
	for key := range buildInputs {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	args := make([]string, 0, len(keys)*2)
	for _, key := range keys {
		flag := "--" + key
		switch value := buildInputs[key].(type) {
		case string:
			args = append(args, flag, value)
		case bool:
			if value {
				args = append(args, flag)
			}
		case []any:
			for _, item := range value {
				text, ok := item.(string)
				if !ok {
					return nil, fmt.Errorf("output %q property %q must contain only string values", imageBuildOutputName, key)
				}
				args = append(args, flag, text)
			}
		case map[string]any:
			subKeys := make([]string, 0, len(value))
			for subKey := range value {
				subKeys = append(subKeys, subKey)
			}
			slices.Sort(subKeys)
			for _, subKey := range subKeys {
				text, ok := value[subKey].(string)
				if !ok {
					return nil, fmt.Errorf("output %q property %q entry %q must be a string", imageBuildOutputName, key, subKey)
				}
				args = append(args, flag, subKey, text)
			}
		case nil:
			return nil, fmt.Errorf("output %q property %q must not be null", imageBuildOutputName, key)
		default:
			return nil, fmt.Errorf("output %q property %q has an unsupported type %T", imageBuildOutputName, key, value)
		}
	}
	return args, nil
}

func imageBuildEnvironment(env []string, dockerConfigDir, resultPath string) []string {
	filtered := make([]string, 0, len(env)+2)
	for _, value := range env {
		name, _, _ := strings.Cut(value, "=")
		if name == dockerConfigEnvName || name == execOutputEnvName {
			continue
		}
		filtered = append(filtered, value)
	}

	return append(filtered,
		dockerConfigEnvName+"="+dockerConfigDir,
		execOutputEnvName+"="+resultPath)
}

func (d *bicepDriver) executeImageBuild(ctx context.Context, script string, buildInputs map[string]any, registry, registrySecretName string, opts driver.ExecuteOptions) (string, error) {
	logger := logr.FromContextOrDiscard(ctx)

	args, err := imageBuildArguments(buildInputs)
	if err != nil {
		return "", err
	}

	tempDir, err := os.MkdirTemp("", "radius-imagebuild-")
	if err != nil {
		return "", fmt.Errorf("failed to create working directory for %q script: %w", imageBuildOutputName, err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	dockerConfigDir := ""
	if registrySecretName != "" {
		dockerConfigDir = filepath.Join(tempDir, "docker")
		if err := d.writeDockerConfig(ctx, registry, registrySecretName, dockerConfigDir, opts); err != nil {
			return "", err
		}
	}

	resultPath := filepath.Join(tempDir, "result.json")
	// Do not let ambient dynamic-rp paths or credentials reach the build.
	env := imageBuildEnvironment(os.Environ(), dockerConfigDir, resultPath)
	stderrTail, err := runScript(ctx, script, args, env, tempDir, logger)
	if err != nil {
		if stderrTail != "" {
			return "", fmt.Errorf("recipe %q script failed: %w\nstderr (tail):\n%s", imageBuildOutputName, err, stderrTail)
		}
		return "", fmt.Errorf("recipe %q script failed: %w", imageBuildOutputName, err)
	}

	return readScriptResult(resultPath)
}

// writeDockerConfig materializes registry credentials without logging Secret values.
func (d *bicepDriver) writeDockerConfig(ctx context.Context, registry, registrySecretName, dir string, opts driver.ExecuteOptions) error {
	runtime := opts.Configuration.Runtime.Kubernetes
	if runtime == nil || runtime.Namespace == "" {
		return fmt.Errorf("output %q references Secret %q but the recipe has no Kubernetes runtime namespace", imageBuildOutputName, registrySecretName)
	}
	if d.clusterAccessResolver == nil {
		return fmt.Errorf("output %q references Secret %q but the driver has no cluster access resolver configured", imageBuildOutputName, registrySecretName)
	}

	config, err := d.clusterAccessResolver.Resolve(ctx, &opts.Configuration)
	if err != nil {
		return fmt.Errorf("failed to resolve the target cluster for registry Secret '%s/%s': %w", runtime.Namespace, registrySecretName, err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create a target-cluster client for registry Secret '%s/%s': %w", runtime.Namespace, registrySecretName, err)
	}
	secret, err := clientset.CoreV1().Secrets(runtime.Namespace).Get(ctx, registrySecretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to read registry Secret '%s/%s': %w", runtime.Namespace, registrySecretName, err)
	}
	username, ok := secret.Data["username"]
	if !ok {
		return fmt.Errorf("registry Secret '%s/%s' has no 'username' key", runtime.Namespace, registrySecretName)
	}
	password, ok := secret.Data["password"]
	if !ok {
		return fmt.Errorf("registry Secret '%s/%s' has no 'password' key", runtime.Namespace, registrySecretName)
	}

	authKey, err := dockerConfigAuthKey(registry)
	if err != nil {
		return fmt.Errorf("output %q has an empty registry host", imageBuildOutputName)
	}
	auth := base64.StdEncoding.EncodeToString([]byte(string(username) + ":" + string(password)))
	configBytes, err := json.Marshal(map[string]any{
		"auths": map[string]any{authKey: map[string]any{"auth": auth}},
	})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "config.json"), configBytes, 0o600)
}

func dockerConfigAuthKey(registry string) (string, error) {
	registryHost := strings.SplitN(registry, "/", 2)[0]
	if registryHost == "" {
		return "", fmt.Errorf("empty registry host")
	}
	switch registryHost {
	case "docker.io", "index.docker.io", "registry-1.docker.io":
		return "https://index.docker.io/v1/", nil
	default:
		return registryHost, nil
	}
}

// runScript streams both output pipes so buildctl cannot deadlock on a full pipe. The context
// bounds execution and the platform-specific process-group setup also terminates child processes.
func runScript(ctx context.Context, script string, args, env []string, workDir string, logger logr.Logger) (string, error) {
	commandArgs := []string{"-c", script, scriptName}
	commandArgs = append(commandArgs, args...)
	cmd := exec.CommandContext(ctx, scriptShell, commandArgs...)
	cmd.Env = env
	cmd.Dir = workDir
	configureScriptProcessGroup(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start %s: %w", scriptShell, err)
	}

	tail := &tailBuffer{limit: stderrTailLimit}
	done := make(chan error, 2)
	go func() { done <- drainScriptStream(stdout, "imageBuild: ", logger, nil) }()
	go func() { done <- drainScriptStream(stderr, "imageBuild(stderr): ", logger, tail) }()
	streamErr := errors.Join(<-done, <-done)

	err = cmd.Wait()
	if err != nil {
		if ctx.Err() != nil {
			return tail.String(), errors.Join(fmt.Errorf("script canceled or timed out: %w", ctx.Err()), streamErr)
		}
		return tail.String(), errors.Join(err, streamErr)
	}
	if streamErr != nil {
		return tail.String(), fmt.Errorf("failed to read script output: %w", streamErr)
	}
	return tail.String(), nil
}

func drainScriptStream(stream io.Reader, logPrefix string, logger logr.Logger, tail *tailBuffer) error {
	reader := bufio.NewReaderSize(stream, scriptLogLineLimit)
	line := make([]byte, 0, scriptLogLineLimit)
	lineStarted := false
	lineTruncated := false

	for {
		fragment, err := reader.ReadSlice('\n')
		if len(fragment) > 0 {
			lineStarted = true
			if tail != nil {
				tail.appendBytes(fragment)
			}
		}

		logFragment := fragment
		if err == nil {
			logFragment = bytes.TrimRight(logFragment, "\r\n")
		}
		if len(logFragment) > 0 {
			remaining := scriptLogLineLimit - len(line)
			if remaining > len(logFragment) {
				remaining = len(logFragment)
			}
			line = append(line, logFragment[:remaining]...)
			lineTruncated = lineTruncated || remaining < len(logFragment)
		}

		if err == nil {
			logScriptLine(logger, logPrefix, line, lineTruncated)
			line = line[:0]
			lineStarted = false
			lineTruncated = false
			continue
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if lineStarted {
			logScriptLine(logger, logPrefix, line, lineTruncated)
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
}

func logScriptLine(logger logr.Logger, prefix string, line []byte, truncated bool) {
	message := prefix + string(line)
	if truncated {
		message += scriptLogTruncationMarker
	}
	logger.Info(message)
}

// readScriptResult requires exactly one non-empty imageReference string.
func readScriptResult(path string) (string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("the %q script completed without writing the %s result file", imageBuildOutputName, execOutputEnvName)
	}
	if err != nil {
		return "", fmt.Errorf("failed to read %s file: %w", execOutputEnvName, err)
	}

	var values map[string]json.RawMessage
	if err := json.Unmarshal(data, &values); err != nil {
		return "", fmt.Errorf("%s file is not a JSON object: %w", execOutputEnvName, err)
	}
	if len(values) != 1 {
		return "", fmt.Errorf("the %s result file must contain only %q", execOutputEnvName, imageReferenceValueName)
	}
	raw, ok := values[imageReferenceValueName]
	if !ok {
		return "", fmt.Errorf("the %s result file must contain only %q", execOutputEnvName, imageReferenceValueName)
	}
	var imageReference string
	if err := json.Unmarshal(raw, &imageReference); err != nil || imageReference == "" {
		return "", fmt.Errorf("the %s result file must contain a non-empty %q string", execOutputEnvName, imageReferenceValueName)
	}
	return imageReference, nil
}

type tailBuffer struct {
	limit int
	data  []byte
}

func (t *tailBuffer) appendBytes(data []byte) {
	if len(data) >= t.limit {
		t.data = append(t.data[:0], data[len(data)-t.limit:]...)
		return
	}
	if overflow := len(t.data) + len(data) - t.limit; overflow > 0 {
		copy(t.data, t.data[overflow:])
		t.data = t.data[:len(t.data)-overflow]
	}
	t.data = append(t.data, data...)
}

func (t *tailBuffer) String() string {
	return string(t.data)
}
