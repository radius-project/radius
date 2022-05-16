// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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

type AsyncValue struct {
	noCopy noCopy //nolint

	Cond  *sync.Cond
	Value interface{}
	Err   error
}

func NewAsyncValue() *AsyncValue {
	return &AsyncValue{Cond: &sync.Cond{L: &sync.Mutex{}}}
}

func (a *AsyncValue) Get(ctx context.Context) (interface{}, error) {
	for {
		a.Cond.L.Lock()
		if a.Value != nil {
			return a.Value, nil
		}

		if a.Err != nil {
			return nil, fmt.Errorf("failed to retrieve value: %w", a.Err)
		}

		if ctx.Err() != nil {
			return nil, fmt.Errorf("failed to retrieve value: %w", ctx.Err())
		}

		a.Cond.Wait()
		a.Cond.L.Unlock()
	}
}

func (a *AsyncValue) Put(value interface{}) {
	a.Cond.L.Lock()
	a.Value = value
	a.Cond.L.Unlock()
	a.Cond.Broadcast()
}

func (a *AsyncValue) PutErr(err error) {
	a.Cond.L.Lock()
	a.Err = err
	a.Cond.L.Unlock()
	a.Cond.Broadcast()
}
