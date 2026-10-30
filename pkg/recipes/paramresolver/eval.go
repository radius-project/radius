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

package paramresolver

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"strings"

	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/recipecontext"
)

const (
	resourcePropertiesPrefix  = "context.resource.properties."
	resourceConnectionsPrefix = "context.resource.connections."
)

// errNullResult is returned when an expression evaluates to null, because there is no value to
// substitute into the parameter.
var errNullResult = errors.New("expression evaluated to null")

// contextResolver resolves context paths against a recipe context.
type contextResolver struct {
	available   bool
	values      map[string]any
	properties  map[string]any
	connections map[string]recipes.ConnectedResource
}

// newContextResolver builds a resolver for the given recipe context. A nil context produces a
// resolver that rejects every path.
func newContextResolver(ctx *recipecontext.Context) *contextResolver {
	if ctx == nil {
		return &contextResolver{}
	}

	values := map[string]any{
		"context.resource.name": ctx.Resource.Name,
		"context.resource.id":   ctx.Resource.ID,
		"context.resource.type": ctx.Resource.Type,

		"context.application.name": ctx.Application.Name,
		"context.application.id":   ctx.Application.ID,

		"context.environment.name": ctx.Environment.Name,
		"context.environment.id":   ctx.Environment.ID,
	}

	if ctx.Runtime.Kubernetes != nil {
		values["context.runtime.kubernetes.namespace"] = ctx.Runtime.Kubernetes.Namespace
		values["context.runtime.kubernetes.environmentNamespace"] = ctx.Runtime.Kubernetes.EnvironmentNamespace
	}

	if ctx.Azure != nil {
		values["context.azure.resourceGroup.name"] = ctx.Azure.ResourceGroup.Name
		values["context.azure.resourceGroup.id"] = ctx.Azure.ResourceGroup.ID
		values["context.azure.subscription.subscriptionId"] = ctx.Azure.Subscription.SubscriptionID
		values["context.azure.subscription.id"] = ctx.Azure.Subscription.ID
		if ctx.Azure.ResourceNameHash != "" {
			values["context.azure.resourceNameHash"] = ctx.Azure.ResourceNameHash
		}
	}

	if ctx.AWS != nil {
		values["context.aws.region"] = ctx.AWS.Region
		values["context.aws.account"] = ctx.AWS.Account
		if ctx.AWS.ResourceNameHash != "" {
			values["context.aws.resourceNameHash"] = ctx.AWS.ResourceNameHash
		}
	}

	for name, conn := range ctx.Resource.Connections {
		prefix := resourceConnectionsPrefix + name
		values[prefix+".id"] = conn.ID
		values[prefix+".name"] = conn.Name
		values[prefix+".type"] = conn.Type
	}

	return &contextResolver{
		available:   true,
		values:      values,
		properties:  ctx.Resource.Properties,
		connections: ctx.Resource.Connections,
	}
}

// resolve returns the value of a context path. Resource and connection properties that are not set
// (including properties of connections that are not present) resolve to nil (null) so optional
// properties can be tested in conditions. Any other unknown path is an error.
func (r *contextResolver) resolve(path string) (any, error) {
	if !r.available {
		return nil, fmt.Errorf("unknown context path %q: recipe context is not available", path)
	}

	if v, ok := r.values[path]; ok {
		return v, nil
	}

	if key, ok := strings.CutPrefix(path, resourcePropertiesPrefix); ok {
		return lookupProperty(r.properties, key, path)
	}

	if rest, ok := strings.CutPrefix(path, resourceConnectionsPrefix); ok {
		name, field, found := strings.Cut(rest, ".")
		if found {
			conn, exists := r.connections[name]
			if key, ok := strings.CutPrefix(field, "properties."); ok {
				if !exists {
					return nil, nil
				}
				return lookupProperty(conn.Properties, key, path)
			}
			if !exists && (field == "id" || field == "name" || field == "type") {
				return nil, nil
			}
		}
	}

	return nil, fmt.Errorf("unknown context path %q", path)
}

// lookupProperty resolves a property key. A top-level property whose name is exactly key (which may
// itself contain dots, such as "service.name") takes precedence. Otherwise a dotted key such as
// "config.tier" is resolved by walking nested objects. A missing or null segment resolves to nil;
// indexing into a value that is not an object is an error.
func lookupProperty(properties map[string]any, key string, path string) (any, error) {
	if v, ok := properties[key]; ok {
		return v, nil
	}
	var current any = properties
	for segment := range strings.SplitSeq(key, ".") {
		if current == nil {
			return nil, nil
		}
		obj, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("cannot resolve context path %q: the value before %q is not an object", path, segment)
		}
		current = obj[segment]
	}
	return current, nil
}

