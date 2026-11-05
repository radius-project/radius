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

package nativetest

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/aws/schemas"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
)

// maxPolls is how many times Lifecycle reads an operation's progress before it fails the test.
const maxPolls = 360

// Lifecycle is a create, read, optional update, and delete of one resource through a real handler, against AWS's
// recorded responses (see awsreplay).
type Lifecycle struct {
	// Handler is the handler under test, using Session's AWS config.
	Handler native.Handler
	Session *awsreplay.Session

	// Properties are the template to create, and Want what Read must return for it, such as Properties without
	// write-only values. Want defaults to Properties.
	Properties map[string]any
	Want       map[string]any

	// Update, when set, is the template to update to, and WantUpdated what Read must return after it.
	Update      map[string]any
	WantUpdated map[string]any

	// Poll is how long to wait between reads of an operation's progress when recording. It defaults to 15 seconds.
	Poll time.Duration

	// Between, when set, runs after the create and before the update, such as the lifecycle of a resource that needs
	// this one.
	Between func()

	// SkipDelete leaves the resource in AWS, for a type whose metadata says delete does nothing in AWS. The recording
	// then deletes the resource itself.
	SkipDelete bool
}

// Run creates the resource, waits for it, and checks Read; updates it when Update is set; then deletes it and waits
// until AWS no longer has it. It returns the resource's identifier.
func (l Lifecycle) Run(t *testing.T) string {
	t.Helper()
	ctx := t.Context()
	region := awsreplay.Region
	h := l.Handler
	want := l.Want
	if want == nil {
		want = l.Properties
	}

	// A new client token for every run, so a recording never gets a resource an earlier recording made. A replay
	// doesn't compare request bodies, so the token can differ from the one recorded.
	identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: l.Properties, ClientToken: uuid.NewString()})
	require.NoError(t, err)
	if identifier == "" {
		encoder, ok := h.(native.IdentifierEncoder)
		require.True(t, ok, "Create returned no identifier, and the handler has no identifier encoding")
		identifier, err = encoder.Identifier(l.Properties)
		require.NoError(t, err)
	}
	l.wait(t, ctx, identifier, native.OperationCreate)
	current := l.read(t, ctx, identifier)
	RequireSubset(t, want, current)
	if l.Between != nil {
		l.Between()
	}

	if l.Update != nil {
		patch, err := patchOf(current, l.Update)
		require.NoError(t, err)
		require.NoError(t, h.Update(ctx, native.UpdateRequest{Region: region, Identifier: identifier, Current: current, Desired: l.Update, Patch: patch}))
		l.wait(t, ctx, identifier, native.OperationUpdate)
		wantUpdated := l.WantUpdated
		if wantUpdated == nil {
			wantUpdated = l.Update
		}
		RequireSubset(t, wantUpdated, l.read(t, ctx, identifier))
	}

	if l.SkipDelete {
		return identifier
	}
	switch handler := h.(type) {
	case native.DeletePreparer:
		token, tokenErr := handler.DeleteToken(ctx, region, identifier)
		require.NoError(t, tokenErr)
		err = handler.DeleteWithToken(ctx, region, identifier, token)
	default:
		err = h.Delete(ctx, region, identifier)
	}
	require.NoError(t, err)
	l.wait(t, ctx, identifier, native.OperationDelete)
	return identifier
}

func (l Lifecycle) read(t *testing.T, ctx context.Context, identifier string) map[string]any {
	t.Helper()
	properties, err := l.Handler.Read(ctx, awsreplay.Region, identifier)
	require.NoError(t, err)
	catalog, err := schemas.Load()
	require.NoError(t, err)
	if schema, ok := catalog.Native[l.Handler.TypeName()]; ok {
		requireSchemaTypes(t, schema, properties, "Read")
	}
	return properties
}

// wait reads the operation's progress until it finishes, or for a delete, until AWS no longer has the resource.
func (l Lifecycle) wait(t *testing.T, ctx context.Context, identifier string, op native.Operation) {
	t.Helper()
	poll := l.Poll
	if poll == 0 {
		poll = 15 * time.Second
	}
	for range maxPolls {
		progress, err := l.Handler.Progress(ctx, awsreplay.Region, identifier, op)
		if op == native.OperationDelete && errors.Is(err, native.ErrNotFound) {
			return
		}
		require.NoError(t, err)
		if progress.Done {
			require.Nil(t, progress.Failure, "the %s failed", op)
			return
		}
		l.Session.Wait(poll)
	}
	t.Fatalf("the %s of %s did not finish", op, identifier)
}

// patchOf returns the patch from current to desired for the properties desired sets.
func patchOf(current map[string]any, desired map[string]any) (jsondiff.Patch, error) {
	relevant := map[string]any{}
	for k := range desired {
		if v, ok := current[k]; ok {
			relevant[k] = v
		}
	}
	a, err := json.Marshal(relevant)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(desired)
	if err != nil {
		return nil, err
	}
	return jsondiff.CompareJSON(a, b)
}

// RequireSubset requires got to have every property of want, with the same value once both are read as JSON.
func RequireSubset(t *testing.T, want map[string]any, got map[string]any) {
	t.Helper()
	requireSubset(t, want, got)
}
