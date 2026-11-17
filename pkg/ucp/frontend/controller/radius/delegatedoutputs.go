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

package radius

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

// expandDelegatedDeploymentOutputs preserves logical ownership when delegates are read symbols in the evaluated template.
func expandDelegatedDeploymentOutputs(ctx context.Context, store database.Client, response *http.Response) error {
	if response.StatusCode != http.StatusOK || response.Body == nil {
		return nil
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("failed to read deployment output inventory: %w", err)
	}
	if err := response.Body.Close(); err != nil {
		return err
	}
	response.Body = io.NopCloser(bytes.NewReader(data))
	if !bytes.Contains(data, []byte(`"outputResources"`)) {
		return nil
	}
	var deployment map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&deployment); err != nil {
		return fmt.Errorf("invalid deployment output inventory: %w", err)
	}
	properties, _ := deployment["properties"].(map[string]any)
	outputs, ok := properties["outputResources"].([]any)
	if !ok {
		return nil
	}
	expanded, err := addDelegatedOutputResources(ctx, store, outputs)
	if err != nil {
		return err
	}
	if len(expanded) == len(outputs) {
		return nil
	}
	properties["outputResources"] = expanded
	data, err = json.Marshal(deployment)
	if err != nil {
		return err
	}
	response.Body = io.NopCloser(bytes.NewReader(data))
	response.ContentLength = int64(len(data))
	if response.Header == nil {
		response.Header = http.Header{}
	}
	response.Header.Set("Content-Length", strconv.Itoa(len(data)))
	return nil
}

func addDelegatedOutputResources(ctx context.Context, store database.Client, outputs []any) ([]any, error) {
	seen := map[string]struct{}{}
	for _, raw := range outputs {
		output, _ := raw.(map[string]any)
		id, _ := output["id"].(string)
		seen[strings.ToLower(id)] = struct{}{}
	}
	result := append([]any{}, outputs...)
	for _, raw := range outputs {
		output, _ := raw.(map[string]any)
		rawID, _ := output["id"].(string)
		id, err := resources.ParseResource(rawID)
		if err != nil || id.FindScope("radius") == "" || strings.EqualFold(id.ProviderNamespace(), "Microsoft.Resources") {
			continue
		}
		object, err := store.Get(ctx, id.String())
		if errors.Is(err, &database.ErrNotFound{}) {
			// Non-Dynamic-RP resources can be represented only in the downstream provider.
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("failed to resolve logical delegated output inventory: %w", err)
		}
		var resource struct {
			Properties struct {
				Status struct {
					DelegatedResources []struct {
						ID         string `json:"id"`
						Type       string `json:"type"`
						APIVersion string `json:"apiVersion"`
					} `json:"delegatedResources"`
				} `json:"status"`
			} `json:"properties"`
		}
		if err := object.As(&resource); err != nil {
			return nil, err
		}
		for _, member := range resource.Properties.Status.DelegatedResources {
			memberID, err := resources.ParseResource(member.ID)
			if err != nil || !strings.EqualFold(memberID.RootScope(), id.RootScope()) || !strings.EqualFold(memberID.Type(), member.Type) || member.APIVersion == "" {
				return nil, fmt.Errorf("invalid logical delegated output inventory")
			}
			key := strings.ToLower(member.ID)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, map[string]any{"id": member.ID, "resourceType": member.Type, "apiVersion": member.APIVersion})
		}
	}
	return result, nil
}
