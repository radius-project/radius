// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resourcesv1alpha3

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/radius/pkg/radrp/armexpr"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
)

type BindingExpressionKind string

const (
	KindStatic          = BindingExpressionKind("static")
	KindResourceBinding = BindingExpressionKind("resource")

	ResourceResourceType = "Microsoft.CustomProviders/resourceProviders/Applications/Resources"
)

// BindingKey is a key used to resolve a binding within an application
type BindingKey struct {
	Resource string
	Binding  string
}

// BindingState represents the output values for expressions to consume
type BindingState struct {
	Resource   string
	Binding    string
	Kind       string
	Properties map[string]interface{}
}

// BindingExpression represents a value that may be static or may be bound to a resource binding.
//
// Note, we store binding expressions using their wire format. Converting to/from JSON will convert
// to the original string.
type BindingExpression struct {
	Kind  BindingExpressionKind
	Value interface{}
}

// BindingExpression represents a value to evaulate based on a binding.
type ResourceBindingValue struct {
	// Application is the application referred to by the binding expression.
	Application string

	// Resource is the resource referred to by the binding expression.
	Resource string

	// Binding is the name of the binding referred to by the binding expression.
	Binding string

	// Property is the property path referred to by the binding expression. May be empty.
	Property string
}

// For unit testing
func NewStaticBindingExpression(value interface{}) BindingExpression {
	return BindingExpression{
		Kind:  KindStatic,
		Value: value,
	}
}

// For unit testing
func NewResourceBindingExpression(application string, resource string, binding string, property string) BindingExpression {
	return BindingExpression{
		Kind: KindResourceBinding,
		Value: &ResourceBindingValue{
			Application: application,
			Resource:    resource,
			Binding:     binding,
			Property:    property,
		},
	}
}

func (be BindingExpression) MarshalJSON() ([]byte, error) {
	value, err := be.ConvertToValue()
	if err != nil {
		return nil, err
	}

	return json.Marshal(value)
}

func (be BindingExpression) MarshalBSONValue() (bsontype.Type, []byte, error) {
	value, err := be.ConvertToValue()
	if err != nil {
		return bsontype.Null, nil, err
	}

	return bson.MarshalValue(value)
}

func (be *BindingExpression) UnmarshalJSON(data []byte) error {
	value := (interface{})(nil)
	err := json.Unmarshal(data, &value)
	if err != nil {
		return err
	}

	expr, err := ConvertFromValue(value)
	if err != nil {
		return err
	}

	be.Kind = expr.Kind
	be.Value = expr.Value
	return nil
}

func (be *BindingExpression) UnmarshalBSONValue(t bsontype.Type, data []byte) error {
	value := (interface{})(nil)

	raw := bson.RawValue{Type: t, Value: data}
	err := raw.Unmarshal(&value)
	if err != nil {
		return err
	}

	expr, err := ConvertFromValue(value)
	if err != nil {
		return err
	}

	be.Kind = expr.Kind
	be.Value = expr.Value
	return nil
}

func (expr BindingExpression) TryGetBindingKey() *BindingKey {
	if expr.Kind == KindStatic {
		return nil
	} else if expr.Kind == KindResourceBinding {
		resource, ok := expr.Value.(*ResourceBindingValue)
		if !ok {
			return nil
		}

		return &BindingKey{Resource: resource.Resource, Binding: resource.Binding}
	} else {
		return nil
	}
}

func (expr BindingExpression) GetMatchingBinding(state map[BindingKey]BindingState) (BindingState, error) {
	if expr.Kind == KindStatic {
		return BindingState{}, errors.New("a resource binding is required")
	}

	if expr.Kind == KindResourceBinding {
		resource, ok := expr.Value.(*ResourceBindingValue)
		if !ok {
			return BindingState{}, fmt.Errorf("unexpected expression kind: %v", expr.Kind)
		}

		key := BindingKey{Resource: resource.Resource, Binding: resource.Binding}
		binding, ok := state[key]
		if !ok {
			return BindingState{}, fmt.Errorf("cannot resolve binding %s from resource %s", resource.Binding, resource.Resource)
		}

		return binding, nil
	}

	return BindingState{}, fmt.Errorf("unsupport binding expression kind %s", expr.Kind)
}

func (expr BindingExpression) Evaluate(state map[BindingKey]BindingState) (interface{}, error) {
	if expr.Kind == KindStatic {
		return expr.Value, nil
	}

	if expr.Kind == KindResourceBinding {
		resource, ok := expr.Value.(*ResourceBindingValue)
		if !ok {
			return nil, fmt.Errorf("unexpected expression kind: %v", expr.Kind)
		}

		key := BindingKey{Resource: resource.Resource, Binding: resource.Binding}
		binding, ok := state[key]
		if !ok {
			return nil, fmt.Errorf("cannot resolve binding %s from resource %s", resource.Binding, resource.Resource)
		}

		value, ok := binding.Properties[resource.Property]
		if !ok {
			return nil, fmt.Errorf("cannot resolve value %s for binding %s from resource %s", resource.Property, resource.Binding, resource.Resource)
		}

		return value, nil
	}

	return nil, fmt.Errorf("unsupport binding expression kind %s", expr.Kind)
}

func (expr BindingExpression) EvaluateString(state map[BindingKey]BindingState) (string, error) {
	value, err := expr.Evaluate(state)
	if err != nil {
		return "", err
	}

	str, ok := value.(string)
	if !ok {
		return "", errors.New("value for binding is not a string")
	}

	return str, nil
}

