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

// package storetest contains SHARED tests for /pkg/ucp/queue
package queuetest

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/components/queue"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

const (
	TestMessageLockTime = time.Duration(1) * time.Second

	pollingInterval = time.Duration(100) * time.Millisecond

	defaultTestDequeueInterval = time.Duration(5) * time.Millisecond
)

type testQueueMessage struct {
	ID      string `json:"id"`
	Message string `json:"msg"`
}

func queueTestMessage(cli queue.Client, num int) error {
	// Enqueue multiple message and dequeue them
	for i := 0; i < num; i++ {
		msg := &testQueueMessage{ID: fmt.Sprintf("%d", i), Message: fmt.Sprintf("hello world %d", i)}

		err := cli.Enqueue(context.Background(), queue.NewMessage(msg))
		if err != nil {
			return err
		}
	}

	return nil
}

// RunTest tests the client's Enqueue, FinishMessage, ExtendMessage, Dequeue methods by enqueuing and dequeuing messages,
// and checking for errors when nil messages are passed. It also tests the StartDequeuer method by dequeuing messages via a
//
//	channel.
func RunTest(t *testing.T, cli queue.Client, clear func(t *testing.T)) {
	ctx, cancel := testcontext.NewWithCancel(t)
	t.Cleanup(cancel)

	t.Run("nil message", func(t *testing.T) {
		err := cli.Enqueue(ctx, &queue.Message{Data: []byte("")})
		require.ErrorIs(t, err, queue.ErrEmptyMessage)
		err = cli.Enqueue(ctx, &queue.Message{Data: nil})
		require.ErrorIs(t, err, queue.ErrEmptyMessage)
		err = cli.Enqueue(ctx, nil)
		require.ErrorIs(t, err, queue.ErrEmptyMessage)
		err = cli.FinishMessage(ctx, nil)
		require.ErrorIs(t, err, queue.ErrEmptyMessage)
		err = cli.ExtendMessage(ctx, nil)
		require.ErrorIs(t, err, queue.ErrEmptyMessage)
	})

	t.Run("enqueue and dequeue messages", func(t *testing.T) {
		clear(t)

		num := 10

		err := queueTestMessage(cli, num)
		require.NoError(t, err)

		checked := map[string]*queue.Message{}
		for i := 0; i < num; i++ {
			msg, err := cli.Dequeue(ctx, queue.QueueClientConfig{})
			require.NoError(t, err)
			result := &testQueueMessage{}
			err = json.Unmarshal(msg.Data, result)
			require.NoError(t, err)
			if _, ok := checked[msg.ID]; ok {
				require.Fail(t, "duplicated message")
			}
			checked[result.ID] = msg
		}

		for _, v := range checked {
			err = cli.FinishMessage(ctx, v)
			require.NoError(t, err)
		}
	})

	t.Run("message lock is expired", func(t *testing.T) {
		clear(t)

		err := queueTestMessage(cli, 2)
		require.NoError(t, err)

		msg1, err := cli.Dequeue(ctx, queue.QueueClientConfig{})
		require.NoError(t, err)
		require.NotNil(t, msg1)

		time.Sleep(10 * time.Millisecond)

		msg2, err := cli.Dequeue(ctx, queue.QueueClientConfig{})
		require.NoError(t, err)
		require.NotNil(t, msg2)

		// Ensure that queue doesn't have any valid messages
		_, err = cli.Dequeue(ctx, queue.QueueClientConfig{})
		require.ErrorIs(t, err, queue.ErrMessageNotFound)

		// Dequeue until message is requeued.
		var msg3 *queue.Message
		for {
			msg3, err = cli.Dequeue(ctx, queue.QueueClientConfig{})
			if err == nil {
				break
			}
			time.Sleep(pollingInterval)
		}

		require.Equal(t, msg1.ID, msg3.ID)
	})

	t.Run("extend valid message lock", func(t *testing.T) {
		clear(t)

		err := queueTestMessage(cli, 2)
		require.NoError(t, err)

		msg1, err := cli.Dequeue(ctx, queue.QueueClientConfig{})
		require.NoError(t, err)
		t.Logf("%s %v", msg1.ID, msg1.NextVisibleAt)

		msg2, err := cli.Dequeue(ctx, queue.QueueClientConfig{})
		require.NoError(t, err)
		t.Logf("%s %v", msg2.ID, msg2.NextVisibleAt)

		// Ensure that queue doesn't have any valid messages
		_, err = cli.Dequeue(ctx, queue.QueueClientConfig{})
		require.ErrorIs(t, err, queue.ErrMessageNotFound)
		// Extend msg1 after sometime
		time.Sleep(TestMessageLockTime / 2)
		err = cli.ExtendMessage(ctx, msg1)
		t.Logf("%s %v", msg1.ID, msg1.NextVisibleAt)
		require.Equal(t, 1, msg1.DequeueCount, "DequeueCount must be 1")
		require.NoError(t, err)

		for {
			// msg2 is requeued. msg3 must be msg2
			msg3, err := cli.Dequeue(ctx, queue.QueueClientConfig{})
			if err == nil {
				t.Logf("%s %v", msg3.ID, msg3.NextVisibleAt)
				require.Equal(t, msg2.ID, msg3.ID)
				break
			}
			time.Sleep(pollingInterval)
		}
	})

	t.Run("extend invalid message lock", func(t *testing.T) {
		clear(t)

		err := queueTestMessage(cli, 2)
		require.NoError(t, err)

		msg1, err := cli.Dequeue(ctx, queue.QueueClientConfig{})
		require.NoError(t, err)
		t.Logf("%s %v", msg1.ID, msg1.NextVisibleAt)

		time.Sleep(TestMessageLockTime / 2)

		msg2, err := cli.Dequeue(ctx, queue.QueueClientConfig{})
		require.NoError(t, err)
		t.Logf("%s %v", msg2.ID, msg2.NextVisibleAt)

		for {
			msg3, err := cli.Dequeue(ctx, queue.QueueClientConfig{})
			if err == nil {
				t.Logf("%s %v", msg3.ID, msg3.NextVisibleAt)
				break
			}
			time.Sleep(pollingInterval)
		}

		// Wait until message lock is released.
		time.Sleep(TestMessageLockTime * 2)
		err = cli.ExtendMessage(ctx, msg2)
		require.ErrorIs(t, err, queue.ErrInvalidMessage)
	})

	t.Run("StartDequeuer dequeues message via channel", func(t *testing.T) {
		clear(t)
		msgCh, err := queue.StartDequeuer(ctx, cli, queue.WithDequeueInterval(defaultTestDequeueInterval))
		require.NoError(t, err)

		recvCnt := 0
		done := make(chan struct{})

		msgCount := 10

		// Consumer
		go func(msgCh <-chan *queue.Message) {
			for msg := range msgCh {
				require.Equal(t, 1, msg.DequeueCount)
				t.Logf("Dequeued Message ID: %s", msg.ID)
				recvCnt++

				if recvCnt == msgCount {
					done <- struct{}{}
				}
			}
		}(msgCh)

		// Producer
		for i := 0; i < msgCount; i++ {
			msg := &testQueueMessage{ID: fmt.Sprintf("%d", i), Message: fmt.Sprintf("hello world %d", i)}
			err = cli.Enqueue(ctx, queue.NewMessage(msg))
			require.NoError(t, err)
		}

		<-done
		cancel()

		require.Equal(t, msgCount, recvCnt)
	})
}
