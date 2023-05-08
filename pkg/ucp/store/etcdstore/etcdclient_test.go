/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

package etcdstore

import (
	"context"
	"testing"

	"github.com/project-radius/radius/pkg/ucp/data"
	"github.com/project-radius/radius/pkg/ucp/hosting"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/stretchr/testify/require"
	etcdclient "go.etcd.io/etcd/client/v3"

	shared "github.com/project-radius/radius/test/ucp/storetest"
)

func Test_ETCDClient(t *testing.T) {
	config := hosting.NewAsyncValue[etcdclient.Client]()
	service := data.NewEmbeddedETCDService(data.EmbeddedETCDServiceOptions{ClientConfigSink: config})

	ctx, cancel := testcontext.New(t)
	defer cancel()

	go func() {
		// We can't pass the test logger into the etcd service because it is forbidden to log
		// using the test logger after the test finishes.
		//
		// https://github.com/golang/go/issues/40343
		//
		// If you need to see the logging output while you are testing, then comment out the next line
		// and you'll be able to see the spam from etcd.
		//
		// This is caught by the race checker and will fail your pr if you do it.
		ctx := context.Background()
		_ = service.Run(ctx)
	}()

	etcdc, err := config.Get(ctx)
	require.NoError(t, err)

	client := NewETCDClient(etcdc)

	clear := func(t *testing.T) {
		keys, err := etcdc.Get(ctx, "", etcdclient.WithKeysOnly(), etcdclient.WithPrefix())
		require.NoError(t, err)

		for _, kv := range keys.Kvs {
			_, err = etcdc.Delete(ctx, string(kv.Key))
			require.NoError(t, err)
		}
	}

	// The actual test logic lives in a shared package, we're just doing the setup here.
	shared.RunTest(t, client, clear)
}
