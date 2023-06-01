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
