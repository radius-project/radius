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

package etcd

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/project-radius/radius/pkg/ucp/data"
	"github.com/project-radius/radius/pkg/ucp/hosting"
	"github.com/project-radius/radius/pkg/ucp/secret"
	"github.com/project-radius/radius/test/testcontext"
	"github.com/stretchr/testify/require"
	etcdclient "go.etcd.io/etcd/client/v3"
)

const (
	testSecretName = "azure-azurecloud-default"
)

func Test_ETCD(t *testing.T) {
	config := hosting.NewAsyncValue[etcdclient.Client]()
	service := data.NewEmbeddedETCDService(data.EmbeddedETCDServiceOptions{ClientConfigSink: config})

	ctx, cancel := testcontext.NewWithCancel(t)
	t.Cleanup(cancel)

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

	runSaveTests(t, etcdc)

}

func runSaveTests(t *testing.T, etcdClient *etcdclient.Client) {
	ctx := context.Background()
	client := Client{
		ETCDClient: etcdClient,
	}
	testSecret, err := json.Marshal("test_secret")
	require.NoError(t, err)
	tests := []struct {
		testName   string
		secretName string
		secret     []byte
		response   []byte
		save       bool
		get        bool
		delete     bool
		err        error
	}{
		{"save-get-delete-secret-success", testSecretName, testSecret, []byte("test_secret"), true, true, true, nil},
		{"save-secret-empty-name", "", testSecret, nil, true, false, false, &secret.ErrInvalid{Message: "invalid argument. 'name' is required"}},
		{"save-secret-empty-secret", testSecretName, nil, nil, true, false, false, &secret.ErrInvalid{Message: "invalid argument. 'value' is required"}},
		{"delete-secret-without-save", testSecretName, nil, nil, false, false, true, &secret.ErrNotFound{}},
		{"get-secret-without-save", testSecretName, nil, nil, false, true, false, &secret.ErrNotFound{}},
		{"get-secret-with-empty-name", "", nil, nil, false, true, false, &secret.ErrInvalid{Message: "invalid argument. 'name' is required"}},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			if tt.save {
				err := client.Save(ctx, tt.secretName, tt.secret)
				if tt.err == nil {
					require.NoError(t, err)
				} else {
					require.Equal(t, err, tt.err)
				}
			}
			if tt.get {
				response, err := client.Get(ctx, tt.secretName)
				if tt.err == nil {
					require.NoError(t, err)
					value, err := strconv.Unquote(string(response))
					require.NoError(t, err)
					require.Equal(t, string(value), string(tt.response))
				} else {
					require.Equal(t, err, tt.err)
				}
			}
			if tt.delete {
				err := client.Delete(ctx, tt.secretName)
				if tt.err == nil {
					require.NoError(t, err)
				} else {
					require.Equal(t, err, tt.err)
				}
			}
		})
	}
}