// evaluate evaluates an AST node. Evaluation is lazy: only the chosen ternary arm is evaluated, and
// && and || short-circuit, so unresolvable paths fail only when they are actually reached.
func (r *contextResolver) evaluate(n node) (any, error) {
	switch n := n.(type) {
	case literalNode:
		return n.value, nil
	case pathNode:
		return r.resolve(n.path)
	case notNode:
		v, err := r.evaluate(n.operand)
		if err != nil {
			return nil, err
		}
		return !truthy(v), nil
	case ternaryNode:
		cond, err := r.evaluate(n.cond)
		if err != nil {
			return nil, err
		}
		if truthy(cond) {
			return r.evaluate(n.ifTrue)
		}
		return r.evaluate(n.ifFalse)
	case binaryNode:
		return r.evaluateBinary(n)
	default:
		return nil, fmt.Errorf("unsupported expression node %T", n)
	}
}

func (r *contextResolver) evaluateBinary(n binaryNode) (any, error) {
	left, err := r.evaluate(n.left)
	if err != nil {
		return nil, err
	}

	switch n.op {
	case tokAnd:
		if !truthy(left) {
			return false, nil
		}
		right, err := r.evaluate(n.right)
		if err != nil {
			return nil, err
		}
		return truthy(right), nil
	case tokOr:
		if truthy(left) {
			return true, nil
		}
		right, err := r.evaluate(n.right)
		if err != nil {
			return nil, err
		}
		return truthy(right), nil
	}

	right, err := r.evaluate(n.right)
	if err != nil {
		return nil, err
	}
	eq, err := valuesEqual(left, right)
	if err != nil {
		return nil, err
	}
	if n.op == tokNeq {
		return !eq, nil
	}
	return eq, nil
}

// valuesEqual compares two values. Numbers compare numerically regardless of Go type, booleans
// compare as booleans, and null equals only null. Any other scalar combination compares by string
// form, which keeps existing expressions such as `context.resource.properties.port == "5432"`
// working. Objects and arrays cannot be compared.
func valuesEqual(a, b any) (bool, error) {
	if a == nil || b == nil {
		return a == nil && b == nil, nil
	}
	if isComposite(a) || isComposite(b) {
		return false, fmt.Errorf("cannot compare object or array values")
	}
	if an, ok := toRat(a); ok {
		if bn, ok := toRat(b); ok {
			return an.Cmp(bn) == 0, nil
		}
	}
	if ab, ok := a.(bool); ok {
		if bb, ok := b.(bool); ok {
			return ab == bb, nil
		}
	}
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b), nil
}

// truthy reports whether a value is considered true in a condition: null, false, "", zero, and
// empty objects or arrays are false; everything else is true.
func truthy(v any) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return val != ""
	}
	if n, ok := toRat(v); ok {
		return n.Sign() != 0
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Map, reflect.Slice, reflect.Array:
		return rv.Len() > 0
	}
	return true
}

func isComposite(v any) bool {
	switch reflect.ValueOf(v).Kind() {
	case reflect.Map, reflect.Slice, reflect.Array, reflect.Struct:
		return true
	}
	return false
}

// toRat converts a number to an exact rational so that equality does not lose precision, for
// example between integers larger than 2^53. Non-finite floats are not converted.
func toRat(v any) (*big.Rat, bool) {
	switch n := v.(type) {
	case int:
		return new(big.Rat).SetInt64(int64(n)), true
	case int8:
		return new(big.Rat).SetInt64(int64(n)), true
	case int16:
		return new(big.Rat).SetInt64(int64(n)), true
	case int32:
		return new(big.Rat).SetInt64(int64(n)), true
	case int64:
		return new(big.Rat).SetInt64(n), true
	case uint:
		return new(big.Rat).SetUint64(uint64(n)), true
	case uint8:
		return new(big.Rat).SetUint64(uint64(n)), true
	case uint16:
		return new(big.Rat).SetUint64(uint64(n)), true
	case uint32:
		return new(big.Rat).SetUint64(uint64(n)), true
	case uint64:
		return new(big.Rat).SetUint64(n), true
	case float32:
		r := new(big.Rat).SetFloat64(float64(n))
		return r, r != nil
	case float64:
		r := new(big.Rat).SetFloat64(n)
		return r, r != nil
	case json.Number:
		return new(big.Rat).SetString(n.String())
	}
	return nil, false
}
