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

package apiserver

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/project-radius/radius/pkg/ucp/queue/client"
	v1alpha1 "github.com/project-radius/radius/pkg/ucp/store/apiserverstore/api/ucp.dev/v1alpha1"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/project-radius/radius/test/ucp/kubeenv"
	sharedtest "github.com/project-radius/radius/test/ucp/queuetest"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestMustParseInt64(t *testing.T) {
	result := mustParseInt64("100")
	require.Equal(t, int64(100), result)

	result = mustParseInt64("abc")
	require.Equal(t, int64(0), result)
}

func TestInt64toa(t *testing.T) {
	result := int64toa(int64(12345))
	require.Equal(t, "12345", result)
}

func TestGetTimeFromString(t *testing.T) {
	now := time.Now().UnixNano()
	unixString := fmt.Sprintf("%d", now)
	result := getTimeFromString(unixString)
	require.Equal(t, now, result.UnixNano())
}

func TestCopyMessage(t *testing.T) {
	msg := &client.Message{
		Metadata: client.Metadata{ID: "testid"},
	}
	now := time.Now()
	queueM := &v1alpha1.QueueMessage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "applications.core.10101010",
			Namespace: "radius-test",
			Labels: map[string]string{
				LabelNextVisibleAt: int64toa(now.UnixNano()),
				LabelQueueName:     "applications.core",
			},
		},
		Spec: v1alpha1.QueueMessageSpec{
			DequeueCount: 2,
			EnqueueAt:    metav1.Time{Time: now.UTC()},
			ExpireAt:     metav1.Time{Time: now.Add(10 * time.Second).UTC()},
			ContentType:  client.JSONContentType, // RawExtension supports only JSON seralized data
			Data:         &runtime.RawExtension{Raw: []byte("hello world")},
		},
	}

	copyMessage(msg, queueM)

	require.Equal(t, queueM.ObjectMeta.Name, msg.ID)
	require.Equal(t, client.JSONContentType, msg.ContentType)
	require.Equal(t, queueM.Spec.DequeueCount, msg.DequeueCount)
	require.Equal(t, queueM.Spec.Data.Raw, msg.Data)
	require.Equal(t, queueM.Spec.ExpireAt.Time, msg.ExpireAt)
	require.Equal(t, queueM.Spec.EnqueueAt.Time, msg.EnqueueAt)
	require.Equal(t, getTimeFromString(queueM.ObjectMeta.Labels[LabelNextVisibleAt]), msg.NextVisibleAt)
}

func TestGenerateID(t *testing.T) {
	cli, err := New(nil, Options{Name: "applications.core", Namespace: "test"})
	require.NoError(t, err)

	id, _ := cli.generateID()
	require.Equal(t, 61, len(id))
}

func TestClient(t *testing.T) {
	rc, env, err := kubeenv.StartEnvironment([]string{filepath.Join("..", "..", "..", "..", "deploy", "Chart", "crds", "ucpd")})

	require.NoError(t, err, "If this step is failing for you, run `make test` inside the repository and try again. If you are still stuck then ask for help.")
	defer func() {
		_ = env.Stop()
	}()

	ctx, cancel := testcontext.New(t)
	defer cancel()

	ns := "radius-test"
	err = kubeenv.EnsureNamespace(ctx, rc, ns)
	require.NoError(t, err)

	cli, err := New(rc, Options{Name: "applications.core", Namespace: ns, MessageLockDuration: sharedtest.TestMessageLockTime})
	require.NoError(t, err)

	clear := func(t *testing.T) {
		err := cli.client.DeleteAllOf(ctx, &v1alpha1.QueueMessage{}, runtimeclient.InNamespace(ns))
		require.NoError(t, err)
	}

	sharedtest.RunTest(t, cli, clear)

	t.Run("ExtendMessage is failed when machine's clock is skewed", func(t *testing.T) {
		clear(t)

		// client1 is executing on node 1
		client1, err := New(rc, Options{Name: "applications.core", Namespace: ns, MessageLockDuration: time.Duration(1) * time.Minute})
		require.NoError(t, err)
		// client2 is executing on node 2
		client2, err := New(rc, Options{Name: "applications.core", Namespace: ns, MessageLockDuration: time.Duration(1) * time.Minute})
		require.NoError(t, err)

		err = client1.Enqueue(ctx, client.NewMessage("{}"))
		require.NoError(t, err)
		msg, err := client2.Dequeue(ctx)
		require.NoError(t, err)

		// Increase DequeueCount to mimic the situation when client1 updates message by the clock skew.
		_, err = client1.extendItem(ctx, msg.ID, msg.DequeueCount, time.Now(), time.Duration(1)*time.Minute, true)
		require.NoError(t, err)

		err = client2.ExtendMessage(ctx, msg)
		require.ErrorIs(t, err, client.ErrDequeuedMessage)
	})
}
