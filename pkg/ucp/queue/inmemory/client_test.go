// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package inmemory

import (
	"context"
	"testing"

	"github.com/project-radius/radius/pkg/ucp/queue/client"
	"github.com/stretchr/testify/require"
)

func TestNamedQueue(t *testing.T) {
	cli1 := NewNamedQueue("queue1")
	cli2 := NewNamedQueue("queue2")

	err := cli1.Enqueue(context.Background(), &client.Message{Data: []byte("test1")})
	require.NoError(t, err)
	err = cli2.Enqueue(context.Background(), &client.Message{Data: []byte("test2")})
	require.NoError(t, err)

	require.Equal(t, 1, cli1.queue.Len())
	require.Equal(t, 1, cli2.queue.Len())

	cli3 := NewNamedQueue("queue1")
	require.Equal(t, 1, cli3.queue.Len())
	require.Equal(t, "test1", string(cli3.queue.Dequeue().Data))
}

func TestDequeue(t *testing.T) {
	ctx := context.Background()
	cli := New(NewInMemQueue(messageLockDuration))
	err := cli.Enqueue(ctx, client.NewMessage("test"))
	require.NoError(t, err)

	msg, err := cli.Dequeue(ctx)
	require.NoError(t, err)
	require.NotNil(t, msg)

	_, err = cli.Dequeue(ctx)
	require.ErrorIs(t, err, client.ErrMessageNotFound)
}

func TestClient(t *testing.T) {
	cli := New(NewInMemQueue(messageLockDuration))

	ctx, cancel := context.WithCancel(context.Background())
	msgCh, err := client.StartDequeuer(ctx, cli)
	require.NoError(t, err)

	recvCnt := 0
	done := make(chan struct{})

	msgCount := 10

	// Consumer
	go func(msgCh <-chan *client.Message) {
		for msg := range msgCh {
			require.Equal(t, 1, msg.DequeueCount)
			require.Equal(t, "test", string(msg.Data))
			recvCnt++

			if recvCnt == msgCount {
				done <- struct{}{}
			}
		}
	}(msgCh)

	// Producer
	for i := 0; i < msgCount; i++ {
		err := cli.Enqueue(ctx, &client.Message{Data: []byte("test")})
		require.NoError(t, err)
	}

	<-done
	cancel()

	require.Equal(t, msgCount, recvCnt)
}

func TestMessageFinish(t *testing.T) {
	cli := New(NewInMemQueue(messageLockDuration))

	ctx := context.Background()

	err := cli.Enqueue(ctx, &client.Message{Data: []byte("test1")})
	require.NoError(t, err)
	err = cli.Enqueue(ctx, &client.Message{Data: []byte("test2")})
	require.NoError(t, err)

	msg, err := cli.Dequeue(ctx)
	require.NoError(t, err)
	require.Equal(t, "test1", string(msg.Data))

	// Ensure that the first element of queue is test1
	first := cli.queue.v.Front().Value.(*element)
	require.Equal(t, "test1", string(first.val.Data))

	// Finish message
	err = cli.FinishMessage(ctx, msg)
	require.NoError(t, err)

	// Ensure that the first element of queue is test2 because we finish message.
	first = cli.queue.v.Front().Value.(*element)
	require.Equal(t, "test2", string(first.val.Data))
}

func TestExtendMessageLock(t *testing.T) {
	cli := New(NewInMemQueue(messageLockDuration))

	ctx := context.Background()
	err := cli.Enqueue(ctx, &client.Message{Data: []byte("test1")})
	require.NoError(t, err)

	msg, err := cli.Dequeue(ctx)
	require.NoError(t, err)
	require.Equal(t, "test1", string(msg.Data))

	old := msg.NextVisibleAt

	// Extend message lock
	err = cli.ExtendMessage(ctx, msg)
	require.NoError(t, err)

	first := cli.queue.v.Front().Value.(*element)
	require.Greater(t, first.val.NextVisibleAt.UnixNano(), old.UnixNano())
}
