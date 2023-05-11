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

package hosting

import (
	"context"
	"fmt"
	"sync"
)

// noCopy may be embedded into structs which must not be copied
// after the first use.
//
// See https://golang.org/issues/8005#issuecomment-190753527
// for details.
type noCopy struct{} //nolint:golint,unused

// Lock is a no-op used by -copylocks checker from `go vet`.
func (*noCopy) Lock()   {} //nolint:golint,unused
func (*noCopy) Unlock() {} //nolint:golint,unused

type AsyncValue[T any] struct {
	noCopy noCopy //nolint

	Cond  *sync.Cond
	Value *T
	Err   error
}

type result[T any] struct {
	Value *T
	Err   error
}

func NewAsyncValue[T any]() *AsyncValue[T] {
	return &AsyncValue[T]{Cond: &sync.Cond{L: &sync.Mutex{}}}
}

func (a *AsyncValue[T]) Get(ctx context.Context) (*T, error) {

	initialized := make(chan result[T], 1)
	go func() {
		a.Cond.L.Lock()

		defer func() {
			a.Cond.L.Unlock()
		}()

		for {
			if a.Value != nil || a.Err != nil {
				break
			}

			// Not ready to proceed, wait to be woken up
			a.Cond.Wait()
		}

		initialized <- result[T]{Value: a.Value, Err: a.Err}
		close(initialized)
	}()

	select {
	case <-ctx.Done():
		close(initialized)
		return nil, fmt.Errorf("failed to retrieve value: %w", ctx.Err())

	case result := <-initialized:
		if result.Err != nil {
			return nil, result.Err
		}
		return result.Value, nil
	}
}

func (a *AsyncValue[T]) Put(value *T) {
	a.Cond.L.Lock()
	a.Value = value
	a.Cond.L.Unlock()
	a.Cond.Broadcast()
}

func (a *AsyncValue[T]) PutErr(err error) {
	a.Cond.L.Lock()
	a.Err = err
	a.Cond.L.Unlock()
	a.Cond.Broadcast()
}
