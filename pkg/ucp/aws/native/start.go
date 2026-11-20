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
	"context"
	"maps"

	"github.com/radius-project/radius/pkg/aws/schemas"
)

// WithName returns properties with the name UCP generates from clientToken, when the type declares a name property
// (Options.NameProperty) and properties leave it out. The name comes from the client token, which identifies the
// create, so every attempt of one create sends the same name. properties is not changed.
func (t *Type) WithName(properties map[string]any, clientToken string) (map[string]any, error) {
	rules := t.NameRules
	if rules == nil {
		return properties, nil
	}
	if value, ok := properties[rules.Property]; ok && value != nil {
		return properties, nil
	}
	name, err := rules.Generate(clientToken)
	if err != nil {
		return nil, err
	}
	out := maps.Clone(properties)
	if out == nil {
		out = map[string]any{}
	}
	out[rules.Property] = name
	return out, nil
}

// StartDelete starts deleting the resource with the identifier, with the call the type declares:
//
//   - Reset, for a type whose onDelete metadata is reset (Resetter).
//   - DeleteScoped with managed, for a handler that removes only the members it manages (ScopedDeleter), even when it
//     also sends a delete token, because a token deletes the whole resource.
//   - DeleteWithToken with token, for a handler that sends the same value on every attempt (DeletePreparer), once UCP
//     has saved one.
//   - Delete otherwise.
func (t *Type) StartDelete(ctx context.Context, region string, identifier string, managed []map[string]any, token string) error {
	resetter, resets := t.Handler.(Resetter)
	scoped, isScoped := t.Handler.(ScopedDeleter)
	preparer, prepares := t.Handler.(DeletePreparer)
	switch {
	case resets && t.Metadata.OnDelete == schemas.OnDeleteReset:
		return resetter.Reset(ctx, region, identifier)
	case isScoped:
		return scoped.DeleteScoped(ctx, region, identifier, managed)
	case prepares && token != "":
		return preparer.DeleteWithToken(ctx, region, identifier, token)
	default:
		return t.Handler.Delete(ctx, region, identifier)
	}
}
