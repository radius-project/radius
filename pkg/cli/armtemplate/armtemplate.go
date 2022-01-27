// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

// This package has TEMPORARY code that we use for fill the role of the ARM deployment engine
// in environments where it can't run right now (K8s, local testing). We don't intend to
// maintain this long-term and we don't intend to achieve parity.
package armtemplate

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const DeploymentResourceType = "Microsoft.Resources/deployments"

// DeploymentTemplate represents an ARM template.
type DeploymentTemplate struct {
	Schema         string `json:"$schema"`
	ContentVersion string `json:"contentVersion"`
	ApiProfile     string `json:"apiProfile"`

	// Parameters stores parameters in the format they appear inside an ARM template.
	// See: https://docs.microsoft.com/en-us/azure/azure-resource-manager/templates/syntax#parameters
	Parameters map[string]map[string]interface{} `json:"parameters"`
	Variables  map[string]interface{}            `json:"variables"`
	Functions  []interface{}                     `json:"functions"`
	Imports    map[string]Provider               `json:"imports"`
	Resources  []map[string]interface{}          `json:"resources"`
	Outputs    map[string]map[string]interface{} `json:"outputs"`
}

// Providers are extension imported in the Bicep file.
// Currently we only support 'Kubernetes'.
type Provider struct {
	Name    string `json:"provider"`
	Version string `json:"version"`
}

// Resource represents a (parsed) resource within an ARM template.
type Resource struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	APIVersion string    `json:"apiVersion"`
	Name       string    `json:"name"`
	DependsOn  []string  `json:"dependsOn"`
	Provider   *Provider `json:"provider,omitempty"`

	// Contains the actual payload that should be submitted (properties, kind, etc)
	// note that properties like type, name, and apiversion are present in deployment
	// templates but not in raw ARM requests. They are not in Body either.
	Body map[string]interface{} `json:"body"`
}

func (r Resource) Convert(obj interface{}) error {
	b, err := json.Marshal(r.Body)
	if err != nil {
		return fmt.Errorf("failed to convert resource to JSON: %w", err)
	}

	err = json.Unmarshal(b, obj)
	if err != nil {
		return fmt.Errorf("failed to convert resource JSON to %T: %w", obj, err)
	}

	return nil
}

// Gets the application name, resource name, and resource type for a radius resource.
// Only use on radius resource types.
func (r Resource) GetRadiusResourceParts() (applicationName string, resourceName string, resourceType string) {
	typeParts := strings.Split(r.Type, "/")
	nameParts := strings.Split(r.Name, "/")

	if len(nameParts) > 1 {
		applicationName = nameParts[1]
		resourceType = typeParts[len(typeParts)-1]
		if len(nameParts) > 2 {
			resourceName = nameParts[2]
		}
	}
	return
}

func Parse(template string) (DeploymentTemplate, error) {
	parsed := DeploymentTemplate{}
	err := json.Unmarshal([]byte(template), &parsed)
	if err != nil {
		return DeploymentTemplate{}, err
	}

	return parsed, nil
}

type TemplateOptions struct {
	SubscriptionID string
	ResourceGroup  string

	// Parameters stores ARM template parameters in the format they appear when submitting a deployment.
	//
	// The full format is documented here: https://docs.microsoft.com/en-us/azure/azure-resource-manager/templates/parameter-files
	//
	// Note that we're only storing the 'parameters' node of the format described above.
	Parameters             map[string]map[string]interface{}
	EvaluatePropertiesNode bool
}

func Eval(template DeploymentTemplate, options TemplateOptions) ([]Resource, error) {
	eva := &DeploymentEvaluator{
		Template:  template,
		Options:   options,
		Variables: map[string]interface{}{},
	}

	for name, variable := range template.Variables {
		value, err := eva.VisitValue(variable)
		if err != nil {
			return nil, err
		}

		eva.Variables[name] = value
	}

	resources := map[string]Resource{}
	for _, j := range template.Resources {
		resource, err := eva.VisitResource(j)
		if err != nil {
			return []Resource{}, err
		}
		resources[resource.ID] = resource
	}
	ordered, err := orderResources(resources)
	if err != nil {
		return []Resource{}, err
	}
	return ordered, nil
}

// TODO: cycle breaking - we rely on the bicep compiler's validation here and don't
// detect cycles.
func orderResources(resources map[string]Resource) ([]Resource, error) {
	// Iterating a map is a random ordering, we want to iterate in a stable order for testing
	sortedIds := []string{}
	for _, v := range resources {
		sortedIds = append(sortedIds, v.ID)
	}

	sort.Strings(sortedIds)

	// Now we can compute the dependency order
	ordered := []Resource{}
	members := map[string]bool{}

	for _, id := range sortedIds {
		resource, ok := resources[id]
		if !ok {
			return nil, fmt.Errorf("could not find resource with id: %s", id)
		}

		var err error
		ordered, err = ensurePresent(resources, ordered, members, resource)
		if err != nil {
			return nil, err
		}
	}

	return ordered, nil
}

func ensurePresent(resources map[string]Resource, ordered []Resource, members map[string]bool, res Resource) ([]Resource, error) {
	_, ok := members[res.ID]
	if ok {
		// already in the set
		return ordered, nil
	}

	for _, id := range res.DependsOn {
		d, ok := resources[id]
		if !ok {
			return nil, fmt.Errorf("could not find resource with id: %s", id)
		}

		// Add dependencies
		var err error
		ordered, err = ensurePresent(resources, ordered, members, d)
		if err != nil {
			return nil, err
		}
	}

	// requirements satisfied, add this one
	ordered = append(ordered, res)
	members[res.ID] = true
	return ordered, nil
}