func (be BindingExpression) ConvertToValue() (interface{}, error) {
	if be.Kind == KindStatic {
		return be.Value, nil
	} else if be.Kind == KindResourceBinding {
		if be.Value == nil {
			return nil, errors.New("binding expression is nil for a resource binding")
		}

		resource, ok := be.Value.(*ResourceBindingValue)
		if !ok {
			return nil, fmt.Errorf("unknown value type %T for a resource binding", be.Value)
		}

		propertyText := "bindings." + resource.Binding
		if resource.Property != "" {
			propertyText = propertyText + "." + resource.Property
		}

		return fmt.Sprintf(
			"[[reference(resourceId('%s', '%s', '%s', '%s')).%s]",
			ResourceResourceType,
			"radius",
			resource.Application,
			resource.Resource,
			propertyText), nil
	} else {
		return nil, fmt.Errorf("unsupported expression kind '%s'", be.Kind)
	}
}

// ConvertFromValue parses a binding value from a JSON value.
func ConvertFromValue(value interface{}) (BindingExpression, error) {
	// Binding expressions use the form:
	//
	// '[[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Resources', 'radius', 'app', 'backend')).bindings.web]'
	//
	// Breaking this down:
	// - This is an *escaped* ARM-JSON expression. The leading '[[' acts as an escape.
	// - The 'reference' function accepts a resource ID. We always expect the 'reference' to refer to a resource.
	// - The 'resourceId' contains type and name segments needed to refer to a resource.
	// - The property path follows the 'resourceId' and refers to a property within the object.
	// - We always expect the property path to refer to a property within 'bindings'.
	//
	// The strategy here is that we use our parser to parse an AST and then a visitor to perform
	// a syntax-directive translation.

	text, ok := value.(string)
	if !ok {
		// This is not a binding expression. Don't treat it as an error either, it's a static value.
		return NewStaticBindingExpression(value), nil
	}

	ok, err := armexpr.IsARMExpression(text)
	if err != nil {
		return BindingExpression{}, err
	} else if !ok {
		// This is not a binding expression. Don't treat it as an error either, it's a static value.
		return NewStaticBindingExpression(value), nil
	}

	syntaxTree, err := armexpr.Parse(text)
	if err != nil {
		return BindingExpression{}, err
	}

	expr, err := eval(text, syntaxTree)
	if err != nil {
		return BindingExpression{}, err
	}

	return BindingExpression{
		Kind:  KindResourceBinding,
		Value: expr,
	}, nil
}

func eval(text string, syntaxTree *armexpr.SyntaxTree) (*ResourceBindingValue, error) {
	visitor := &visitor{}
	err := syntaxTree.Expression.Accept(visitor)
	if err != nil {
		return nil, err
	}

	ref, ok := visitor.value.(*resourceReference)
	if !ok {
		return nil, errors.New("a binding expression must contain a resource property reference")
	}

	if len(ref.Properties) < 2 || ref.Properties[0] != "bindings" {
		return nil, errors.New("a binding expression must reference the '.bindings' property of a resource")
	}

	return &ResourceBindingValue{
		Application: ref.Application,
		Resource:    ref.Resource,
		Binding:     ref.Properties[1],
		Property:    strings.Join(ref.Properties[2:], "."),
	}, nil
}

type resourceID struct {
	Type  string
	Names []string
}

type resourceReference struct {
	Application string
	Resource    string
	Properties  []string
}

type visitor struct {
	value  interface{}
	Result *ResourceBindingValue
}

var _ armexpr.Visitor = (*visitor)(nil)

func (visitor *visitor) VisitFunctionCall(node *armexpr.FunctionCallNode) error {
	if node.Identifier.Text == "reference" {
		ref, err := visitor.handleReference(node)
		if err != nil {
			return err
		}

		// Store reference for caller
		visitor.value = ref

	} else if node.Identifier.Text == "resourceId" {
		id, err := visitor.handleResourceID(node)
		if err != nil {
			return err
		}

		// Store ID for caller
		visitor.value = id
	}

	return nil
}

func (visitor *visitor) VisitStringLiteral(node *armexpr.StringLiteralNode) error {
	visitor.value = node.Text[1 : len(node.Text)-1]
	return nil
}

func (visitor *visitor) VisitPropertyAccess(node *armexpr.PropertyAccessNode) error {
	err := node.Base.Accept(visitor)
	if err != nil {
		return err
	}

	ref, ok := visitor.value.(*resourceReference)
	if !ok {
		return errors.New("property access accepts a resource reference")
	}

	// Append property to existing reference
	ref.Properties = append(ref.Properties, node.Identifier.Text)
	return nil
}

func (visitor *visitor) handleResourceID(node *armexpr.FunctionCallNode) (*resourceID, error) {
	if len(node.Args) <= 1 {
		return nil, errors.New("'resourceId' requires two or more arguments")
	}

	args := []string{}
	for _, a := range node.Args {
		err := a.Accept(visitor)
		if err != nil {
			return nil, err
		}

		str, ok := visitor.value.(string)
		if !ok {
			return nil, errors.New("argument to 'resourceId' must be a string")
		}
		args = append(args, str)
	}

	return &resourceID{
		Type:  args[0],
		Names: args[1:],
	}, nil
}

func (visitor *visitor) handleReference(node *armexpr.FunctionCallNode) (*resourceReference, error) {
	if len(node.Args) != 1 {
		return nil, errors.New("'reference' accepts a single argument")
	}

	err := node.Args[0].Accept(visitor)
	if err != nil {
		return nil, err
	}

	id, ok := visitor.value.(*resourceID)
	if !ok {
		return nil, errors.New("argument to 'reference' must be a resourceId")
	}

	if id.Type != ResourceResourceType || len(id.Names) != 3 {
		return nil, errors.New("expected a reference to a resource")
	}

	ref := resourceReference{
		Application: id.Names[1],
		Resource:    id.Names[2],
	}

	return &ref, nil
}
