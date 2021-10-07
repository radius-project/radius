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
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/radrp/armexpr"
)

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
	Resources  []map[string]interface{}          `json:"resources"`
	Outputs    map[string]interface{}            `json:"outputs"`
}

// Resource represents a (parsed) resource within an ARM template.
type Resource struct {
	ID         string   `json:"id"`
	Type       string   `json:"type"`
	APIVersion string   `json:"apiVersion"`
	Name       string   `json:"name"`
	DependsOn  []string `json:"dependsOn"`

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

type evaluator struct {
	Template  DeploymentTemplate
	Options   TemplateOptions
	Resources []Resource

	// Intermediate expression evaluation state
	Value               interface{}
	PreserveExpressions bool
}

func Eval(template DeploymentTemplate, options TemplateOptions) ([]Resource, error) {
	eva := &evaluator{
		Template: template,
		Options:  options,
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

func (eva *evaluator) VisitResource(input map[string]interface{}) (Resource, error) {
	// In order to produce a resource we need to process ARM expressions using the "loosely-typed" representation
	// and then read it into an object.

	// Special case for evaluating resource bodies
	evaluated := map[string]interface{}{}
	for k, v := range input {
		eva.PreserveExpressions = !eva.Options.EvaluatePropertiesNode && k == "properties"

		v, err := eva.VisitValue(v)
		if err != nil {
			return Resource{}, err
		}

		evaluated[k] = v
		eva.PreserveExpressions = false
	}

	name, ok := evaluated["name"].(string)
	if !ok {
		return Resource{}, errors.New("resource does not contain a name")
	}

	t, ok := evaluated["type"].(string)
	if !ok {
		return Resource{}, errors.New("resource does not contain a type")
	}

	apiVersion, ok := evaluated["apiVersion"].(string)
	if !ok {
		return Resource{}, errors.New("resource does not contain an apiVersion")
	}

	dependsOn := []string{}
	obj, ok := evaluated["dependsOn"]
	if ok {
		ds, ok := obj.([]interface{})
		if !ok {
			return Resource{}, errors.New("dependsOn is the wrong type")
		}

		for _, d := range ds {
			dt, ok := d.(string)
			if !ok {
				return Resource{}, errors.New("dependsOn is the wrong type")
			}

			dependsOn = append(dependsOn, dt)
		}
	}

	nameParts := strings.Split(name, "/")
	args := []interface{}{}
	for _, part := range nameParts {
		args = append(args, part)
	}

	id, err := eva.EvaluateResourceID(t, args)
	if err != nil {
		return Resource{}, err
	}

	// remove properties that are not part of the body
	body := map[string]interface{}{}
	for k, v := range input {
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

func (eva *evaluator) VisitValue(input interface{}) (interface{}, error) {
	str, ok := input.(string)
	if ok {
		str, err := eva.VisitString(str)
		if err != nil {
			return nil, err
		}

		return str, err
	}

	m, ok := input.(map[string]interface{})
	if ok {
		m, err := eva.VisitMap(m)
		if err != nil {
			return nil, err
		}

		return m, err
	}

	slice, ok := input.([]interface{})
	if ok {
		slice, err := eva.VisitSlice(slice)
		if err != nil {
			return nil, err
		}

		return slice, err
	}

	// No need to analyze a null, bool, or number
	return input, nil
}

func (eva *evaluator) VisitMap(input map[string]interface{}) (map[string]interface{}, error) {
	for k, v := range input {
		v, err := eva.VisitValue(v)
		if err != nil {
			return nil, err
		}

		// Maps are pointer-like, so we can just modify them in place
		input[k] = v
	}

	return input, nil
}

func (eva *evaluator) VisitSlice(input []interface{}) ([]interface{}, error) {
	copy := []interface{}{}
	for _, v := range input {
		v, err := eva.VisitValue(v)
		if err != nil {
			return nil, err
		}

		copy = append(copy, v)
	}

	return copy, nil
}

func (eva *evaluator) VisitString(input string) (interface{}, error) {
	if eva.PreserveExpressions {
		return input, nil
	}

	isExpr, err := armexpr.IsStandardARMExpression(input)
	if err != nil {
		return "", err
	} else if !isExpr {
		// Not an expression
		return input, nil
	}

	syntaxTree, err := armexpr.Parse(input)
	if err != nil {
		return "", err
	}

	err = syntaxTree.Expression.Accept(eva)
	if err != nil {
		return "", err
	}

	return eva.Value, nil
}

func (eva *evaluator) VisitStringLiteral(node *armexpr.StringLiteralNode) error {
	eva.Value = node.Text[1 : len(node.Text)-1]
	return nil
}

func (eva *evaluator) VisitPropertyAccess(node *armexpr.PropertyAccessNode) error {
	return errors.New("property access is not supported")
}

func (eva *evaluator) VisitFunctionCall(node *armexpr.FunctionCallNode) error {
	name := node.Identifier.Text

	args := []interface{}{}
	for _, argexpr := range node.Args {
		err := argexpr.Accept(eva)
		if err != nil {
			return err
		}

		args = append(args, eva.Value)
	}

	if name == "format" {
		if len(args) < 1 {
			return fmt.Errorf("at least 1 argument is required for %s", "format")
		}

		eva.Value = eva.EvaluateFormat(args[0], args[1:])
		return nil
	} else if name == "parameters" {
		if len(args) != 1 {
			return fmt.Errorf("exactly 1 argument is required for %s", "parameter")
		}

		result, err := eva.EvaluateParameter(args[0].(string))
		if err != nil {
			return err
		}

		eva.Value = result
		return nil
	} else if name == "resourceId" {
		if len(args) < 2 {
			return fmt.Errorf("at least 2 arguments are required for %s", "resourceId")
		}

		result, err := eva.EvaluateResourceID(args[0], args[1:])
		if err != nil {
			return err
		}

		eva.Value = result
		return nil
	} else {
		return fmt.Errorf("unsupported function '%s'", name)
	}
}

func (eva *evaluator) EvaluateFormat(format interface{}, values []interface{}) string {
	r := regexp.MustCompile(`\{\d+\}`)
	format = r.ReplaceAllString(format.(string), "%v")

	return fmt.Sprintf(format.(string), values...)
}

func (eva *evaluator) EvaluateParameter(name string) (interface{}, error) {
	parameter, ok := eva.Options.Parameters[name]
	if ok {
		value, ok := parameter["value"]
		if !ok {
			return nil, fmt.Errorf("parameter %q has no value", name)
		}

		return value, nil
	}

	parameter, ok = eva.Template.Parameters[name]
	if ok {
		value, ok := parameter["defaultValue"]
		if !ok {
			return nil, fmt.Errorf("parameter %q has no default value", name)
		}

		return value, nil
	}

	return nil, fmt.Errorf("parameter %q is not defined by the template", name)
}

func (eva *evaluator) EvaluateResourceID(resourceType interface{}, names []interface{}) (string, error) {
	typeSegments := strings.Split(resourceType.(string), "/")

	if len(typeSegments)-1 != len(names) {
		return "", errors.New("invalid arguments: wrong number of names")
	}

	head := azresources.ResourceType{
		Type: typeSegments[0] + "/" + typeSegments[1],
		Name: names[0].(string),
	}

	tail := []azresources.ResourceType{}
	for i := 1; i < len(names); i++ {
		tail = append(tail, azresources.ResourceType{
			Type: typeSegments[i+1],
			Name: names[i].(string),
		})
	}

	id := azresources.MakeID(
		eva.Options.SubscriptionID,
		eva.Options.ResourceGroup,
		head,
		tail...)
	return id, nil
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
