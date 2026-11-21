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

package handlers

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// iamPrefixes maps the AWS SDK service package a handler's client uses to the service's prefix in IAM actions.
var iamPrefixes = map[string]string{
	"appsync":                  "appsync",
	"cloudwatch":               "cloudwatch",
	"codebuild":                "codebuild",
	"connect":                  "connect",
	"databasemigrationservice": "dms",
	"dax":                      "dax",
	"docdb":                    "rds",
	"ec2":                      "ec2",
	"elasticloadbalancingv2":   "elasticloadbalancing",
	"emr":                      "elasticmapreduce",
	"fsx":                      "fsx",
	"iam":                      "iam",
	"iot":                      "iot",
	"kinesisanalyticsv2":       "kinesisanalytics",
	"lakeformation":            "lakeformation",
	"medialive":                "medialive",
	"s3":                       "s3",
	"s3control":                "s3",
	"s3tables":                 "s3tables",
	"sagemaker":                "sagemaker",
	"servicecatalog":           "servicecatalog",
	"sns":                      "sns",
	"sts":                      "sts",
	"transfer":                 "transfer",
}

// operationsOf maps a handler method to the schema handler operations whose permissions must cover the AWS calls it
// makes. Find runs when a PUT or a DELETE looks for a lost create, and Progress while UCP waits for a create, an
// update, or a delete.
var operationsOf = map[string][]string{
	"Create":     {"create"},
	"Find":       {"create", "delete"},
	"Read":       {"read"},
	"ReadScoped": {"read"},
	// UCP resolves an identifier on a PUT and on `existing`.
	"ResolveIdentifier": {"create", "read", "update"},
	"Update":            {"update"},
	"Delete":            {"delete"},
	"DeleteScoped":      {"delete"},
	"DeleteToken":       {"delete"},
	"DeleteWithToken":   {"delete"},
	"Reset":             {"delete"},
	"List":              {"list"},
	"Progress":          {"create", "update", "delete"},
}

// Test_SchemaPermissionsCoverAWSCalls checks that every AWS call a native handler makes is in its schema's handler
// permissions, for each operation that can make it. It reads the handler's source: a method's calls are the methods
// of the package's AWS SDK client interfaces it, or a function or method of the package it calls, refers to, directly
// or through an SDK paginator.
func Test_SchemaPermissionsCoverAWSCalls(t *testing.T) {
	registry, err := NewRegistry(aws.Config{})
	require.NoError(t, err)
	packages := map[string]handlerSource{}
	missing := []string{}
	for _, h := range All(aws.Config{}) {
		typ := reflect.TypeOf(h).Elem()
		src, ok := packages[typ.PkgPath()]
		if !ok {
			src = parseHandlerSource(t, filepath.Base(typ.PkgPath()))
			packages[typ.PkgPath()] = src
		}

		var schema struct {
			Handlers map[string]struct {
				Permissions []string `json:"permissions"`
			} `json:"handlers"`
		}
		require.NoError(t, json.Unmarshal([]byte(registry.Lookup(h.TypeName()).Schema), &schema), h.TypeName())
		for method, ops := range operationsOf {
			for _, call := range src.Calls(typ.Name(), method) {
				action := src.iam[call] + ":" + call
				for _, op := range ops {
					gap := h.TypeName() + " " + op + ": " + action
					if !slices.Contains(schema.Handlers[op].Permissions, action) && !slices.Contains(missing, gap) {
						missing = append(missing, gap)
					}
				}
			}
		}
	}
	slices.Sort(missing)
	require.Empty(t, missing, "add each AWS call to the handler permissions of the operation that makes it")
}

// Test_ResendDeletePermissionsCoverCreate checks that the delete permissions of a handler whose Recovery is
// RecoverByResend cover its create: a DELETE of a record whose create may have reached AWS sends the create again to
// find the resource. They must list every AWS call Create makes, and every create permission, which also covers what
// the call does, such as tagging the resource or iam:PassRole. The resend has no write-only values, so a permission in
// writeOnlyOnly, for a call Create makes only when the write-only property is set, isn't needed.
func Test_ResendDeletePermissionsCoverCreate(t *testing.T) {
	writeOnlyOnly := map[string]map[string]string{
		"AWS::FSx::FileSystem": {"fsx:CreateFileSystemFromBackup": "/properties/BackupId"},
	}
	registry, err := NewRegistry(aws.Config{})
	require.NoError(t, err)
	missing := []string{}
	for _, h := range All(aws.Config{}) {
		if h.Options().Recovery != native.RecoverByResend {
			continue
		}
		typ := reflect.TypeOf(h).Elem()
		src := parseHandlerSource(t, filepath.Base(typ.PkgPath()))
		var schema struct {
			Handlers map[string]struct {
				Permissions []string `json:"permissions"`
			} `json:"handlers"`
			WriteOnlyProperties []string `json:"writeOnlyProperties"`
		}
		require.NoError(t, json.Unmarshal([]byte(registry.Lookup(h.TypeName()).Schema), &schema), h.TypeName())
		required := slices.Clone(schema.Handlers["create"].Permissions)
		for _, call := range src.Calls(typ.Name(), "Create") {
			required = append(required, src.iam[call]+":"+call)
		}
		for _, action := range required {
			if property, ok := writeOnlyOnly[h.TypeName()][action]; ok && slices.Contains(schema.WriteOnlyProperties, property) {
				continue
			}
			gap := h.TypeName() + " delete: " + action
			if !slices.Contains(schema.Handlers["delete"].Permissions, action) && !slices.Contains(missing, gap) {
				missing = append(missing, gap)
			}
		}
	}
	slices.Sort(missing)
	require.Empty(t, missing, "add each permission of the create to the delete permissions of a handler that recovers by resending the create")
}

