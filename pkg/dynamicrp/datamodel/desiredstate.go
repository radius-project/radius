/*
Copyright 2026 The Radius Authors.

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

package datamodel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/radius-project/radius/pkg/crypto/encryption"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/schema"
)

// DesiredState retains protected application inputs independently of computed resource properties.
type DesiredState struct {
	ResourceID string         `json:"resourceId"`
	Type       string         `json:"type"`
	APIVersion string         `json:"apiVersion"`
	Revision   string         `json:"revision"`
	Payload    map[string]any `json:"payload"`
}

type desiredPayload struct {
	Properties map[string]any `json:"properties"`
	Schema     map[string]any `json:"schema"`
}

// NewDesiredState validates and encrypts a private input/schema snapshot under its source identity.
func NewDesiredState(ctx context.Context, incoming recipes.IncomingReference, sourceSchema map[string]any, handler *encryption.SensitiveDataHandler) (*DesiredState, error) {
	if handler == nil || sourceSchema == nil {
		return nil, fmt.Errorf("desired state requires schema and encryption protection")
	}
	if err := recipes.ValidateIncomingReferences([]recipes.IncomingReference{incoming}); err != nil {
		return nil, err
	}
	if err := schema.ValidateDesiredResourceInputs(ctx, incoming.Properties, sourceSchema); err != nil {
		return nil, err
	}
	state := &DesiredState{ResourceID: incoming.ID, Type: incoming.Type, APIVersion: incoming.APIVersion, Revision: incoming.Revision}
	envelope := map[string]any{"value": desiredPayload{Properties: incoming.Properties, Schema: sourceSchema}}
	if err := handler.EncryptSensitiveFields(envelope, []string{"value"}, state.encryptionContext()); err != nil {
		return nil, fmt.Errorf("failed to protect desired state: %w", err)
	}
	protected, ok := envelope["value"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("desired state encryption returned an invalid envelope")
	}
	state.Payload = protected
	return state, nil
}

// Materialize creates private plaintext inputs for execution; it never mutates the saved snapshot.
func (s *DesiredState) Materialize(ctx context.Context, handler *encryption.SensitiveDataHandler) (recipes.IncomingReference, map[string]any, error) {
	if s == nil || handler == nil || s.Payload == nil {
		return recipes.IncomingReference{}, nil, fmt.Errorf("desired state requires protected data and encryption handler")
	}
	encoded, err := json.Marshal(s.Payload)
	if err != nil {
		return recipes.IncomingReference{}, nil, fmt.Errorf("failed to encode desired state: %w", err)
	}
	if !encryption.IsEncryptedData(encoded) {
		return recipes.IncomingReference{}, nil, fmt.Errorf("desired state payload is not encrypted")
	}
	var protected map[string]any
	if err := json.Unmarshal(encoded, &protected); err != nil {
		return recipes.IncomingReference{}, nil, err
	}
	envelope := map[string]any{"value": protected}
	if err := handler.DecryptSensitiveFields(ctx, envelope, []string{"value"}, s.encryptionContext()); err != nil {
		return recipes.IncomingReference{}, nil, fmt.Errorf("failed to materialize desired state: %w", err)
	}
	encoded, err = json.Marshal(envelope["value"])
	if err != nil {
		return recipes.IncomingReference{}, nil, err
	}
	var payload desiredPayload
	if err := json.Unmarshal(encoded, &payload); err != nil {
		return recipes.IncomingReference{}, nil, err
	}
	incoming := recipes.IncomingReference{ID: s.ResourceID, Type: s.Type, APIVersion: s.APIVersion, Revision: s.Revision, Properties: payload.Properties}
	if payload.Schema == nil {
		return recipes.IncomingReference{}, nil, fmt.Errorf("desired state is missing its source schema")
	}
	if err := recipes.ValidateIncomingReferences([]recipes.IncomingReference{incoming}); err != nil {
		return recipes.IncomingReference{}, nil, err
	}
	return incoming, payload.Schema, nil
}

func (s *DesiredState) encryptionContext() string {
	return fmt.Sprintf("radius-delegated-desired:%q:%q:%q:%q", s.ResourceID, s.Type, s.APIVersion, s.Revision)
}

// RestoreDesiredInputs restores saved application inputs for shared recipe orchestration.
// A resource without a protected snapshot retains its ordinary recipe behavior.
func (d *DynamicResource) RestoreDesiredInputs(ctx context.Context, provider encryption.KeyProvider) (map[string]any, error) {
	if d.DesiredState == nil {
		return nil, nil
	}
	handler, err := encryption.NewSensitiveDataHandlerFromProvider(ctx, provider)
	if err != nil {
		return nil, fmt.Errorf("failed to load desired-input encryption key: %w", err)
	}
	incoming, _, err := d.DesiredState.Materialize(ctx, handler)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(incoming.ID, d.ID) || !strings.EqualFold(incoming.Type, d.Type) || incoming.APIVersion != d.UpdatedAPIVersion {
		return nil, fmt.Errorf("protected desired inputs do not match the resource identity and API version")
	}
	return incoming.Properties, nil
}
