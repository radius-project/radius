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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/driver"
	"github.com/radius-project/radius/pkg/recipes/kubernetes/clusteraccess"
	"github.com/radius-project/radius/test/testcontext"
)

func validImageBuildValue() map[string]any {
	return map[string]any{
		"resourceName":       "testimage",
		"registry":           "ghcr.io/radius-project",
		"registrySecretName": "ghcr-creds",
		"tag":                "v1",
		"tagProvided":        true,
		"source":             "git::https://github.com/radius-project/samples.git?ref=main",
		"dockerfile":         "samples/demo/Dockerfile",
		"platforms":          []any{"linux/amd64", "linux/arm64"},
		"buildArgs":          map[string]any{"MODE": "release", "VERSION": "1"},
	}
}

func imageBuildOutputs(value any) map[string]any {
	return map[string]any{
		imageBuildOutputName: map[string]any{"type": "Object", "value": value},
	}
}

func Test_ExtractImageBuildSpec_Absent(t *testing.T) {
	spec, err := extractImageBuildSpec(map[string]any{"result": map[string]any{}})
	require.NoError(t, err)
	require.Nil(t, spec)

	spec, err = extractImageBuildSpec(nil)
	require.NoError(t, err)
	require.Nil(t, spec)
}

func Test_ExtractImageBuildSpec_Valid(t *testing.T) {
	spec, err := extractImageBuildSpec(imageBuildOutputs(validImageBuildValue()))
	require.NoError(t, err)
	require.Equal(t, &imageBuildSpec{
		ResourceName:       "testimage",
		Registry:           "ghcr.io/radius-project",
		RegistrySecretName: "ghcr-creds",
		Tag:                "v1",
		TagProvided:        true,
		Source:             "git::https://github.com/radius-project/samples.git?ref=main",
		Dockerfile:         "samples/demo/Dockerfile",
		Platforms:          []string{"linux/amd64", "linux/arm64"},
		BuildArgs:          map[string]string{"MODE": "release", "VERSION": "1"},
	}, spec)
}

func Test_ExtractImageBuildSpec_Invalid(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(map[string]any) any
		errPart string
	}{
		{"non-object", func(map[string]any) any { return "value" }, "must evaluate to an object"},
		{"unknown property", func(v map[string]any) any { v["script"] = "echo"; return v }, `unknown field "script"`},
		{"missing property", func(v map[string]any) any { delete(v, "registry"); return v }, `missing required property "registry"`},
		{"null property", func(v map[string]any) any { v["registry"] = nil; return v }, `property "registry" must not be null`},
		{"wrong string type", func(v map[string]any) any { v["tag"] = 42; return v }, "cannot unmarshal number"},
		{"wrong boolean type", func(v map[string]any) any { v["tagProvided"] = "true"; return v }, "cannot unmarshal string"},
		{"platforms not array", func(v map[string]any) any { v["platforms"] = "linux/amd64"; return v }, "cannot unmarshal string"},
		{"platform not string", func(v map[string]any) any { v["platforms"] = []any{42}; return v }, "cannot unmarshal number"},
		{"build args not object", func(v map[string]any) any { v["buildArgs"] = []any{}; return v }, "cannot unmarshal array"},
		{"build arg not string", func(v map[string]any) any { v["buildArgs"] = map[string]any{"PORT": 8080}; return v }, "cannot unmarshal number"},
		{"case mismatch", func(v map[string]any) any { delete(v, "tag"); v["Tag"] = "v1"; return v }, `missing required property "tag"`},
		{"duplicate case alias", func(v map[string]any) any { v["Tag"] = "v2"; return v }, "must contain exactly 9 properties"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := extractImageBuildSpec(imageBuildOutputs(tc.mutate(validImageBuildValue())))
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.errPart)
		})
	}
}

func Test_ExtractImageBuildScript(t *testing.T) {
	script := "#!/bin/sh\nset -eu\n"
	actual, err := extractImageBuildScript(map[string]any{
		"variables": map[string]any{
			containerImagesBuildScriptVariableName: script,
		},
	})
	require.NoError(t, err)
	require.Equal(t, script, actual)
}

