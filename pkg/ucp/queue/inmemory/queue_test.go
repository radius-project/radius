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

package inmemory

import (
	"fmt"
	"testing"

	"time"

	"github.com/project-radius/radius/pkg/ucp/queue/client"
	"github.com/stretchr/testify/require"
)

func TestEnqueueDequeueMulti(t *testing.T) {
	q := NewInMemQueue(messageLockDuration)
	for i := 0; i < 10; i++ {
		q.Enqueue(&client.Message{
			Data: []byte(fmt.Sprintf("test%d", i)),
		})
	}

	for i := 0; i < 10; i++ {
		msg := q.Dequeue()
		require.Equal(t, []byte(fmt.Sprintf("test%d", i)), msg.Data)

		err := q.Complete(msg)
		require.NoError(t, err)
	}

	require.Nil(t, q.v.Front())
}

func TestMessageLock(t *testing.T) {
	q := NewInMemQueue(2 * time.Millisecond)

	q.Enqueue(&client.Message{
		Data: []byte("test"),
	})

	msg := q.Dequeue()
	require.Equal(t, []byte("test"), msg.Data)
	require.Equal(t, 1, msg.DequeueCount)

	// Message Lock duration is 2 ms, after 10 ms, mesage will be visible on the client.
	time.Sleep(10 * time.Millisecond)

	msg2 := q.Dequeue()
	require.NotNil(t, msg2)
	require.Equal(t, 2, msg2.DequeueCount)
}

func TestExpiry(t *testing.T) {
	q := NewInMemQueue(messageLockDuration)

	q.Enqueue(&client.Message{
		Data: []byte("test"),
	})

	msg := q.Dequeue()
	require.Equal(t, []byte("test"), msg.Data)
	require.Equal(t, 1, msg.DequeueCount)

	// Override expiry to the current time.
	msg.ExpireAt = time.Now().UTC()
	time.Sleep(10 * time.Millisecond)

	msg2 := q.Dequeue()
	require.Nil(t, msg2)
}

func TestComplete(t *testing.T) {
	q := NewInMemQueue(messageLockDuration)

	q.Enqueue(&client.Message{
		Data: []byte("test"),
	})

	msg := q.Dequeue()
	err := q.Complete(msg)
	require.NoError(t, err)

	// Try to complete the message again.
	err = q.Complete(msg)
	require.ErrorIs(t, client.ErrInvalidMessage, err)

	msg2 := q.Dequeue()
	require.Nil(t, msg2)
}
