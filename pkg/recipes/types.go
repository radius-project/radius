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

package recipes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

// Configuration represents kubernetes runtime and cloud provider configuration, which is used by the driver while deploying recipes.
type Configuration struct {
	// Kubernetes Runtime configuration for the environment.
	Runtime RuntimeConfiguration
	// Cloud providers configuration for the environment
	Providers datamodel.Providers
	// Simulated represents whether the environment is simulated or not.
	Simulated bool

	RecipeConfig datamodel.RecipeConfigProperties
}

// RuntimeConfiguration represents Kubernetes Runtime configuration for the environment.
type RuntimeConfiguration struct {
	Kubernetes *KubernetesRuntime `json:"kubernetes,omitempty"`
}

// KubernetesRuntime represents application and environment namespaces.
type KubernetesRuntime struct {
	// Namespace is set to the application namespace when the portable resource is application-scoped, and set to the environment namespace when it is environment scoped
	Namespace string `json:"namespace,omitempty"`
	// EnvironmentNamespace is set to environment namespace.
	EnvironmentNamespace string `json:"environmentNamespace"`
}

// EnvironmentDefinition represents the recipe configuration details.
type EnvironmentDefinition struct {
	// Name represents the name of the recipe within the environment
	Name string
	// Driver represents the kind of infrastructure language used to define recipe.
	Driver string
	// ResourceType represents the type of the portable resource this recipe can be consumed by.
	ResourceType string
	// Parameters represents key/value pairs to pass into the recipe template for every resource using this recipe. Specified during recipe registration to environment. Can be overridden by the radius resource consuming this recipe.
	Parameters map[string]any
	// TemplatePath represents path to the template provided by the recipe.
	TemplatePath string
	// TemplateVersion represents the version of the terraform module provided by the recipe.
	TemplateVersion string
	// Allows insecure connections to registry without SSL check.
	PlainHTTP bool
}

// ResourceMetadata represents recipe details provided while creating a portable resource.
type ResourceMetadata struct {
	// Name represents the name of the recipe within the environment
	Name string
	// ApplicationID represents fully qualified resource ID for the application that the portable resource is consumed by
	ApplicationID string
	// EnvironmentID represents fully qualified resource ID for the environment that the portable resource is linked to
	EnvironmentID string
	// ResourceID represents fully qualified resource ID for the resource the recipe is deploying
	ResourceID string
	// Parameters represents key/value pairs to pass into the recipe template. Overrides any parameters set by the environment.
	Parameters map[string]any
}

const (
	TemplateKindBicep     = "bicep"
	TemplateKindTerraform = "terraform"

	// Recipe outputs are expected to be wrapped under an object named "result"
	ResultPropertyName = "result"
)

var (
	SupportedTemplateKind = []string{TemplateKindBicep, TemplateKindTerraform}
)

// RecipeOutput represents recipe deployment output.
type RecipeOutput struct {
	// Resources represents the list of output resources deployed recipe.
	Resources []string

	// Secrets represents the key/value pairs of secret values of the deployed resource.
	Secrets map[string]any

	// Values represents the key/value pairs of properties of the deployed resource.
	Values map[string]any

	// Status represents the recipe status at deployment time of resource.
	Status *rpv1.RecipeStatus
}

// PrepareRecipeOutput populates the recipe output from the recipe deployment output stored in the "result" object.
// outputs map is the value of "result" output from the recipe deployment response.
func (ro *RecipeOutput) PrepareRecipeResponse(resultValue map[string]any) error {
	b, err := json.Marshal(&resultValue)
	if err != nil {
		return err
	}

	// Using a decoder to block unknown fields.
	decoder := json.NewDecoder(bytes.NewBuffer(b))
	decoder.DisallowUnknownFields()
	err = decoder.Decode(ro)
	if err != nil {
		return err
	}

	// Make sure maps are non-nil (it's just friendly).
	if ro.Secrets == nil {
		ro.Secrets = map[string]any{}
	}
	if ro.Values == nil {
		ro.Values = map[string]any{}
	}
	if ro.Resources == nil {
		ro.Resources = []string{}
	}

	return nil
}

// GetSecretStoreID returns secretstore resource ID associated with git private terraform repository source.
func GetSecretStoreID(envConfig Configuration, templatePath string) (string, error) {
	if strings.HasPrefix(templatePath, "git::") {
		url, err := GetGitURL(templatePath)
		if err != nil {
			return "", err
		}

		// get the secret store id associated with the git domain of the template path.
		return envConfig.RecipeConfig.Terraform.Authentication.Git.PAT[strings.TrimPrefix(url.Hostname(), "www.")].Secret, nil
	}
	return "", nil
}

// GetGitURL returns git url from generic git module source.
// git::https://exmaple.com/project/module -> https://exmaple.com/project/module
func GetGitURL(templatePath string) (*url.URL, error) {
	paths := strings.Split(templatePath, "git::")
	gitUrl := paths[len(paths)-1]
	url, err := url.Parse(gitUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse git url %s : %w", gitUrl, err)
	}

	return url, nil
}

// GetEnvAppResourceNames returns the application, environment and resource names.
func GetEnvAppResourceNames(resourceMetadata *ResourceMetadata) (string, string, string, error) {
	app, err := resources.ParseResource(resourceMetadata.ApplicationID)
	if err != nil {
		return "", "", "", err
	}

	env, err := resources.ParseResource(resourceMetadata.EnvironmentID)
	if err != nil {
		return "", "", "", err
	}

	resource, err := resources.ParseResource(resourceMetadata.ResourceID)
	if err != nil {
		return "", "", "", err
	}

	return env.Name(), app.Name(), resource.Name(), nil
}

// GetURLPrefix returns the url prefix to be added to the template path before adding it to the .gitconfig and terraform config.
func GetURLPrefix(resourceRecipe *ResourceMetadata) (string, error) {
	env, app, resource, err := GetEnvAppResourceNames(resourceRecipe)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("https://%s-%s-%s-", env, app, resource), nil
}