func Test_ExtractImageBuildScript_RejectsMissingOrDynamicVariable(t *testing.T) {
	tests := []struct {
		name     string
		template map[string]any
		errPart  string
	}{
		{"no variables", map[string]any{}, "has no variables"},
		{"missing script", map[string]any{"variables": map[string]any{}}, "must be a non-empty string"},
		{"expression", map[string]any{"variables": map[string]any{containerImagesBuildScriptVariableName: "[parameters('script')]"}}, "must be static"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := extractImageBuildScript(tc.template)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.errPart)
		})
	}
}

func Test_SupportsImageBuildHook(t *testing.T) {
	require.True(t, supportsImageBuildHook("Radius.Compute/containerImages"))
	require.True(t, supportsImageBuildHook("radius.compute/containerimages"))
	require.False(t, supportsImageBuildHook("Radius.Compute/containers"))
}

func Test_ImageBuildArguments_AreTypedAndDeterministic(t *testing.T) {
	args := imageBuildArguments(&imageBuildSpec{
		ResourceName:       "testimage",
		Registry:           "ghcr.io/radius-project",
		RegistrySecretName: "creds",
		Tag:                "",
		TagProvided:        true,
		Source:             "git::https://example.com/repo.git?ref=main&submodules=true",
		Dockerfile:         "Dockerfile",
		Platforms:          []string{"linux/arm64", "linux/amd64"},
		BuildArgs:          map[string]string{"Z_ARG": "last", "A_ARG": "value; $(false)"},
	})
	require.Equal(t, []string{
		"--resource-name", "testimage",
		"--registry", "ghcr.io/radius-project",
		"--tag", "",
		"--source", "git::https://example.com/repo.git?ref=main&submodules=true",
		"--dockerfile", "Dockerfile",
		"--tag-provided",
		"--platform", "linux/arm64",
		"--platform", "linux/amd64",
		"--build-arg", "A_ARG", "value; $(false)",
		"--build-arg", "Z_ARG", "last",
	}, args)

	args = imageBuildArguments(&imageBuildSpec{Tag: "", TagProvided: false})
	require.NotContains(t, args, "--tag-provided")
}

func Test_ImageBuildEnvironment_ReplacesControlledValues(t *testing.T) {
	env := imageBuildEnvironment([]string{
		"PATH=/usr/bin",
		dockerConfigEnvName + "=/ambient/first",
		execOutputEnvName + "=/ambient/result",
		dockerConfigEnvName + "=/ambient/second",
	}, "/controlled/docker", "/controlled/result")

	require.Equal(t, []string{
		"PATH=/usr/bin",
		dockerConfigEnvName + "=/controlled/docker",
		execOutputEnvName + "=/controlled/result",
	}, env)
}

func Test_ExecuteImageBuildHook_IgnoresOtherResourceTypes(t *testing.T) {
	d := &bicepDriver{}
	response := &recipes.RecipeOutput{Values: map[string]any{imageBuildOutputName: "ordinary"}}
	err := d.executeImageBuildHook(testcontext.New(t), map[string]any{}, imageBuildOutputs("invalid"), response, driver.ExecuteOptions{
		BaseOptions: driver.BaseOptions{Definition: recipes.EnvironmentDefinition{ResourceType: "Radius.Compute/containers"}},
	})
	require.NoError(t, err)
	require.Equal(t, "ordinary", response.Values[imageBuildOutputName])
}

