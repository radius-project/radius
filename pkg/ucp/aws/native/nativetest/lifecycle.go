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
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
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

// cleanupTimeout bounds the delete a failed recording makes in cleanup, including the wait for AWS to finish it.
const cleanupTimeout = 10 * time.Minute

// Run creates the resource, waits for it, and checks Read; updates it when Update is set; then deletes it and waits
// until AWS no longer has it. It returns the resource's identifier. When recording, a run that fails once the create
// may have made the resource, including a create that failed partway, still deletes it in cleanup, so a failed
// recording leaves nothing running in AWS. A create AWS rejected made nothing, so its identifier, which may name a
// resource that already existed, is never deleted.
func (l Lifecycle) Run(t testing.TB) string {
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
	if identifier == "" && err == nil {
		encoder, ok := h.(native.IdentifierEncoder)
		require.True(t, ok, "Create returned no identifier, and the handler has no identifier encoding")
		identifier, err = encoder.Identifier(l.Properties)
		require.NoError(t, err)
	}
	// deleteToken is the token of the delete Run started, deleting is true once it started, and deleted once AWS no
	// longer has the resource.
	var deleteToken string
	deleting, deleted := false, false
	if l.Session.Recording() && !l.SkipDelete && identifier != "" && mayHaveCreated(err) {
		t.Cleanup(func() {
			if deleted {
				return
			}
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
			defer cancel()
			if err := l.cleanup(cleanupCtx, identifier, deleting, deleteToken); err != nil {
				t.Errorf("failed to delete %s after the recording failed, so delete it in AWS: %v", identifier, err)
				return
			}
			t.Logf("deleted %s after the recording failed", identifier)
		})
	}
	require.NoError(t, err)
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
	deleteToken, err = l.delete(ctx, identifier, "")
	require.NoError(t, err)
	deleting = true
	l.wait(t, ctx, identifier, native.OperationDelete)
	deleted = true
	return identifier
}

// mayHaveCreated reports whether a create that returned err may have made the resource: it succeeded, or failed after
// AWS applied some of it, or failed without saying. A create AWS rejected, by its outcome or as a client fault, made
// nothing.
func mayHaveCreated(err error) bool {
	if err == nil {
		return true
	}
	var failure *native.Failure
	if errors.As(err, &failure) {
		return failure.Outcome != native.OutcomeRejected
	}
	return !ucp_aws.IsClientFault(err)
}

// cleanup deletes the resource after a failed recording and waits until AWS no longer has it, or ctx ends. When Run
// already started a delete, it first waits for that delete, and sends it again, with the same token, only when the
// resource is still there.
func (l Lifecycle) cleanup(ctx context.Context, identifier string, deleting bool, token string) error {
	if deleting {
		err := l.waitDeleted(ctx, identifier)
		if err == nil || ctx.Err() != nil {
			return err
		}
	}
	if _, err := l.delete(ctx, identifier, token); err != nil {
		if errors.Is(err, native.ErrNotFound) {
			return nil
		}
		return err
	}
	return l.waitDeleted(ctx, identifier)
}

// waitDeleted reads the delete's progress until AWS no longer has the resource, or ctx ends. A delete AWS reports as
// done while the resource is still there didn't delete it.
func (l Lifecycle) waitDeleted(ctx context.Context, identifier string) error {
	for ctx.Err() == nil {
		progress, err := l.Handler.Progress(ctx, awsreplay.Region, identifier, native.OperationDelete)
		switch {
		case errors.Is(err, native.ErrNotFound):
			return nil
		case err != nil:
			return err
		case progress.Failure != nil:
			return progress.Failure
		case progress.Done:
			if _, err := l.Handler.Read(ctx, awsreplay.Region, identifier); !errors.Is(err, native.ErrNotFound) {
				return errors.Join(errors.New("AWS still has the resource after the delete finished"), err)
			}
			return nil
		}
		l.Session.Wait(l.poll())
	}
	return ctx.Err()
}

// poll returns how long to wait between reads of an operation's progress.
func (l Lifecycle) poll() time.Duration {
	if l.Poll == 0 {
		return 15 * time.Second
	}
	return l.Poll
}

// delete starts deleting the resource. For a handler that sends a delete token, it sends token, or a new one when
// token is "", and returns the token it sent.
func (l Lifecycle) delete(ctx context.Context, identifier string, token string) (string, error) {
	preparer, ok := l.Handler.(native.DeletePreparer)
	if !ok {
		return "", l.Handler.Delete(ctx, awsreplay.Region, identifier)
	}
	if token == "" {
		var err error
		if token, err = preparer.DeleteToken(ctx, awsreplay.Region, identifier); err != nil {
			return "", err
		}
	}
	return token, preparer.DeleteWithToken(ctx, awsreplay.Region, identifier, token)
}

func (l Lifecycle) read(t testing.TB, ctx context.Context, identifier string) map[string]any {
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
func (l Lifecycle) wait(t testing.TB, ctx context.Context, identifier string, op native.Operation) {
	t.Helper()
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
		l.Session.Wait(l.poll())
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
func RequireSubset(t testing.TB, want map[string]any, got map[string]any) {
	t.Helper()
	requireSubset(t, want, got)
}
