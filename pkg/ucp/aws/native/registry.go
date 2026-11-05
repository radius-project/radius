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

package native

import (
	"encoding/json"
	"fmt"
	"strings"

	awsoperations "github.com/radius-project/radius/pkg/aws/operations"
	"github.com/radius-project/radius/pkg/aws/schemas"
)

// Type is a type a native handler serves.
type Type struct {
	Handler Handler

	// Schema is the type's schema in CloudFormation format. UCP uses it instead of the one CloudFormation returns,
	// with either handler.
	Schema string

	// HandWritten is true for a type with no CloudFormation type. UCP always serves it with the handler. Otherwise,
	// UCP serves it with the handler only where Cloud Control can't create it.
	HandWritten bool

	// NameRules are the rules for the name UCP generates when the template leaves out Options().NameProperty, or nil
	// when the handler has no name property.
	NameRules *NameRules

	// WriteOnly are the paths of the schema's write-only properties, relative to the properties object, such as
	// MasterUserPassword. UCP never stores their values and removes them from every response, with either handler.
	WriteOnly []string

	// Metadata is the type's per-type metadata from pkg/aws/schemas, with defaults where the type has none. UCP applies
	// it with either handler.
	Metadata schemas.Metadata
}

// Registry holds the types native handlers serve. A nil Registry has none.
type Registry struct {
	types map[string]*Type
}

// NewRegistry returns a Registry of handlers, each with its schema and metadata from catalog. A handler with no
// schema, two handlers for one type, a RecoverBySearch handler that isn't a Finder, a type whose onDelete is reset
// with a handler that isn't a Resetter, and a name property the schema doesn't define as a string, or whose rules no
// generated name fits, fail.
func NewRegistry(catalog schemas.Catalog, handlers ...Handler) (*Registry, error) {
	r := &Registry{types: map[string]*Type{}}
	for _, h := range handlers {
		name := h.TypeName()
		key := strings.ToLower(name)
		if _, ok := r.types[key]; ok {
			return nil, fmt.Errorf("two native handlers serve the AWS type %s", name)
		}
		t := &Type{Handler: h}
		if schema, ok := lookup(catalog.Native, name); ok {
			t.Schema = schema
		} else if schema, ok := lookup(catalog.HandWritten, name); ok {
			t.Schema, t.HandWritten = schema, true
		} else {
			return nil, fmt.Errorf("the native handler for the AWS type %s has no schema in pkg/aws/schemas", name)
		}
		var parsed awsoperations.ResourceTypeSchema
		if err := json.Unmarshal([]byte(t.Schema), &parsed); err != nil {
			return nil, fmt.Errorf("the schema of the AWS type %s can't be read: %w", name, err)
		}
		t.WriteOnly = awsoperations.PropertyPaths(parsed.WriteOnlyProperties)
		t.Metadata = catalog.MetadataOf(name)
		if _, ok := h.(Resetter); t.Metadata.OnDelete == schemas.OnDeleteReset && !ok {
			return nil, fmt.Errorf("the AWS type %s resets on delete, but its native handler has no Reset method", name)
		}
		_, reads := h.(ScopedReader)
		_, deletes := h.(ScopedDeleter)
		if reads != deletes {
			return nil, fmt.Errorf("the native handler for the AWS type %s must have both ReadScoped and DeleteScoped, or neither", name)
		}
		opts := h.Options()
		if _, ok := h.(Finder); opts.Recovery == RecoverBySearch && !ok {
			return nil, fmt.Errorf("the native handler for the AWS type %s finds a lost create by search, but has no Find method", name)
		}
		if opts.NameProperty != "" {
			rules, err := nameRules(t.Schema, opts.NameProperty)
			if err != nil {
				return nil, fmt.Errorf("the native handler for the AWS type %s: %w", name, err)
			}
			t.NameRules = rules
		}
		r.types[key] = t
	}
	return r, nil
}

// Lookup returns the type typeName, such as AWS::DocDB::DBCluster, ignoring case. It returns nil when no handler
// serves it.
func (r *Registry) Lookup(typeName string) *Type {
	if r == nil {
		return nil
	}
	return r.types[strings.ToLower(typeName)]
}

func lookup(schemas map[string]string, typeName string) (string, bool) {
	for name, schema := range schemas {
		if strings.EqualFold(name, typeName) {
			return schema, true
		}
	}
	return "", false
}