func Test_ExecuteImageBuildHook_UsesOperatorOwnedRegistryParameters(t *testing.T) {
	script := `set -eu
[ "$1" = "--resource-name" ]
[ "$2" = "testimage" ]
[ "$3" = "--registry" ]
printf '{"imageReference":"%s/%s:built"}' "$4" "$2" > "$RADIUS_EXEC_OUTPUT"`
	value := validImageBuildValue()
	value["registry"] = "attacker.example/exfiltration"
	value["registrySecretName"] = "attacker-controlled-secret"
	response := &recipes.RecipeOutput{Values: map[string]any{imageBuildOutputName: "plumbing"}}
	err := (&bicepDriver{}).executeImageBuildHook(
		testcontext.New(t),
		map[string]any{"variables": map[string]any{containerImagesBuildScriptVariableName: script}},
		imageBuildOutputs(value),
		response,
		driver.ExecuteOptions{BaseOptions: driver.BaseOptions{Definition: recipes.EnvironmentDefinition{
			ResourceType: "radius.compute/containerimages",
			Parameters:   map[string]any{registryParameterName: "ghcr.io/radius-project"},
		}}},
	)
	require.NoError(t, err)
	require.Equal(t, "ghcr.io/radius-project/testimage:built", response.Values[imageReferenceValueName])
	require.NotContains(t, response.Values, imageBuildOutputName)
}

func Test_ExecuteImageBuildHook_UsesOnlyOperatorOwnedCredentials(t *testing.T) {
	secret := &corev1.Secret{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{Name: "operator-secret", Namespace: "testapp"},
		Data: map[string][]byte{
			"username": []byte("operator"),
			"password": []byte("s3cret"),
		},
	}
	secretJSON, err := json.Marshal(secret)
	require.NoError(t, err)

	requestPaths := make(chan string, 1)
	targetCluster := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPaths <- r.URL.Path
		if r.URL.Path != "/api/v1/namespaces/testapp/secrets/operator-secret" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(secretJSON)
	}))
	defer targetCluster.Close()

	kubeconfigPath := filepath.Join(t.TempDir(), "target.kubeconfig")
	kubeconfig := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: target
  cluster:
    server: %s
contexts:
- name: target
  context:
    cluster: target
    user: target
current-context: target
users:
- name: target
  user: {}
`, targetCluster.URL)
	require.NoError(t, os.WriteFile(kubeconfigPath, []byte(kubeconfig), 0o600))
	t.Setenv(clusteraccess.TargetKubeconfigEnvVar, kubeconfigPath)

	script := `set -eu
