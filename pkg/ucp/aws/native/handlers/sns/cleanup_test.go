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

package sns

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// errFailNow stops a recordingTB's test body, as runtime.Goexit stops a test.
var errFailNow = errors.New("FailNow")

// recordingTB runs a recording's test body without failing the test that runs it, so a test can check what the
// recording's cleanup does after the recording fails.
type recordingTB struct {
	testing.TB
	ctx      context.Context
	cancel   context.CancelFunc
	failed   bool
	cleanups []func()
}

func (f *recordingTB) Helper()                           {}
func (f *recordingTB) Context() context.Context          { return f.ctx }
func (f *recordingTB) Failed() bool                      { return f.failed }
func (f *recordingTB) Cleanup(fn func())                 { f.cleanups = append(f.cleanups, fn) }
func (f *recordingTB) Logf(format string, args ...any)   {}
func (f *recordingTB) Errorf(format string, args ...any) { f.failed = true }
func (f *recordingTB) Fatalf(format string, args ...any) { f.Errorf(format, args...); f.FailNow() }
func (f *recordingTB) FailNow()                          { f.failed = true; panic(errFailNow) }

// run runs fn, then cancels the context and runs the cleanups last first, as the testing package does.
func (f *recordingTB) run(fn func()) {
	func() {
		defer func() {
			if r := recover(); r != nil && r != errFailNow {
				panic(r)
			}
		}()
		fn()
	}()
	f.cancel()
	for _, cleanup := range slices.Backward(f.cleanups) {
		cleanup()
	}
}

// lostCreate is a fake whose CreatePlatformApplication creates the application, then fails with err, as when AWS
// applied the call and lists showed it, but its response was lost.
type lostCreate struct {
	*fakeAPI
	err error
}

func (f lostCreate) CreatePlatformApplication(ctx context.Context, in *sns.CreatePlatformApplicationInput, optFns ...func(*sns.Options)) (*sns.CreatePlatformApplicationOutput, error) {
	if _, err := f.fakeAPI.CreatePlatformApplication(ctx, in, optFns...); err != nil {
		return nil, err
	}
	f.stale = map[string]*stale{}
	return nil, f.err
}

// Test_Lifecycle_CleanupAfterFailedCreate checks that a recording whose create AWS may have applied, but that failed,
// deletes the application in cleanup, and that one AWS rejected leaves an application of the same name alone. Only a
// create that returns the identifier with its failure gets the cleanup.
func Test_Lifecycle_CleanupAfterFailedCreate(t *testing.T) {
	serverFault := &smithy.OperationError{ServiceID: "SNS", OperationName: "CreatePlatformApplication", Err: &smithyhttp.ResponseError{
		Response: &smithyhttp.Response{Response: &http.Response{StatusCode: http.StatusInternalServerError}},
		Err:      &smithy.GenericAPIError{Code: "InternalError", Message: "internal error", Fault: smithy.FaultServer},
	}}
	run := func(t *testing.T, api API, change func(h *PlatformApplication)) *recordingTB {
		t.Setenv(awsreplay.RecordEnv, "1")
		h := NewPlatformApplication(api)
		h.wait = noWait
		if change != nil {
			change(h)
		}
		ctx, cancel := context.WithCancel(t.Context())
		f := &recordingTB{TB: t, ctx: ctx, cancel: cancel}
		l := nativetest.Lifecycle{Handler: h, Session: awsreplay.New(t, filepath.Join(t.TempDir(), "cassette.json")), Properties: application(nil), Poll: time.Millisecond}
		f.run(func() { l.Run(f) })
		require.True(t, f.failed)
		return f
	}

	t.Run("a create whose response was lost", func(t *testing.T) {
		store := awsfake.NewCloudControl()
		run(t, lostCreate{fakeAPI: newFakeAPI(store), err: serverFault}, nil)
		require.Zero(t, store.Count(platformApplicationType), "the cleanup deletes the application AWS created")
	})

	t.Run("a create no read shows", func(t *testing.T) {
		store := awsfake.NewCloudControl()
		api := newFakeAPI(store)
		run(t, api, func(h *PlatformApplication) {
			lagged := false
			h.wait = func(context.Context, time.Duration) {
				// Lists show the application only after the create's last read.
				if s := api.stale[testIdentifier]; !lagged && s != nil {
					s.reads, lagged = confirmReads-1, true
				}
			}
		})
		require.Zero(t, store.Count(platformApplicationType), "the cleanup deletes the application AWS created")
	})

	t.Run("a create AWS rejected leaves an application of the same name alone", func(t *testing.T) {
		store := awsfake.NewCloudControl()
		existing := application(func(p map[string]any) { p["ApplePlatformBundleID"] = "com.example.other" })
		h, _ := newHandler(store)
		_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: existing})
		require.NoError(t, err)
		run(t, newFakeAPI(store), nil)
		require.Equal(t, 1, store.Count(platformApplicationType), "the cleanup doesn't delete an application the create didn't make")
	})
}
