// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package inmemory

import (
	"context"
	"testing"
	"time"

	"github.com/project-radius/radius/pkg/queue"
	"github.com/stretchr/testify/require"
)

func TestClient(t *testing.T) {
	cli := NewClient()

	ctx, cancel := context.WithCancel(context.Background())
	msgCh, err := cli.Dequeue(ctx)
	require.NoError(t, err)

	recvCnt := 0
	done := make(chan struct{})

	// Consumer
	go func(msgCh <-chan *queue.Message) {
		for msg := range msgCh {
			require.Equal(t, "test", msg.Data)
			recvCnt++
		}
		done <- struct{}{}
	}(msgCh)

	// Producer
	for i := 0; i < 10; i++ {
		err := cli.Enqueue(ctx, &queue.Message{Data: "test"})
		require.NoError(t, err)
	}
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done
	require.Equal(t, 10, recvCnt)
}

func TestMessageFinish(t *testing.T) {
	cli := NewClient()

	ctx, cancel := context.WithCancel(context.Background())
	msgCh, _ := cli.Dequeue(ctx)

	_ = cli.Enqueue(ctx, &queue.Message{Data: "test1"})
	_ = cli.Enqueue(ctx, &queue.Message{Data: "test2"})
	recv := <-msgCh
	require.Equal(t, "test1", recv.Data)

	// Ensure that the first element of queue is test1
	first := cli.queue.v.Front().Value.(*element)
	require.Equal(t, "test1", first.val.Data)

	// Finish message
	_ = recv.Finish(nil)

	// Ensure that the first element of queue is test2 because we finish message.
	first = cli.queue.v.Front().Value.(*element)
	require.Equal(t, "test2", first.val.Data)

	cancel()
}

func TestExtendMessageLock(t *testing.T) {
	cli := NewClient()

	ctx, cancel := context.WithCancel(context.Background())
	msgCh, _ := cli.Dequeue(ctx)

	_ = cli.Enqueue(ctx, &queue.Message{Data: "test1"})
	recv := <-msgCh
	require.Equal(t, "test1", recv.Data)

	old := recv.NextVisibleAt

	// Extend message lock
	_ = recv.Extend()

	first := cli.queue.v.Front().Value.(*element)

	require.Greater(t, first.val.NextVisibleAt.UnixNano(), old.UnixNano())

	cancel()
}