[ "$4" = "operator.example/team" ]
grep -F '"operator.example"' "$DOCKER_CONFIG/config.json" >/dev/null
if grep -F 'attacker.example' "$DOCKER_CONFIG/config.json" >/dev/null; then exit 1; fi
printf '{"imageReference":"operator.example/team/testimage:v1"}' > "$RADIUS_EXEC_OUTPUT"`
	value := validImageBuildValue()
	value["registry"] = "attacker.example/exfiltration"
	value["registrySecretName"] = "attacker-controlled-secret"
	response := &recipes.RecipeOutput{}
	d := &bicepDriver{clusterAccessResolver: clusteraccess.NewResolver()}
	err = d.executeImageBuildHook(
		testcontext.New(t),
		map[string]any{"variables": map[string]any{containerImagesBuildScriptVariableName: script}},
		imageBuildOutputs(value),
		response,
		driver.ExecuteOptions{BaseOptions: driver.BaseOptions{
			Configuration: recipes.Configuration{Runtime: recipes.RuntimeConfiguration{
				Kubernetes: &recipes.KubernetesRuntime{Namespace: "testapp"},
			}},
			Definition: recipes.EnvironmentDefinition{
				ResourceType: containerImagesResourceType,
				Parameters: map[string]any{
					registryParameterName:           "operator.example/team",
					registrySecretNameParameterName: "operator-secret",
				},
			},
		}},
	)
	require.NoError(t, err)
	require.Equal(t, "/api/v1/namespaces/testapp/secrets/operator-secret", <-requestPaths)
	require.Equal(t, "operator.example/team/testimage:v1", response.Values[imageReferenceValueName])
}

func Test_ApplyOperatorImageBuildParameters(t *testing.T) {
	spec := &imageBuildSpec{
		Registry:           "attacker.example/exfiltration",
		RegistrySecretName: "attacker-controlled-secret",
	}
	err := applyOperatorImageBuildParameters(spec, map[string]any{
		registryParameterName:           "ghcr.io/radius-project",
		registrySecretNameParameterName: "operator-secret",
	})
	require.NoError(t, err)
	require.Equal(t, "ghcr.io/radius-project", spec.Registry)
	require.Equal(t, "operator-secret", spec.RegistrySecretName)

	// Omitting the optional operator parameter must clear an output value supplied by a developer.
	spec.RegistrySecretName = "attacker-controlled-secret"
	err = applyOperatorImageBuildParameters(spec, map[string]any{
		registryParameterName: "ttl.sh/radius-project",
	})
	require.NoError(t, err)
	require.Equal(t, "", spec.RegistrySecretName)
}

func Test_ApplyOperatorImageBuildParameters_RejectsInvalidDefinition(t *testing.T) {
	tests := []struct {
		name       string
		parameters map[string]any
		errPart    string
	}{
		{"missing registry", nil, `missing required parameter "registry"`},
		{"empty registry", map[string]any{registryParameterName: ""}, `parameter "registry" must be a non-empty string`},
		{"wrong registry type", map[string]any{registryParameterName: 42}, `parameter "registry" must be a non-empty string`},
		{"wrong secret name type", map[string]any{registryParameterName: "ghcr.io/radius-project", registrySecretNameParameterName: []any{"secret"}}, `parameter "registrySecretName" must be a string`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := applyOperatorImageBuildParameters(&imageBuildSpec{}, tc.parameters)
			require.ErrorContains(t, err, tc.errPart)
		})
	}
}

func Test_ExecuteImageBuild_FailureSurfacesStderr(t *testing.T) {
	_, err := (&bicepDriver{}).executeImageBuild(testcontext.New(t), `echo "registry unreachable" >&2; exit 7`, &imageBuildSpec{}, driver.ExecuteOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "exit status 7")
	require.Contains(t, err.Error(), "registry unreachable")
}

func Test_ExecuteImageBuild_StrictResultContract(t *testing.T) {
	tests := []struct {
		name    string
		script  string
		errPart string
	}{
		{"missing result", "true", "without writing"},
		{"empty reference", `printf '{"imageReference":""}' > "$RADIUS_EXEC_OUTPUT"`, "non-empty"},
		{"non-string reference", `printf '{"imageReference":42}' > "$RADIUS_EXEC_OUTPUT"`, "non-empty"},
		{"unexpected key", `printf '{"imageReference":"r/app:t","digest":"x"}' > "$RADIUS_EXEC_OUTPUT"`, "must contain only"},
		{"wrong key", `printf '{"reference":"r/app:t"}' > "$RADIUS_EXEC_OUTPUT"`, "must contain only"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := (&bicepDriver{}).executeImageBuild(testcontext.New(t), tc.script, &imageBuildSpec{}, driver.ExecuteOptions{})
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.errPart)
		})
	}
}

func Test_ExecuteImageBuild_UnauthenticatedBlanksDockerConfig(t *testing.T) {
	t.Setenv(dockerConfigEnvName, "/ambient/docker")
	script := `printf '{"imageReference":"probe%s"}' "${DOCKER_CONFIG-unset}" > "$RADIUS_EXEC_OUTPUT"`
	imageReference, err := (&bicepDriver{}).executeImageBuild(testcontext.New(t), script, &imageBuildSpec{}, driver.ExecuteOptions{})
	require.NoError(t, err)
	require.Equal(t, "probe", imageReference)
}

func Test_WriteDockerConfig_ReadsDecodedSecretFromTargetCluster(t *testing.T) {
	secret := &corev1.Secret{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{Name: "ghcr-creds", Namespace: "testapp"},
		Data: map[string][]byte{
			"username": []byte("octocat"),
			"password": []byte("s3cret"),
		},
	}
	secretJSON, err := json.Marshal(secret)
	require.NoError(t, err)
	requestPaths := make(chan string, 1)
	targetCluster := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPaths <- r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(secretJSON)
	}))
	defer targetCluster.Close()

	kubeconfigPath := filepath.Join(t.TempDir(), "target.kubeconfig")
	kubeconfig := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: target
  cluster:
    server: %s
contexts:
- name: target
  context:
    cluster: target
    user: target
current-context: target
users:
- name: target
  user: {}
`, targetCluster.URL)
	require.NoError(t, os.WriteFile(kubeconfigPath, []byte(kubeconfig), 0o600))
	t.Setenv(clusteraccess.TargetKubeconfigEnvVar, kubeconfigPath)

	d := &bicepDriver{clusterAccessResolver: clusteraccess.NewResolver()}
	dir := filepath.Join(t.TempDir(), "docker")
	err = d.writeDockerConfig(testcontext.New(t), &imageBuildSpec{
		Registry:           "ghcr.io/radius-project",
		RegistrySecretName: "ghcr-creds",
	}, dir, driver.ExecuteOptions{BaseOptions: driver.BaseOptions{Configuration: recipes.Configuration{
		Runtime: recipes.RuntimeConfiguration{Kubernetes: &recipes.KubernetesRuntime{Namespace: "testapp"}},
	}}})
	require.NoError(t, err)
	require.Equal(t, "/api/v1/namespaces/testapp/secrets/ghcr-creds", <-requestPaths)

	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	require.NoError(t, err)
	dirInfo, err := os.Stat(dir)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())
	configInfo, err := os.Stat(filepath.Join(dir, "config.json"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), configInfo.Mode().Perm())
	var config map[string]map[string]map[string]string
	require.NoError(t, json.Unmarshal(data, &config))
	require.Equal(t, "b2N0b2NhdDpzM2NyZXQ=", config["auths"]["ghcr.io"]["auth"])
}

