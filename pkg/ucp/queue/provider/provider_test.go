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

package provider

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetClient_ValidQueue(t *testing.T) {
	p := New("Applications.Core", QueueProviderOptions{
		Provider: TypeInmemory,
		InMemory: &InMemoryQueueOptions{},
	})

	oldcli, err := p.GetClient(context.TODO())
	require.NoError(t, err)
	require.NotNil(t, oldcli)
	newcli, err := p.GetClient(context.TODO())
	require.NoError(t, err)
	require.Equal(t, oldcli, newcli)
}

func TestGetClient_InvalidQueue(t *testing.T) {
	p := New("Applications.Core", QueueProviderOptions{
		Provider: QueueProviderType("undefined"),
	})

	_, err := p.GetClient(context.TODO())
	require.ErrorIs(t, ErrUnsupportedStorageProvider, err)
}
