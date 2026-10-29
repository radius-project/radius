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

package awsproxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	awsoperations "github.com/radius-project/radius/pkg/aws/operations"
	"github.com/radius-project/radius/pkg/retry"
	ucpaws "github.com/radius-project/radius/pkg/ucp/aws"
)

type readIdentifierSchema struct {
	PrimaryIdentifier     []string   `json:"primaryIdentifier"`
	AdditionalIdentifiers [][]string `json:"additionalIdentifiers"`
}

func identifierValues(properties map[string]any, identifiers []string) ([]string, error) {
	if len(identifiers) == 0 {
		return nil, fmt.Errorf("identifier set is empty")
	}
	values := make([]string, 0, len(identifiers))
	for _, identifier := range identifiers {
		name, err := awsoperations.ParsePropertyName(identifier)
		if err != nil {
			return nil, err
		}
		var value any = properties
		for part := range strings.SplitSeq(name, "/") {
			part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
			object, ok := value.(map[string]any)
			if !ok {
				return nil, &ucpaws.AWSMissingPropertyError{PropertyName: name}
			}
			value = object[part]
		}
		if value == nil {
			return nil, &ucpaws.AWSMissingPropertyError{PropertyName: name}
		}
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("identifier property %s must be a string", name)
		}
		values = append(values, text)
	}
	return values, nil
}

func (p *GetAWSResourceWithPost) resolveReadIdentifier(ctx context.Context, properties map[string]any, schema, resourceType, region string) (string, *cloudcontrol.GetResourceOutput, armrpc_rest.Response, error) {
	var identifiers readIdentifierSchema
	if err := json.Unmarshal([]byte(schema), &identifiers); err != nil {
		return "", nil, armrpc_rest.NewBadRequestResponse(err.Error()), nil
	}
	primary, primaryErr := identifierValues(properties, identifiers.PrimaryIdentifier)
	if primaryErr == nil {
		return strings.Join(primary, "|"), nil, nil, nil
	}
	var missing *ucpaws.AWSMissingPropertyError
	if !errors.As(primaryErr, &missing) {
		return "", nil, armrpc_rest.NewBadRequestResponse(primaryErr.Error()), nil
	}

	for _, additional := range identifiers.AdditionalIdentifiers {
		values, err := identifierValues(properties, additional)
		if err != nil {
			continue
		}
		secondary := map[string]any{}
		for i, path := range additional {
			parts := strings.Split(strings.TrimPrefix(path, "/properties/"), "/")
			object := secondary
			for j, part := range parts {
				part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
				if j == len(parts)-1 {
					object[part] = values[i]
					break
				}
				if object[part] == nil {
					object[part] = map[string]any{}
				}
				object = object[part].(map[string]any)
			}
		}
		identifier, err := json.Marshal(secondary)
		if err != nil {
			return "", nil, nil, err
		}
		var output *cloudcontrol.GetResourceOutput
		err = p.retryer.RetryFunc(ctx, func(ctx context.Context) error {
			var err error
			output, err = p.awsClients.CloudControl.GetResource(ctx, &cloudcontrol.GetResourceInput{
				TypeName: aws.String(resourceType), Identifier: aws.String(string(identifier)),
			}, CloudControlRegionOption(region))
			if ucpaws.IsAWSResourceNotFoundError(err) {
				return retry.RetryableError(err)
			}
			return err
		})
		var unsupported *types.UnsupportedActionException
		if errors.As(err, &unsupported) {
			// JSON secondary identifiers work for Image, FoundationModel, and VpnConnectionDeviceType
			// in us-west-2. Only explicit unsupported-operation responses enable list fallback;
			// authorization, validation, and not-found errors must not trigger a scan.
			id, response, err := p.findReadIdentifier(ctx, properties, additional, values, resourceType, region)
			return id, nil, response, err
		}
		if ucpaws.IsAWSResourceNotFoundError(err) {
			return "", nil, armrpc_rest.NewNotFoundMessageResponse("No AWS resource matches the additional identifiers."), nil
		}
		if err != nil {
			response, err := ucpaws.HandleAWSError(ctx, err)
			return "", nil, response, err
		}
		if output == nil || output.ResourceDescription == nil || aws.ToString(output.ResourceDescription.Identifier) == "" {
			return "", nil, nil, fmt.Errorf("Cloud Control GetResource returned no primary identifier for %s", resourceType)
		}
		return *output.ResourceDescription.Identifier, output, nil, nil
	}
	return "", nil, armrpc_rest.NewBadRequestResponse(primaryErr.Error()), nil
}

