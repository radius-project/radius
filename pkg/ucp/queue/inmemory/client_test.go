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

	sharedtest "github.com/project-radius/radius/test/ucp/queuetest"
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

func TestClient(t *testing.T) {
	inmem := NewInMemQueue(sharedtest.TestMessageLockTime)
	cli := New(inmem)

	clean := func(t *testing.T) {
		inmem.DeleteAll()
	}

	sharedtest.RunTest(t, cli, clean)
}
