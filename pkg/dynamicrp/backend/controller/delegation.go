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

package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"sort"
	"strings"
	"uuid"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/crypto/encryption"
	"github.com/radius-project/radius/pkg/dynamicrp/backend/processor"
	"github.com/radius-project/radius/pkg/dynamicrp/backend/secret"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	recipecontroller "github.com/radius-project/radius/pkg/portableresources/backend/controller"
	"github.com/radius-project/radius/pkg/portableresources/processors"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	"github.com/radius-project/radius/pkg/recipes/engine"
	recipesutil "github.com/radius-project/radius/pkg/recipes/util"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/schema"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

type delegationRuntime struct {
	opts   ctrl.Options
	engine engine.Engine
	loader configloader.ConfigurationLoader
	secret secret.Materializer
}

type delegationMember struct {
	object   *database.Object
	resource datamodel.DynamicResource
	incoming recipes.IncomingReference
	schema   map[string]any
}

func readDynamicResource(ctx context.Context, store database.Client, id string) (*database.Object, *datamodel.DynamicResource, error) {
	object, err := store.Get(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	var resource datamodel.DynamicResource
	if err := object.As(&resource); err != nil {
		return nil, nil, err
	}
	return object, &resource, nil
}

func delegationResultContext(target *datamodel.DynamicResource) string {
	return target.ID + ":delegation-result:" + target.DelegationOperationID
}

func delegationResultID(target *datamodel.DynamicResource) (string, error) {
	id, err := resources.ParseResource(target.ID)
	if err != nil {
		return "", err
	}
	return id.RootScope() + "/providers/Radius.Internal/delegationResults/" + target.DelegationOperationID, nil
}

func (r *delegationRuntime) removeDelegationResult(ctx context.Context, targetID, operationID string) error {
	target := &datamodel.DynamicResource{DelegationOperationID: operationID}
	target.ID = targetID
	id, err := delegationResultID(target)
	if err != nil {
		return err
	}
	object, err := r.opts.DatabaseClient.Get(ctx, id)
	if errors.Is(err, &database.ErrNotFound{}) {
		return nil
	}
	if err != nil {
		return err
	}
	return r.opts.DatabaseClient.Delete(ctx, id, database.WithETag(object.ETag))
}
func restoreDelegationResult(ctx context.Context, target *datamodel.DynamicResource, handler *encryption.SensitiveDataHandler) (*recipes.RecipeOutput, error) {
	encoded, err := json.Marshal(target.DelegationResult)
	if err != nil {
		return nil, err
	}
	if !encryption.IsEncryptedData(encoded) {
		return nil, fmt.Errorf("the saved delegation result is not protected")
	}
	var protected map[string]any
	if err := json.Unmarshal(encoded, &protected); err != nil {
		return nil, err
	}
	envelope := map[string]any{"value": protected}
	if err := handler.DecryptSensitiveFields(ctx, envelope, []string{"value"}, delegationResultContext(target)); err != nil {
		return nil, err
	}
	encoded, err = json.Marshal(envelope["value"])
	if err != nil {
		return nil, err
	}
	var saved struct {
		Output              recipes.RecipeOutput `json:"output"`
		ProviderResourceIDs map[string]string    `json:"providerResourceIDs"`
	}
	if err := json.Unmarshal(encoded, &saved); err != nil {
		return nil, err
	}
	saved.Output.ProviderResourceIDs = saved.ProviderResourceIDs
	return &saved.Output, nil
}
func (r *delegationRuntime) handler(ctx context.Context) (*encryption.SensitiveDataHandler, error) {
	if r.opts.KubeClient == nil {
		return nil, fmt.Errorf("delegated provisioning requires the protected-input encryption key")
	}
	return encryption.NewSensitiveDataHandlerFromProvider(ctx, encryption.NewKubernetesKeyProvider(r.opts.KubeClient, nil))
}

func (r *delegationRuntime) target(ctx context.Context, request *ctrl.Request, source *datamodel.DynamicResource) (*database.Object, *datamodel.DynamicResource, error) {
	id := source.ID
	if source.DelegationTargetID != "" {
		id = source.DelegationTargetID
	}
	object, target, err := readDynamicResource(ctx, r.opts.DatabaseClient, id)
	if err != nil {
		return nil, nil, err
	}
	if target.DelegationOperationID != request.OperationID.String() &&
		(source.DesiredState == nil || source.DesiredState.Revision != target.DelegationOperationID || !target.DelegationExecutionStarted) {
		return nil, nil, fmt.Errorf("the operation does not own the delegation target")
	}
	if target.DelegationExecutionStarted && target.DelegationResult == nil {
		resultID, err := delegationResultID(target)
		if err != nil {
			return nil, nil, err
		}
		result, err := r.opts.DatabaseClient.Get(ctx, resultID)
		if err == nil {
			var saved struct {
				Payload map[string]any `json:"payload"`
			}
			if err := result.As(&saved); err != nil {
				return nil, nil, err
			}
			target.DelegationResult = saved.Payload
		} else if !errors.Is(err, &database.ErrNotFound{}) {
			return nil, nil, err
		}
	}
	if target.DelegationExecutionStarted && target.DelegationResult == nil && target.ProvisioningState() != v1.ProvisioningStateFailed {
		return nil, nil, fmt.Errorf("the original delegation worker has no terminal failure; automatic replay is unsafe while it may still publish")
	}
	if target.DesiredState == nil || target.DelegationTargetID != "" {
		return nil, nil, fmt.Errorf("the delegation target requires saved desired inputs and cannot itself delegate")
	}
	return object, target, nil
}

func desiredMetadata(resource *datamodel.DynamicResource, incoming recipes.IncomingReference) recipes.ResourceMetadata {
	private := *resource
	private.Properties = incoming.Properties
	return recipes.ResourceMetadata{ResourceID: resource.ID, Name: private.GetRecipe().Name,
		EnvironmentID: private.ResourceMetadata().EnvironmentID(), ApplicationID: private.ResourceMetadata().ApplicationID()}
}

func (r *delegationRuntime) loadMembers(ctx context.Context, target *datamodel.DynamicResource, own recipes.IncomingReference,
	definition *recipes.EnvironmentDefinition, handler *encryption.SensitiveDataHandler, exclude string,
) ([]delegationMember, error) {
	id, err := resources.ParseResource(target.ID)
	if err != nil {
		return nil, err
	}
	members := []delegationMember{}
	seen := map[string]bool{}
	sourceTypes := slices.Clone(target.DelegationResourceTypes)
	sourceTypes = append(sourceTypes, definition.AcceptsDelegationFrom...)
	queried := map[string]bool{}
	for _, sourceType := range sourceTypes {
		if queried[strings.ToLower(sourceType)] {
			continue
		}
		queried[strings.ToLower(sourceType)] = true
		token := ""
		for {
			result, err := r.opts.DatabaseClient.Query(ctx, database.Query{RootScope: id.RootScope(), ResourceType: sourceType,
				Filters: []database.QueryFilter{{Field: "delegationTargetId", Value: target.ID}}}, database.WithPaginationToken(token))
			if err != nil {
				return nil, err
			}
			for i := range result.Items {
				object := &result.Items[i]
				var member datamodel.DynamicResource
				if err := object.As(&member); err != nil {
					return nil, err
				}
				if strings.EqualFold(member.ID, exclude) || seen[strings.ToLower(member.ID)] {
					continue
				}
				seen[strings.ToLower(member.ID)] = true
				if !strings.EqualFold(member.DelegationTargetID, target.ID) || !strings.EqualFold(member.TenantID, target.TenantID) {
					return nil, fmt.Errorf("stored delegate binding does not match target ownership")
				}
				incoming, sourceSchema, err := member.DesiredState.Materialize(ctx, handler)
				if err != nil {
					return nil, err
				}
				if !strings.EqualFold(incoming.ID, member.ID) || !strings.EqualFold(incoming.Type, member.Type) || incoming.APIVersion != member.UpdatedAPIVersion {
					return nil, fmt.Errorf("stored delegate snapshot does not match its logical resource identity")
				}
				metadata := desiredMetadata(&member, incoming)
				sourceDefinition, err := r.loader.LoadRecipe(ctx, &metadata)
				if err != nil {
					return nil, err
				}
				private := member
				private.Properties = incoming.Properties
				if sourceDefinition.Driver != "delegated" || len(sourceDefinition.AcceptsDelegationFrom) > 0 || len(private.GetRecipe().Parameters) > 0 {
					return nil, fmt.Errorf("stored delegate must use an effective delegated registration without source parameters")
				}
				selected, err := datamodel.SelectDelegationTarget(incoming.Properties, sourceSchema, sourceDefinition.DelegatesTo)
				if err != nil || !strings.EqualFold(selected.String(), target.ID) {
					return nil, fmt.Errorf("stored delegate inputs must select their saved target")
				}
				if err := recipes.ValidateDelegationBinding(metadata, desiredMetadata(target, own), definition, member.DelegationTargetID); err != nil {
					return nil, err
				}
				members = append(members, delegationMember{object: object, resource: member, incoming: incoming, schema: sourceSchema})
			}
			token = result.PaginationToken
			if token == "" {
				break
			}
		}
	}
	sort.Slice(members, func(i, j int) bool {
		return strings.ToLower(members[i].resource.ID) < strings.ToLower(members[j].resource.ID)
	})
	return members, nil
}

func (r *delegationRuntime) persistSuppliedMembers(ctx context.Context, target *datamodel.DynamicResource) error {
	if target.DelegatingDesiredStates == nil {
		return nil
	}
	for _, member := range *target.DelegatingDesiredStates {
		if !strings.EqualFold(member.DelegationTargetID, target.ID) || member.DesiredState == nil || len(member.Properties) != 0 {
			return fmt.Errorf("group admission contains an unprotected or incorrectly bound member")
		}
		object, previous, err := readDynamicResource(ctx, r.opts.DatabaseClient, member.ID)
		options := []database.SaveOptions{database.WithCreateOnly()}
		if err == nil {
			if !strings.EqualFold(previous.DelegationTargetID, target.ID) || !strings.EqualFold(previous.TenantID, target.TenantID) {
				return fmt.Errorf("an existing group member has incompatible ownership")
			}
			member.CreatedAPIVersion = previous.CreatedAPIVersion
			options = []database.SaveOptions{database.WithETag(object.ETag)}
		} else if !errors.Is(err, &database.ErrNotFound{ID: member.ID}) {
			return err
		} else {
			member.CreatedAPIVersion = member.UpdatedAPIVersion
		}
		member.SetProvisioningState(v1.ProvisioningStateUpdating)
		update := &database.Object{ID: member.ID, Data: &member}
		if err := r.opts.DatabaseClient.Save(ctx, update, options...); err != nil {
			return err
		}
	}
	return nil
}

func (r *delegationRuntime) put(ctx context.Context, request *ctrl.Request, source *datamodel.DynamicResource, exclude string) (ctrl.Result, error) {
	object, target, err := r.target(ctx, request, source)
	if err != nil {
		return ctrl.Result{}, err
	}
	// A failed-source retry observes the admitted native operation, not the new HTTP operation.
	originalRequest := *request
	originalRequest.OperationID, err = uuid.Parse(target.DelegationOperationID)
	if err != nil {
		return ctrl.Result{}, err
	}
	request = &originalRequest
	handler, err := r.handler(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	own, _, err := target.DesiredState.Materialize(ctx, handler)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !strings.EqualFold(own.ID, target.ID) || !strings.EqualFold(own.Type, target.Type) || own.APIVersion != target.UpdatedAPIVersion {
		return ctrl.Result{}, fmt.Errorf("the target snapshot does not match its resource identity and API version")
	}
	metadata := desiredMetadata(target, own)
	deleting, err := r.applicationDeleting(ctx, metadata.ApplicationID)
	if err != nil {
		return ctrl.Result{}, err
	}
	if deleting || target.DelegationDeleting || target.ProvisioningState() == v1.ProvisioningStateDeleting {
		if exclude == "" {
			return ctrl.Result{}, fmt.Errorf("cannot provision while the application or target is deleting")
		}
		target.DelegationDeleting = true
		delete(target.Status(), "delegatedResources")
		return r.removeSource(ctx, source, target, object)
	}
	definition, err := r.loader.LoadRecipe(ctx, &metadata)
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := r.persistSuppliedMembers(ctx, target); err != nil {
		return ctrl.Result{}, err
	}
	members, err := r.loadMembers(ctx, target, own, definition, handler, exclude)
	if err != nil {
		return ctrl.Result{}, err
	}
	for i := range members {
		member := &members[i]
		member.resource.SetProvisioningState(v1.ProvisioningStateUpdating)
		// Saved desired inputs survive failure, but old success values do not masquerade as new outputs.
		member.resource.Properties = map[string]any{}
		member.object.Data = &member.resource
		if err := r.opts.DatabaseClient.Save(ctx, member.object, database.WithETag(member.object.ETag)); err != nil {
			return ctrl.Result{}, err
		}
	}
	recoverExecution := target.DelegationExecutionStarted
	target.SetProvisioningState(v1.ProvisioningStateUpdating)
	target.DelegationExecutionStarted = true
	object.Data = target
	if err := r.opts.DatabaseClient.Save(ctx, object, database.WithETag(object.ETag)); err != nil {
		return ctrl.Result{}, err
	}

	wrapper := &delegationEngine{Engine: r.engine, members: members, executionID: target.DelegationOperationID, recoverExecution: recoverExecution}
	if target.DelegationResult != nil {
		output, err := restoreDelegationResult(ctx, target, handler)
		if err != nil {
			return ctrl.Result{}, err
		}
		wrapper.restoredOutput = output
	}
	wrapper.checkpoint = func(output *recipes.RecipeOutput) error {
		if output == nil {
			return nil
		}
		encoded, err := json.Marshal(map[string]any{"output": output, "providerResourceIDs": output.ProviderResourceIDs})
		if err != nil {
			return err
		}
		var result map[string]any
		if err := json.Unmarshal(encoded, &result); err != nil {
			return err
		}
		envelope := map[string]any{"value": result}
		if err := handler.EncryptSensitiveFields(envelope, []string{"value"}, delegationResultContext(target)); err != nil {
			return err
		}
		_, current, err := readDynamicResource(ctx, r.opts.DatabaseClient, target.ID)
		if err != nil {
			return err
		}
		if current.DelegationOperationID != request.OperationID.String() {
			return fmt.Errorf("the target changed before native result recovery could be saved")
		}
		resultID, err := delegationResultID(current)
		if err != nil {
			return err
		}
		protected, ok := envelope["value"].(map[string]any)
		if !ok {
			return fmt.Errorf("native result protection returned an invalid envelope")
		}
		// Keep the checkpoint separate from the target ETag held by shared orchestration.
		if err := r.opts.DatabaseClient.Save(ctx, &database.Object{ID: resultID, Data: map[string]any{"payload": protected}}, database.WithCreateOnly()); err != nil {
			if errors.Is(err, &database.ErrConcurrency{}) && wrapper.restoredOutput != nil {
				return nil
			}
			return err
		}
		return nil
	}
	controller, err := recipecontroller.NewCreateOrUpdateResource(r.opts, &delegationTargetProcessor{DynamicProcessor: &processor.DynamicProcessor{SecretMaterializer: r.secret}}, wrapper, r.loader)
	if err != nil {
		return ctrl.Result{}, err
	}
	targetRequest := *request
	targetRequest.ResourceID, targetRequest.APIVersion = target.ID, target.UpdatedAPIVersion
	result, runErr := controller.Run(ctx, &targetRequest)
	if runErr != nil || result.ProvisioningState() != v1.ProvisioningStateSucceeded {
		if err := r.recordFailure(ctx, target.ID, members, wrapper.output, wrapper.executed && !wrapper.preNativeFailure); err != nil {
			return ctrl.Result{}, err
		}
		if runErr != nil {
			return ctrl.NewFailedResult(v1.ErrorDetails{Code: v1.CodeInvalidRequestContent, Message: runErr.Error()}), nil
		}
		return result, runErr
	}
	if !wrapper.executed || wrapper.output == nil {
		return ctrl.Result{}, fmt.Errorf("the accepting target did not execute a recipe with complete member inputs")
	}
	executionTargetID := target.ID
	publicationFailure := func(cause error) (ctrl.Result, error) {
		if err := r.recordFailure(ctx, executionTargetID, members, wrapper.output, true); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.NewFailedResult(v1.ErrorDetails{Code: v1.CodeInvalidRequestContent, Message: cause.Error()}), nil
	}
	for i := range members {
		member := &members[i]
		values := wrapper.values[strings.ToLower(member.resource.ID)]
		member.resource.Properties = maps.Clone(member.incoming.Properties)
		if err := (&processor.DynamicProcessor{SecretMaterializer: r.secret}).Process(ctx, &member.resource, processors.Options{
			RecipeOutput: &recipes.RecipeOutput{Values: values}, UcpClient: r.opts.UcpClient,
		}); err != nil {
			return publicationFailure(err)
		}
		schema.RedactFields(member.resource.Properties, schema.ExtractSensitiveFieldPaths(member.schema, ""))
		member.resource.SetProvisioningState(v1.ProvisioningStateUpdating)
		member.object.Data = &member.resource
		if err := r.opts.DatabaseClient.Save(ctx, member.object, database.WithETag(member.object.ETag)); err != nil {
			return publicationFailure(err)
		}
	}
	// Publish success only after every logical projection is validated and persisted.
	for i := range members {
		member := &members[i]
		member.resource.SetProvisioningState(v1.ProvisioningStateSucceeded)
		member.object.Data = &member.resource
		if err := r.opts.DatabaseClient.Save(ctx, member.object, database.WithETag(member.object.ETag)); err != nil {
			return publicationFailure(err)
		}
	}
	object, target, err = readDynamicResource(ctx, r.opts.DatabaseClient, target.ID)
	if err != nil {
		return publicationFailure(err)
	}
	logical := make([]map[string]any, 0, len(members))
	for _, member := range members {
		logical = append(logical, map[string]any{"id": member.resource.ID, "type": member.resource.Type, "apiVersion": member.resource.UpdatedAPIVersion})
	}
	target.Status()["delegatedResources"] = logical
	target.DelegatingDesiredStates = nil
	target.DelegationOperationID, target.DelegationExecutionStarted = "", false
	target.DelegationResult = nil
	target.SetProvisioningState(v1.ProvisioningStateSucceeded)
	object.Data = target
	if exclude != "" {
		result, err := r.removeSource(ctx, source, target, object)
		if err != nil {
			return publicationFailure(err)
		}
		if err := r.removeDelegationResult(ctx, target.ID, request.OperationID.String()); err != nil {
			return ctrl.Result{}, err
		}
		return result, nil
	}
	if err := r.opts.DatabaseClient.Save(ctx, object, database.WithETag(object.ETag)); err != nil {
		return publicationFailure(err)
	}
	if err := r.removeDelegationResult(ctx, target.ID, request.OperationID.String()); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// delegationEngine adds only the related input set and projection validation; shared orchestration
// continues to own the native drivers, inventory, configuration and target processor.
type delegationEngine struct {
	engine.Engine
	members          []delegationMember
	output           *recipes.RecipeOutput
	values           map[string]map[string]any
	executed         bool
	restoredOutput   *recipes.RecipeOutput
	checkpoint       func(*recipes.RecipeOutput) error
	executionID      string
	recoverExecution bool
	preNativeFailure bool
}

type delegationTargetProcessor struct {
	*processor.DynamicProcessor
}

func (p *delegationTargetProcessor) Process(ctx context.Context, resource *datamodel.DynamicResource, options processors.Options) error {
	if err := p.DynamicProcessor.Process(ctx, resource, options); err != nil {
		return err
	}
	if len(options.RecipeOutput.Resources) == 0 {
		delete(resource.Status(), "outputResources")
	}
	return nil
}

func (e *delegationEngine) Execute(ctx context.Context, options engine.ExecuteOptions) (*recipes.RecipeOutput, error) {
	if e.executed {
		return nil, fmt.Errorf("the target recipe can execute only once per admitted operation")
	}
	e.executed = true
	options.Recipe.ExecutionID = e.executionID
	options.Recipe.RecoverExecution = e.recoverExecution
	options.Recipe.IncomingReferences = make([]recipes.IncomingReference, 0, len(e.members))
	for _, member := range e.members {
		options.Recipe.IncomingReferences = append(options.Recipe.IncomingReferences, member.incoming)
	}
	output := e.restoredOutput
	var err error
	if output == nil {
		output, err = e.Engine.Execute(ctx, options)
	}
	if recipeErr, ok := errors.AsType[*recipes.RecipeError](err); ok && !e.recoverExecution && recipeErr.DeploymentStatus == recipesutil.RecipeSetupError {
		e.preNativeFailure = true
	}
	e.output = output
	if e.checkpoint != nil {
		if checkpointErr := e.checkpoint(output); checkpointErr != nil {
			return output, checkpointErr
		}
	}
	if err != nil {
		return output, err
	}
	if output == nil {
		return nil, fmt.Errorf("the target recipe returned no grouped result")
	}
	if err := output.ValidateDelegatedOutputs(options.Recipe.IncomingReferences); err != nil {
		return output, err
	}
	e.values = map[string]map[string]any{}
	for _, member := range e.members {
		for _, projection := range output.DelegatedOutputs {
			if !strings.EqualFold(projection.ID, member.resource.ID) {
				continue
			}
			if err := validateProjection(ctx, member, projection.Values); err != nil {
				return output, err
			}
			e.values[strings.ToLower(member.resource.ID)] = projection.Values
		}
	}
	return output, nil
}

func validateProjection(ctx context.Context, member delegationMember, values map[string]any) error {
	fields, _ := member.schema["properties"].(map[string]any)
	for key := range values {
		switch key {
		case "application", "environment", "recipe", "status", "provisioningState", "secrets":
			return fmt.Errorf("delegated outputs cannot overwrite framework-owned properties")
		}
		if _, declared := fields[key]; !declared {
			return fmt.Errorf("delegated outputs contain a property not declared by the source schema")
		}
		field, _ := fields[key].(map[string]any)
		if readOnly, _ := field["readOnly"].(bool); !readOnly {
			return fmt.Errorf("delegated outputs must be declared read-only computed properties")
		}
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return fmt.Errorf("delegated outputs must be JSON values")
	}
	var redacted map[string]any
	if err := json.Unmarshal(encoded, &redacted); err != nil {
		return err
	}
	var original map[string]any
	if err := json.Unmarshal(encoded, &original); err != nil {
		return err
	}
	schema.RedactFields(redacted, schema.ExtractSensitiveFieldPaths(member.schema, ""))
	if !reflect.DeepEqual(original, redacted) {
		return fmt.Errorf("delegated outputs cannot publish sensitive source fields as computed values")
	}
	properties := maps.Clone(member.incoming.Properties)
	maps.Copy(properties, values)
	if err := schema.ValidateResourceRequestAgainstSchema(ctx, map[string]any{"properties": properties}, member.schema); err != nil {
		return fmt.Errorf("delegated outputs do not satisfy their source schema")
	}
	return nil
}

func (r *delegationRuntime) recordFailure(ctx context.Context, targetID string, members []delegationMember, output *recipes.RecipeOutput, executed bool) error {
	object, target, err := readDynamicResource(ctx, r.opts.DatabaseClient, targetID)
	if err != nil {
		return err
	}
	operationID := target.DelegationOperationID
	if output != nil {
		inventory, err := processors.GetOutputResourcesFromRecipe(output)
		if err != nil {
			return err
		}
		delete(target.Status(), "outputResources")
		if err := target.ApplyDeploymentOutput(rpv1.DeploymentOutput{DeployedOutputResources: inventory}); err != nil {
			return err
		}

	}
	target.SetProvisioningState(v1.ProvisioningStateFailed)
	delete(target.Status(), "delegatedResources")
	object.Data = target
	if err := r.opts.DatabaseClient.Save(ctx, object, database.WithETag(object.ETag)); err != nil {
		return err
	}
	for _, member := range members {
		object, current, err := readDynamicResource(ctx, r.opts.DatabaseClient, member.resource.ID)
		if err != nil {
			return err
		}
		current.SetProvisioningState(v1.ProvisioningStateFailed)
		current.Properties = map[string]any{}
		object.Data = current
		if err := r.opts.DatabaseClient.Save(ctx, object, database.WithETag(object.ETag)); err != nil {
			return err
		}
	}
	if output != nil || !executed {
		object, target, err := readDynamicResource(ctx, r.opts.DatabaseClient, targetID)
		if err != nil {
			return err
		}
		if target.DelegationOperationID != operationID {
			return fmt.Errorf("the target operation changed during failure publication")
		}
		// A returned result is recoverable inventory, not a successful logical projection.
		target.DelegationOperationID, target.DelegationExecutionStarted = "", false
		target.DelegationResult = nil
		object.Data = target
		return r.opts.DatabaseClient.Save(ctx, object, database.WithETag(object.ETag))
	}
	return nil
}

func (r *delegationRuntime) applicationDeleting(ctx context.Context, applicationID string) (bool, error) {
	if applicationID == "" {
		return false, nil
	}
	object, err := r.opts.DatabaseClient.Get(ctx, applicationID)
	if errors.Is(err, &database.ErrNotFound{ID: applicationID}) {
		// A removed application must never be recreated indirectly by a bound member.
		return true, nil
	}
	if err != nil {
		return false, err
	}
	var application v1.BaseResource
	if err := object.As(&application); err != nil {
		return false, err
	}
	if application.DeletionRequested || application.ProvisioningState() == v1.ProvisioningStateDeleting {
		return true, nil
	}
	if !application.ProvisioningState().IsTerminal() {
		// The shared worker represents both PUT and DELETE as Updating. Without a deletion
		// marker, treating an active application as live could resurrect its infrastructure.
		return false, fmt.Errorf("the application has an active operation; provisioning and cleanup require an unambiguous application deletion state")
	}
	return false, nil
}

func (r *delegationRuntime) removeSource(ctx context.Context, source, target *datamodel.DynamicResource, targetObject *database.Object) (ctrl.Result, error) {
	object, current, err := readDynamicResource(ctx, r.opts.DatabaseClient, source.ID)
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := (&processor.DynamicProcessor{SecretMaterializer: r.secret}).Delete(ctx, current, processors.Options{}); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.opts.DatabaseClient.Delete(ctx, source.ID, database.WithETag(object.ETag)); err != nil {
		return ctrl.Result{}, err
	}
	target.DelegationOperationID, target.DelegationExecutionStarted = "", false
	targetObject.Data = target
	return ctrl.Result{}, r.opts.DatabaseClient.Save(ctx, targetObject, database.WithETag(targetObject.ETag))
}

func (r *delegationRuntime) delete(ctx context.Context, request *ctrl.Request, source *datamodel.DynamicResource) (ctrl.Result, error) {
	if source.DelegationTargetID != "" {
		_, err := r.opts.DatabaseClient.Get(ctx, source.DelegationTargetID)
		if errors.Is(err, &database.ErrNotFound{ID: source.DelegationTargetID}) {
			object, current, err := readDynamicResource(ctx, r.opts.DatabaseClient, source.ID)
			if err != nil {
				return ctrl.Result{}, err
			}
			if err := (&processor.DynamicProcessor{SecretMaterializer: r.secret}).Delete(ctx, current, processors.Options{}); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, r.opts.DatabaseClient.Delete(ctx, source.ID, database.WithETag(object.ETag))
		}
		if err != nil {
			return ctrl.Result{}, err
		}
		return r.put(ctx, request, source, source.ID)
	}
	object, target, err := r.target(ctx, request, source)
	if err != nil {
		return ctrl.Result{}, err
	}
	handler, err := r.handler(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	own, _, err := target.DesiredState.Materialize(ctx, handler)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !strings.EqualFold(own.ID, target.ID) || !strings.EqualFold(own.Type, target.Type) || own.APIVersion != target.UpdatedAPIVersion {
		return ctrl.Result{}, fmt.Errorf("the target snapshot does not match its resource identity and API version")
	}
	metadata := desiredMetadata(target, own)
	definition, err := r.loader.LoadRecipe(ctx, &metadata)
	if err != nil {
		return ctrl.Result{}, err
	}
	members, err := r.loadMembers(ctx, target, own, definition, handler, "")
	if err != nil {
		return ctrl.Result{}, err
	}
	deleting, err := r.applicationDeleting(ctx, metadata.ApplicationID)
	if err != nil {
		return ctrl.Result{}, err
	}
	if len(members) > 0 && !deleting {
		target.DelegationOperationID = ""
		target.DelegationDeleting = false
		object.Data = target
		if err := r.opts.DatabaseClient.Save(ctx, object, database.WithETag(object.ETag)); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.NewFailedResult(v1.ErrorDetails{Code: v1.CodeInvalidRequestContent,
			Message: "delete the live delegating resources before deleting their target"}), nil
	}
	target.DelegationExecutionStarted = true
	target.SetProvisioningState(v1.ProvisioningStateDeleting)
	object.Data = target
	if err := r.opts.DatabaseClient.Save(ctx, object, database.WithETag(object.ETag)); err != nil {
		return ctrl.Result{}, err
	}
	// Application cleanup deletes logical members without ever invoking their delegated drivers.
	for _, member := range members {
		if err := (&processor.DynamicProcessor{SecretMaterializer: r.secret}).Delete(ctx, &member.resource, processors.Options{}); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.opts.DatabaseClient.Delete(ctx, member.resource.ID, database.WithETag(member.object.ETag)); err != nil {
			return ctrl.Result{}, err
		}
	}
	controller, err := recipecontroller.NewDeleteResource(r.opts, &processor.DynamicProcessor{SecretMaterializer: r.secret}, r.engine, r.loader)
	if err != nil {
		return ctrl.Result{}, err
	}
	return controller.Run(ctx, request)
}
