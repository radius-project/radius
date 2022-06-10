// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package inmemory

import (
	"context"
	"testing"

	"github.com/project-radius/radius/pkg/queue"
	"github.com/stretchr/testify/require"
)

func TestNamedQueue(t *testing.T) {
	cli1 := NewNamedQueue("queue1")
	cli2 := NewNamedQueue("queue2")

	cli1.Enqueue(context.Background(), &queue.Message{Data: "test1"})
	cli2.Enqueue(context.Background(), &queue.Message{Data: "test2"})

	require.Equal(t, 1, cli1.queue.Len())
	require.Equal(t, 1, cli2.queue.Len())

	cli3 := NewNamedQueue("queue1")
	require.Equal(t, 1, cli3.queue.Len())
	require.Equal(t, "test1", cli3.queue.Dequeue().Data.(string))
}

func TestClient(t *testing.T) {
	cli := New(NewInMemQueue(messageLockDuration))

	ctx, cancel := context.WithCancel(context.Background())
	msgCh, err := cli.Dequeue(ctx)
	require.NoError(t, err)

	recvCnt := 0
	done := make(chan struct{})

	msgCount := 10

	// Consumer
	go func(msgCh <-chan *queue.Message) {
		for msg := range msgCh {
			require.Equal(t, 1, msg.DequeueCount)
			require.Equal(t, "test", msg.Data)
			recvCnt++

			if recvCnt == msgCount {
				done <- struct{}{}
			}
		}
	}(msgCh)

	// Producer
	for i := 0; i < msgCount; i++ {
		err := cli.Enqueue(ctx, &queue.Message{Data: "test"})
		require.NoError(t, err)
	}

	<-done
	cancel()

	require.Equal(t, msgCount, recvCnt)
}

func TestMessageFinish(t *testing.T) {
	cli := New(NewInMemQueue(messageLockDuration))

	ctx, cancel := context.WithCancel(context.Background())
	msgCh, err := cli.Dequeue(ctx)
	require.NoError(t, err)

	err = cli.Enqueue(ctx, &queue.Message{Data: "test1"})
	require.NoError(t, err)
	err = cli.Enqueue(ctx, &queue.Message{Data: "test2"})
	require.NoError(t, err)
	recv := <-msgCh
	require.Equal(t, "test1", recv.Data)

	// Ensure that the first element of queue is test1
	first := cli.queue.v.Front().Value.(*element)
	require.Equal(t, "test1", first.val.Data)

	// Finish message
	err = recv.Finish(nil)
	require.NoError(t, err)

	// Ensure that the first element of queue is test2 because we finish message.
	first = cli.queue.v.Front().Value.(*element)
	require.Equal(t, "test2", first.val.Data)

	cancel()
}

func TestExtendMessageLock(t *testing.T) {
	cli := New(NewInMemQueue(messageLockDuration))

	ctx, cancel := context.WithCancel(context.Background())
	msgCh, _ := cli.Dequeue(ctx)

	err := cli.Enqueue(ctx, &queue.Message{Data: "test1"})
	require.NoError(t, err)

	recv := <-msgCh
	require.Equal(t, "test1", recv.Data)

	old := recv.NextVisibleAt

	// Extend message lock
	err = recv.Extend()
	require.NoError(t, err)

	first := cli.queue.v.Front().Value.(*element)

	require.Greater(t, first.val.NextVisibleAt.UnixNano(), old.UnixNano())

	cancel()
}
