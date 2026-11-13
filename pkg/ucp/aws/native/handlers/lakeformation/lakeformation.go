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

// Package lakeformation has the native handlers for AWS Lake Formation data lake settings, permissions, and registered
// resources. Every Lake Formation call takes effect at once, so the handlers never wait for a state.
package lakeformation

import (
	"context"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lakeformation"
	"github.com/aws/aws-sdk-go-v2/service/lakeformation/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

// notFoundCode is the error code Lake Formation returns for a resource, catalog entry, or setting it doesn't have.
const notFoundCode = "EntityNotFoundException"

// API is the part of the Lake Formation client the handlers use.
type API interface {
	GetDataLakeSettings(ctx context.Context, params *lakeformation.GetDataLakeSettingsInput, optFns ...func(*lakeformation.Options)) (*lakeformation.GetDataLakeSettingsOutput, error)
	PutDataLakeSettings(ctx context.Context, params *lakeformation.PutDataLakeSettingsInput, optFns ...func(*lakeformation.Options)) (*lakeformation.PutDataLakeSettingsOutput, error)
	GrantPermissions(ctx context.Context, params *lakeformation.GrantPermissionsInput, optFns ...func(*lakeformation.Options)) (*lakeformation.GrantPermissionsOutput, error)
	RevokePermissions(ctx context.Context, params *lakeformation.RevokePermissionsInput, optFns ...func(*lakeformation.Options)) (*lakeformation.RevokePermissionsOutput, error)
	ListPermissions(ctx context.Context, params *lakeformation.ListPermissionsInput, optFns ...func(*lakeformation.Options)) (*lakeformation.ListPermissionsOutput, error)
	RegisterResource(ctx context.Context, params *lakeformation.RegisterResourceInput, optFns ...func(*lakeformation.Options)) (*lakeformation.RegisterResourceOutput, error)
	DescribeResource(ctx context.Context, params *lakeformation.DescribeResourceInput, optFns ...func(*lakeformation.Options)) (*lakeformation.DescribeResourceOutput, error)
	UpdateResource(ctx context.Context, params *lakeformation.UpdateResourceInput, optFns ...func(*lakeformation.Options)) (*lakeformation.UpdateResourceOutput, error)
	DeregisterResource(ctx context.Context, params *lakeformation.DeregisterResourceInput, optFns ...func(*lakeformation.Options)) (*lakeformation.DeregisterResourceOutput, error)
	ListResources(ctx context.Context, params *lakeformation.ListResourcesInput, optFns ...func(*lakeformation.Options)) (*lakeformation.ListResourcesOutput, error)
}

var inRegion = awsapi.InRegion[lakeformation.Options]

// principalIDs returns the DataLakePrincipalIdentifier of each principal in the list property name of properties.
func principalIDs(properties map[string]any, name string) []string {
	list, _ := properties[name].([]any)
	out := []string{}
	for _, item := range list {
		m, _ := item.(map[string]any)
		if id, ok := m["DataLakePrincipalIdentifier"].(string); ok && id != "" {
			out = append(out, id)
		}
	}
	return out
}

// principals returns the SDK principals with ids.
func principals(ids []string) []types.DataLakePrincipal {
	out := []types.DataLakePrincipal{}
	for _, id := range ids {
		out = append(out, types.DataLakePrincipal{DataLakePrincipalIdentifier: aws.String(id)})
	}
	return out
}

// sdkPrincipalIDs returns the identifiers of the SDK principals.
func sdkPrincipalIDs(list []types.DataLakePrincipal) []string {
	out := []string{}
	for _, p := range list {
		out = append(out, aws.ToString(p.DataLakePrincipalIdentifier))
	}
	return out
}

// union returns the values of lists, in order, each once.
func union(lists ...[]string) []string {
	out := []string{}
	for _, list := range lists {
		for _, v := range list {
			if !slices.Contains(out, v) {
				out = append(out, v)
			}
		}
	}
	return out
}

// minus returns the values of a that aren't in b, in order.
func minus(a []string, b []string) []string {
	out := []string{}
	for _, v := range a {
		if !slices.Contains(b, v) {
			out = append(out, v)
		}
	}
	return out
}

// only returns the values of a that are in b, in order.
func only(a []string, b []string) []string {
	out := []string{}
	for _, v := range a {
		if slices.Contains(b, v) {
			out = append(out, v)
		}
	}
	return out
}
