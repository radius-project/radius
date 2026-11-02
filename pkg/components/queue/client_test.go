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

package queue

import (
	"bytes"
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"
)

const (
	// defaultTestDequeueInterval is the default duration for the dequeue interval for inmemory test queue
	defaultTestDequeueInterval = time.Duration(5) * time.Millisecond
)

func TestStartDequeuer(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mockCli := NewMockClient(mctrl)

	lastDequeueCh := make(chan struct{})

	firstCall := mockCli.EXPECT().Dequeue(gomock.Any(), gomock.Any()).Return(&Message{
		ID:            "testID",
		DequeueCount:  1,
		EnqueueAt:     time.Now(),
		ExpireAt:      time.Now().Add(10 * time.Hour),
		NextVisibleAt: time.Now().Add(5 * time.Minute),
		ContentType:   JSONContentType,
		Data:          []byte("{}"),
	}, nil)

	secondCall := mockCli.EXPECT().Dequeue(gomock.Any(), gomock.Any()).Return(nil, ErrInvalidMessage).After(firstCall.Call)
	mockCli.EXPECT().Dequeue(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, cfg QueueClientConfig) (*Message, error) {
			close(lastDequeueCh)
			return nil, ErrInvalidMessage
		}).AnyTimes().After(secondCall)

	ctx, cancel := context.WithCancel(t.Context())
	msgCh, err := StartDequeuer(ctx, mockCli, WithDequeueInterval(defaultTestDequeueInterval))
	require.NoError(t, err)

	recvCnt := 0
	doneCh := make(chan struct{})

	// Consumer
	go func() {
		for msg := range msgCh {
			t.Logf("Dequeued Message ID: %s", msg.ID)
			recvCnt++
		}
		close(doneCh)
	}()

	<-lastDequeueCh
	cancel()
	<-doneCh

	require.Equal(t, 1, recvCnt)
}

func TestStartDequeuerPollsAgainAfterMessage(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mockCli := NewMockClient(mctrl)
	message := func(id string) *Message {
		return &Message{ID: id, ContentType: JSONContentType, Data: []byte("{}")}
	}
	emptyCh := make(chan struct{})
	first := mockCli.EXPECT().Dequeue(gomock.Any(), gomock.Any()).Return(message("first"), nil)
	second := mockCli.EXPECT().Dequeue(gomock.Any(), gomock.Any()).Return(message("second"), nil).After(first.Call)
	mockCli.EXPECT().Dequeue(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, cfg QueueClientConfig) (*Message, error) {
			close(emptyCh)
			return nil, ErrMessageNotFound
		}).After(second)

	// Only an empty queue waits for the interval, so both messages arrive long before it ends.
	ctx, cancel := context.WithCancel(t.Context())
	msgCh, err := StartDequeuer(ctx, mockCli, WithDequeueInterval(time.Hour))
	require.NoError(t, err)

	ids := []string{}
	for range 2 {
		select {
		case msg := <-msgCh:
			ids = append(ids, msg.ID)
		case <-time.After(10 * time.Second):
			require.FailNow(t, "the dequeuer waited for the interval after dequeuing a message")
		}
	}
	<-emptyCh
	cancel()
	_, open := <-msgCh
	require.False(t, open)
	require.Equal(t, []string{"first", "second"}, ids)
}

func TestStartDequeuerStopsWhileTheConsumerIsBusy(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mockCli := NewMockClient(mctrl)
	dequeued := make(chan struct{}, 10)
	mockCli.EXPECT().Dequeue(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, cfg QueueClientConfig) (*Message, error) {
			dequeued <- struct{}{}
			return &Message{ID: "busy", ContentType: JSONContentType, Data: []byte("{}")}, nil
		}).AnyTimes()

	require.Eventually(t, func() bool { return !dequeuerRunning() }, 10*time.Second, 10*time.Millisecond)
	ctx, cancel := context.WithCancel(t.Context())
	msgCh, err := StartDequeuer(ctx, mockCli, WithDequeueInterval(defaultTestDequeueInterval))
	require.NoError(t, err)

	// Nothing reads: the first message fills the buffer and the second send blocks.
	<-dequeued
	<-dequeued
	cancel()

	// Reading would release a blocked send, so the producer must stop while msgCh stays unread.
	require.Eventually(t, func() bool { return !dequeuerRunning() }, 10*time.Second, 10*time.Millisecond,
		"the dequeuer did not stop while its consumer was busy")
	for range msgCh {
	}
}

// dequeuerRunning reports whether a StartDequeuer producer goroutine is still running.
func dequeuerRunning() bool {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	return bytes.Contains(buf[:n], []byte("queue.StartDequeuer.func1"))
}
