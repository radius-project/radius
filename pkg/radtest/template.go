// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radtest

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/Azure/go-autorest/autorest/to"
	radresources "github.com/Azure/radius/pkg/curp/resources"
)

// DeploymentTemplate represents an ARM template.
type DeploymentTemplate struct {
	Schema         string                   `json:"$schema"`
	ContentVersion string                   `json:"contentVersion"`
	ApiProfile     string                   `json:"apiProfile"`
	Paramertes     map[string]interface{}   `json:"parameters"`
	Variables      map[string]interface{}   `json:"variables"`
	Functions      []interface{}            `json:"functions"`
	Resources      []map[string]interface{} `json:"resources"`
	Outputs        map[string]interface{}   `json:"outputs"`
}

// Resource represents a (parsed) resource within an ARM template.
type Resource struct {
	ID         string
	Type       string
	APIVersion string
	Name       string
	DependsOn  []string

	// Contains the actual payload that should be submitted (properties, kind, etc)
	// note that properties like type, name, and apiversion are present in deployment
	// templates but not in raw ARM requests. They are not in Body either.
	Body map[string]interface{}
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

func Parse(template string) ([]Resource, error) {
	parsed := DeploymentTemplate{}
	err := json.Unmarshal([]byte(template), &parsed)
	if err != nil {
		return []Resource{}, err
	}

	resources := map[string]Resource{}
	for _, j := range parsed.Resources {
		res, err := readResource(j)
		if err != nil {
			return []Resource{}, err
		}

		resources[res.ID] = res
	}

	ordered, err := orderResources(resources)
	if err != nil {
		return []Resource{}, err
	}

	return ordered, nil
}

func readResource(j map[string]interface{}) (Resource, error) {
	name, ok := j["name"].(string)
	if !ok {
		return Resource{}, fmt.Errorf("Resource does not contain a name.")
	}

	t, ok := j["type"].(string)
	if !ok {
		return Resource{}, fmt.Errorf("Resource does not contain a type.")
	}

	apiVersion, ok := j["apiVersion"].(string)
	if !ok {
		return Resource{}, fmt.Errorf("Resource does not contain an apiVersion.")
	}

	dependsOn := []string{}
	obj, ok := j["dependsOn"]
	if ok {
		ds, ok := obj.([]interface{})
		if !ok {
			return Resource{}, errors.New("dependson is the wrong type.")
		}

		for _, d := range ds {
			dt, ok := d.(string)
			if !ok {
				return Resource{}, errors.New("dependson is the wrong type.")
			}

			processed, err := processResourceId(dt)
			if err != nil {
				return Resource{}, err
			}

			dependsOn = append(dependsOn, processed)
		}
	}

	name, err := processFormat(name)
	if err != nil {
		return Resource{}, err
	}

	id, err := resourceID(t, strings.Split(name, "/"))
	if err != nil {
		return Resource{}, err
	}

	// remove properties that are not part of the body
	body := map[string]interface{}{}
	for k, v := range j {
		body[k] = v
	}

	delete(body, "name")
	delete(body, "type")
	delete(body, "apiVersion")
	delete(body, "dependsOn")

	result := Resource{
		ID:         id,
		Type:       t,
		APIVersion: apiVersion,
		Name:       name,
		DependsOn:  dependsOn,
		Body:       body,
	}

	return result, nil
}

func processFormat(text string) (string, error) {
	// TODO: this is not a real parser
	if !strings.HasPrefix(text, "[format(") || !strings.HasSuffix(text, ")]") {
		return text, nil
	}

	tokens := parseTokens(strings.TrimPrefix(strings.TrimSuffix(text, ")]"), "[format("))
	return format(tokens[0], tokens[1:]), nil
}

func format(format string, values []string) string {
	r := regexp.MustCompile(`\{\d+\}`)
	format = r.ReplaceAllString(format, "%v")

	v := []interface{}{}
	for _, val := range values {
		v = append(v, val)
	}
	return fmt.Sprintf(format, v...)
}

func processResourceId(text string) (string, error) {
	// TODO: this is not a real parser
	if !strings.HasPrefix(text, "[resourceId(") || !strings.HasSuffix(text, ")]") {
		return text, nil
	}

	tokens := parseTokens(strings.TrimPrefix(strings.TrimSuffix(text, ")]"), "[resourceId("))
	return resourceID(tokens[0], tokens[1:])
}

func resourceID(resourceType string, names []string) (string, error) {
	typeSegments := strings.Split(resourceType, "/")

	if len(typeSegments)-1 != len(names) {
		return "", errors.New("invalid arguments: wrong number of names")
	}

	head := radresources.ResourceType{
		Type: typeSegments[0] + "/" + typeSegments[1],
		Name: names[0],
	}

	tail := []radresources.ResourceType{}
	for i := 1; i < len(names); i++ {
		tail = append(tail, radresources.ResourceType{
			Type: typeSegments[i+1],
			Name: names[i],
		})
	}

	id := radresources.MakeID(
		TestSubscriptionID,
		TestResourceGroup,
		head,
		tail...)
	return id, nil
}

// parses a set of comma-separated single-quoted strings
func parseTokens(text string) []string {
	tokens := []string{}

	var start *int
	for i := range text {
		if text[i] == '\'' && start == nil {
			start = to.IntPtr(i)
		} else if text[i] == '\'' {
			tokens = append(tokens, text[*start+1:i])
			start = nil
		}
	}

	return tokens
}

// TODO: cycle breaking - we rely on the bicep compiler's validation here and don't
// detect cycles.
func orderResources(resources map[string]Resource) ([]Resource, error) {
	ordered := []Resource{}
	members := map[string]bool{}

	for _, res := range resources {
		ordered = ensurepresent(resources, ordered, members, res)
	}

	return ordered, nil
}

func ensurepresent(resources map[string]Resource, ordered []Resource, members map[string]bool, res Resource) []Resource {
	_, ok := members[res.ID]
	if ok {
		// already in the set
		return ordered
	}

	for _, id := range res.DependsOn {
		d := resources[id]

		// Add dependencies
		ordered = ensurepresent(resources, ordered, members, d)
	}

	// requirements satisfied, add this one
	ordered = append(ordered, res)
	members[res.ID] = true
	return ordered
}
