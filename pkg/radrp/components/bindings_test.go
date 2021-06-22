// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package components

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// NOTE: we don't cover invalid syntax here - just invalid structure. Invalid expression syntax
// is tested in the expression parser.

func Test_ConvertFromValue_StaticValues(t *testing.T) {
	// We allow JSON-like values that aren't ARM expressions
	inputs := []interface{}{
		30,
		"foo",
		"[bar",
		false,
	}

	for _, input := range inputs {
		t.Run(fmt.Sprintf("Convert-%v", input), func(t *testing.T) {
			expr, err := ConvertFromValue(input)
			require.NoError(t, err)

			require.Equal(t, KindStatic, expr.Kind)
			require.Equal(t, input, expr.Value)
		})
	}
}

func Test_ConvertFromValue_BindingExpression(t *testing.T) {
	inputs := []string{
		// Syntax error
		"[[reference(resourceId((']",

		// Not a resource id
		"[[reference('foo').bindings.web]",

		// Wrong resource type
		"[[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Scopes', 'radius', 'app', 'backend')).bindings.web]",
		"[[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Components')).bindings.web]",

		// Wrong number of name segments
		"[[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Components', 'app', 'backend')).bindings.web]",

		// Invalid arguments
		"[[reference(resourceId(resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Scopes', 'radius', 'app', 'backend'))).bindings.web]",
		"[[reference().bindings.web]",

		// Not a reference to bindings
		"[[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Components', 'radius', 'app', 'backend')).anotherproperty.web]",
		"[[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Components', 'radius', 'app', 'backend'))]",

		// Property access on the wrong thing
		"[[resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Scopes', 'radius', 'app', 'backend').bindings.web]",
	}

	for _, input := range inputs {
		t.Run(fmt.Sprintf("Convert-%v", input), func(t *testing.T) {
			expr, err := ConvertFromValue(input)
			require.Errorf(t, err, "should have failed converting: %+v", expr)
		})
	}
}

func Test_ConvertFromValue_InvalidBindingExpression(t *testing.T) {
	inputs := []convertInput{
		{
			Text: "[[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Components', 'radius', 'app', 'backend')).bindings.web]",
			Expected: &ComponentBindingValue{
				Application: "app",
				Component:   "backend",
				Binding:     "web",
				Property:    "",
			},
		},
		{
			Text: "[[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Components', 'radius', 'app', 'backend')).bindings.web.host.value]",
			Expected: &ComponentBindingValue{
				Application: "app",
				Component:   "backend",
				Binding:     "web",
				Property:    "host.value",
			},
		},
	}

	for _, input := range inputs {
		t.Run(fmt.Sprintf("Convert-%v", input.Text), func(t *testing.T) {
			expr, err := ConvertFromValue(input.Text)
			require.NoError(t, err)

			require.Equal(t, KindComponentBinding, expr.Kind)
			require.Equal(t, input.Expected, expr.Value)
		})
	}
}

type convertInput struct {
	Text     string
	Expected *ComponentBindingValue
}

func Test_JSON_RoundTrip(t *testing.T) {
	inputs := []jsonInput{
		{
			Expr: BindingExpression{
				Kind:  KindStatic,
				Value: 30.0,
			},
			Expected: "30",
		},
		{
			Expr: BindingExpression{
				Kind:  KindStatic,
				Value: "hello",
			},
			Expected: "\"hello\"",
		},
		{
			Expr: BindingExpression{
				Kind: KindComponentBinding,
				Value: &ComponentBindingValue{
					Application: "testapp",
					Component:   "testcomponent",
					Binding:     "testbinding",
				},
			},
			Expected: "\"[[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Components', 'radius', 'testapp', 'testcomponent')).bindings.testbinding]\"",
		},
		{
			Expr: BindingExpression{
				Kind: KindComponentBinding,
				Value: &ComponentBindingValue{
					Application: "testapp",
					Component:   "testcomponent",
					Binding:     "testbinding",
					Property:    "foo.bar",
				},
			},
			Expected: "\"[[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Components', 'radius', 'testapp', 'testcomponent')).bindings.testbinding.foo.bar]\"",
		},
	}

	for i, input := range inputs {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			b, err := json.Marshal(&input.Expr)
			require.NoError(t, err)

			require.Equal(t, input.Expected, string(b))

			actual := BindingExpression{}
			err = json.Unmarshal(b, &actual)
			require.NoError(t, err)

			require.Equal(t, input.Expr, actual)
		})
	}
}

type jsonInput struct {
	Expr     BindingExpression
	Expected string
}

func Test_TryGetBindingKey(t *testing.T) {
	inputs := []bindingInput{
		{
			Expr: BindingExpression{
				Kind:  "invalid",
				Value: 30.0,
			},
			Expected: nil,
		},
		{
			Expr: BindingExpression{
				Kind:  KindStatic,
				Value: 30.0,
			},
			Expected: nil,
		},
		{
			Expr: BindingExpression{
				Kind: KindComponentBinding,
				Value: &ComponentBindingValue{
					Application: "testapp",
					Component:   "testcomponent",
					Binding:     "testbinding",
				},
			},
			Expected: &BindingKey{
				Component: "testcomponent",
				Binding:   "testbinding",
			},
		},
	}

	for i, input := range inputs {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			key := input.Expr.TryGetBindingKey()

			require.Equal(t, input.Expected, key)
		})
	}
}

type bindingInput struct {
	Expr     BindingExpression
	Expected *BindingKey
}
