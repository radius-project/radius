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
	execOutputEnvName                      = "RADIUS_EXEC_OUTPUT"
	dockerConfigEnvName                    = "DOCKER_CONFIG"
	scriptShell                            = "/bin/sh"
	scriptName                             = "radius-container-images-build"
	stderrTailLimit                        = 4096
)

// imageBuildSpec is the private contract between the containerImages Bicep recipe and
// dynamic-rp. The recipe maps the public resource schema into this build-specific shape.
// The build script is deliberately not part of this evaluated output: it is read from a
// static compiled-template variable instead, so developer-controlled parameters can only be data.
type imageBuildSpec struct {
	ResourceName       string            `json:"resourceName"`
	Registry           string            `json:"registry"`
	RegistrySecretName string            `json:"registrySecretName"`
	Tag                string            `json:"tag"`
	TagProvided        bool              `json:"tagProvided"`
	Source             string            `json:"source"`
	Dockerfile         string            `json:"dockerfile"`
	Platforms          []string          `json:"platforms"`
	BuildArgs          map[string]string `json:"buildArgs"`
}

var imageBuildSpecProperties = [...]string{
	"resourceName",
	"registry",
	"registrySecretName",
	"tag",
	"tagProvided",
	"source",
	"dockerfile",
	"platforms",
	"buildArgs",
}

func supportsImageBuildHook(resourceType string) bool {
	return strings.EqualFold(resourceType, containerImagesResourceType)
}

// extractImageBuildSpec parses the reserved imageBuild deployment output. A missing output is
// allowed so existing Bicep recipes for containerImages remain unaffected.
func extractImageBuildSpec(outputs any) (*imageBuildSpec, error) {
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

	for _, name := range imageBuildSpecProperties {
		rawValue, ok := value[name]
		if !ok {
			return nil, fmt.Errorf("output %q is missing required property %q", imageBuildOutputName, name)
		}
		if rawValue == nil {
			return nil, fmt.Errorf("output %q property %q must not be null", imageBuildOutputName, name)
		}
	}

	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("failed to encode output %q: %w", imageBuildOutputName, err)
	}

	spec := &imageBuildSpec{}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(spec); err != nil {
		return nil, fmt.Errorf("output %q has an invalid value: %w", imageBuildOutputName, err)
	}
	// encoding/json matches struct fields case-insensitively. The canonical-key checks above
	// plus an exact property count reject duplicate aliases such as both "tag" and "Tag".
	if len(value) != len(imageBuildSpecProperties) {
		return nil, fmt.Errorf("output %q must contain exactly %d properties", imageBuildOutputName, len(imageBuildSpecProperties))
	}

	return spec, nil
}

// extractImageBuildScript reads the platform-engineer-controlled script from one specifically
// named compiled-template variable. Bicep's loadTextContent embeds this value at compile time;
// ARM expressions are rejected as a defense in depth for templates authored without Bicep.
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

