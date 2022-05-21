// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package inmemory

import (
	"fmt"
	"testing"

	"time"

	"github.com/project-radius/radius/pkg/queue"
	"github.com/stretchr/testify/require"
)

func TestEnqueueDequeueMulti(t *testing.T) {
	q := newInMemQueue(messageLockDuration)
	for i := 0; i < 10; i++ {
		q.Enqueue(&queue.Message{
			Data: fmt.Sprintf("test%d", i),
		})
	}

	for i := 0; i < 10; i++ {
		msg := q.Dequeue()
		require.Equal(t, fmt.Sprintf("test%d", i), msg.Data)

		err := q.Complete(msg)
		require.NoError(t, err)
	}

	require.Nil(t, q.v.Front())
}

func TestMessageLock(t *testing.T) {
	q := newInMemQueue(2 * time.Millisecond)

	q.Enqueue(&queue.Message{
		Data: "test",
	})

	msg := q.Dequeue()
	require.Equal(t, "test", msg.Data)
	require.Equal(t, 1, msg.DequeueCount)

	// Message Lock duration is 2 ms, after 10 ms, mesage will be visible on the queue.
	time.Sleep(10 * time.Millisecond)

	msg2 := q.Dequeue()
	require.NotNil(t, msg2)
	require.Equal(t, 2, msg2.DequeueCount)
}

func TestExpiry(t *testing.T) {
	q := newInMemQueue(messageLockDuration)

	q.Enqueue(&queue.Message{
		Data: "test",
	})

	msg := q.Dequeue()
	require.Equal(t, "test", msg.Data)
	require.Equal(t, 1, msg.DequeueCount)

	// Override expiry to the current time.
	msg.ExpireAt = time.Now().UTC()
	time.Sleep(10 * time.Millisecond)

	msg2 := q.Dequeue()
	require.Nil(t, msg2)
}

func TestComplete(t *testing.T) {
	q := newInMemQueue(messageLockDuration)

	q.Enqueue(&queue.Message{
		Data: "test",
	})

	msg := q.Dequeue()
	err := q.Complete(msg)
	require.NoError(t, err)

	// Try to complete the message again.
	err = q.Complete(msg)
	require.ErrorIs(t, ErrAlreadyCompletedMessage, err)

	msg2 := q.Dequeue()
	require.Nil(t, msg2)
}