func Test_WriteDockerConfig_SecretFailures(t *testing.T) {
	spec := &imageBuildSpec{Registry: "ghcr.io/org", RegistrySecretName: "missing"}
	opts := driver.ExecuteOptions{BaseOptions: driver.BaseOptions{Configuration: recipes.Configuration{
		Runtime: recipes.RuntimeConfiguration{Kubernetes: &recipes.KubernetesRuntime{Namespace: "testapp"}},
	}}}

	err := (&bicepDriver{}).writeDockerConfig(testcontext.New(t), spec, t.TempDir(), opts)
	require.ErrorContains(t, err, "no cluster access resolver configured")

	missingPath := filepath.Join(t.TempDir(), "missing-target.kubeconfig")
	t.Setenv(clusteraccess.TargetKubeconfigEnvVar, missingPath)
	d := &bicepDriver{clusterAccessResolver: clusteraccess.NewResolver()}
	err = d.writeDockerConfig(testcontext.New(t), spec, t.TempDir(), opts)
	require.ErrorContains(t, err, clusteraccess.TargetKubeconfigEnvVar)
	require.ErrorContains(t, err, missingPath)
}

func Test_RunScript_LongOutputLineDoesNotHang(t *testing.T) {
	ctx, cancel := context.WithTimeout(testcontext.New(t), 10*time.Second)
	defer cancel()
	stderrTail, err := runScript(ctx, "head -c 1100000 /dev/zero | tr '\\0' x >&2", nil, os.Environ(), t.TempDir(), logr.Discard())
	require.NoError(t, err)
	require.Equal(t, strings.Repeat("x", stderrTailLimit), stderrTail)
}

func Test_RunScript_CancelKillsSpawnedProcesses(t *testing.T) {
	ctx, cancel := context.WithTimeout(testcontext.New(t), 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := runScript(ctx, "sleep 30 & wait", nil, os.Environ(), t.TempDir(), logr.Discard())
	require.Error(t, err)
	require.Contains(t, err.Error(), "canceled or timed out")
	require.Less(t, time.Since(start), 10*time.Second)
}

func Test_ReadScriptResult_InvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "result.json")
	require.NoError(t, os.WriteFile(path, []byte("not json"), 0o600))
	_, err := readScriptResult(path)
	require.ErrorContains(t, err, "not a JSON object")
}
