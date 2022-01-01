// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armtemplate

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/cli/armtemplate/providers"
	"github.com/project-radius/radius/pkg/radrp/armexpr"
)

type DeploymentEvaluator struct {
	Context   context.Context
	Template  DeploymentTemplate
	Options   TemplateOptions
	Deployed  map[string]map[string]interface{}
	Variables map[string]interface{}
	Outputs   map[string]map[string]interface{}

	CustomActionCallback func(id string, apiVersion string, action string, payload interface{}) (interface{}, error)

	// Intermediate expression evaluation state
	Value interface{}

	Providers map[string]providers.Provider

	// PreserveExpressions is a stateful property telling the evaluator to skip over expressions when
	// processing. This is used when doing a 'first-pass' to evaluate resources before deployment starts.
	preserveExpressions bool
}

var _ armexpr.Visitor = &DeploymentEvaluator{}

func (eva *DeploymentEvaluator) VisitResource(input map[string]interface{}) (Resource, error) {
	// In order to produce a resource we need to process ARM expressions using the "loosely-typed" representation
	// and then read it into an object.

	// Special case for evaluating resource bodies
	evaluated := map[string]interface{}{}
	for k, v := range input {
		eva.preserveExpressions = !eva.Options.EvaluatePropertiesNode && k == "properties"

		v, err := eva.VisitValue(v)
		if err != nil {
			return Resource{}, err
		}

		evaluated[k] = v
		eva.preserveExpressions = false
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
		if !strings.Contains(t, "@") {
			return Resource{}, fmt.Errorf("resource %#v does not contain an apiVersion", input)
		}
		// This is a K8s resource, whom API version is embedded in type string.
		// For example: "kubernetes.core/Service@v1", which translates to
		// - type=kubernetes.core/Service, and
		// - apiVersion=v1.
		tokens := strings.SplitN(t, "@", 2)
		apiVersion = tokens[1]
		t = tokens[0]
	}

	providerKey := ""
	if importSpec, ok := evaluated["import"].(map[string]interface{}); ok {
		providerKey, _ = importSpec["provider"].(string)
	}
	var providerPtr *Provider
	if provider, hasProvider := eva.Template.Imports[providerKey]; hasProvider {
		providerPtr = &provider
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
	delete(body, "import")
	result := Resource{
		ID:         id,
		Type:       t,
		APIVersion: apiVersion,
		Name:       name,
		DependsOn:  dependsOn,
		Body:       body,
		Provider:   providerPtr,
	}
	return result, nil
}

func (eva *DeploymentEvaluator) VisitValue(input interface{}) (interface{}, error) {
	str, ok := input.(string)
	if ok {
		slice, err := eva.VisitString(str)
		if err != nil {
			return nil, err
		}

		return slice, err
	}

	slice, ok := input.([]interface{})
	if ok {
		slice, err := eva.VisitSlice(slice)
		if err != nil {
			return nil, err
		}

		return slice, err
	}

	m, ok := input.(map[string]interface{})
	if ok {
		m, err := eva.VisitMap(m)
		if err != nil {
			return nil, err
		}

		return m, err
	}

	tree, ok := input.(*armexpr.SyntaxTree)
	if ok {
		err := tree.Expression.Accept(eva)
		if err != nil {
			return nil, err
		}

		return eva.Value, nil
	}

	// No need to analyze a null, bool, or number
	return input, nil
}

func (eva *DeploymentEvaluator) VisitString(input string) (interface{}, error) {
	if eva.preserveExpressions {
		return input, nil
	}

	isExpr, err := armexpr.IsARMExpression(input)
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

func (eva *DeploymentEvaluator) VisitResourceBody(resource Resource) (map[string]interface{}, error) {
	// For a nested deployment we need special evaluation rules, just evaluate the
	// parameters.
	if resource.Type == DeploymentResourceType {
		obj, ok := resource.Body["parameters"]
		if !ok {
			// Parameters can be optional
			return resource.Body, nil
		}

		parameters, ok := obj.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("deployment parameters should be a map, got %T", obj)
		}

		parameters, err := eva.VisitMap(parameters)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate deployment parameters: %w", err)
		}

		output := map[string]interface{}{}
		for k, v := range resource.Body {
			output[k] = v
		}

		output["parameters"] = parameters
		return output, nil
	}

	return eva.VisitMap(resource.Body)
}