func (p *GetAWSResourceWithPost) findReadIdentifier(ctx context.Context, properties map[string]any, identifiers, values []string, resourceType, region string) (string, armrpc_rest.Response, error) {
	// Parent properties may be required to list a type. ResourceModel is not a portable filter:
	// every returned resource must still be matched locally.
	model, err := json.Marshal(properties)
	if err != nil {
		return "", nil, err
	}
	input := &cloudcontrol.ListResourcesInput{
		TypeName:      aws.String(resourceType),
		ResourceModel: aws.String(string(model)),
	}
	opts := []func(*cloudcontrol.Options){CloudControlRegionOption(region)}
	matches := map[string]struct{}{}
	seenTokens := map[string]struct{}{}
	for {
		page, err := p.awsClients.CloudControl.ListResources(ctx, input, opts...)
		if err != nil {
			response, err := ucpaws.HandleAWSError(ctx, err)
			return "", response, err
		}
		if page == nil {
			return "", nil, fmt.Errorf("Cloud Control ListResources returned no response for %s", resourceType)
		}
		for _, candidate := range page.ResourceDescriptions {
			identifier := aws.ToString(candidate.Identifier)
			if identifier == "" {
				return "", nil, fmt.Errorf("Cloud Control ListResources returned an empty identifier for %s", resourceType)
			}
			candidateProperties := map[string]any{}
			if candidate.Properties != nil {
				if err := json.Unmarshal([]byte(*candidate.Properties), &candidateProperties); err != nil {
					return "", nil, fmt.Errorf("decoding Cloud Control list properties: %w", err)
				}
			}
			candidateValues, err := identifierValues(candidateProperties, identifiers)
			if err != nil {
				// ListResources may return only the primary identifier, without matching properties.
				full, err := p.awsClients.CloudControl.GetResource(ctx, &cloudcontrol.GetResourceInput{
					TypeName: input.TypeName, Identifier: candidate.Identifier,
				}, opts...)
				if ucpaws.IsAWSResourceNotFoundError(err) {
					continue
				}
				if err != nil {
					response, err := ucpaws.HandleAWSError(ctx, err)
					return "", response, err
				}
				if full == nil || full.ResourceDescription == nil || full.ResourceDescription.Properties == nil {
					return "", nil, fmt.Errorf("Cloud Control GetResource returned no properties for %s", resourceType)
				}
				candidateProperties = map[string]any{}
				if err := json.Unmarshal([]byte(*full.ResourceDescription.Properties), &candidateProperties); err != nil {
					return "", nil, fmt.Errorf("decoding Cloud Control read properties: %w", err)
				}
				candidateValues, err = identifierValues(candidateProperties, identifiers)
				if err != nil {
					var missing *ucpaws.AWSMissingPropertyError
					if errors.As(err, &missing) {
						continue
					}
					return "", nil, fmt.Errorf("invalid additional identifiers in Cloud Control read for %s: %w", resourceType, err)
				}
			}
			if !slices.Equal(values, candidateValues) {
				continue
			}
			matches[identifier] = struct{}{}
			if len(matches) > 1 {
				return "", armrpc_rest.NewConflictResponse("More than one AWS resource matches the additional identifiers. Supply the primary identifier."), nil
			}
		}
		if aws.ToString(page.NextToken) == "" {
			break
		}
		if _, exists := seenTokens[*page.NextToken]; exists {
			return "", nil, fmt.Errorf("Cloud Control ListResources returned a repeated pagination token for %s", resourceType)
		}
		seenTokens[*page.NextToken] = struct{}{}
		input.NextToken = page.NextToken
	}
	for identifier := range matches {
		return identifier, nil, nil
	}
	return "", armrpc_rest.NewNotFoundMessageResponse("No AWS resource matches the additional identifiers."), nil
}
