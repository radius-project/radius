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

package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/crypto/encryption"
	"github.com/radius-project/radius/pkg/portableresources/datamodel"
	"github.com/radius-project/radius/pkg/portableresources/processors"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	"github.com/radius-project/radius/pkg/recipes/engine"
	"github.com/radius-project/radius/pkg/recipes/util"
	"github.com/radius-project/radius/pkg/resourceutil"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	schemautil "github.com/radius-project/radius/pkg/schema"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// secretReferenceResourceType is the resource type of a Radius.Security/secrets resource named by an
// x-radius-secret-reference property.
const secretReferenceResourceType = "Radius.Security/secrets"

// CreateOrUpdateResource is the async operation controller to create or update portable resources.
type CreateOrUpdateResource[P interface {
	*T
	rpv1.RadiusResourceModel
}, T any] struct {
	ctrl.BaseController
	processor           processors.ResourceProcessor[P, T]
	engine              engine.Engine
	configurationLoader configloader.ConfigurationLoader
}

// NewCreateOrUpdateResource creates a new controller for creating or updating a resource with the given processor, engine,
// client, configurationLoader and options. The processor function will be called to process updates to the resource.
func NewCreateOrUpdateResource[P interface {
	*T
	rpv1.RadiusResourceModel
}, T any](opts ctrl.Options, processor processors.ResourceProcessor[P, T], eng engine.Engine, configurationLoader configloader.ConfigurationLoader) (ctrl.Controller, error) {
	return &CreateOrUpdateResource[P, T]{
		ctrl.NewBaseAsyncController(opts),
		processor,
		eng,
		configurationLoader,
	}, nil
}