func (eva *DeploymentEvaluator) VisitMap(input map[string]interface{}) (map[string]interface{}, error) {
	output := map[string]interface{}{}

	for k, v := range input {
		k, err := eva.VisitString(k)
		if err != nil {
			return nil, err
		}

		key, ok := k.(string)
		if !ok {
			return nil, fmt.Errorf("map key must evaluate to a string, was: %+v", k)
		}

		v, err := eva.VisitValue(v)
		if err != nil {
			return nil, err
		}

		output[key] = v
	}

	return output, nil
}

func (eva *DeploymentEvaluator) VisitSlice(input []interface{}) ([]interface{}, error) {
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

func (eva *DeploymentEvaluator) VisitStringLiteral(node *armexpr.StringLiteralNode) error {
	eva.Value = node.Text[1 : len(node.Text)-1]
	return nil
}

func (eva *DeploymentEvaluator) VisitIntLiteral(node *armexpr.IntLiteralNode) error {
	eva.Value = node.Value
	return nil
}

func (eva *DeploymentEvaluator) VisitIndexingNode(node *armexpr.IndexingNode) error {
	// Recursively evaluate the LHS
	err := node.Base.Accept(eva)
	if err != nil {
		return err
	}

	if eva.Value == nil {
		return errors.New("value to access is null")
	}

	switch obj := eva.Value.(type) {
	case map[string]interface{}:
		key := node.Identifier.Text
		if key == "" {
			// must be a string
			err = node.IndexExpr.Accept(eva)
			if err != nil {
				return err
			}
			ok := false
			key, ok = eva.Value.(string)
			if !ok {
				return fmt.Errorf("map key must be string, was %+v", eva.Value)
			}
		}
		value, ok := obj[key]
		if !ok {
			return fmt.Errorf("value did not contain property '%s', was: %+v", key, obj)
		}

		eva.Value = value
	case []interface{}:
		err = node.IndexExpr.Accept(eva)
		if err != nil {
			return err
		}
		idx, ok := eva.Value.(int)
		if !ok {
			return fmt.Errorf("array index must be int, was %+v", eva.Value)
		}
		if idx >= len(obj) {
			return fmt.Errorf("array index out of range %d>=%d", idx, len(obj))
		}
		eva.Value = obj[idx]
	default:
		return fmt.Errorf("value to access should be a map or array, was: %+v", eva.Value)
	}
	return nil
}

func (eva *DeploymentEvaluator) VisitFunctionCall(node *armexpr.FunctionCallNode) error {
	name := node.Identifier.Text

	args := []interface{}{}
	for _, argexpr := range node.Args {
		err := argexpr.Accept(eva)
		if err != nil {
			return err
		}

		args = append(args, eva.Value)
	}

	if strings.HasPrefix(name, "list") {
		if len(args) < 2 {
			return fmt.Errorf("at least 2 arguments are required for %s", name)
		}

		result, err := eva.EvaluateCustomAction(name, args)
		if err != nil {
			return err
		}

		eva.Value = result
		return nil
	} else if name == "createObject" {
		if len(args)%2 != 0 {
			return fmt.Errorf("an even number of arguments is required for %s", "createObject")
		}

		result, err := eva.EvaluateCreateObject(args)
		if err != nil {
			return err
		}

		eva.Value = result
		return nil
	} else if name == "format" {
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
	} else if name == "reference" {
		if len(args) < 1 || len(args) > 3 {
			return fmt.Errorf("between 1-3 arguments are required for %s", "reference")
		}

		id, err := eva.bindStringArgument(args, 0, nil, "reference", "id")
		if err != nil {
			return err
		}

		version, err := eva.bindStringArgument(args, 1, to.StringPtr(""), "reference", "apiVersion")
		if err != nil {
			return err
		}

		str, err := eva.bindStringArgument(args, 2, to.StringPtr(""), "reference", "full")
		if err != nil {
			return err
		}

		full := strings.EqualFold(str, "Full")
		result, err := eva.EvaluateReference(id, version, full)
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
	} else if name == "split" {
		if len(args) != 2 {
			return fmt.Errorf("exactly 2 arguments are required for %s", "split")
		}
		result, err := eva.EvaluateSplit(args[0].(string), args[1].(string))
		if err != nil {
			return err
		}

		eva.Value = result
		return nil
	} else if name == "string" {
		if len(args) != 1 {
			return fmt.Errorf("exactly 1 argument is required for %s", "string")
		}

		eva.Value = eva.EvaluateString(args[0])
		return nil
	} else if name == "base64ToString" {
		if len(args) != 1 {
			return fmt.Errorf("exactly 1 argument is required for %s", "base64ToString")
		}
		out, err := base64.StdEncoding.DecodeString(eva.EvaluateString(args[0]))
		if err != nil {
			return err
		}
		eva.Value = string(out)
		return nil
	} else if name == "variables" {
		if len(args) != 1 {
			return fmt.Errorf("exactle 1 argument is required for %s", "variables")
		}

		result, err := eva.EvaluateVariable(args[0])
		if err != nil {
			return err
		}

		eva.Value = result
		return nil
	} else {
		return fmt.Errorf("unsupported function '%s'", name)
	}
}

func (eva *DeploymentEvaluator) EvaluateCustomAction(name string, values []interface{}) (interface{}, error) {

	id, ok := values[0].(string)
	if !ok {
		return nil, fmt.Errorf("resource id must be a string, was: %v", values[0])
	}

	apiVersion, ok := values[1].(string)
	if !ok {
		return nil, fmt.Errorf("API Version must be a string, was: %v", values[1])
	}

	var body interface{}
	if len(values) == 3 {
		body = values[2]
	}

	if eva.CustomActionCallback == nil {
		return nil, errors.New("custom actions are not supported by this host")
	}

	return eva.CustomActionCallback(id, apiVersion, name, body)
}

func (eva *DeploymentEvaluator) EvaluateCreateObject(values []interface{}) (map[string]interface{}, error) {
	result := map[string]interface{}{}
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			return nil, fmt.Errorf("key must be a string, was: %v", values[i])
		}

		result[key] = values[i+1]
	}

	return result, nil
}

