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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/driver"
	"github.com/radius-project/radius/pkg/recipes/kubernetes/clusteraccess"
)

func requireScriptShell(t *testing.T) {
	t.Helper()
	if filepath.Separator == '\\' {
		t.Skip("requires POSIX /bin/sh")
	}
}

func validImageBuildValue() map[string]any {
	return map[string]any{
		"resourceName": "testimage",
		"tag":          "v1",
		"tagProvided":  true,
		"source":       "git::https://github.com/radius-project/samples.git?ref=main",
		"dockerfile":   "samples/demo/Dockerfile",
		"platforms":    []any{"linux/amd64", "linux/arm64"},
		"buildArgs":    map[string]any{"MODE": "release", "VERSION": "1"},
	}
}

func imageBuildOutputs(value any) map[string]any {
	return map[string]any{
		imageBuildOutputName: map[string]any{"type": "Object", "value": value},
	}
}

func Test_ExtractImageBuild_Absent(t *testing.T) {
	imageBuild, err := extractImageBuild(map[string]any{"result": map[string]any{}})
	require.NoError(t, err)
	require.Nil(t, imageBuild)

	imageBuild, err = extractImageBuild(nil)
	require.NoError(t, err)
	require.Nil(t, imageBuild)
}

func Test_ExtractImageBuild_Valid(t *testing.T) {
	// The driver returns the imageBuild object verbatim; the recipe and its script own the schema.
	imageBuild, err := extractImageBuild(imageBuildOutputs(validImageBuildValue()))
	require.NoError(t, err)
	require.Equal(t, validImageBuildValue(), imageBuild)

	// Extra fields flow through untouched instead of being rejected.
	extended := validImageBuildValue()
	extended["newBuildField"] = "future"
	imageBuild, err = extractImageBuild(imageBuildOutputs(extended))
	require.NoError(t, err)
	require.Equal(t, "future", imageBuild["newBuildField"])
}