// Run retrieves an existing resource, executes a recipe if needed, loads runtime configuration,
// processes the resource, cleans up any obsolete output resources, and saves the updated resource.
func (c *CreateOrUpdateResource[P, T]) Run(ctx context.Context, req *ctrl.Request) (ctrl.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	storedResource, err := c.DatabaseClient().Get(ctx, req.ResourceID)
	if errors.Is(&database.ErrNotFound{ID: req.ResourceID}, err) {
		return ctrl.Result{}, err
	} else if err != nil {
		return ctrl.Result{}, err
	}

	resource := P(new(T))
	if err = storedResource.As(resource); err != nil {
		return ctrl.Result{}, err
	}

	currentETag := storedResource.ETag
	var recipeProperties map[string]any
	var secretReferencePaths []string
	var secretBindingPaths []string
	redactionCompleted := false

	// Attempt to decrypt and redact sensitive fields before recipe execution.
	apiVersion := getResourceAPIVersion(resource)
	if apiVersion != "" {
		resourceType := resource.GetBaseResource().Type
		schema, schemaErr := schemautil.GetSchema(ctx, c.UcpClient(), req.ResourceID, resourceType, apiVersion)
		if schemaErr != nil {
			return ctrl.Result{}, fmt.Errorf("failed to fetch schema for sensitive field detection: %w", schemaErr)
		} else if schema != nil {
			// Detect properties that reference a Radius.Security/secrets resource by name so the engine can
			// resolve them into recipe context (context.resource.secrets.<key>).
			secretReferencePaths = schemautil.ExtractSecretReferenceFieldPaths(schema, "")

			// Detect secrets arrays (x-radius-secret-binding) so the engine can load each listed
			// secret's keys into recipe context (context.resource.secrets.<secretName>.<key>).
			secretBindingPaths = schemautil.ExtractSecretBindingPaths(schema, "")

			sensitiveFieldPaths := schemautil.ExtractSensitiveFieldPaths(schema, "")
			retainFieldPaths := schemautil.ExtractRetainFieldPaths(schema, "")
			if len(sensitiveFieldPaths) > 0 {
				logger.Info("Sensitive fields detected for resource", "resourceID", req.ResourceID, "paths", sensitiveFieldPaths, "retain", retainFieldPaths)

				properties, err := resourceutil.GetPropertiesFromResource(resource)
				if err != nil {
					return ctrl.Result{}, err
				}

				recipeProperties, err = deepCopyProperties(properties)
				if err != nil {
					return ctrl.Result{}, err
				}

				if c.KubeClient() == nil {
					err = fmt.Errorf("kubernetes client not configured for sensitive data decryption")
					logger.Error(err, "Failed to initialize encryption key provider", "resourceID", req.ResourceID)
					return ctrl.NewFailedResult(v1.ErrorDetails{Message: err.Error()}), err
				}

				keyProvider := encryption.NewKubernetesKeyProvider(c.KubeClient(), nil)
				handler, err := encryption.NewSensitiveDataHandlerFromProvider(ctx, keyProvider)
				if err != nil {
					logger.Error(err, "Failed to initialize sensitive data handler", "resourceID", req.ResourceID)
					return ctrl.NewFailedResult(v1.ErrorDetails{Message: err.Error()}), err
				}

				if err = handler.DecryptSensitiveFieldsWithSchema(ctx, recipeProperties, sensitiveFieldPaths, req.ResourceID, schema); err != nil {
					logger.Error(err, "Failed to decrypt sensitive fields", "resourceID", req.ResourceID)
					return ctrl.NewFailedResult(v1.ErrorDetails{Message: err.Error()}), err
				}

				// Derive the redacted copy from the original encrypted properties,
				// NOT from the decrypted recipeProperties. recipeProperties
				// (decrypted) is kept exclusively for in-memory recipe execution.
				redactedProperties, err := deepCopyProperties(properties)
				if err != nil {
					return ctrl.Result{}, err
				}
				// Retain-marked fields (e.g. Radius.Security/secrets data values) keep their encrypted
				// value at rest so the secrets loader can decrypt from the store instead of reading a
				// cluster (multi-cluster safe). Only non-retain sensitive fields are redacted to nil here;
				// retain fields are redacted on read (GET/LIST) instead.
				redactFieldPaths := pathsExcept(sensitiveFieldPaths, retainFieldPaths)
				schemautil.RedactFields(redactedProperties, redactFieldPaths)

				if err = applyPropertiesToResource(resource, redactedProperties); err != nil {
					logger.Error(err, "Failed to apply redacted properties", "resourceID", req.ResourceID)
					return ctrl.NewFailedResult(v1.ErrorDetails{Message: err.Error()}), err
				}

				update := &database.Object{
					Metadata: database.Metadata{ID: req.ResourceID},
					Data:     resource,
				}
				if err = c.DatabaseClient().Save(ctx, update, database.WithETag(currentETag)); err != nil {
					logger.Error(err, "Failed to persist redacted resource", "resourceID", req.ResourceID)
					return ctrl.NewFailedResult(v1.ErrorDetails{Message: err.Error()}), err
				}
				currentETag = update.ETag
				redactionCompleted = true
			}
		}
	} else {
		logger.Info("Skipping sensitive field detection due to missing apiVersion", "resourceID", req.ResourceID)
	}

	// Clone existing output resources so we can diff them later.
	previousOutputResources := c.copyOutputResources(resource)

	// Load configuration
	metadata := recipes.ResourceMetadata{EnvironmentID: resource.ResourceMetadata().EnvironmentID(), ApplicationID: resource.ResourceMetadata().ApplicationID(), ResourceID: resource.GetBaseResource().ID}
	config, err := c.configurationLoader.LoadConfiguration(ctx, metadata)
	if err != nil {
		if redactionCompleted {
			return ctrl.NewFailedResult(v1.ErrorDetails{Message: err.Error()}), err
		}
		return ctrl.Result{}, err
	}

	// Now we're ready to process recipes (if needed).
	recipeDataModel, supportsRecipes := any(resource).(datamodel.RecipeDataModel)
	var recipeOutput *recipes.RecipeOutput
	if supportsRecipes && recipeDataModel.GetRecipe() != nil {
		recipeOutput, err = c.executeRecipeIfNeeded(ctx, resource, recipeDataModel, previousOutputResources, config.Simulated, recipeProperties, secretReferencePaths, secretBindingPaths)
		if err != nil {
			return c.handleRecipeError(ctx, err, recipeDataModel, req.ResourceID, currentETag, logger, redactionCompleted)
		}
	}

	if config.Simulated {
		logger.Info("The recipe was executed in simulation mode. No resources were deployed.")
	} else {
		// Now we're ready to process the resource. This will handle the updates to any user-visible state.
		err = c.processor.Process(ctx, resource, processors.Options{RecipeOutput: recipeOutput, RuntimeConfiguration: config.Runtime, UcpClient: c.BaseController.UcpClient()})
		if err != nil {
			if redactionCompleted {
				return ctrl.NewFailedResult(v1.ErrorDetails{Message: err.Error()}), err
			}
			return ctrl.Result{}, err
		}
	}

	if supportsRecipes {
		recipeData := recipeDataModel.GetRecipe()
		if recipeData != nil {
			recipeData.DeploymentStatus = util.Success
			recipeDataModel.SetRecipe(recipeData)
		}
		if recipeOutput != nil && recipeOutput.Status != nil {
			setRecipeStatus(resource, *recipeOutput.Status)
		}
	}

	update := &database.Object{
		Metadata: database.Metadata{
			ID: req.ResourceID,
		},
		Data: recipeDataModel.(rpv1.RadiusResourceModel),
	}
	err = c.DatabaseClient().Save(ctx, update, database.WithETag(currentETag))
	if err != nil {
		if redactionCompleted {
			return ctrl.NewFailedResult(v1.ErrorDetails{Message: err.Error()}), err
		}
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, err
}

// handleRecipeError handles recipe execution errors: logs, updates status, persists, and returns the appropriate result.
// When redactionCompleted is true, all failure paths return NewFailedResult to prevent retries that would fail
// because sensitive data has already been nullified from the database.
func (c *CreateOrUpdateResource[P, T]) handleRecipeError(ctx context.Context, err error, recipeDataModel datamodel.RecipeDataModel, resourceID string, etag string, logger logr.Logger, redactionCompleted bool) (ctrl.Result, error) {
	var recipeErr *recipes.RecipeError
	if errors.As(err, &recipeErr) {
		logger.Error(recipeErr, fmt.Sprintf("failed to execute recipe. Encountered error while processing %s ", recipeErr.ErrorDetails.Target))

		// Set the deployment status to the recipe error code
		recipeDataModel.GetRecipe().DeploymentStatus = util.RecipeDeploymentStatus(recipeErr.DeploymentStatus)
		update := &database.Object{
			Metadata: database.Metadata{ID: resourceID},
			Data:     recipeDataModel.(rpv1.RadiusResourceModel),
		}

		// Save portable resource with updated deployment status to track errors during deletion.
		err = c.DatabaseClient().Save(ctx, update, database.WithETag(etag))
		if err != nil {
			if redactionCompleted {
				return ctrl.NewFailedResult(v1.ErrorDetails{Message: err.Error()}), err
			}
			return ctrl.Result{}, err
		}

		return ctrl.NewFailedResult(recipeErr.ErrorDetails), nil
	}

	// For non-RecipeError: if sensitive data was redacted, prevent retry since
	// the data is gone from the database and retry would fail.
	if redactionCompleted {
		return ctrl.NewFailedResult(v1.ErrorDetails{Message: err.Error()}), err
	}

	return ctrl.Result{}, err
}

func (c *CreateOrUpdateResource[P, T]) copyOutputResources(resource P) []string {
	previousOutputResources := []string{}
	for _, outputResource := range resource.OutputResources() {
		previousOutputResources = append(previousOutputResources, outputResource.ID.String())
	}
	return previousOutputResources
}

func (c *CreateOrUpdateResource[P, T]) executeRecipeIfNeeded(ctx context.Context, resource P, recipeDataModel datamodel.RecipeDataModel, prevState []string, simulated bool, recipeProperties map[string]any, secretReferencePaths []string, secretBindingPaths []string) (*recipes.RecipeOutput, error) {
	// Caller ensures recipeDataModel supports recipes and has a non-nil recipe
	recipe := recipeDataModel.GetRecipe()

	resourceProperties := recipeProperties
	if resourceProperties == nil {
		var err error
		resourceProperties, err = resourceutil.GetPropertiesFromResource(resource)
		if err != nil {
			return nil, err
		}
	}

	connectionsAndSourceIDs, err := resourceutil.GetConnectionNameandSourceIDs(resource)
	if err != nil {
		return nil, fmt.Errorf("failed to get connected resource IDs: %w", err)
	}
	connectedResourcesMetadata := make(map[string]recipes.ConnectedResource)

	// If there are connected resources, we need to fetch their properties and add them to the recipe context.
	for connName, connectedResourceID := range connectionsAndSourceIDs {
		connectedResource, err := c.DatabaseClient().Get(ctx, connectedResourceID)
		if errors.Is(&database.ErrNotFound{ID: connectedResourceID}, err) {
			return nil, fmt.Errorf("connected resource %s not found: %w", connectedResourceID, err)
		} else if err != nil {
			return nil, fmt.Errorf("failed to get connected resource %s: %w", connectedResourceID, err)
		}

		connectedResourceMetadata, err := resourceutil.GetAllPropertiesFromResource(connectedResource.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to get metadata from connected resource %s: %w", connectedResourceID, err)
		}

		// Redact sensitive fields from a connected Radius.Security/secrets resource before exposing its
		// properties to the recipe. Its value is retained encrypted at rest (x-radius-retain), and must not
		// be surfaced — even as ciphertext — through the recipe context; secrets are consumed via secret
		// bindings/references, not connections. Other resource types still redact their sensitive fields to
		// nil at rest, so they need no redaction here.
		if strings.EqualFold(connectedResourceMetadata.Type, secretReferenceResourceType) && len(connectedResourceMetadata.Properties) > 0 {
			if apiVersion := connectedResourceAPIVersion(connectedResource.Data); apiVersion != "" {
				connectedSchema, err := schemautil.GetSchema(ctx, c.UcpClient(), connectedResourceID, connectedResourceMetadata.Type, apiVersion)
				if err != nil {
					return nil, fmt.Errorf("failed to fetch schema to redact connected secret %s: %w", connectedResourceID, err)
				}
				if connectedSchema != nil {
					schemautil.RedactFields(connectedResourceMetadata.Properties, schemautil.ExtractSensitiveFieldPaths(connectedSchema, ""))
				}
			}
		}

		connectedResourcesMetadata[connName] = recipes.ConnectedResource{
			ID:         connectedResourceMetadata.ID,
			Name:       connectedResourceMetadata.Name,
			Type:       connectedResourceMetadata.Type,
			Properties: connectedResourceMetadata.Properties,
		}
	}

	metadata := recipes.ResourceMetadata{
		Name:                         recipe.Name,
		Parameters:                   recipe.Parameters,
		EnvironmentID:                resource.ResourceMetadata().EnvironmentID(),
		ApplicationID:                resource.ResourceMetadata().ApplicationID(),
		ResourceID:                   resource.GetBaseResource().ID,
		Properties:                   resourceProperties,
		ConnectedResourcesProperties: connectedResourcesMetadata,
	}

	// Resolve x-radius-secret-reference properties to Radius.Security/secrets resource IDs so the engine
	// can load their secret material into the recipe context.
	secretReferences, err := buildSecretReferences(resource.GetBaseResource().ID, resourceProperties, secretReferencePaths)
	if err != nil {
		return nil, err
	}
	metadata.SecretReferences = secretReferences

	// Resolve the x-radius-secret-binding array property to the list of secret IDs it references so the engine
	// can load each bound secret's keys into the recipe context.
	secretBindings, err := buildSecretBindings(resourceProperties, secretBindingPaths)
	if err != nil {
		return nil, err
	}
	metadata.SecretBindings = secretBindings

	return c.engine.Execute(ctx, engine.ExecuteOptions{
		BaseOptions: engine.BaseOptions{
			Recipe: metadata,
		},
		PreviousState: prevState,
		Simulated:     simulated,
	})
}

func getResourceAPIVersion[P rpv1.RadiusResourceModel](resource P) string {
	return resource.GetBaseResource().InternalMetadata.UpdatedAPIVersion
}

// connectedResourceAPIVersion extracts the api-version a connected resource was last updated with from its
// raw stored representation, so the connected resource's schema can be fetched to redact its sensitive fields.
// Returns "" if the api-version cannot be determined.
func connectedResourceAPIVersion(data any) string {
	var partial struct {
		UpdatedAPIVersion string `json:"updatedApiVersion"`
	}

	bs, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	if err := json.Unmarshal(bs, &partial); err != nil {
		return ""
	}

	return partial.UpdatedAPIVersion
}

// buildSecretReferences resolves x-radius-secret-reference property values (each the name of a
// Radius.Security/secrets resource) to fully qualified resource IDs scoped to the consuming resource's
// resource group. Unset reference properties are skipped; a malformed reference is an error so the
// deployment fails closed rather than passing an unresolved value to the recipe.
func buildSecretReferences(resourceID string, properties map[string]any, secretReferencePaths []string) (map[string]string, error) {
	if len(secretReferencePaths) == 0 {
		return nil, nil
	}

	parsed, err := resources.ParseResource(resourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse resource ID %q for secret reference resolution: %w", resourceID, err)
	}

	references := map[string]string{}
	for _, path := range secretReferencePaths {
		name, ok := stringValueAtPath(properties, path)
		if !ok || name == "" {
			// The reference property is optional and unset; nothing to resolve.
			continue
		}

		secretID := fmt.Sprintf("%s/providers/%s/%s", parsed.RootScope(), secretReferenceResourceType, name)
		if _, err := resources.ParseResource(secretID); err != nil {
			return nil, fmt.Errorf("failed to construct secret resource ID for property %q with value %q: %w", path, name, err)
		}
		references[path] = secretID
	}

	if len(references) == 0 {
		return nil, nil
	}

	return references, nil
}

// stringValueAtPath returns the string value at the given dot-separated path in a nested properties map.
func stringValueAtPath(properties map[string]any, path string) (string, bool) {
	var current any = properties
	for _, segment := range strings.Split(path, ".") {
		m, ok := current.(map[string]any)
		if !ok {
			return "", false
		}
		current, ok = m[segment]
		if !ok {
			return "", false
		}
	}

	value, ok := current.(string)
	return value, ok
}

// buildSecretBindings resolves an x-radius-secret-binding array property into the list of Radius.Security/secrets
// resource IDs it references. Each entry must be a non-empty string that parses as a resource ID; the engine
// loads every key of each secret into the recipe's Secrets under a "<secretName>.<key>" namespace for the
// context.resource.secrets.<secretName>.<key> expression path. An unset property is skipped; a malformed entry
// (non-string, empty, or not a resource ID) is an error so the deployment fails closed rather than silently
// dropping a secret the recipe expects.
func buildSecretBindings(properties map[string]any, secretBindingPaths []string) ([]string, error) {
	if len(secretBindingPaths) == 0 {
		return nil, nil
	}

	var secretIDs []string
	for _, path := range secretBindingPaths {
		raw, ok := valueAtPath(properties, path)
		if !ok || raw == nil {
			// The property is optional and unset; nothing to resolve.
			continue
		}

		entries, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("secret binding property %q must be an array of secret IDs", path)
		}

		for i, entry := range entries {
			id, ok := entry.(string)
			if !ok || id == "" {
				return nil, fmt.Errorf("secret binding %q[%d] must be a non-empty secret ID string", path, i)
			}
			if _, err := resources.ParseResource(id); err != nil {
				return nil, fmt.Errorf("secret binding %q[%d] has invalid secret ID %q: %w", path, i, id, err)
			}
			secretIDs = append(secretIDs, id)
		}
	}

	if len(secretIDs) == 0 {
		return nil, nil
	}

	return secretIDs, nil
}