func (eva *DeploymentEvaluator) EvaluateFormat(format interface{}, values []interface{}) string {
	r := regexp.MustCompile(`\{\d+\}`)
	format = r.ReplaceAllString(format.(string), "%v")

	return fmt.Sprintf(format.(string), values...)
}

func (eva *DeploymentEvaluator) EvaluateParameter(name string) (interface{}, error) {
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

func (eva *DeploymentEvaluator) EvaluateReference(id interface{}, version string, full bool) (map[string]interface{}, error) {
	obj, ok := eva.Deployed[id.(string)]
	if !ok {
		parsed, err := azresources.Parse(id.(string))
		if err != nil {
			return nil, err
		}

		// TODO(tcnghia/rynowak): Right now we don't use symbolic references so we have to
		// hack this based on the ARM resource type. Long-term we will be able to use symbolic
		// references to find the provider by its ID.
		provider, err := GetProvider(eva.Providers, "", "", parsed.Type())
		if err != nil {
			return nil, err
		}

		return provider.GetDeployedResource(eva.Context, id.(string), version)
	}

	// Note: we assume 'full' mode for references
	// see: https://docs.microsoft.com/en-us/azure/azure-resource-manager/templates/template-functions-resource#reference
	if full {
		return obj, nil
	}

	properties, ok := obj["properties"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("value did not contain property '%s', was: %+v", "properties", obj)
	}

	return properties, nil
}

func (eva *DeploymentEvaluator) EvaluateResourceID(resourceType interface{}, names []interface{}) (string, error) {
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

func (eva *DeploymentEvaluator) EvaluateSplit(input string, delimiter string) ([]interface{}, error) {
	strs := strings.Split(input, delimiter)

	result := []interface{}{}
	for _, s := range strs {
		result = append(result, s)
	}

	return result, nil
}

func (eva *DeploymentEvaluator) EvaluateString(input interface{}) string {
	return fmt.Sprintf("%v", input)
}

func (eva *DeploymentEvaluator) EvaluateVariable(variable interface{}) (interface{}, error) {
	value, ok := eva.Variables[variable.(string)]
	if !ok {
		return nil, fmt.Errorf("no variable matches: %s", variable)
	}

	return value, nil
}

func (eva *DeploymentEvaluator) bindStringArgument(args []interface{}, index int, defaultValue *string, function string, parameter string) (string, error) {
	if len(args) <= index && defaultValue == nil {
		return "", fmt.Errorf("the %s function requires at least %d arguments", function, index+1)
	} else if len(args) <= index && defaultValue != nil {
		return *defaultValue, nil
	}

	return eva.requireString(args[index], function, parameter)
}

func (eva *DeploymentEvaluator) requireString(value interface{}, function string, parameter string) (string, error) {
	str, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("the %s parameter to the function %s expects a string, got %T", parameter, function, value)
	}

	return str, nil
}
