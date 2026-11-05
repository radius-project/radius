// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package userauth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/radius-project/radius/pkg/authz/records"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/radius-project/radius/pkg/ucp/authorization"
	"github.com/radius-project/radius/pkg/ucp/authzcheck"
	"github.com/radius-project/radius/pkg/ucp/resources"
	ucplog "github.com/radius-project/radius/pkg/ucp/ucplog"
)

func canonicalFields(value any) error {
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			for _, protected := range []string{"properties", "environment", "application", "connections", "source", "recipePacks", "providers", "azure", "aws", "mode", "template", "resources", "type", "name", "apiVersion", "scope", "resourceGroup", "subscriptionId", "copy", "condition", "providerconfig"} {
				if strings.EqualFold(key, protected) && key != protected {
					return fmt.Errorf("AuthorizationIndeterminate: use canonical JSON field %s, not %s", protected, key)
				}
			}
			if err := canonicalFields(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range value {
			if err := canonicalFields(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func deploymentBody(target string, raw []byte) ([]byte, error) {
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("Invalid: %w", err)
	}
	if err := canonicalFields(body); err != nil {
		return nil, err
	}
	properties, ok := body["properties"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("AuthorizationIndeterminate: deployment properties required")
	}
	for key := range properties {
		if !slices.Contains([]string{"mode", "template", "parameters", "providerconfig"}, key) {
			return nil, fmt.Errorf("AuthorizationIndeterminate: unsupported deployment property %s", key)
		}
	}
	group := target[:strings.LastIndex(strings.ToLower(target), "/providers/microsoft.resources/deployments/")]
	expected := map[string]any{
		"radius":      map[string]any{"type": "radius", "value": map[string]any{"scope": group}},
		"deployments": map[string]any{"type": "Microsoft.Resources", "value": map[string]any{"scope": group}},
	}
	if provided, exists := properties["providerconfig"]; exists {
		// The SDK and normalized request path use different casing for the same IDs.
		if providers, ok := provided.(map[string]any); ok {
			for name, expectedType := range map[string]string{"radius": "radius", "deployments": "Microsoft.Resources"} {
				if provider, ok := providers[name].(map[string]any); ok {
					if typ, ok := provider["type"].(string); ok && strings.EqualFold(typ, expectedType) {
						provider["type"] = expectedType
					}
					if value, ok := provider["value"].(map[string]any); ok {
						if scope, ok := value["scope"].(string); ok && strings.EqualFold(scope, group) {
							value["scope"] = group
						}
					}
				}
			}
		}
		a, err := json.Marshal(provided)
		if err != nil {
			return nil, err
		}
		b, err := json.Marshal(expected)
		if err != nil {
			return nil, err
		}
		if string(a) != string(b) {
			return nil, fmt.Errorf("AuthorizationIndeterminate: provider configuration must target the deployment resource group")
		}
	}
	properties["providerconfig"] = expected
	template, ok := properties["template"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("AuthorizationIndeterminate: inline template required")
	}
	parameters := map[string]any{}
	if value, exists := properties["parameters"]; exists {
		var ok bool
		parameters, ok = value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("Invalid: deployment parameters must be an object")
		}
	}
	resolved, err := authorization.ResolveDeploymentTemplate(template, parameters, group)
	if err != nil {
		return nil, err
	}
	properties["template"] = resolved
	properties["parameters"] = map[string]any{}
	return json.Marshal(body)
}

func engineDeploymentBody(raw []byte) ([]byte, error) {
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	properties, ok := body["properties"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("AuthorizationIndeterminate: deployment properties required")
	}
	template, ok := properties["template"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("AuthorizationIndeterminate: inline template required")
	}
	for key := range template {
		if !slices.Contains([]string{"$schema", "contentVersion", "resources", "outputs"}, key) {
			return nil, fmt.Errorf("AuthorizationIndeterminate: unsupported literal template field %s", key)
		}
	}
	items, ok := template["resources"].([]any)
	if !ok {
		return nil, fmt.Errorf("AuthorizationIndeterminate: literal resource array required")
	}
	resources := map[string]any{}
	for index, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("AuthorizationIndeterminate: invalid template resource")
		}
		if templatePolicyResource(item) {
			continue
		}
		envelope := map[string]any{}
		for key, value := range item {
			if key == "type" || key == "apiVersion" || key == "dependsOn" {
				continue
			}
			if !slices.Contains([]string{"name", "properties", "tags", "location", "sku", "kind", "identity", "extendedLocation", "managedBy", "plan", "zones"}, key) {
				return nil, fmt.Errorf("AuthorizationIndeterminate: unsupported literal resource field %s", key)
			}
			envelope[key] = engineLiteral(value)
		}
		typ, _ := item["type"].(string)
		version, _ := item["apiVersion"].(string)
		resources[fmt.Sprintf("r%d", index)] = map[string]any{
			"import": "radius", "type": typ + "@" + version, "properties": envelope,
		}
		dependencies, _ := item["dependsOn"].([]any)
		resolvedDependencies := []any{}
		for _, dependency := range dependencies {
			target, ok := dependency.(string)
			if !ok {
				return nil, fmt.Errorf("Invalid: resolved dependency ID required")
			}
			found := false
			for otherIndex, otherRaw := range items {
				other, ok := otherRaw.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("Invalid: resource object required")
				}
				otherType, _ := other["type"].(string)
				otherName, _ := other["name"].(string)
				if strings.HasSuffix(strings.ToLower(target), strings.ToLower("/providers/"+otherType+"/"+otherName)) {
					found = true
					if !templatePolicyResource(other) {
						resolvedDependencies = append(resolvedDependencies, fmt.Sprintf("r%d", otherIndex))
					}
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("AuthorizationIndeterminate: dependency is outside the resolved deployment")
			}
		}
		if len(resolvedDependencies) > 0 {
			resources[fmt.Sprintf("r%d", index)].(map[string]any)["dependsOn"] = resolvedDependencies
		}
	}
	// The real engine requires the Radius import and symbolic resource format.
	outputs := template["outputs"]
	if outputs == nil {
		outputs = map[string]any{}
	}
	properties["template"] = map[string]any{
		"$schema":         "https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#",
		"languageVersion": "2.1-experimental", "contentVersion": "1.0.0.0",
		"imports":   map[string]any{"radius": map[string]string{"provider": "Radius", "version": "latest"}},
		"resources": resources,
		"outputs":   engineLiteral(outputs),
	}
	return json.Marshal(body)
}

