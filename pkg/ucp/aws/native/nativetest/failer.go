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
	"sync"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

// Failer makes a fake AWS client fail one call that changes AWS, for Suite.FailCall. A fake calls Before before each
// such call and After once the call took effect, and returns their error when it isn't nil.
type Failer struct {
	mu    sync.Mutex
	armed bool
	at    int
	after bool
	err   error
	count int
}

// FailOn makes the call-th call that changes AWS fail with err, counting from 0, after the calls before it took
// effect. When after is true, the call takes effect first, like a call whose response is lost, as Suite.FailCall
// asks for when call is Suite.Calls.
func (f *Failer) FailOn(call int, err error, after bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.armed, f.at, f.after, f.err, f.count = true, call, after, err, 0
}

// FailCall returns a Suite.FailCall for a real handler whose fake AWS client has the Failer failer returns. Call
// Suite.Calls, a call after the last change, fails the last call after it took effect.
func FailCall(calls int, failer func(handler native.Handler) *Failer) func(handler native.Handler, call int, err error) {
	return func(handler native.Handler, call int, err error) {
		if call >= calls {
			failer(handler).FailOn(calls-1, err, true)
			return
		}
		failer(handler).FailOn(call, err, false)
	}
}

// Before returns the error for the next call that changes AWS, before it takes effect.
func (f *Failer) Before() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.armed && !f.after && f.count == f.at {
		f.armed = false
		return f.err
	}
	return nil
}

// After returns the error for the call that just took effect, and counts it.
func (f *Failer) After() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	defer func() { f.count++ }()
	if f.armed && f.after && f.count == f.at {
		f.armed = false
		return f.err
	}
	return nil
}
