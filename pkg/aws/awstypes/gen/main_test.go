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

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/require"
)

// Test_GeneratedFileIsCurrent fails when zz_generated_readonly.go does not match what the generator renders from the
// snapshot. Run `go generate ./pkg/aws/awstypes` to fix it.
func Test_GeneratedFileIsCurrent(t *testing.T) {
	f, err := os.Open(filepath.Join("..", defaultSnapshot))
	require.NoError(t, err)
	defer f.Close()
	entries, err := parseSnapshot(f)
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	want, err := render(readOnlyTypes(entries))
	require.NoError(t, err)
	got, err := os.ReadFile(filepath.Join("..", defaultOut))
	require.NoError(t, err)
	require.Equal(t, string(want), string(got), "run `go generate ./pkg/aws/awstypes`")
}

func Test_ParseSnapshot(t *testing.T) {
	entries, err := parseSnapshot(strings.NewReader("# comment\n\nAWS::A::ReadOnly list,read\nAWS::A::None none\nAWS::A::Legacy create,delete,update\n"))
	require.NoError(t, err)
	require.Equal(t, []entry{
		{TypeName: "AWS::A::ReadOnly", Handlers: []string{"list", "read"}},
		{TypeName: "AWS::A::None"},
		{TypeName: "AWS::A::Legacy", Handlers: []string{"create", "delete", "update"}},
	}, entries)

	_, err = parseSnapshot(strings.NewReader("AWS::A::Missing\n"))
	require.ErrorContains(t, err, "line 1")
}

func Test_ReadOnlyTypes(t *testing.T) {
	entries := []entry{
		{TypeName: "AWS::B::ReadOnly", Handlers: []string{"list", "read"}},
		{TypeName: "AWS::A::ReadOnly", Handlers: []string{"read"}},
		{TypeName: "AWS::A::Creatable", Handlers: []string{"create", "read"}},
		{TypeName: "AWS::A::None"},
	}
	require.Equal(t, []string{"AWS::A::ReadOnly", "AWS::B::ReadOnly"}, readOnlyTypes(entries))
}

// fakeRegistry is a CloudFormation registry with two pages of NON_PROVISIONABLE types.
type fakeRegistry struct {
	// schemas maps a type name to its schema. A type with no schema is not found.
	schemas map[string]string
	// throttles is how many times DescribeType fails for each type before it succeeds.
	throttles int
	// listErr is returned by ListTypes.
	listErr error

	calls     map[string]int
	listInput *cloudformation.ListTypesInput
}

func (f *fakeRegistry) ListTypes(_ context.Context, params *cloudformation.ListTypesInput, _ ...func(*cloudformation.Options)) (*cloudformation.ListTypesOutput, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	f.listInput = params
	if params.NextToken == nil {
		return &cloudformation.ListTypesOutput{
			TypeSummaries: []types.TypeSummary{{TypeName: aws.String("AWS::B::ReadOnly")}, {TypeName: aws.String("AWS::A::Legacy")}},
			NextToken:     aws.String("page-2"),
		}, nil
	}
	return &cloudformation.ListTypesOutput{TypeSummaries: []types.TypeSummary{{TypeName: aws.String("AWS::A::None")}}}, nil
}

func (f *fakeRegistry) DescribeType(_ context.Context, params *cloudformation.DescribeTypeInput, _ ...func(*cloudformation.Options)) (*cloudformation.DescribeTypeOutput, error) {
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	name := aws.ToString(params.TypeName)
	f.calls[name]++
	if f.calls[name] <= f.throttles {
		return nil, errors.New("Throttling: Rate exceeded")
	}
	schema, ok := f.schemas[name]
	if !ok {
		return nil, &types.TypeNotFoundException{Message: aws.String(name)}
	}
	return &cloudformation.DescribeTypeOutput{Schema: aws.String(schema)}, nil
}

func newFakeRegistry() *fakeRegistry {
	return &fakeRegistry{schemas: map[string]string{
		"AWS::B::ReadOnly": `{"handlers": {"read": {}, "list": {}}}`,
		"AWS::A::Legacy":   `{"handlers": {"create": {}, "update": {}, "delete": {}}}`,
		"AWS::A::None":     `{}`,
	}}
}

// noSleep replaces the retry delay for the duration of the test.
func noSleep(t *testing.T) *[]time.Duration {
	slept := []time.Duration{}
	original := sleep
	sleep = func(d time.Duration) { slept = append(slept, d) }
	t.Cleanup(func() { sleep = original })
	return &slept
}

// Test_Run_Refresh runs the generator with -refresh against a fake registry and checks the snapshot and the output.
func Test_Run_Refresh(t *testing.T) {
	noSleep(t)
	t.Setenv("AWS_REGION", "eu-west-1")
	dir := t.TempDir()
	snapshot := filepath.Join(dir, "snapshot.txt")
	out := filepath.Join(dir, "out.go")
	registry := newFakeRegistry()

	var region string
	newClient := func(_ context.Context, r string) (registryClient, error) {
		region = r
		return registry, nil
	}
	var stdout bytes.Buffer
	require.NoError(t, run(t.Context(), []string{"-refresh", "-snapshot", snapshot, "-out", out}, &stdout, newClient))

	require.Equal(t, "eu-west-1", region)
	require.Equal(t, types.ProvisioningTypeNonProvisionable, registry.listInput.ProvisioningType)
	require.Equal(t, types.VisibilityPublic, registry.listInput.Visibility)
	require.Equal(t, types.CategoryAwsTypes, registry.listInput.Filters.Category)

	b, err := os.ReadFile(snapshot)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	require.Contains(t, lines[0], "(eu-west-1, ")
	require.Equal(t, []string{
		"AWS::A::Legacy create,delete,update",
		"AWS::A::None none",
		"AWS::B::ReadOnly list,read",
	}, lines[2:])

	src, err := os.ReadFile(out)
	require.NoError(t, err)
	want, err := render([]string{"AWS::B::ReadOnly"})
	require.NoError(t, err)
	require.Equal(t, string(want), string(src))
	require.Equal(t, "wrote 1 read-only types to "+out+"\n", stdout.String())
}