func Test_ExtractImageBuild_Invalid(t *testing.T) {
	tests := []struct {
		name    string
		outputs any
		errPart string
	}{
		{"output not object", imageBuildOutputs("value"), "must evaluate to an object"},
		{"imageBuild not object", map[string]any{imageBuildOutputName: "scalar"}, "must be an object output"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := extractImageBuild(tc.outputs)
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

func Test_ImageBuildArguments_AreGenericAndDeterministic(t *testing.T) {
	// Flags are derived from property keys and ordered, so adding a build field needs no driver change.
	args, err := imageBuildArguments(map[string]any{
		"resourceName": "testimage",
		"registry":     "ghcr.io/radius-project",
		"tag":          "",
		"tagProvided":  true,
		"source":       "git::https://example.com/repo.git?ref=main&submodules=true",
		"dockerfile":   "Dockerfile",
		"platforms":    []any{"linux/arm64", "linux/amd64"},
		"buildArgs":    map[string]any{"Z_ARG": "last", "A_ARG": "value; $(false)"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{
		"--buildArgs", "A_ARG", "value; $(false)",
		"--buildArgs", "Z_ARG", "last",
		"--dockerfile", "Dockerfile",
		"--platforms", "linux/arm64",
		"--platforms", "linux/amd64",
		"--registry", "ghcr.io/radius-project",
		"--resourceName", "testimage",
		"--source", "git::https://example.com/repo.git?ref=main&submodules=true",
		"--tag", "",
		"--tagProvided",
	}, args)
}

func Test_ImageBuildArguments_OmitsFalseAndEmptyCollections(t *testing.T) {
	args, err := imageBuildArguments(map[string]any{
		"disabled":    false,
		"emptyArray":  []any{},
		"emptyObject": map[string]any{},
	})
	require.NoError(t, err)
	require.Empty(t, args)
}

func Test_ImageBuildArguments_RejectsUnsupportedValues(t *testing.T) {
	tests := []struct {
		name        string
		buildInputs map[string]any
		errPart     string
	}{
		{"null value", map[string]any{"source": nil}, `property "source" must not be null`},
		{"array item not string", map[string]any{"platforms": []any{42}}, `property "platforms" must contain only string values`},
		{"object entry not string", map[string]any{"buildArgs": map[string]any{"PORT": 8080}}, `property "buildArgs" entry "PORT" must be a string`},
		{"unsupported type", map[string]any{"tag": 42}, `property "tag" has an unsupported type`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := imageBuildArguments(tc.buildInputs)
			require.ErrorContains(t, err, tc.errPart)
		})
	}
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

func Test_HasImageBuildProperty_IgnoresOtherResourceTypes(t *testing.T) {
	d := &bicepDriver{}
	should, err := d.hasImageBuildProperty("Radius.Compute/containers", imageBuildOutputs("invalid"))
	require.NoError(t, err)
	require.False(t, should)
}

func Test_HasImageBuildProperty(t *testing.T) {
	d := &bicepDriver{}

	should, err := d.hasImageBuildProperty(containerImagesResourceType, imageBuildOutputs(validImageBuildValue()))
	require.NoError(t, err)
	require.True(t, should)

	should, err = d.hasImageBuildProperty(containerImagesResourceType, map[string]any{"result": map[string]any{}})
	require.NoError(t, err)
	require.False(t, should)

	should, err = d.hasImageBuildProperty(containerImagesResourceType, imageBuildOutputs("invalid"))
	require.Error(t, err)
	require.False(t, should)
}

func Test_ExecuteImageBuildHook_PassesBuildContractWithOperatorRegistry(t *testing.T) {
	requireScriptShell(t)

	script := `set -eu
build_args=
dockerfile=
platforms=
registry=
resource_name=
source=
tag=
tag_provided=0
while [ "$#" -gt 0 ]; do
  case "$1" in
    --buildArgs)
      pair="$2=$3"
      if [ -z "$build_args" ]; then build_args=$pair; else build_args="$build_args,$pair"; fi
      shift 3
      ;;
    --dockerfile) dockerfile=$2; shift 2 ;;
    --platforms)
      if [ -z "$platforms" ]; then platforms=$2; else platforms="$platforms,$2"; fi
      shift 2
      ;;
    --registry) registry=$2; shift 2 ;;
    --resourceName) resource_name=$2; shift 2 ;;
    --source) source=$2; shift 2 ;;
    --tag) tag=$2; shift 2 ;;
    --tagProvided) tag_provided=1; shift ;;
    *) exit 9 ;;
  esac
done
[ "$build_args" = "MODE=release,VERSION=1" ]
[ "$dockerfile" = "samples/demo/Dockerfile" ]
[ "$platforms" = "linux/amd64,linux/arm64" ]
[ "$registry" = "ghcr.io/radius-project" ]
[ "$resource_name" = "testimage" ]
[ "$source" = "git::https://github.com/radius-project/samples.git?ref=main" ]
[ "$tag" = "v1" ]
[ "$tag_provided" -eq 1 ]
printf '{"imageReference":"%s/%s:built"}' "$registry" "$resource_name" > "$RADIUS_EXEC_OUTPUT"`
	value := validImageBuildValue()
	value[registryParameterName] = "developer.example/exfiltration"
	value["disabled"] = false
	value["emptyArray"] = []any{}
	value["emptyObject"] = map[string]any{}
	response := &recipes.RecipeOutput{Values: map[string]any{imageBuildOutputName: "plumbing"}}
	err := (&bicepDriver{}).executeImageBuildHook(
		t.Context(),
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
	require.Equal(t, "developer.example/exfiltration", value[registryParameterName])
}

func Test_ExecuteImageBuildHook_UsesOnlyOperatorOwnedCredentials(t *testing.T) {
	requireScriptShell(t)

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
reg=
while [ "$#" -gt 0 ]; do
  case "$1" in
    --registry) reg=$2; shift 2 ;;
    *) shift ;;
  esac
done
[ "$reg" = "operator.example/team" ]
grep -F '"operator.example"' "$DOCKER_CONFIG/config.json" >/dev/null
if grep -F 'attacker.example' "$DOCKER_CONFIG/config.json" >/dev/null; then exit 1; fi
printf '{"imageReference":"operator.example/team/testimage:v1"}' > "$RADIUS_EXEC_OUTPUT"`
	value := validImageBuildValue()
	response := &recipes.RecipeOutput{}
	d := &bicepDriver{clusterAccessResolver: clusteraccess.NewResolver()}
	err = d.executeImageBuildHook(
		t.Context(),
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

func Test_OperatorRegistryParameters(t *testing.T) {
	registry, registrySecretName, err := operatorRegistryParameters(map[string]any{
		registryParameterName:           "ghcr.io/radius-project",
		registrySecretNameParameterName: "operator-secret",
	})
	require.NoError(t, err)
	require.Equal(t, "ghcr.io/radius-project", registry)
	require.Equal(t, "operator-secret", registrySecretName)

	// Omitting the optional operator parameter yields an empty secret name.
	registry, registrySecretName, err = operatorRegistryParameters(map[string]any{
		registryParameterName: "ttl.sh/radius-project",
	})
	require.NoError(t, err)
	require.Equal(t, "ttl.sh/radius-project", registry)
	require.Equal(t, "", registrySecretName)

	// JSON null from recipe registration is equivalent to omitting the optional parameter.
	_, registrySecretName, err = operatorRegistryParameters(map[string]any{
		registryParameterName:           "ttl.sh/radius-project",
		registrySecretNameParameterName: nil,
	})
	require.NoError(t, err)
	require.Equal(t, "", registrySecretName)
}

func Test_OperatorRegistryParameters_RejectsInvalidDefinition(t *testing.T) {
	tests := []struct {
		name       string
		parameters map[string]any
		errPart    string
	}{
		{"missing registry", nil, `recipe registration to set a non-empty "registry" parameter`},
		{"empty registry", map[string]any{registryParameterName: ""}, `recipe registration to set a non-empty "registry" parameter`},
		{"wrong registry type", map[string]any{registryParameterName: 42}, `recipe registration to set a non-empty "registry" parameter`},
		{"wrong secret name type", map[string]any{registryParameterName: "ghcr.io/radius-project", registrySecretNameParameterName: []any{"secret"}}, `parameter "registrySecretName" must be a string`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := operatorRegistryParameters(tc.parameters)
			require.ErrorContains(t, err, tc.errPart)
		})
	}
}

func Test_ExecuteImageBuild_FailureSurfacesStderr(t *testing.T) {
	requireScriptShell(t)

	_, err := (&bicepDriver{}).executeImageBuild(t.Context(), `echo "registry unreachable" >&2; exit 7`, map[string]any{}, "", "", driver.ExecuteOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "exit status 7")
	require.Contains(t, err.Error(), "registry unreachable")
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	require.Equal(t, 7, exitErr.ExitCode())
}

func Test_ExecuteImageBuild_StrictResultContract(t *testing.T) {
	requireScriptShell(t)

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
			_, err := (&bicepDriver{}).executeImageBuild(t.Context(), tc.script, map[string]any{}, "", "", driver.ExecuteOptions{})
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.errPart)
		})
	}
}

func Test_ExecuteImageBuild_UnauthenticatedBlanksDockerConfig(t *testing.T) {
	requireScriptShell(t)

	t.Setenv(dockerConfigEnvName, "/ambient/docker")
	script := `printf '{"imageReference":"probe%s"}' "${DOCKER_CONFIG-unset}" > "$RADIUS_EXEC_OUTPUT"`
	imageReference, err := (&bicepDriver{}).executeImageBuild(t.Context(), script, map[string]any{}, "", "", driver.ExecuteOptions{})
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
	err = d.writeDockerConfig(t.Context(), "ghcr.io/radius-project", "ghcr-creds", dir, driver.ExecuteOptions{BaseOptions: driver.BaseOptions{Configuration: recipes.Configuration{
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

func Test_DockerConfigAuthKey(t *testing.T) {
	tests := []struct {
		name     string
		registry string
		want     string
		wantErr  bool
	}{
		{"ghcr path", "ghcr.io/radius-project", "ghcr.io", false},
		{"docker hub", "docker.io/radius-project", "https://index.docker.io/v1/", false},
		{"docker hub index alias", "index.docker.io/radius-project", "https://index.docker.io/v1/", false},
		{"docker hub registry alias", "registry-1.docker.io/radius-project", "https://index.docker.io/v1/", false},
		{"localhost port", "localhost:5000/radius-project", "localhost:5000", false},
		{"empty", "", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := dockerConfigAuthKey(tc.registry)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func Test_WriteDockerConfig_SecretFailures(t *testing.T) {
	registry, registrySecretName := "ghcr.io/org", "missing"
	opts := driver.ExecuteOptions{BaseOptions: driver.BaseOptions{Configuration: recipes.Configuration{
		Runtime: recipes.RuntimeConfiguration{Kubernetes: &recipes.KubernetesRuntime{Namespace: "testapp"}},
	}}}

	err := (&bicepDriver{}).writeDockerConfig(t.Context(), registry, registrySecretName, t.TempDir(), opts)
	require.ErrorContains(t, err, "no cluster access resolver configured")

	missingPath := filepath.Join(t.TempDir(), "missing-target.kubeconfig")
	t.Setenv(clusteraccess.TargetKubeconfigEnvVar, missingPath)
	d := &bicepDriver{clusterAccessResolver: clusteraccess.NewResolver()}
	err = d.writeDockerConfig(t.Context(), registry, registrySecretName, t.TempDir(), opts)
	require.ErrorContains(t, err, clusteraccess.TargetKubeconfigEnvVar)
	require.ErrorContains(t, err, missingPath)
}

func Test_DrainScriptStream_LongLineBoundsLoggingAndTail(t *testing.T) {
	var messages []string
	logInfoLevel := ""
	logger := funcr.New(func(prefix, args string) {
		messages = append(messages, prefix+args)
	}, funcr.Options{LogInfoLevel: &logInfoLevel})
	tail := &bytes.Buffer{}

	err := drainScriptStream(strings.NewReader(strings.Repeat("x", 1_100_000)), "imageBuild(stderr): ", logger, tail, stderrTailLimit)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Contains(t, messages[0], scriptLogTruncationMarker)
	require.LessOrEqual(t, len(messages[0]), scriptLogLineLimit+128)
	require.Equal(t, strings.Repeat("x", stderrTailLimit), tail.String())
}

func Test_RunScript_CancelKillsSpawnedProcesses(t *testing.T) {
	requireScriptShell(t)

	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := runScript(ctx, "sleep 30 & wait", nil, os.Environ(), t.TempDir(), logr.Discard())
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Contains(t, err.Error(), "canceled or timed out")
	require.Less(t, time.Since(start), 10*time.Second)
}

func Test_ReadScriptResult_InvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "result.json")
	require.NoError(t, os.WriteFile(path, []byte("not json"), 0o600))
	_, err := readScriptResult(path)
	require.ErrorContains(t, err, "not a JSON object")
}