// valueAtPath returns the value at the given dot-separated path in a nested properties map.
func valueAtPath(properties map[string]any, path string) (any, bool) {
	var current any = properties
	for _, segment := range strings.Split(path, ".") {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = m[segment]
		if !ok {
			return nil, false
		}
	}

	return current, true
}

// pathsExcept returns the elements of all that are not present in exclude. Used to redact only the
// sensitive field paths that are NOT retain-marked (retain fields keep their encrypted value at rest).
func pathsExcept(all []string, exclude []string) []string {
	if len(exclude) == 0 {
		return all
	}
	excluded := make(map[string]struct{}, len(exclude))
	for _, p := range exclude {
		excluded[p] = struct{}{}
	}
	result := make([]string, 0, len(all))
	for _, p := range all {
		if _, found := excluded[p]; !found {
			result = append(result, p)
		}
	}
	return result
}

func deepCopyProperties(source map[string]any) (map[string]any, error) {
	if source == nil {
		return map[string]any{}, nil
	}

	bytes, err := json.Marshal(source)
	if err != nil {
		return nil, err
	}

	var copy map[string]any
	if err = json.Unmarshal(bytes, &copy); err != nil {
		return nil, err
	}

	return copy, nil
}

func applyPropertiesToResource[P rpv1.RadiusResourceModel](resource P, properties map[string]any) error {
	payload := map[string]any{
		"properties": properties,
	}

	bytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	return json.Unmarshal(bytes, resource)
}

// setRecipeStatus sets the recipe status for the given resource model.
// It retrieves the resource metadata from the provided model, deep copies the current resource status,
// updates the Recipe field with the supplied recipeStatus, and then applies the updated status back to the resource.
func setRecipeStatus[P rpv1.RadiusResourceModel](data P, recipeStatus rpv1.RecipeStatus) {
	rm := data.ResourceMetadata()
	status := rm.GetResourceStatus().DeepCopyRecipeStatus()
	status.Recipe = &recipeStatus
	rm.SetResourceStatus(status)
}