func Test_Run_DefaultRegion(t *testing.T) {
	noSleep(t)
	t.Setenv("AWS_REGION", "")
	dir := t.TempDir()
	var region string
	newClient := func(_ context.Context, r string) (registryClient, error) {
		region = r
		return newFakeRegistry(), nil
	}
	require.NoError(t, run(t.Context(), []string{"-refresh", "-snapshot", filepath.Join(dir, "s.txt"), "-out", filepath.Join(dir, "o.go")}, &bytes.Buffer{}, newClient))
	require.Equal(t, "us-west-2", region)
}

// Test_Run_WithoutRefresh checks that the generator renders from the snapshot without creating an AWS client.
func Test_Run_WithoutRefresh(t *testing.T) {
	dir := t.TempDir()
	snapshot := filepath.Join(dir, "snapshot.txt")
	out := filepath.Join(dir, "out.go")
	require.NoError(t, os.WriteFile(snapshot, []byte("AWS::A::ReadOnly read\nAWS::A::None none\n"), 0o644))

	newClient := func(context.Context, string) (registryClient, error) {
		t.Fatal("the generator must not call AWS without -refresh")
		return nil, nil
	}
	var stdout bytes.Buffer
	require.NoError(t, run(t.Context(), []string{"-snapshot", snapshot, "-out", out}, &stdout, newClient))
	require.Equal(t, "wrote 1 read-only types to "+out+"\n", stdout.String())
}

func Test_Run_Errors(t *testing.T) {
	noSleep(t)
	dir := t.TempDir()
	valid := filepath.Join(dir, "valid.txt")
	require.NoError(t, os.WriteFile(valid, []byte("AWS::A::ReadOnly read\n"), 0o644))
	invalid := filepath.Join(dir, "invalid.txt")
	require.NoError(t, os.WriteFile(invalid, []byte("AWS::A::Missing\n"), 0o644))
	clientErr := errors.New("no credentials")
	listErr := errors.New("list failed")

	tests := []struct {
		name      string
		args      []string
		newClient func(context.Context, string) (registryClient, error)
		want      string
	}{
		{name: "unknown flag", args: []string{"-unknown"}, want: "flag provided but not defined"},
		{name: "missing snapshot", args: []string{"-snapshot", filepath.Join(dir, "missing.txt"), "-out", filepath.Join(dir, "o.go")}, want: "no such file"},
		{name: "invalid snapshot", args: []string{"-snapshot", invalid, "-out", filepath.Join(dir, "o.go")}, want: "invalid.txt: line 1"},
		{name: "unwritable output", args: []string{"-snapshot", valid, "-out", filepath.Join(dir, "missing-dir", "o.go")}, want: "no such file"},
		{
			name:      "client error",
			args:      []string{"-refresh", "-snapshot", filepath.Join(dir, "s1.txt")},
			newClient: func(context.Context, string) (registryClient, error) { return nil, clientErr },
			want:      "no credentials",
		},
		{
			name: "list error",
			args: []string{"-refresh", "-snapshot", filepath.Join(dir, "s2.txt")},
			newClient: func(context.Context, string) (registryClient, error) {
				return &fakeRegistry{listErr: listErr}, nil
			},
			want: "list failed",
		},
		{
			name: "describe error",
			args: []string{"-refresh", "-snapshot", filepath.Join(dir, "s3.txt")},
			newClient: func(context.Context, string) (registryClient, error) {
				r := newFakeRegistry()
				delete(r.schemas, "AWS::A::None")
				return r, nil
			},
			want: "AWS::A::None: ",
		},
		{
			name: "invalid schema",
			args: []string{"-refresh", "-snapshot", filepath.Join(dir, "s4.txt")},
			newClient: func(context.Context, string) (registryClient, error) {
				r := newFakeRegistry()
				r.schemas["AWS::A::None"] = "not json"
				return r, nil
			},
			want: "AWS::A::None: invalid character",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newClient := tt.newClient
			if newClient == nil {
				newClient = func(context.Context, string) (registryClient, error) { return newFakeRegistry(), nil }
			}
			err := run(t.Context(), tt.args, &bytes.Buffer{}, newClient)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

// Test_DescribeHandlers_Retries checks that DescribeType is retried with a growing delay, and that it gives up after
// maxDescribeAttempts.
func Test_DescribeHandlers_Retries(t *testing.T) {
	t.Run("succeeds after throttling", func(t *testing.T) {
		slept := noSleep(t)
		registry := newFakeRegistry()
		registry.throttles = 2
		handlers, err := describeHandlers(t.Context(), registry, "AWS::B::ReadOnly")
		require.NoError(t, err)
		require.Equal(t, []string{"list", "read"}, handlers)
		require.Equal(t, 3, registry.calls["AWS::B::ReadOnly"])
		require.Equal(t, []time.Duration{time.Second, 2 * time.Second}, *slept)
	})

	t.Run("gives up", func(t *testing.T) {
		slept := noSleep(t)
		registry := newFakeRegistry()
		registry.throttles = maxDescribeAttempts
		_, err := describeHandlers(t.Context(), registry, "AWS::B::ReadOnly")
		require.ErrorContains(t, err, "Throttling")
		require.Equal(t, maxDescribeAttempts, registry.calls["AWS::B::ReadOnly"])
		require.Len(t, *slept, maxDescribeAttempts-1)
	})
}
