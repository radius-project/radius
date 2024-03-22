package shared

import (
	"sync/atomic"
	"testing"
)

type TestRetryCounter struct {
	*testing.T

	failed atomic.Int32
}

func NewTestRetryCounter(maxRetry int32) *TestRetryCounter {
	counter := &TestRetryCounter{}
	counter.failed.Store(maxRetry)
	return counter
}

func (t *TestRetryCounter) SetT(tt *testing.T) {
	t.T = tt
}

func (t *TestRetryCounter) Fail() {
	if t.failed.Load() > 0 {
		t.FailNow()
	} else {
		t.T.Fail()
	}
}

func (t *TestRetryCounter) FailNow() {
	if t.failed.Load() > 0 {
		t.failed.Add(-1)
		panic("retry")
	} else {
		t.T.FailNow()
	}
}