func engineLiteral(value any) any {
	switch value := value.(type) {
	case string:
		if strings.HasPrefix(value, "[") {
			return "[" + value
		}
	case map[string]any:
		result := make(map[string]any, len(value))
		for key, child := range value {
			result[key] = engineLiteral(child)
		}
		return result
	case []any:
		result := make([]any, len(value))
		for index, child := range value {
			result[index] = engineLiteral(child)
		}
		return result
	}
	return value
}

func templatePolicyResource(item map[string]any) bool {
	typ, _ := item["type"].(string)
	return strings.EqualFold(typ, "Radius.Core/roleDefinitions") || strings.EqualFold(typ, "Radius.Core/roleAssignments")
}

func deploymentPolicyMutations(target string, raw []byte) ([]authorization.PolicyMutation, error) {
	var body struct {
		Properties struct {
			Template struct {
				Resources []map[string]any `json:"resources"`
			} `json:"template"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	group := target[:strings.LastIndex(strings.ToLower(target), "/providers/microsoft.resources/deployments/")]
	mutations := []authorization.PolicyMutation{}
	for _, item := range body.Properties.Template.Resources {
		if !templatePolicyResource(item) {
			continue
		}
		id, err := authorization.DeploymentResourceID(group, item)
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(item["properties"])
		if err != nil {
			return nil, err
		}
		mutation := authorization.PolicyMutation{Target: id}
		typ, _ := item["type"].(string)
		if strings.EqualFold(typ, "Radius.Core/roleDefinitions") {
			mutation.Role = &api.RoleDefinitionProperties{}
			err = json.Unmarshal(raw, mutation.Role)
		} else {
			mutation.Assignment = &api.RoleAssignmentProperties{}
			err = json.Unmarshal(raw, mutation.Assignment)
		}
		if err != nil {
			return nil, fmt.Errorf("Invalid: policy resource %s: %w", id, err)
		}
		mutations = append(mutations, mutation)
	}
	return mutations, nil
}

func (r *Runtime) writeChecks(target string, raw []byte) ([]authorization.Check, []records.ResourceApproval, error) {
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil || body == nil {
		return nil, nil, fmt.Errorf("Invalid: expected a JSON resource object")
	}
	if err := canonicalFields(body); err != nil {
		return nil, nil, err
	}
	mapping, err := r.Catalog.MapRequest(http.MethodPut, target)
	if err != nil || len(mapping.Checks) != 1 {
		return nil, nil, fmt.Errorf("AuthorizationIndeterminate: unknown write target %s", target)
	}
	approval := records.ResourceApproval{ResourceID: target, Action: mapping.Checks[0].Permission, InputHash: records.InputHash(raw)}
	approvals := []records.ResourceApproval{approval}
	checks := []authorization.Check{}
	id, err := resources.Parse(target)
	if err != nil {
		return nil, nil, fmt.Errorf("AuthorizationIndeterminate: %w", err)
	}
	if id.IsScope() {
		return checks, approvals, nil
	}
	if !strings.Contains(strings.ToLower(target), "/providers/microsoft.resources/deployments/") {
		references, err := r.Catalog.ReferenceChecks(http.MethodPut, target, body, authorization.ReferenceOptions{})
		return references, approvals, err
	}
	properties, ok := body["properties"].(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("AuthorizationIndeterminate: deployment properties are required")
	}
	if properties["mode"] != "Incremental" {
		return nil, nil, fmt.Errorf("AuthorizationIndeterminate: only Incremental deployment mode is supported")
	}
	template, ok := properties["template"].(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("AuthorizationIndeterminate: inline literal template required")
	}
	for _, key := range []string{"templateLink", "parameters", "expressionEvaluationOptions"} {
		if value, exists := properties[key]; exists {
			object, ok := value.(map[string]any)
			if !ok || len(object) != 0 {
				return nil, nil, fmt.Errorf("AuthorizationIndeterminate: %s is not supported by literal-template preflight", key)
			}
		}
	}
	resources, ok := template["resources"].([]any)
	if !ok || len(resources) == 0 || len(resources) > 100 {
		return nil, nil, fmt.Errorf("AuthorizationIndeterminate: template requires 1-100 literal resources")
	}
	group := target[:strings.LastIndex(strings.ToLower(target), "/providers/microsoft.resources/deployments/")]
	seen := map[string]bool{}
	for _, resource := range resources {
		item, ok := resource.(map[string]any)
		if !ok {
			return nil, nil, fmt.Errorf("AuthorizationIndeterminate: invalid template resource")
		}
		for _, key := range []string{"copy", "condition", "resources", "subscriptionId", "resourceGroup"} {
			if _, exists := item[key]; exists {
				return nil, nil, fmt.Errorf("AuthorizationIndeterminate: template resource %s is unsupported", key)
			}
		}
		typ, _ := item["type"].(string)
		name, _ := item["name"].(string)
		version, _ := item["apiVersion"].(string)
		if name == "" || strings.ContainsAny(name, "/[]") || version == "" || strings.Count(typ, "/") != 1 || strings.HasPrefix(strings.ToLower(typ), "microsoft.") {
			return nil, nil, fmt.Errorf("AuthorizationIndeterminate: only literal, top-level Radius resources are supported")
		}
		id, err := authorization.DeploymentResourceID(group, item)
		if err != nil {
			return nil, nil, err
		}
		if seen[strings.ToLower(id)] {
			return nil, nil, fmt.Errorf("AuthorizationIndeterminate: duplicate template resource")
		}
		seen[strings.ToLower(id)] = true
		mapped, err := r.Catalog.MapRequest(http.MethodPut, id)
		if err != nil {
			return nil, nil, fmt.Errorf("AuthorizationIndeterminate: %w", err)
		}
		if strings.EqualFold(typ, "Radius.Core/authorizationSettings") {
			return nil, nil, fmt.Errorf("AuthorizationIndeterminate: authorization settings require the explicit enforcement workflow")
		}
		checks = append(checks, mapped.Checks...)
		if templatePolicyResource(item) {
			continue
		}
		// The engine sends the resource envelope, not deployment-only fields.
		input := map[string]any{}
		for _, key := range []string{"properties", "tags", "location", "sku", "kind", "identity", "extendedLocation", "managedBy", "plan", "zones"} {
			if value, exists := item[key]; exists {
				input[key] = value
			}
		}
		inputRaw, err := json.Marshal(input)
		if err != nil {
			return nil, nil, err
		}
		references, err := r.Catalog.ReferenceChecks(http.MethodPut, id, input, authorization.ReferenceOptions{})
		if err != nil {
			return nil, nil, fmt.Errorf("AuthorizationIndeterminate: %w", err)
		}
		checks = append(checks, references...)
		approvals = append(approvals, records.ResourceApproval{ResourceID: id, Action: mapped.Checks[0].Permission, InputHash: records.InputHash(inputRaw)})
	}
	return checks, approvals, nil
}

func (r *Runtime) component(w http.ResponseWriter, req *http.Request, next http.Handler, path string) {
	// The internal data endpoint performs its own operation/component checks.
	if strings.HasPrefix(strings.ToLower(path), "/authz/data/") {
		next.ServeHTTP(w, req)
		return
	}
	if strings.Contains(strings.ToLower(path), "/providers/radius.core/role") ||
		strings.Contains(strings.ToLower(path), "/providers/radius.core/authorization") ||
		strings.Contains(strings.ToLower(path), "/providers/system.authorization/") {
		fail(w, fmt.Errorf("AuthorizationFailed: components cannot administer user policy or create test records"))
		return
	}
	snapshot, err := r.Cache.Snapshot()
	if err != nil {
		fail(w, err)
		return
	}
	policy, err := snapshot.Policy()
	if err != nil {
		fail(w, err)
		return
	}
	if *policy.Settings.Properties.Mode == api.UserAuthorizationModeDisabled {
		next.ServeHTTP(w, req)
		return
	}
	deny := func(err error) {
		if *policy.Settings.Properties.Mode == api.UserAuthorizationModeDryRun {
			ucplog.FromContextOrDiscard(req.Context()).Info("user authorization would deny", "wouldDeny", true, "reason", err.Error(), "target", path)
			next.ServeHTTP(w, req)
			return
		}
		fail(w, err)
	}
	caller, _ := authzcheck.CallerFromContext(req.Context())
	if req.Method == http.MethodGet {
		// Existing internal read isolation is incomplete in the reused prototype.
		// This is restricted to verified components, never an asserted user.
		next.ServeHTTP(w, req)
		return
	}
	// Controllers need an explicit workload assignment protocol. Namespace
	// mappings alone must not mint a privileged human identity.
	if caller.Name == authzcheck.ComponentController {
		deny(fmt.Errorf("AuthorizationIndeterminate: controller workload delegation is not supported by user RBAC POC"))
		return
	}
	store := records.NewStore(r.DB)
	var record *records.Record
	if id := req.Header.Get(records.HeaderRecord); id != "" {
		record, _, err = store.GetRecord(req.Context(), id)
	} else if caller.Name == authzcheck.ComponentDeploymentEngine {
		var active []*records.Record
		active, err = store.ActiveRecords(req.Context(), caller.Name)
		for _, candidate := range active {
			if candidate.UserApproval != nil && candidate.CoversTarget(path) && (record == nil || candidate.CreatedAt.After(record.CreatedAt)) {
				record = candidate
			}
		}
	}
	if err != nil || record == nil || !record.Active(store.CurrentTime()) || record.UserApproval == nil {
		deny(fmt.Errorf("AuthorizationFailed: active user-approved execution record required"))
		return
	}
	mapping, err := r.Catalog.MapRequest(req.Method, path)
	if err != nil {
		deny(fmt.Errorf("AuthorizationIndeterminate: %w", err))
		return
	}
	body, err := requestBody(req)
	if err != nil {
		deny(err)
		return
	}
	approved := false
	for _, resource := range record.UserApproval.Resources {
		if strings.EqualFold(resource.ResourceID, path) && len(mapping.Checks) == 1 &&
			strings.EqualFold(resource.Action, mapping.Checks[0].Permission) && resource.InputHash == records.InputHash(body) {
			approved = true
		}
	}
	if !approved {
		deny(fmt.Errorf("AuthorizationFailed: input or target differs from deployment preflight"))
		return
	}
	var identity authorization.Identity
	if err := json.Unmarshal([]byte(record.UserApproval.Identity), &identity); err != nil {
		deny(fmt.Errorf("AuthorizationUnavailable: corrupt execution identity"))
		return
	}
	checks := mapping.Checks
	if req.Method == http.MethodPut || req.Method == http.MethodPatch {
		references, _, err := r.writeChecks(path, body)
		if err != nil {
			deny(err)
			return
		}
		checks = append(checks, references...)
	}
	if err := r.check(req, snapshot, identity, checks, *policy.Settings.Properties.Mode); err != nil {
		fail(w, err)
		return
	}
	req.Header.Set(records.HeaderRecord, record.ID)
	next.ServeHTTP(w, req)
}