// Test_SchemaPermissionsAreUsed checks the other way: every permission of the handler's own service in a schema is
// for a call the handler makes for that operation, or for Read, which UCP calls during every operation, so a schema
// never asks for more than the handler needs. AWS also checks some permissions for what a call does, such as tagging
// a resource while creating it or taking a final backup while deleting it, and those are listed in implied.
func Test_SchemaPermissionsAreUsed(t *testing.T) {
	implied := map[string][]string{
		"create": {
			"dms:AddTagsToResource", "rds:AddTagsToResource", "fsx:TagResource", "dax:TagResource", "elasticmapreduce:AddTags",
			// Tags sent in the create call itself: TagSpecifications, or Tags.
			"ec2:CreateTags", "medialive:CreateTags", "sagemaker:AddTags",
		},
		"delete": {"rds:CreateDBClusterSnapshot", "fsx:CreateBackup", "fsx:TagResource"},
	}
	// PutTableReplication also needs these on the source and destination tables, as its API reference says.
	tableReplication := []string{
		"s3tables:GetTable", "s3tables:CreateTable", "s3tables:CreateNamespace",
		"s3tables:GetTableMaintenanceConfiguration", "s3tables:PutTableMaintenanceConfiguration",
	}
	implied["create"] = append(implied["create"], tableReplication...)
	implied["update"] = append(implied["update"], tableReplication...)
	registry, err := NewRegistry(aws.Config{})
	require.NoError(t, err)
	unused := []string{}
	for _, h := range All(aws.Config{}) {
		typ := reflect.TypeOf(h).Elem()
		src := parseHandlerSource(t, filepath.Base(typ.PkgPath()))
		reads := []string{}
		for _, call := range src.Calls(typ.Name(), "Read") {
			reads = append(reads, src.iam[call]+":"+call)
		}
		used := map[string][]string{}
		for method, ops := range operationsOf {
			for _, call := range src.Calls(typ.Name(), method) {
				for _, op := range ops {
					used[op] = append(used[op], src.iam[call]+":"+call)
				}
			}
		}
		if h.Options().Recovery == native.RecoverByResend {
			// A DELETE that finds a lost create sends the create again (see Test_ResendDeletePermissionsCoverCreate).
			used["delete"] = slices.Concat(used["delete"], used["create"], implied["create"])
		}
		// The services the handler's clients call. Permissions of other services, such as iam:PassRole, are for
		// what a call does, and aren't checked.
		services := map[string]bool{}
		for _, prefix := range src.iam {
			services[prefix] = true
		}
		var schema struct {
			Handlers map[string]struct {
				Permissions []string `json:"permissions"`
			} `json:"handlers"`
		}
		require.NoError(t, json.Unmarshal([]byte(registry.Lookup(h.TypeName()).Schema), &schema), h.TypeName())
		for op, handler := range schema.Handlers {
			for _, action := range handler.Permissions {
				service, _, _ := strings.Cut(action, ":")
				if services[service] && !slices.Contains(used[op], action) && !slices.Contains(reads, action) && !slices.Contains(implied[op], action) {
					unused = append(unused, h.TypeName()+" "+op+": "+action)
				}
			}
		}
	}
	slices.Sort(unused)
	require.Empty(t, unused, "remove each permission the handler doesn't need for the operation")
}

// handlerSource is a handler package's parsed source, with iam mapping each method of the package's AWS SDK client
// interfaces to the IAM prefix of its service.
type handlerSource struct {
	*nativetest.HandlerSource
	iam map[string]string
}

func parseHandlerSource(t *testing.T, dir string) handlerSource {
	src := handlerSource{HandlerSource: nativetest.ParseHandlerSource(t, dir), iam: map[string]string{}}
	require.NotEmpty(t, src.API, "the handler package %s has no AWS SDK client interface", dir)
	for method, service := range src.API {
		prefix, ok := iamPrefixes[service]
		require.True(t, ok, "add the IAM prefix of the AWS SDK package %s to iamPrefixes", service)
		src.iam[method] = prefix
	}
	return src
}
