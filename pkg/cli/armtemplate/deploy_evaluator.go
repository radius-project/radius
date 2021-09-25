// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armtemplate

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/radrp/armexpr"
)

type DeploymentEvaluator struct {
	Template  DeploymentTemplate
	Options   TemplateOptions
	Deployed  map[string]map[string]interface{}
	Variables map[string]interface{}

	// Intermediate expression evaluation state
	Value interface{}
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

func (eva *DeploymentEvaluator) VisitPropertyAccess(node *armexpr.PropertyAccessNode) error {
	// Recursively evaluate the LHS
	err := node.Base.Accept(eva)
	if err != nil {
		return err
	}

	if eva.Value == nil {
		return errors.New("value to access is null")
	}

	obj, ok := eva.Value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("value to access should be a map, was: %+v", eva.Value)
	}

	value, ok := obj[node.Identifier.Text]
	if !ok {
		return fmt.Errorf("value did not contain property '%s', was: %+v", node.Identifier.Text, obj)
	}

	eva.Value = value
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

	if name == "format" {
		if len(args) < 1 {
			return fmt.Errorf("at least 1 argument is required for %s", "format")
		}

		eva.Value = eva.EvaluateFormat(args[0], args[1:])
		return nil
	} else if name == "reference" {
		if len(args) != 1 {
			return fmt.Errorf("exactly 1 argument required for %s", "reference")
		}

		result, err := eva.EvaluateReference(args[0])
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
	} else if name == "string" {
		if len(args) != 1 {
			return fmt.Errorf("exactly 1 argument is required for %s", "string")
		}

		eva.Value = eva.EvaluateString(args[0])
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

func (eva *DeploymentEvaluator) EvaluateFormat(format interface{}, values []interface{}) string {
	r := regexp.MustCompile(`\{\d+\}`)
	format = r.ReplaceAllString(format.(string), "%v")

	return fmt.Sprintf(format.(string), values...)
}

func (eva *DeploymentEvaluator) EvaluateReference(id interface{}) (map[string]interface{}, error) {
	obj, ok := eva.Deployed[id.(string)]
	if !ok {
		return nil, fmt.Errorf("no resource matches id: %s", id)
	}

	// Note: we assume 'full' mode for references
	// see: https://docs.microsoft.com/en-us/azure/azure-resource-manager/templates/template-functions-resource#reference
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