// executeImageBuildHook runs only for Radius.Compute/containerImages. Other Bicep recipes are
// unaffected even if they happen to declare an output named imageBuild.
func (d *bicepDriver) executeImageBuildHook(ctx context.Context, recipeData map[string]any, outputs any, recipeResponse *recipes.RecipeOutput, opts driver.ExecuteOptions) error {
	if !supportsImageBuildHook(opts.BaseOptions.Definition.ResourceType) {
		return nil
	}

	spec, err := extractImageBuildSpec(outputs)
	if err != nil || spec == nil {
		return err
	}
	script, err := extractImageBuildScript(recipeData)
	if err != nil {
		return err
	}

	imageReference, err := d.executeImageBuild(ctx, script, spec, opts)
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

func imageBuildArguments(spec *imageBuildSpec) []string {
	args := []string{
		"--resource-name", spec.ResourceName,
		"--registry", spec.Registry,
		"--tag", spec.Tag,
		"--source", spec.Source,
		"--dockerfile", spec.Dockerfile,
	}
	if spec.TagProvided {
		args = append(args, "--tag-provided")
	}
	for _, platform := range spec.Platforms {
		args = append(args, "--platform", platform)
	}

	names := make([]string, 0, len(spec.BuildArgs))
	for name := range spec.BuildArgs {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		args = append(args, "--build-arg", name, spec.BuildArgs[name])
	}
	return args
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

func (d *bicepDriver) executeImageBuild(ctx context.Context, script string, spec *imageBuildSpec, opts driver.ExecuteOptions) (string, error) {
	logger := logr.FromContextOrDiscard(ctx)
	tempDir, err := os.MkdirTemp("", "radius-imagebuild-")
	if err != nil {
		return "", fmt.Errorf("failed to create working directory for %q script: %w", imageBuildOutputName, err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	dockerConfigDir := ""
	if spec.RegistrySecretName != "" {
		dockerConfigDir = filepath.Join(tempDir, "docker")
		if err := d.writeDockerConfig(ctx, spec, dockerConfigDir, opts); err != nil {
			return "", err
		}
	}

	resultPath := filepath.Join(tempDir, "result.json")
	// Do not let ambient dynamic-rp paths or credentials reach the build.
	env := imageBuildEnvironment(os.Environ(), dockerConfigDir, resultPath)
	stderrTail, err := runScript(ctx, script, imageBuildArguments(spec), env, tempDir, logger)
	if err != nil {
		message := fmt.Sprintf("recipe %q script failed: %s", imageBuildOutputName, err.Error())
		if stderrTail != "" {
			message += "\nstderr (tail):\n" + stderrTail
		}
		return "", errors.New(message)
	}

	return readScriptResult(resultPath)
}

// writeDockerConfig materializes registry credentials from the recipe runtime namespace. The
// Secret values are already decoded by client-go and are never logged.
func (d *bicepDriver) writeDockerConfig(ctx context.Context, spec *imageBuildSpec, dir string, opts driver.ExecuteOptions) error {
	runtime := opts.Configuration.Runtime.Kubernetes
	if runtime == nil || runtime.Namespace == "" {
		return fmt.Errorf("output %q references Secret %q but the recipe has no Kubernetes runtime namespace", imageBuildOutputName, spec.RegistrySecretName)
	}
	if d.clusterAccessResolver == nil {
		return fmt.Errorf("output %q references Secret %q but the driver has no cluster access resolver configured", imageBuildOutputName, spec.RegistrySecretName)
	}

	config, err := d.clusterAccessResolver.Resolve(ctx, &opts.Configuration)
	if err != nil {
		return fmt.Errorf("failed to resolve the target cluster for registry Secret '%s/%s': %w", runtime.Namespace, spec.RegistrySecretName, err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create a target-cluster client for registry Secret '%s/%s': %w", runtime.Namespace, spec.RegistrySecretName, err)
	}
	secret, err := clientset.CoreV1().Secrets(runtime.Namespace).Get(ctx, spec.RegistrySecretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to read registry Secret '%s/%s': %w", runtime.Namespace, spec.RegistrySecretName, err)
	}
	username, ok := secret.Data["username"]
	if !ok {
		return fmt.Errorf("registry Secret '%s/%s' has no 'username' key", runtime.Namespace, spec.RegistrySecretName)
	}
	password, ok := secret.Data["password"]
	if !ok {
		return fmt.Errorf("registry Secret '%s/%s' has no 'password' key", runtime.Namespace, spec.RegistrySecretName)
	}

	registryHost := strings.SplitN(spec.Registry, "/", 2)[0]
	if registryHost == "" {
		return fmt.Errorf("output %q has an empty registry host", imageBuildOutputName)
	}
	auth := base64.StdEncoding.EncodeToString([]byte(string(username) + ":" + string(password)))
	configBytes, err := json.Marshal(map[string]any{
		"auths": map[string]any{registryHost: map[string]any{"auth": auth}},
	})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "config.json"), configBytes, 0o600)
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
	reader := bufio.NewReader(stream)
	for {
		text, err := reader.ReadString('\n')
		if text != "" {
			logger.Info(logPrefix + strings.TrimRight(text, "\r\n"))
			if tail != nil {
				tail.appendText(text)
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// readScriptResult enforces the complete public result contract. The file must contain only a
// non-empty imageReference string and is read only after the script exits successfully.
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

func (t *tailBuffer) appendText(text string) {
	t.data = append(t.data, []byte(text)...)
	if len(t.data) > t.limit {
		t.data = t.data[len(t.data)-t.limit:]
	}
}

func (t *tailBuffer) String() string {
	return string(t.data)
}
